package integration

import (
	"context"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
	"surge-controller/internal/classifier"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/tests/mocks"
)

func BenchmarkCachedDispatcher(b *testing.B) {
	adapter := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	}}
	engine, err := classifier.New(context.Background(), adapter, classifier.DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(engine.Close)
	real := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		return mocks.AddressReply(r.Message, "203.0.113.7"), nil
	}}
	gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}
	dispatcher := &dnsserver.Dispatcher{Classifier: engine, Real: real, Surge: adapter, Gate: gate}
	query := new(dns.Msg)
	query.SetQuestion("benchmark.test.", dns.TypeA)
	ctx := context.Background()
	qctx := model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.50")}
	_ = dispatcher.Dispatch(ctx, qctx, query)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r := dispatcher.Dispatch(ctx, qctx, query)
			if r.Rcode != 0 {
				b.Fatal("cached dispatch failed")
			}
		}
	})
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/s")
	if adapter.Calls.Load() != 1 {
		b.Fatal("cached queries invoked Surge evaluator")
	}
}
