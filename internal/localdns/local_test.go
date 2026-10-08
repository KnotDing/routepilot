package localdns

import (
	"context"
	"github.com/miekg/dns"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"testing"
)

func query(t *testing.T, r *Resolver, name string, typ uint16) (*dns.Msg, bool) {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(name, typ)
	m.SetEdns0(4096, true)
	return r.ResolveLocal(context.Background(), model.DNSQueryContext{Question: m.Question[0]}, m)
}
func TestLocalDNSPrecedenceFamiliesAndGlob(t *testing.T) {
	entries := []config.Host{
		{ID: "s1", Domain: "*.example.test", Match: "wildcard", Addresses: []string{"192.0.2.1"}, TTL: 60, Source: "surge", Order: 1, Enabled: true},
		{ID: "s2", Domain: "nas.example.test", Match: "exact", Addresses: []string{"192.0.2.2"}, TTL: 60, Source: "surge", Order: 2, Enabled: true},
	}
	r := New(entries)
	m, ok := query(t, r, "nas.example.test.", dns.TypeA)
	if !ok || m.Answer[0].(*dns.A).A.String() != "192.0.2.1" {
		t.Fatal("Surge first-match order lost")
	}
	if _, ok := query(t, r, "example.test.", dns.TypeA); ok {
		t.Fatal("wildcard matched apex")
	}
	entries = append(entries, config.Host{ID: "m", Domain: "NAS.EXAMPLE.TEST", Match: "exact", Addresses: []string{"192.0.2.53", "2001:db8::53"}, TTL: 120, Source: "manual", Enabled: true})
	r.Update(entries)
	entries[2].Addresses[0] = "192.0.2.99"
	for _, tc := range []struct {
		typ   uint16
		count int
	}{{dns.TypeA, 1}, {dns.TypeAAAA, 1}, {dns.TypeHTTPS, 0}, {dns.TypeSVCB, 0}} {
		m, ok := query(t, r, "nas.example.test.", tc.typ)
		if !ok || len(m.Answer) != tc.count || m.AuthenticatedData || m.Rcode != dns.RcodeSuccess || m.IsEdns0().UDPSize() != 1232 || m.IsEdns0().Do() {
			t.Fatal(tc.typ, m, ok)
		}
		if tc.count > 0 && m.Answer[0].Header().Ttl != 120 {
			t.Fatal("manual mapping did not win")
		}
		if tc.typ == dns.TypeA && m.Answer[0].(*dns.A).A.String() != "192.0.2.53" {
			t.Fatal("caller mutation leaked")
		}
	}
	if _, ok := query(t, r, "nas.example.test.", dns.TypeTXT); ok {
		t.Fatal("unsupported RR intercepted")
	}
	r.Update([]config.Host{{ID: "q", Domain: "nas?.test", Match: "wildcard", Addresses: []string{"192.0.2.1"}, TTL: 30, Enabled: true, Source: "manual"}})
	if _, ok := query(t, r, "nas1.test.", dns.TypeA); !ok {
		t.Fatal("? glob ignored")
	}
	if _, ok := query(t, r, "nas12.test.", dns.TypeA); ok {
		t.Fatal("? matched multiple characters")
	}
	m, ok = query(t, r, "nas1.test.", dns.TypeAAAA)
	if !ok || len(m.Answer) != 0 || m.Rcode != dns.RcodeSuccess {
		t.Fatal("missing family must be NODATA")
	}
}
