package dnsserver_test

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

func TestMonitoringConservesRequestsBoundsPoliciesAndRestores(t *testing.T) {
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 100; i++ {
				m.Begin()
				m.Finish(worker%2 == 0, model.DecisionResult{Decision: model.Unknown, Policy: fmt.Sprintf("policy-%d-%d", worker, i)}, "surge", "NOERROR", time.Millisecond)
			}
		}(worker)
	}
	workers.Wait()
	m.Begin() // persisted in-flight request unconfirmed by a process exit
	s := m.Snapshot()
	var flows, policies, decisions uint64
	for _, f := range s.Flows {
		flows += f.Queries
	}
	for _, p := range s.Policies {
		policies += p.Queries
	}
	for _, n := range s.Decisions {
		decisions += n
	}
	if s.Received != 801 || s.Completed != 800 || s.InFlight != 1 || flows != 800 || policies != 800 || decisions != 800 || len(s.Policies) != 513 {
		t.Fatalf("counts are not conserved or bounded: %+v", s)
	}
	s.Decisions["UNKNOWN"] = 0
	if m.Snapshot().Decisions["UNKNOWN"] != 800 {
		t.Fatal("snapshot aliases live maps")
	}
	s = m.Snapshot()
	restored := dnsserver.NewMonitoring(s).Snapshot()
	if restored.Received != 801 || restored.Completed != 800 || restored.Unconfirmed != 1 || restored.InFlight != 0 || restored.LatencyMS != 800 || restored.StartedAt != s.StartedAt {
		t.Fatalf("invalid restored statistics: %+v", restored)
	}
}

func TestMonitoringSourceNormalizationIsolationAndLegacyAttribution(t *testing.T) {
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	for _, src := range []string{"192.0.2.10", "::ffff:192.0.2.10", "192.0.2.20"} {
		ip := netip.MustParseAddr(src)
		m.BeginSource(ip)
		m.FinishSource(ip, true, model.DecisionResult{Decision: model.Surge, Policy: src}, "surge", "NOERROR", time.Millisecond)
	}
	s := m.View()
	if len(s.Sources[0].Counts.Flows) != 1 || s.Sources[0].Counts.Flows[0].Queries != 2 {
		t.Fatalf("source flow detail missing: %+v", s.Sources)
	}
	if len(s.Sources) != 2 || s.Sources[0].Counts.Received != 2 || len(s.Sources[0].Counts.Policies) != 0 {
		t.Fatal("source normalization or lightweight view failed", s)
	}
	one, ok := m.SourceSnapshot("192.0.2.20")
	if !ok || one.Completed != 1 || len(one.Policies) != 1 || one.Policies[0].Policy != "192.0.2.20" {
		t.Fatal("source policies leaked", one)
	}
	saved := m.Snapshot()
	saved.Sources = nil
	legacy := dnsserver.NewMonitoring(saved).View()
	if len(legacy.Sources) != 1 || legacy.Sources[0].Source != "legacy" || legacy.Sources[0].Counts.Completed != 3 {
		t.Fatal("old counts were attributed to a fabricated source", legacy)
	}
}

func TestMonitoringSourceAndPerSourcePolicyBoundsConserveTotals(t *testing.T) {
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	for i := 0; i < 520; i++ {
		ip := netip.AddrFrom4([4]byte{10, 0, byte(i / 256), byte(i % 256)})
		m.BeginSource(ip)
		m.FinishSource(ip, false, model.DecisionResult{Decision: model.Direct}, "real", "NOERROR", time.Millisecond)
	}
	view := m.View()
	var total uint64
	found := false
	for _, src := range view.Sources {
		total += src.Counts.Received
		if src.Source == "other" {
			found = src.Counts.Received == 8
		}
	}
	if total != 520 || len(view.Sources) != 513 || !found {
		t.Fatal("source overflow lost requests")
	}
	ip := netip.MustParseAddr("10.0.0.0")
	for i := 0; i < 70; i++ {
		m.BeginSource(ip)
		m.FinishSource(ip, false, model.DecisionResult{Decision: model.Direct, Policy: fmt.Sprint(i)}, "real", "NOERROR", time.Millisecond)
	}
	counts, _ := m.SourceSnapshot(ip.String())
	var policies uint64
	for _, p := range counts.Policies {
		policies += p.Queries
	}
	if counts.Completed != 71 || policies != 71 || len(counts.Policies) != 65 {
		t.Fatal("per-source overflow lost requests", counts)
	}
}

type monitorLocal struct{}

func (monitorLocal) ResolveLocal(_ context.Context, _ model.DNSQueryContext, q *dns.Msg) (*dns.Msg, bool) {
	return mocks.AddressReply(q, "192.0.2.1"), true
}

func TestMonitoringCountsActualPathsWithoutQueryHistory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     uint16
		decision model.Decision
		path     string
		setup    func(*dnsserver.Dispatcher)
	}{
		{name: "direct", kind: dns.TypeA, decision: model.Direct, path: "real"},
		{name: "non-address query", kind: dns.TypeTXT, decision: model.Direct, path: "real"},
		{name: "local", kind: dns.TypeA, decision: model.Direct, path: "local", setup: func(d *dnsserver.Dispatcher) { d.Local = monitorLocal{} }},
		{name: "reject", kind: dns.TypeA, decision: model.Reject, path: "blocked"},
		{name: "unknown goes to surge once", kind: dns.TypeA, decision: model.Unknown, path: "surge"},
		{name: "surge", kind: dns.TypeA, decision: model.Surge, path: "surge"},
		{name: "surge HTTPS local empty", kind: dns.TypeHTTPS, decision: model.Surge, path: "blocked"},
		{name: "gate blocks surge", kind: dns.TypeA, decision: model.Surge, path: "unavailable", setup: func(d *dnsserver.Dispatcher) { d.FakeIssuer = nil }},
		{name: "surge policy uses real DNS", kind: dns.TypeA, decision: model.Surge, path: "real", setup: func(d *dnsserver.Dispatcher) {
			d.PolicyDNS = []dnsserver.PolicyDNSBinding{{Field: "policy", Policy: "Proxy", Resolver: d.Real}}
		}},
		{name: "response cache", kind: dns.TypeA, decision: model.Direct, path: "cache", setup: func(d *dnsserver.Dispatcher) {
			d.Real = &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
				r.Trace.CacheHit = true
				return mocks.AddressReply(r.Message, "203.0.113.1"), nil
			}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}
			d := &dnsserver.Dispatcher{Monitoring: dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{}), Gate: gate, FakeIssuer: gate, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, Classifier: policyClassifier{result: model.DecisionResult{Decision: tc.decision, Confidence: model.Certain, Policy: "Proxy", FinalPolicy: "Node A"}}, Real: &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
				return mocks.AddressReply(r.Message, "203.0.113.1"), nil
			}}, Surge: &mocks.Surge{Fake: func(_ context.Context, q *dns.Msg) (*dns.Msg, error) { return mocks.AddressReply(q, "198.18.1.1"), nil }}}
			if tc.setup != nil {
				tc.setup(d)
			}
			q := new(dns.Msg)
			q.SetQuestion("monitor.test.", tc.kind)
			d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
			s := d.Monitoring.Snapshot()
			if s.Completed != 1 || s.Received != 1 || s.InFlight != 0 || len(s.Flows) != 1 || s.Flows[0].Destination != tc.path || s.Decisions[string(tc.decision)] != 1 {
				t.Fatalf("wrong actual flow: %+v", s)
			}
		})
	}
}

func TestMonitoringPolicyFallbackAndRealCache(t *testing.T) {
	wire := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.1"), nil
	}}
	real, err := resolver.NewReal(wire, []resolver.ResolverEndpoint{{Protocol: "udp", Address: "192.0.2.53:53", Enabled: true}}, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	primary := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		return nil, context.DeadlineExceeded
	}}
	d, _, q := fallbackDispatcher(primary, "upstreams", real)
	d.Monitoring = dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	for i := 0; i < 2; i++ {
		d.Dispatch(context.Background(), model.DNSQueryContext{Intercepted: true, OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53}, q)
	}
	s := d.Monitoring.Snapshot()
	paths := map[string]uint64{}
	for _, f := range s.Flows {
		paths[f.Destination] += f.Queries
	}
	if s.Completed != 2 || paths["real"] != 1 || paths["cache"] != 1 || len(s.Policies) != 2 || s.Policies[0].Queries+s.Policies[1].Queries != 2 || s.Policies[0].Policy != "Proxy" || wire.Calls.Load() != 1 {
		t.Fatalf("fallback or cache double counted: %+v", s)
	}
}
