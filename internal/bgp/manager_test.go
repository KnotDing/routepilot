package bgp

import (
	"context"
	"surge-controller/internal/config"
	"testing"
)

type mockSpeaker struct {
	routes        map[string]Advertisement
	adds, deletes int
}

func (s *mockSpeaker) Add(_ context.Context, r Advertisement) ([]byte, error) {
	s.adds++
	s.routes[r.Prefix] = r
	return []byte(r.Prefix), nil
}
func (s *mockSpeaker) Delete(_ context.Context, id []byte) error {
	s.deletes++
	delete(s.routes, string(id))
	return nil
}
func (s *mockSpeaker) Peers(context.Context) ([]PeerStatus, error) {
	return []PeerStatus{{Address: "192.0.2.1", State: "ESTABLISHED"}}, nil
}
func (s *mockSpeaker) Exported(context.Context, string) (map[string]bool, error) {
	out := map[string]bool{}
	for k := range s.routes {
		out[k] = true
	}
	return out, nil
}
func (s *mockSpeaker) Close() error { return nil }
func TestBindingsHardLimitAndNoDNSCoupling(t *testing.T) {
	c := config.Default()
	c.Settings.Surge.NextHop4 = "192.168.20.5"
	c.Settings.DNS.InterceptionEnabled = true
	c.Settings.DNS.Source4 = "192.168.20.2"
	c.Interception = []config.Interception{{ID: "dns", Prefix: "8.8.8.8/32", Enabled: true}}
	c.Routes = []config.Route{{ID: "manual", Prefix: "10.20.0.0/16", NextHop: "192.168.20.1", Enabled: true, Binding: "none"}}
	all, e := Desired(c, true, true, true)
	if e != nil || len(all) != 3 {
		t.Fatal(e, len(all))
	}
	down, e := Desired(c, true, true, false)
	if e != nil || len(down) != 2 {
		t.Fatal("Surge failure removed independent route", e)
	}
	s := &mockSpeaker{routes: map[string]Advertisement{}}
	m := NewManager(s, 3, nil)
	if e = m.Reconcile(context.Background(), all); e != nil {
		t.Fatal(e)
	}
	if ready, e := m.Refresh(context.Background()); e != nil || !ready {
		t.Fatal(ready, e)
	}
	if e = m.Reconcile(context.Background(), down); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.routes["8.8.8.8/32"]; !ok {
		t.Fatal("DNS route withdrawn on Surge failure")
	}
	if _, ok := s.routes["10.20.0.0/16"]; !ok {
		t.Fatal("manual None route withdrawn")
	}
	before := s.adds
	tooMany := append(down, Advertisement{Prefix: "192.0.2.0/24"}, Advertisement{Prefix: "198.51.100.0/24"})
	if e = m.Reconcile(context.Background(), tooMany); e == nil || s.adds != before {
		t.Fatal("hard limit failed")
	}
	if len(s.routes) != 2 {
		t.Fatal("DNS-to-BGP route explosion")
	}
}
func TestIPv6AndRouteAttributes(t *testing.T) {
	r := Advertisement{Prefix: "2001:db8::/32", NextHop: "2001:db8::1", MED: 42, LocalPreference: 200, Communities: []string{"64512:100"}}
	p, e := path(r)
	if e != nil || p.Family.Afi.String() != "AFI_IP6" || len(p.Pattrs) != 5 {
		t.Fatal(e, p)
	}
	r.NextHop = "192.0.2.1"
	if _, e = path(r); e == nil {
		t.Fatal("mixed family next hop")
	}
}
