package routing

import (
	"context"
	"errors"
	"surge-controller/internal/config"
	"surge-controller/internal/state"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestAutomaticNextHopWaitsForIssuedTTLAndKeepsIndependentRoutes(t *testing.T) {
	d := config.Default()
	d.Settings.Surge.NextHop4 = ""
	d.Settings.Health.GraceSeconds = 0
	d.Settings.Health.StabilizeSeconds = 1
	d.Routes = []config.Route{{ID: "independent", Prefix: "10.20.0.0/16", NextHop: "192.0.2.1", Binding: "none", Enabled: true}}
	b := &backend{ready: true}
	m := newManager(d, b, "")
	machine := state.New(d.Settings.Health, "auto")
	now := time.Now()
	input := state.Input{Controller: true, DNS: true, Surge: true, FakeIP: true}
	step := func(at time.Time) state.Plan {
		t.Helper()
		input.Surge = m.NextHop4Ready()
		input.Routes, _ = m.Refresh(context.Background())
		plan := machine.Step(at, input)
		if !plan.KeepSurgeRoutes {
			machine.DrainResponses()
			m.ApplyNextHop4()
		}
		if err := m.Reconcile(context.Background(), m.Desired(true, true, plan.KeepSurgeRoutes)); err != nil {
			t.Fatal(err)
		}
		input.Routes, _ = m.Refresh(context.Background())
		return machine.Step(at, input)
	}
	if !m.NextHop4ProbeDue(now) {
		t.Fatal("startup probe missing")
	}
	step(now)
	if len(b.rows) != 1 || b.rows[0].Kind != "manual" {
		t.Fatal("unresolved path published", b.rows)
	}
	m.ObserveNextHop4("192.0.2.141", nil, now)
	if m.NextHop4ProbeDue(now.Add(29*time.Second)) || !m.NextHop4ProbeDue(now.Add(30*time.Second)) {
		t.Fatal("unexpected probe frequency")
	}
	step(now)
	step(now)
	if p := step(now.Add(2 * time.Second)); !p.AllowFakeIP {
		t.Fatal("did not open after route announcement", p)
	}
	lease, ok := machine.AcquireFakeIP()
	if !ok {
		t.Fatal("missing lease")
	}
	reply := new(dns.Msg)
	reply.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}}}
	lease.(interface{ Observe(*dns.Msg) }).Observe(reply)
	lease.Release()
	m.ObserveNextHop4("", errors.New("probe unavailable"), now.Add(3*time.Second))
	if p := step(now.Add(3 * time.Second)); p.AllowFakeIP || !p.KeepSurgeRoutes {
		t.Fatal("failed probe discarded unexpired paths", p)
	}
	m.ObserveNextHop4("192.0.2.142", nil, now.Add(4*time.Second))
	if p := step(now.Add(4 * time.Second)); p.AllowFakeIP || !p.KeepSurgeRoutes {
		t.Fatal("old TTL not retained", p)
	}
	if got := m.NextHop4(); got.Address != "192.0.2.141" || !got.Pending {
		t.Fatal(got)
	}
	for _, row := range b.rows {
		if row.Kind == "fake-ip" && row.NextHop != "192.0.2.141" {
			t.Fatal("changed before TTL expiry", row)
		}
	}
	step(now.Add(61 * time.Second))
	if got := m.NextHop4(); got.Address != "192.0.2.142" || got.Pending {
		t.Fatal(got)
	}
	step(now.Add(62 * time.Second))
	if p := step(now.Add(63 * time.Second)); !p.AllowFakeIP {
		t.Fatal("new route did not recover", p)
	}
	m.ObserveNextHop4("", errors.New("probe unavailable"), now.Add(64*time.Second))
	if p := step(now.Add(64 * time.Second)); p.AllowFakeIP {
		t.Fatal("failed probe opened issuance", p)
	}
	if len(b.rows) != 1 || b.rows[0].Kind != "manual" || b.closed {
		t.Fatal("unrelated route/backend changed", b.rows)
	}
	if m.NextHop4().Address != "" || m.NextHop4().Error == "" {
		t.Fatal(m.NextHop4())
	}
	if m.document.Settings.Surge.NextHop4 != "" {
		t.Fatal("detection overwrote saved setting")
	}
}

func TestAutomaticNextHopStaticConfirmationAndSafety(t *testing.T) {
	d := staticDocument()
	d.Settings.Surge.NextHop4 = ""
	d.Settings.Routing.StaticFakeIPNextHop4 = "192.0.2.141"
	m := newManager(d, nil, "")
	now := time.Now()
	m.ObserveNextHop4("192.0.2.141", nil, now)
	m.ApplyNextHop4()
	if ready, _ := m.Refresh(context.Background()); !ready {
		t.Fatal("saved confirmation lost")
	}
	m.ObserveNextHop4("192.0.2.142", nil, now)
	m.ApplyNextHop4()
	if ready, _ := m.Refresh(context.Background()); ready || m.Status().Routes[0].OperatorConfirmed {
		t.Fatal("changed address reused confirmation")
	}
	d.Settings.Routing.StaticFakeIPNextHop4 = "192.0.2.142"
	m.UpdateDocument(d)
	if ready, _ := m.Refresh(context.Background()); !ready {
		t.Fatal("explicit confirmation did not recover")
	}
	restarted := newManager(d, nil, "")
	restarted.ObserveNextHop4("192.0.2.141", nil, now)
	restarted.ApplyNextHop4()
	if ready, _ := restarted.Refresh(context.Background()); ready {
		t.Fatal("restart confirmed a different VM address")
	}
	for _, address := range []string{"", "198.18.1.1", "127.0.0.1", "169.254.1.1", "::1"} {
		m.ObserveNextHop4(address, nil, now)
		if m.NextHop4Ready() || m.NextHop4().Error == "" {
			t.Fatal("unsafe detection accepted", address)
		}
	}
	m.ObserveNextHop4("192.0.2.142", nil, now)
	d.Interception = []config.Interception{{ID: "unsafe", Prefix: "192.0.2.142/32"}}
	m.UpdateDocument(d)
	if m.NextHop4Ready() || !m.NextHop4ProbeDue(now) {
		t.Fatal("new interception bypassed safety validation")
	}
	d.Settings.Surge.NextHop4 = "192.0.2.150"
	m.UpdateDocument(d)
	if m.NextHop4ProbeDue(now.Add(time.Hour)) || !m.NextHop4Ready() || m.NextHop4().Automatic || m.NextHop4().Address != "192.0.2.150" {
		t.Fatal("manual override not preferred")
	}
}
