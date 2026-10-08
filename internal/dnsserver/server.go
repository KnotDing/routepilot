package dnsserver

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
)

type DispatchFunc func(context.Context, model.DNSQueryContext, *dns.Msg) *dns.Msg
type ServerConfig struct {
	Workers, QueueSize, MaxTCP   int
	QueryTimeout, TCPIdleTimeout time.Duration
	ResponseBarrier              interface {
		BeginResponse()
		EndResponse()
	}
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{Workers: 32, QueueSize: 256, MaxTCP: 256, QueryTimeout: 3 * time.Second, TCPIdleTimeout: 10 * time.Second}
}

type udpWork struct {
	conn    *net.UDPConn
	ingress Ingress
	local   netip.AddrPort
	session *dns.SessionUDP
	msg     *dns.Msg
}
type bound struct {
	ingress Ingress
	udp     *net.UDPConn
	tcp     *net.TCPListener
	addr    netip.AddrPort
}
type Server struct {
	config                      ServerConfig
	dispatch                    DispatchFunc
	ctx                         context.Context
	cancel                      context.CancelFunc
	sockets                     []bound
	queue                       chan udpWork
	tcpSlots                    chan struct{}
	loops, workers, connections sync.WaitGroup
	connMu                      sync.Mutex
	active                      map[net.Conn]struct{}
	closeOnce                   sync.Once
	Dropped                     atomic.Uint64
	errors                      chan error
}

func Listen(parent context.Context, ingresses []Ingress, c ServerConfig, dispatch DispatchFunc) (*Server, error) {
	if len(ingresses) == 0 || len(ingresses) > 16 || dispatch == nil || c.Workers < 1 || c.Workers > 256 || c.QueueSize < 1 || c.QueueSize > 65536 || c.MaxTCP < 1 || c.MaxTCP > 4096 || c.QueryTimeout <= 0 || c.TCPIdleTimeout <= 0 {
		return nil, errors.New("invalid DNS server limits")
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Server{config: c, dispatch: dispatch, ctx: ctx, cancel: cancel, queue: make(chan udpWork, c.QueueSize), tcpSlots: make(chan struct{}, c.MaxTCP), active: make(map[net.Conn]struct{}), errors: make(chan error, 1)}
	rollback := func() {
		for _, b := range s.sockets {
			_ = b.udp.Close()
			_ = b.tcp.Close()
		}
		cancel()
	}
	for _, in := range ingresses {
		if err := in.Validate(); err != nil {
			rollback()
			return nil, err
		}
		u, tcp, addr, err := listenPair(in.Bind)
		if err != nil {
			rollback()
			return nil, err
		}
		s.sockets = append(s.sockets, bound{in, u, tcp, addr})
	}
	for i := 0; i < c.Workers; i++ {
		s.workers.Add(1)
		go s.udpWorker()
	}
	for _, b := range s.sockets {
		s.loops.Add(2)
		go s.readUDP(b)
		go s.acceptTCP(b)
	}
	return s, nil
}

func listenPair(bind netip.AddrPort) (*net.UDPConn, *net.TCPListener, netip.AddrPort, error) {
	// TCP port allocation accounts for active/TIME_WAIT TCP sockets; UDP port
	// allocation does not. Reserve TCP first, then the same UDP port. An ephemeral
	// UDP collision may retry, but a requested fixed port always fails explicitly.
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		var tcp *net.TCPListener
		tcp, err = net.ListenTCP("tcp", net.TCPAddrFromAddrPort(bind))
		if err != nil {
			return nil, nil, netip.AddrPort{}, err
		}
		addr := tcp.Addr().(*net.TCPAddr).AddrPort()
		var udp *net.UDPConn
		udp, err = net.ListenUDP("udp", net.UDPAddrFromAddrPort(addr))
		if err == nil {
			return udp, tcp, addr, nil
		}
		_ = tcp.Close()
		if bind.Port() != 0 || !errors.Is(err, syscall.EADDRINUSE) {
			break
		}
	}
	return nil, nil, netip.AddrPort{}, err
}
func (s *Server) Addresses() []netip.AddrPort {
	out := make([]netip.AddrPort, len(s.sockets))
	for i, b := range s.sockets {
		out[i] = b.addr
	}
	return out
}
func (s *Server) Errors() <-chan error { return s.errors }
func (s *Server) fail(err error) {
	if s.ctx.Err() != nil {
		return
	}
	select {
	case s.errors <- err:
	default:
	}
	s.cancel()
}
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		for _, b := range s.sockets {
			_ = b.udp.Close()
			_ = b.tcp.Close()
		}
		s.loops.Wait()
		s.connMu.Lock()
		for conn := range s.active {
			_ = conn.Close()
		}
		s.connMu.Unlock()
		s.connections.Wait()
		s.workers.Wait()
	})
}
func (s *Server) readUDP(b bound) {
	defer s.loops.Done()
	buf := make([]byte, dns.MaxMsgSize)
	for {
		n, session, err := dns.ReadFromSessionUDP(b.udp, buf)
		if err != nil {
			s.fail(err)
			return
		}
		m := new(dns.Msg)
		if err = m.Unpack(buf[:n]); err != nil {
			s.Dropped.Add(1)
			continue
		}
		work := udpWork{b.udp, b.ingress, b.addr, session, m}
		select {
		case <-s.ctx.Done():
			return
		case s.queue <- work:
		default:
			s.Dropped.Add(1)
			_ = s.writeUDP(work, failure(m, dns.RcodeServerFailure))
		}
	}
}
func (s *Server) udpWorker() {
	defer s.workers.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case w := <-s.queue:
			if s.config.ResponseBarrier != nil {
				s.config.ResponseBarrier.BeginResponse()
			}
			ctx, cancel := context.WithTimeout(s.ctx, s.config.QueryTimeout)
			client := w.session.RemoteAddr().(*net.UDPAddr).AddrPort()
			q, err := w.ingress.QueryContext(ctx, model.UDP, client, w.local)
			var r *dns.Msg
			if err != nil {
				r = failure(w.msg, dns.RcodeServerFailure)
			} else {
				q.ReceivedAt = time.Now()
				if len(w.msg.Question) == 1 {
					q.Question = w.msg.Question[0]
				}
				r = s.dispatch(ctx, q, w.msg)
			}
			cancel()
			if r != nil {
				_ = s.writeUDP(w, r)
			}
			if s.config.ResponseBarrier != nil {
				s.config.ResponseBarrier.EndResponse()
			}
		}
	}
}
func (s *Server) writeUDP(w udpWork, m *dns.Msg) error {
	maxSize := uint16(512)
	if opt := w.msg.IsEdns0(); opt != nil {
		maxSize = max(uint16(512), min(opt.UDPSize(), uint16(1232)))
	}
	m = m.Copy()
	m.Truncate(int(maxSize))
	raw, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = dns.WriteToSessionUDP(w.conn, raw, w.session)
	return err
}
func (s *Server) acceptTCP(b bound) {
	defer s.loops.Done()
	for {
		conn, err := b.tcp.AcceptTCP()
		if err != nil {
			s.fail(err)
			return
		}
		select {
		case s.tcpSlots <- struct{}{}:
		default:
			s.Dropped.Add(1)
			_ = conn.Close()
			continue
		}
		if s.ctx.Err() != nil {
			<-s.tcpSlots
			_ = conn.Close()
			return
		}
		s.connMu.Lock()
		s.active[conn] = struct{}{}
		s.connMu.Unlock()
		s.connections.Add(1)
		go s.serveTCP(b, conn)
	}
}
func (s *Server) serveTCP(b bound, conn *net.TCPConn) {
	defer func() {
		_ = conn.Close()
		s.connMu.Lock()
		delete(s.active, conn)
		s.connMu.Unlock()
		<-s.tcpSlots
		s.connections.Done()
	}()
	q, err := b.ingress.QueryContext(s.ctx, model.TCP, conn.RemoteAddr().(*net.TCPAddr).AddrPort(), conn.LocalAddr().(*net.TCPAddr).AddrPort())
	if err != nil {
		return
	}
	var head [2]byte
	for {
		if err = conn.SetReadDeadline(time.Now().Add(s.config.TCPIdleTimeout)); err != nil {
			return
		}
		if _, err = io.ReadFull(conn, head[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(head[:])
		if n < 12 {
			return
		}
		raw := make([]byte, int(n))
		if _, err = io.ReadFull(conn, raw); err != nil {
			return
		}
		m := new(dns.Msg)
		if err = m.Unpack(raw); err != nil {
			return
		}
		q.ReceivedAt = time.Now()
		if len(m.Question) == 1 {
			q.Question = m.Question[0]
		}
		ctx, cancel := context.WithTimeout(s.ctx, s.config.QueryTimeout)
		if s.config.ResponseBarrier != nil {
			s.config.ResponseBarrier.BeginResponse()
		}
		r := s.dispatch(ctx, q, m)
		cancel()
		// Keep the route-withdrawal barrier until the response is written.
		writeResponse := func() error {
			if s.config.ResponseBarrier != nil {
				defer s.config.ResponseBarrier.EndResponse()
			}
			if r == nil {
				return nil
			}
			raw, err = r.Pack()
			if err != nil || len(raw) > dns.MaxMsgSize {
				return errors.New("invalid DNS response")
			}
			binary.BigEndian.PutUint16(head[:], uint16(len(raw)))
			packet := append(head[:], raw...)
			if err = conn.SetWriteDeadline(time.Now().Add(s.config.QueryTimeout)); err != nil {
				return err
			}
			if _, err = io.Copy(conn, bytesReader(packet)); err != nil {
				return err
			}
			return nil
		}
		if writeResponse() != nil || r == nil {
			return
		}
	}
}

// packetReader avoids an additional TCP framing allocation.
type packetReader struct{ b []byte }

func (r *packetReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
func bytesReader(b []byte) io.Reader { return &packetReader{b} }
