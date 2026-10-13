package resolver_test

import (
	"context"
	"github.com/miekg/dns"
	"net/netip"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
	"sync/atomic"
	"testing"
	"time"
)

func TestScopedSurgeResponsesPartitionClientsAndRejectOldGenerationWrites(t *testing.T) {
	var epoch atomic.Uint64
	epoch.Store(1)
	var allowed atomic.Bool
	allowed.Store(true)
	var block atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	tr := &mocks.Transport{Func: func(ctx context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		if block.Load() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return mocks.AddressReply(q, "198.18.1.1"), nil
	}}
	r, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "127.0.0.1:6153"}}, false, 10, resolver.OptimisticOptions{Enabled: true, SourceIdentity: "surge://127.0.0.1:6153", PartitionByClient: true, Scope: func() (uint64, bool) { return epoch.Load(), allowed.Load() }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	q := new(dns.Msg)
	q.SetQuestion("scoped.test.", dns.TypeA)
	req := model.ResolveRequest{Query: model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.1")}, Message: q, Trace: &model.ResolutionTrace{}}
	if _, err = r.Resolve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve(context.Background(), req); err != nil || !req.Trace.CacheHit {
		t.Fatal("fresh cache miss", err)
	}
	req.Query.ClientIP = netip.MustParseAddr("192.0.2.2")
	*req.Trace = model.ResolutionTrace{}
	if _, err = r.Resolve(context.Background(), req); err != nil || req.Trace.CacheHit {
		t.Fatal("client cache partition lost", err)
	}
	if tr.Calls.Load() != 2 {
		t.Fatal("unexpected exchange count", tr.Calls.Load())
	}
	r.ConfigurationChanged("refresh", true)
	block.Store(true)
	if _, err = r.Resolve(context.Background(), req); err != nil || !req.Trace.OptimisticCacheHit {
		t.Fatal("stale cache not served", err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	allowed.Store(false)
	epoch.Store(2)
	if _, err = r.Resolve(context.Background(), req); err == nil {
		t.Fatal("disabled Fake-IP scope served cache")
	}
	close(release)
	r.Close()
	s := r.Cache.Stats()
	if s.Entries != 0 || s.OptimisticHits != 1 || s.OptimisticCompared != 0 {
		t.Fatal("obsolete refresh counted as verified or wrote cache", s)
	}
	if s.Groups["surge://127.0.0.1:6153"].OptimisticHits != 1 {
		t.Fatal("Surge response attributed to real DNS endpoint", s)
	}
}
