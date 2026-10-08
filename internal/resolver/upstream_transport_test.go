package resolver

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func reply(q *dns.Msg) *dns.Msg {
	r := new(dns.Msg)
	r.SetReply(q)
	r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP("203.0.113.7")}}
	return r
}
func TestUDPTruncationUsesTCP(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcp, err := net.Listen("tcp", udp.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	us := &dns.Server{PacketConn: udp, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) { r := reply(q); r.Truncated = true; _ = w.WriteMsg(r) })}
	ts := &dns.Server{Listener: tcp, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) { _ = w.WriteMsg(reply(q)) })}
	go func() { _ = us.ActivateAndServe() }()
	go func() { _ = ts.ActivateAndServe() }()
	defer us.Shutdown()
	defer ts.Shutdown()
	tr, _ := NewWireTransport(&net.Dialer{}, time.Second)
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	r, err := tr.Exchange(context.Background(), ResolverEndpoint{Protocol: "udp", Address: udp.LocalAddr().String()}, q)
	if err != nil || r.Truncated || r.Id != q.Id || len(r.Answer) != 1 {
		t.Fatalf("TCP fallback failed: %v, %v", r, err)
	}
}
func TestDoTAndDoHCertificateValidation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 65536))
		if err != nil {
			t.Error(err)
			return
		}
		q := new(dns.Msg)
		if err = q.Unpack(raw); err != nil {
			t.Error(err)
			return
		}
		packet, _ := reply(q).Pack()
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(packet)
	})
	https := httptest.NewTLSServer(handler)
	defer https.Close()
	roots := https.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	tr, _ := NewWireTransport(&net.Dialer{}, time.Second)
	tr.TLSConfig = &tls.Config{RootCAs: roots}
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	address := https.Listener.Addr().String()
	ep := ResolverEndpoint{Protocol: "doh", Address: address, URL: "https://example.com/dns-query"}
	if _, err := tr.Exchange(context.Background(), ep, q); err != nil {
		t.Fatal(err)
	}
	ep.URL = "https://wrong.test/dns-query"
	if _, err := tr.Exchange(context.Background(), ep, q); err == nil {
		t.Fatal("DoH accepted invalid certificate name")
	}
	secure, err := tls.Listen("tcp", "127.0.0.1:0", https.TLS)
	if err != nil {
		t.Fatal(err)
	}
	server := &dns.Server{Listener: secure, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) { _ = w.WriteMsg(reply(q)) })}
	go func() { _ = server.ActivateAndServe() }()
	defer server.Shutdown()
	ep = ResolverEndpoint{Protocol: "dot", Address: secure.Addr().String(), TLSName: "example.com"}
	if _, err := tr.Exchange(context.Background(), ep, q); err != nil {
		t.Fatal(err)
	}
	ep.TLSName = "wrong.test"
	if _, err := tr.Exchange(context.Background(), ep, q); err == nil {
		t.Fatal("DoT accepted invalid certificate")
	}
}
func TestRejectHostnameBootstrapAndInvalidResponses(t *testing.T) {
	tr, _ := NewWireTransport(&net.Dialer{}, time.Second)
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	if _, err := tr.Exchange(context.Background(), ResolverEndpoint{Protocol: "udp", Address: "resolver.test:53"}, q); err == nil {
		t.Fatal("system DNS bootstrap allowed")
	}
	r := reply(q)
	r.Id++
	if ValidateResponse(q, r) == nil {
		t.Fatal("mismatched transaction ID accepted")
	}
	r.Id = q.Id
	r.Question[0].Name = "other.test."
	if ValidateResponse(q, r) == nil {
		t.Fatal("mismatched question accepted")
	}
}
func TestPinnedDialerFailsClosed(t *testing.T) {
	d, err := NewPinnedDialer("interfaceDoesNotExist", netip.MustParseAddr("192.0.2.2"), netip.Addr{}, time.Second, 20000, 20001)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.DialContext(context.Background(), "udp", "8.8.8.8:53"); err == nil {
		t.Fatal("missing interface fell back to system routing")
	}
}
