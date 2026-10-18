package dhcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
	"surge-controller/internal/config"
)

type Storage interface {
	LoadDHCP(context.Context) (State, error)
	SaveDHCP(context.Context, State) error
}
type Status struct {
	DeviceSettings bool         `json:"device_settings"`
	Running        bool         `json:"running"`
	Preflight      *CheckResult `json:"preflight,omitempty"`
	Supported      bool         `json:"supported"`
	Enabled        bool         `json:"enabled"`
	State          string       `json:"state"`
	Interface      string       `json:"interface"`
	Error          string       `json:"error,omitempty"`
	Received       uint64       `json:"received"`
	Sent           uint64       `json:"sent"`
	Leases         []Lease      `json:"leases"`
}
type running struct {
	conn   *net.UDPConn
	done   chan struct{}
	engine *Engine
	config config.DHCP
}
type Service struct {
	addressCheck  func(context.Context, string) (string, error)
	discoverCheck func(context.Context, *net.Interface, string) ([]DiscoveredServer, error)
	applyMu       sync.Mutex
	mu            sync.Mutex
	current       *running
	storage       Storage
	status        Status
}

func NewService(storage Storage) *Service {
	return &Service{storage: storage, addressCheck: addressMode, discoverCheck: discover, status: Status{DeviceSettings: true, Supported: true, State: "disabled", Leases: []Lease{}}}
}
func (s *Service) Snapshot() Status {
	s.mu.Lock()
	v := s.status
	rt := s.current
	s.mu.Unlock()
	if rt != nil {
		v.Interface = rt.config.Interface
		select {
		case <-rt.done:
			v.Running = false
		default:
			v.Running = true
		}
		v.Leases = rt.engine.Snapshot().Leases
	}
	return v
}
func (s *Service) Apply(ctx context.Context, d config.DHCP) (resultErr error) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	defer func() {
		if resultErr != nil {
			s.mu.Lock()
			s.status.Enabled = d.Enabled
			s.status.Interface = d.Interface
			s.status.State = "failed"
			s.status.Error = resultErr.Error()
			s.mu.Unlock()
		}
	}()
	if err := d.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	old := s.current
	s.mu.Unlock()
	if old != nil && reflect.DeepEqual(old.config, d) {
		select {
		case <-old.done:
		default:
			return nil
		}
	}
	if d.Enabled {
		report, checkErr := s.Check(ctx, d)
		if checkErr != nil {
			return checkErr
		}
		s.mu.Lock()
		s.status.Preflight = &report
		s.mu.Unlock()
	}
	var saved State
	var err error
	if old != nil {
		old.conn.Close()
		<-old.done
		saved = old.engine.Snapshot()
	} else if d.Enabled {
		saved, err = s.storage.LoadDHCP(ctx)
		if err != nil {
			return fmt.Errorf("load DHCP leases: %w", err)
		}
	}
	s.mu.Lock()
	s.current = nil
	s.status.Enabled = d.Enabled
	s.status.Interface = d.Interface
	s.status.Error = ""
	s.status.State = "disabled"
	s.mu.Unlock()
	if !d.Enabled {
		return nil
	}
	next, err := s.start(ctx, d, saved)
	if err != nil {
		var restoreErr error
		if old != nil {
			next, restoreErr = s.start(ctx, old.config, saved)
		}
		s.mu.Lock()
		s.current = next
		s.status.State = "failed"
		s.status.Error = err.Error()
		if restoreErr != nil {
			s.status.Error += "; restore: " + restoreErr.Error()
		}
		s.mu.Unlock()
		return fmt.Errorf("DHCP configuration failed: %s", s.Snapshot().Error)
	}
	s.mu.Lock()
	s.current = next
	if s.status.Error == "" {
		s.status.State = "running"
	}
	s.mu.Unlock()
	return nil
}
func (s *Service) start(ctx context.Context, d config.DHCP, saved State) (*running, error) {
	iface, err := net.InterfaceByName(d.Interface)
	if err != nil {
		return nil, fmt.Errorf("DHCP interface: %w", err)
	}
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 || iface.Flags&net.FlagLoopback != 0 {
		return nil, errors.New("DHCP needs an active broadcast-capable LAN interface")
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	assigned := false
	for _, raw := range addrs {
		p, e := netip.ParsePrefix(raw.String())
		if e == nil && p.Addr().String() == d.ServerIP && p.Masked().String() == d.Subnet {
			assigned = true
		}
	}
	if !assigned {
		return nil, errors.New("DHCP server address and subnet must be assigned to the selected interface")
	}
	engine, err := New(d, saved, func(v State) error {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return s.storage.SaveDHCP(c, v)
	})
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 67})
	if err != nil {
		return nil, fmt.Errorf("DHCP UDP 67 (administrator permission and exclusive port required): %w", err)
	}
	if err = bindInterface(conn, iface); err != nil {
		conn.Close()
		return nil, err
	}
	pc := ipv4.NewPacketConn(conn)
	if err = pc.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		conn.Close()
		return nil, err
	}
	rt := &running{conn: conn, done: make(chan struct{}), engine: engine, config: d.Clone()}
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-rt.done:
		}
	}()
	go func() {
		defer close(rt.done)
		defer conn.Close()
		buf := make([]byte, 4097)
		for {
			n, cm, peer, readErr := pc.ReadFrom(buf)
			if readErr != nil {
				if !errors.Is(readErr, net.ErrClosed) {
					s.recordError("DHCP receive: " + readErr.Error())
					s.mu.Lock()
					s.status.State = "failed"
					s.mu.Unlock()
				}
				return
			}
			remote, ok := peer.(*net.UDPAddr)
			if !ok || remote.Port != 68 || (remote.IP.To4() != nil && !remote.IP.IsUnspecified() && !netip.MustParsePrefix(d.Subnet).Contains(netip.AddrFrom4([4]byte(remote.IP.To4())))) || n > 4096 || cm == nil || cm.IfIndex != iface.Index {
				continue
			}
			s.mu.Lock()
			s.status.Received++
			s.mu.Unlock()
			response, dest, e := engine.Handle(buf[:n])
			if e != nil {
				s.recordError(e.Error())
				continue
			}
			if len(response) == 0 {
				continue
			}
			if _, e = pc.WriteTo(response, &ipv4.ControlMessage{IfIndex: iface.Index, Src: net.ParseIP(d.ServerIP).To4()}, dest); e != nil {
				s.recordError("DHCP send: " + e.Error())
				continue
			}
			s.mu.Lock()
			s.status.Sent++
			if s.status.State == "degraded" {
				s.status.State = "running"
				s.status.Error = ""
			}
			s.mu.Unlock()
		}
	}()
	return rt, nil
}
func (s *Service) recordError(message string) {
	s.mu.Lock()
	s.status.Error = message
	s.status.State = "degraded"
	s.mu.Unlock()
}
func (s *Service) Close() {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	s.mu.Lock()
	rt := s.current
	s.current = nil
	s.status.State = "disabled"
	s.mu.Unlock()
	if rt != nil {
		rt.conn.Close()
		<-rt.done
	}
}
