package state

import (
	"github.com/miekg/dns"
	"surge-controller/internal/config"
	"testing"
	"time"
)

func TestRouteBeforeFakeGraceAndRecovery(t *testing.T) {
	c := config.Default().Settings.Health
	c.GraceSeconds = 5
	c.StabilizeSeconds = 2
	m := New(c, "auto")
	now := time.Now()
	live := Input{Controller: true, DNS: true, Surge: true, FakeIP: true}
	if p := m.Step(now, live); p.AllowFakeIP || !p.KeepSurgeRoutes || p.State != Recovering {
		t.Fatal(p)
	}
	live.Routes = true
	m.Step(now, live)
	if p := m.Step(now.Add(time.Second), live); p.AllowFakeIP {
		t.Fatal("issuance before stabilization")
	}
	if p := m.Step(now.Add(2*time.Second), live); !p.AllowFakeIP || p.State != Healthy {
		t.Fatal(p)
	}
	currentLease, ok := m.AcquireFakeIP()
	if !ok {
		t.Fatal("no issuance lease")
	}
	observed := currentLease.(*lease)
	r := new(dns.Msg)
	rr, _ := dns.NewRR("example.com. 30 IN A 198.18.1.1")
	r.Answer = []dns.RR{rr}
	observed.Observe(r)
	live.Surge = false
	p := m.Step(now.Add(3*time.Second), live)
	if p.AllowFakeIP || !p.DirectFallback || !p.KeepSurgeRoutes || p.State != DirectFallback || currentLease.Valid() {
		t.Fatal(p)
	}
	currentLease.Release()
	if p = m.Step(now.Add(10*time.Second), live); !p.KeepSurgeRoutes {
		t.Fatal("withdrawal before issued TTL expired")
	}
	if p = m.Step(now.Add(40*time.Second), live); p.KeepSurgeRoutes || p.State != DirectFallback {
		t.Fatal(p)
	}
	live.Surge = true
	live.Routes = false
	p = m.Step(now.Add(41*time.Second), live)
	if p.AllowFakeIP || !p.KeepSurgeRoutes {
		t.Fatal("recovery did not order routes first")
	}
	live.Routes = true
	m.Step(now.Add(42*time.Second), live)
	p = m.Step(now.Add(44*time.Second), live)
	if !p.AllowFakeIP {
		t.Fatal(p)
	}
	live.Routes = false
	p = m.Step(now.Add(45*time.Second), live)
	if p.AllowFakeIP {
		t.Fatal("Fake-IP survives routing readiness loss")
	}
}
func TestModesControllerLossAndShutdown(t *testing.T) {
	c := config.Default().Settings.Health
	c.StabilizeSeconds = 1
	m := New(c, "force-direct")
	i := Input{true, true, true, true, true}
	now := time.Now()
	p := m.Step(now, i)
	if !p.DirectFallback || p.AllowFakeIP || p.KeepSurgeRoutes {
		t.Fatal(p)
	}
	m.SetMode("force-surge")
	m.Step(now, i)
	p = m.Step(now.Add(time.Second), i)
	if !p.AllowFakeIP {
		t.Fatal(p)
	}
	m.Stop()
	p = m.Step(now.Add(2*time.Second), i)
	if p.State != Stopping || p.AllowFakeIP || p.KeepSurgeRoutes {
		t.Fatal(p)
	}
	m = New(c, "auto")
	i.Controller = false
	p = m.Step(now, i)
	if p.State != Stopping || p.AllowFakeIP {
		t.Fatal(p)
	}
}
func TestHealthHysteresis(t *testing.T) {
	h := NewHealth(3, 3)
	for i := 0; i < 2; i++ {
		if h.Observe("Surge", true, 0, "") {
			t.Fatal("early UP")
		}
	}
	if !h.Observe("Surge", true, 0, "") {
		t.Fatal("no UP")
	}
	for i := 0; i < 2; i++ {
		if !h.Observe("Surge", false, 0, "down") {
			t.Fatal("early DOWN")
		}
	}
	if h.Observe("Surge", false, 0, "down") {
		t.Fatal("no DOWN")
	}
}

func TestWithdrawalWaitsForResponseWrite(t *testing.T) {
	m := New(config.Default().Settings.Health, "auto")
	m.BeginResponse()
	m.Stop()
	if _, ok := m.AcquireFakeIP(); ok {
		t.Fatal("issuance survived stop")
	}
	drained := make(chan struct{})
	go func() { m.DrainResponses(); close(drained) }()
	select {
	case <-drained:
		t.Fatal("route withdrawal passed an active response write")
	case <-time.After(20 * time.Millisecond):
	}
	m.EndResponse()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("response barrier failed to drain")
	}
}
