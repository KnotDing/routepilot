//go:build darwin

package resolver

import (
	"context"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestPinnedUDPWireFramingAndPortRelease(t *testing.T) {
	socket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &dns.Server{PacketConn: socket, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, m *dns.Msg) { _ = w.WriteMsg(reply(m)) })}
	go func() { _ = server.ActivateAndServe() }()
	defer server.Shutdown()
	dialer, err := NewPinnedDialer("lo0", netip.MustParseAddr("127.0.0.1"), netip.Addr{}, time.Second, 20300, 20303)
	if err != nil {
		t.Fatal(err)
	}
	tr, _ := NewWireTransport(dialer, time.Second)
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	for i := 0; i < 8; i++ {
		if _, err = tr.Exchange(context.Background(), ResolverEndpoint{Protocol: "udp", Address: socket.LocalAddr().String()}, q); err != nil {
			t.Fatalf("pinned UDP exchange %d: %v", i, err)
		}
	}
	if len(dialer.ports) != cap(dialer.ports) {
		t.Fatal("upstream socket source port leaked")
	}
}
