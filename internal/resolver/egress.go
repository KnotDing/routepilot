package resolver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

// PinnedDialer binds every non-local upstream socket to an explicit source and
// interface. Gateway source-policy bypass is also required in a single-NIC
// deployment; socket pinning cannot override the gateway's BGP table.
type PinnedDialer struct {
	Interface        string
	Source4, Source6 netip.Addr
	Timeout          time.Duration
	ports            chan uint16
}

func NewPinnedDialer(iface string, source4, source6 netip.Addr, timeout time.Duration, first, last uint16) (*PinnedDialer, error) {
	if iface == "" || timeout <= 0 || !source4.IsValid() && !source6.IsValid() {
		return nil, errors.New("pinned egress interface, source address and timeout are required")
	}
	if first < 1024 || last < first || last >= 49152 || int(last)-int(first) > 4095 {
		return nil, errors.New("reserve 1..4096 egress source ports below the OS ephemeral range")
	}
	d := &PinnedDialer{Interface: iface, Source4: source4, Source6: source6, Timeout: timeout, ports: make(chan uint16, int(last)-int(first)+1)}
	for p := uint32(first); p <= uint32(last); p++ {
		d.ports <- uint16(p)
	}
	return d, nil
}

type leasedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

// Preserve net.PacketConn on UDP wrappers: miekg/dns uses that interface to
// distinguish a datagram from a length-prefixed TCP stream.
type leasedUDPConn struct {
	*net.UDPConn
	once    sync.Once
	release func()
}

func (c *leasedUDPConn) Close() error { err := c.UDPConn.Close(); c.once.Do(c.release); return err }

func (c *leasedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
func (d *PinnedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	target, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil, errors.New("upstream destination must be literal IP:port")
	}
	iface, err := net.InterfaceByName(d.Interface)
	if err != nil {
		return nil, err
	}
	if iface.Flags&net.FlagUp == 0 {
		return nil, errors.New("egress interface is down")
	}
	source := d.Source4
	if target.Addr().Is6() {
		source = d.Source6
	}
	if !source.IsValid() || source.Is4() != target.Addr().Is4() || source.IsUnspecified() {
		return nil, errors.New("no pinned egress source for destination family")
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	found := false
	for _, a := range addresses {
		p, err := netip.ParsePrefix(a.String())
		if err == nil && p.Addr().Unmap() == source.Unmap() {
			found = true
		}
	}
	if !found {
		return nil, errors.New("pinned source is not assigned to egress interface")
	}
	if d.ports == nil {
		return nil, errors.New("pinned dialer has no reserved source-port pool")
	}
	var port uint16
	select {
	case port = <-d.ports:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, errors.New("upstream socket capacity exhausted")
	}
	release := func() { d.ports <- port }
	dial := &net.Dialer{Timeout: d.Timeout, Control: interfaceControl(iface.Index, target.Addr().Is6())}
	if network == "udp" {
		dial.LocalAddr = &net.UDPAddr{IP: net.IP(source.AsSlice()), Port: int(port), Zone: source.Zone()}
	} else {
		dial.LocalAddr = &net.TCPAddr{IP: net.IP(source.AsSlice()), Port: int(port), Zone: source.Zone()}
	}
	conn, err := dial.DialContext(ctx, network, address)
	if err != nil {
		release()
		return nil, fmt.Errorf("pinned upstream socket: %w", err)
	}
	if udp, ok := conn.(*net.UDPConn); ok {
		return &leasedUDPConn{UDPConn: udp, release: release}, nil
	}
	return &leasedConn{Conn: conn, release: release}, nil
}
