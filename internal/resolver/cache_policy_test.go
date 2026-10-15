package resolver_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"surge-controller/internal/cache"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
	"testing"
	"time"
)

func TestPerUpstreamOptimisticDisablePreservesOrdinaryTTLRestore(t *testing.T) {
	ep := resolver.ResolverEndpoint{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}
	q := new(dns.Msg)
	q.SetQuestion("policy.test.", dns.TypeA)
	req := model.ResolveRequest{Message: q}
	tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.1"), nil
	}}
	old, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{ep}, false, 10, resolver.OptimisticOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Resolve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	saved := old.ResponseSnapshot()
	old.Close()
	off := false
	r, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{ep}, false, 10, resolver.OptimisticOptions{Enabled: true, Policies: map[string]cache.Policy{ep.CacheIdentity(): {Optimistic: &off}}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.RestoreResponses(saved) != 1 {
		t.Fatal("TTL-valid ordinary response not retained")
	}
	before := tr.Calls.Load()
	r.Resolve(context.Background(), req)
	if tr.Calls.Load() != before || r.Cache.Stats().OptimisticHits != 0 {
		t.Fatal("ordinary restore refreshed or counted stale hit")
	}
	if len(r.ResponseSnapshot(true)) != 0 {
		t.Fatal("non-optimistic upstream persisted")
	}
	r.Cache.Flush()
	expired := append([]resolver.ResponseCacheRecord(nil), saved...)
	age := time.Now().Sub(expired[0].Stored) + resolver.ResponseTTL(mocks.AddressReply(q, "203.0.113.1")) + time.Second
	expired[0].Stored = expired[0].Stored.Add(-age)
	expired[0].ExpiresAt = expired[0].ExpiresAt.Add(-age)
	if r.RestoreResponses(expired) != 0 {
		t.Fatal("expired ordinary response restored")
	}
	for i := 0; i < 2; i++ {
		if _, err := r.Resolve(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if tr.Calls.Load() != before+1 || r.Cache.Stats().OptimisticHits != 0 || r.Cache.Stats().Entries != 1 {
		t.Fatal("non-optimistic upstream failed ordinary caching")
	}
	r.ConfigurationChanged("updated", true)
	if r.Cache.Stats().Entries != 0 {
		t.Fatal("non-optimistic upstream retained invalidated response")
	}
}

func TestResponseRefreshAttributesMismatchToActualOldUpstream(t *testing.T) {
	for _, disableCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("disable_cache=%v", disableCache), func(t *testing.T) {
			endpoints := []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.1:53"}, {Enabled: true, Protocol: "udp", Address: "192.0.2.2:53"}}
			oldID := endpoints[1].CacheIdentity()
			newID := endpoints[0].CacheIdentity()
			off := false
			entered, release := make(chan struct{}), make(chan struct{})
			tr := &mocks.Transport{Func: func(ctx context.Context, ep resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
				if ep.CacheIdentity() == oldID {
					return mocks.AddressReply(q, "203.0.113.1"), nil
				}
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return mocks.AddressReply(q, "203.0.113.2"), nil
			}}
			policy := cache.Policy{Optimistic: &off}
			wantEntries := 1
			if disableCache {
				policy.Enabled = &off
				wantEntries = 0
			}
			r, err := resolver.NewReal(tr, endpoints, false, 10, resolver.OptimisticOptions{Enabled: true, Policies: map[string]cache.Policy{newID: policy}})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			q := new(dns.Msg)
			q.SetQuestion("refresh.test.", dns.TypeA)
			req := model.ResolveRequest{Message: q}
			r.Resolve(context.Background(), req)
			r.ConfigurationChanged("updated", true)
			for i := 0; i < 4; i++ {
				r.Resolve(context.Background(), req)
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("refresh missing")
			}
			close(release)
			deadline := time.Now().Add(time.Second)
			for r.Cache.Stats().OptimisticCompared < 4 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			s := r.Cache.Stats()
			old := s.Groups[oldID]
			if s.Entries != wantEntries || s.Groups[newID].Entries != wantEntries || old.OptimisticHits != 4 || old.OptimisticCompared != 4 || old.OptimisticMismatches != 4 || s.Groups[newID].OptimisticCompared != 0 {
				t.Fatal("missing fresh ordinary result or wrong endpoint attribution", s)
			}

		})
	}
}

func TestSelectiveResponseFailedRefreshRemainsUnverified(t *testing.T) {
	ep := resolver.ResolverEndpoint{Enabled: true, Protocol: "udp", Address: "192.0.2.1:53"}
	failed := make(chan struct{}, 1)
	first := true
	tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		if first {
			first = false
			return mocks.AddressReply(q, "203.0.113.1"), nil
		}
		failed <- struct{}{}
		return nil, errors.New("failed")
	}}
	r, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{ep}, false, 10, resolver.OptimisticOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	q := new(dns.Msg)
	q.SetQuestion("failed.test.", dns.TypeA)
	req := model.ResolveRequest{Message: q}
	r.Resolve(context.Background(), req)
	r.ConfigurationChanged("updated", true)
	r.Resolve(context.Background(), req)
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("failed refresh missing")
	}
	s := r.Cache.Stats().Groups[ep.CacheIdentity()]
	if s.OptimisticHits != 1 || s.OptimisticCompared != 0 || s.OptimisticMismatches != 0 {
		t.Fatal("failure counted as consistency", s)
	}
}

func TestUpstreamIdentityIncludesDoHPathAndTLSName(t *testing.T) {
	a := resolver.ResolverEndpoint{Protocol: "doh", Address: "192.0.2.53:443", URL: "https://dns.test/first"}
	b := a
	b.URL = "https://dns.test/second"
	if a.CacheIdentity() == b.CacheIdentity() {
		t.Fatal("DoH URL identity collapsed")
	}
	a.Protocol = "dot"
	a.TLSName = "one.test"
	b = a
	b.TLSName = "two.test"
	if a.CacheIdentity() == b.CacheIdentity() {
		t.Fatal("TLS name identity collapsed")
	}
}

func TestDisabledResponseCacheDoesNotReuseStoreOrRestore(t *testing.T) {
	ep := resolver.ResolverEndpoint{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}
	q := new(dns.Msg)
	q.SetQuestion("disabled.test.", dns.TypeA)
	req := model.ResolveRequest{Message: q}
	tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.1"), nil
	}}
	old, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{ep}, false, 10, resolver.OptimisticOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Resolve(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	saved := old.ResponseSnapshot()
	old.Close()
	for _, global := range []bool{false, true} {
		t.Run(fmt.Sprintf("global=%v", global), func(t *testing.T) {
			off := false
			options := resolver.OptimisticOptions{Enabled: true}
			if global {
				options.CacheEnabled = &off
			} else {
				options.Policies = map[string]cache.Policy{ep.CacheIdentity(): {Enabled: &off}}
			}
			r, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{ep}, false, 10, options)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if r.RestoreResponses(saved) != 0 {
				t.Fatal("disabled response cache restored data")
			}
			before := tr.Calls.Load()
			for i := 0; i < 2; i++ {
				trace := new(model.ResolutionTrace)
				req.Trace = trace
				msg, err := r.Resolve(context.Background(), req)
				if err != nil || msg.Answer[0].(*dns.A).A.String() != "203.0.113.1" || trace.CacheHit {
					t.Fatal("disabled cache changed result", msg, err, trace)
				}
			}
			if tr.Calls.Load() != before+2 || r.Cache.Stats().Entries != 0 || len(r.ResponseSnapshot()) != 0 || len(r.ResponseSnapshot(true)) != 0 {
				t.Fatal("disabled cache reused, stored or persisted data", r.Cache.Stats())
			}
		})
	}
}
