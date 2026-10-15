package app

import (
	"context"
	"github.com/miekg/dns"
	"net/netip"
	"surge-controller/internal/cache"
	"surge-controller/internal/config"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/internal/state"
	"surge-controller/internal/surge"
	"surge-controller/tests/mocks"
	"sync/atomic"
	"testing"
	"time"
)

func TestSurgeOptimisticResponseTTLRefreshSafetyAndCounterPersistence(t *testing.T) {
	var changed atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	transport := &mocks.Transport{Func: func(ctx context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		address := "198.18.1.1"
		if changed.Load() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			address = "198.18.1.2"
		}
		reply := mocks.AddressReply(q, address)
		reply.Answer[0].Header().Ttl = 1
		return reply, nil
	}}
	d := config.Default()
	d.Settings.Cache.OptimisticResponses = true
	rt := &Runtime{settings: d, machine: state.New(config.Health{}, "auto"), responseScope: "test"}
	rt.machine.Step(time.Now(), state.Input{Controller: true, DNS: true, Surge: true, FakeIP: true, Routes: true})
	rt.adapter = &surge.Adapter{FakeTransport: transport, FakeEndpoint: resolver.ResolverEndpoint{Enabled: true, Protocol: "udp", Address: "127.0.0.1:6153"}}
	maxTTL := uint32(1)
	rt.dispatcher = &dnsserver.Dispatcher{Classifier: &fixedSurgeDecision{}, Gate: rt.machine, FakeIssuer: rt.machine, FakeTTLMax: &maxTTL, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, History: dnsserver.NewHistory(10, true)}
	rt.dispatcher.Surge = &observedSurge{SurgeAdapter: rt.adapter, rt: rt}
	var err error
	rt.surgeResponses, err = resolver.NewReal(&surgeResponseTransport{rt}, []resolver.ResolverEndpoint{rt.adapter.FakeEndpoint}, false, 8, resolver.OptimisticOptions{Enabled: true, SourceIdentity: "surge://127.0.0.1:6153", PartitionByClient: true, Scope: func() (uint64, bool) { s := rt.machine.Snapshot(); return s.Generation, s.AllowFakeIP }})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.surgeResponses.Close()
	q := new(dns.Msg)
	q.SetQuestion("surge-cache.test.", dns.TypeA)
	query := model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), Transport: model.UDP, Question: q.Question[0]}
	first := rt.dispatcher.Dispatch(context.Background(), query, q)
	if first.Rcode != dns.RcodeSuccess || len(first.Answer) != 1 {
		t.Fatal("Surge initial lookup failed", first)
	}
	rt.dispatcher.Dispatch(context.Background(), query, q)
	if transport.Calls.Load() != 1 {
		t.Fatal("fresh Surge response not reused")
	}
	time.Sleep(1100 * time.Millisecond)
	changed.Store(true)
	stale := rt.dispatcher.Dispatch(context.Background(), query, q)
	if stale.Rcode != dns.RcodeSuccess || stale.Answer[0].(*dns.A).A.String() != "198.18.1.1" || stale.Answer[0].Header().Ttl > maxTTL {
		t.Fatal("expired Surge response or issuance TTL wrong", stale)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for rt.surgeResponses.Cache.Stats().OptimisticCompared != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stats := responseCacheStats(rt)["groups"].(map[string]cache.GroupStats)["surge://127.0.0.1:6153"]
	if stats.OptimisticHits != 1 || stats.OptimisticCompared != 1 || stats.OptimisticMismatches != 1 {
		t.Fatal("Surge comparison missing", stats)
	}
	snapshot := rt.responseSnapshot(true)
	if len(snapshot.Pools) != 0 || snapshot.OptimisticStats.ByGroup["surge://127.0.0.1:6153"].Mismatches != 1 {
		t.Fatal("Fake-IP answers persisted or reliability lost", snapshot)
	}
	next := &Runtime{settings: d, responseScope: "test"}
	next.restoreResponses(&snapshot)
	if responseCacheStats(next)["optimistic_mismatches"] != uint64(1) {
		t.Fatal("Surge reliability not restored")
	}
	rt.machine.Step(time.Now(), state.Input{Controller: true, DNS: true, Surge: false, FakeIP: true, Routes: true})
	denied := rt.dispatcher.Dispatch(context.Background(), query, q)
	if len(denied.Answer) != 0 {
		t.Fatal("Fake-IP cached answer bypassed health gate", denied)
	}
}

type fixedSurgeDecision struct{}

func (*fixedSurgeDecision) Classify(context.Context, model.DNSQueryContext) (model.DecisionResult, error) {
	return model.DecisionResult{Decision: model.Surge, Confidence: model.Certain}, nil
}

func TestSurgeResponseTransportRejectsInvalidAddressesAndEpochChanges(t *testing.T) {
	machine := state.New(config.Health{}, "auto")
	healthy := state.Input{Controller: true, DNS: true, Surge: true, FakeIP: true, Routes: true}
	machine.Step(time.Now(), healthy)
	rt := &Runtime{machine: machine, dispatcher: &dnsserver.Dispatcher{FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}}}
	rt.adapter = &surge.Adapter{FakeTransport: &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.1"), nil
	}}}
	q := new(dns.Msg)
	q.SetQuestion("invalid.test.", dns.TypeA)
	transport := &surgeResponseTransport{rt}
	if _, err := transport.Exchange(context.Background(), resolver.ResolverEndpoint{}, q); err == nil {
		t.Fatal("invalid response admitted for refresh")
	}
	rt.adapter.FakeTransport = &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		machine.Step(time.Now(), state.Input{Controller: true})
		return mocks.AddressReply(q, "198.18.1.1"), nil
	}}
	if _, err := transport.Exchange(context.Background(), resolver.ResolverEndpoint{}, q); err == nil {
		t.Fatal("obsolete response admitted for refresh")
	}
	if rt.fakeErrors.Load() != 2 {
		t.Fatal("rejections not recorded")
	}
}

func TestSurgeResponseToggleOnlyCreatesAllowedPool(t *testing.T) {
	a := fixtureApp(t)
	d := a.Configuration()
	d.Settings.Cache.OptimisticResponses = true
	off := false
	identity := "surge://" + d.Settings.Surge.DNS
	d.Settings.Cache.Upstreams = map[string]cache.Policy{identity: {Enabled: &off}}
	if err := a.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if a.runtime.surgeResponses != nil {
		t.Fatal("disabled Surge source still cached")
	}
	delete(d.Settings.Cache.Upstreams, identity)
	if err := a.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if a.runtime.surgeResponses == nil {
		t.Fatal("enabled Surge source missing")
	}
}
