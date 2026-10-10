package resolver_test

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

func TestForwardingStrategyAndCacheIsolation(t *testing.T) {
	calls := make(chan string, 8)
	tr := &mocks.Transport{Func: func(_ context.Context, e resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		calls <- e.Address
		return mocks.AddressReply(q, "203.0.113.7"), nil
	}}
	endpoints := []resolver.ResolverEndpoint{{Name: "configured", Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}
	r, err := resolver.NewReal(tr, endpoints, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	req := model.ResolveRequest{Query: model.DNSQueryContext{OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53, Transport: model.UDP, Intercepted: true}, Message: q}
	first, err := r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got := <-calls; got != "8.8.8.8:53" {
		t.Fatal("original resolver was not preserved")
	}
	q.Id++
	second, err := r.Resolve(context.Background(), req)
	if err != nil || second.Id != q.Id {
		t.Fatal("cached query ID not restored")
	}
	if tr.Calls.Load() != 1 {
		t.Fatal("cache missed")
	}
	first.Answer = nil
	if len(second.Answer) != 1 {
		t.Fatal("cached answer shared mutable storage")
	}
	req.Query.OriginalDestinationIP = netip.MustParseAddr("1.1.1.1")
	_, err = r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if <-calls != "1.1.1.1:53" {
		t.Fatal("split-horizon cache contamination")
	}
	configured, err := resolver.NewReal(tr, endpoints, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = configured.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if <-calls != "192.0.2.53:53" {
		t.Fatal("configured forwarding strategy failed")
	}
	req.UseConfiguredUpstreams = true
	_, err = r.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if <-calls != "192.0.2.53:53" {
		t.Fatal("explicit configured-upstream request still used original destination")
	}
}
func TestNegativeTTLAndFailure(t *testing.T) {
	q := new(dns.Msg)
	q.SetQuestion("missing.test.", dns.TypeA)
	r := new(dns.Msg)
	r.SetRcode(q, dns.RcodeNameError)
	if resolver.ResponseTTL(r) != 0 {
		t.Fatal("negative answer without SOA cached")
	}
	r.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Name: "test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60}, Ns: "ns.test.", Mbox: "host.test.", Minttl: 10}}
	if resolver.ResponseTTL(r) != 10*time.Second {
		t.Fatal("negative TTL is not min(SOA TTL, MINIMUM)")
	}
	tr := &mocks.Transport{Func: func(context.Context, resolver.ResolverEndpoint, *dns.Msg) (*dns.Msg, error) {
		return nil, errors.New("down")
	}}
	real, _ := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10)
	if _, err := real.Resolve(context.Background(), model.ResolveRequest{Message: q}); err == nil {
		t.Fatal("resolver failure hidden")
	}
	if real.Stats()[0].Failures != 1 {
		t.Fatal("failure statistics missing")
	}
}

func TestPolicyPoolRetriesDNSFailuresButKeepsNegativeAnswers(t *testing.T) {
	for _, code := range []int{dns.RcodeServerFailure, dns.RcodeRefused, dns.RcodeNameError, dns.RcodeSuccess} {
		t.Run(dns.RcodeToString[code], func(t *testing.T) {
			calls := 0
			tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
				calls++
				if calls == 1 {
					m := new(dns.Msg)
					m.SetRcode(q, code)
					return m, nil
				}
				return mocks.AddressReply(q, "203.0.113.2"), nil
			}}
			real, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}, {Enabled: true, Protocol: "tcp", Address: "192.0.2.54:53"}}, false, 10)
			if err != nil {
				t.Fatal(err)
			}
			q := new(dns.Msg)
			q.SetQuestion("test.example.", dns.TypeA)
			r, err := real.Resolve(context.Background(), model.ResolveRequest{Message: q, RetryDNSFailures: true})
			if err != nil {
				t.Fatal(err)
			}
			if code == dns.RcodeServerFailure || code == dns.RcodeRefused {
				if calls != 2 || r.Rcode != 0 || len(r.Answer) != 1 {
					t.Fatal("pool did not retry", r, calls)
				}
			} else if calls != 1 || r.Rcode != code {
				t.Fatal("completed answer retried", r, calls)
			}
		})
	}
}

func TestOptimisticResponseReturnsCacheBeforeRefresh(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	tr := &mocks.Transport{Func: func(ctx context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		if q.Id == 100 {
			return mocks.AddressReply(q, "203.0.113.1"), nil
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return mocks.AddressReply(q, "203.0.113.2"), nil
	}}
	r, _ := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10, resolver.OptimisticOptions{Enabled: true})
	defer r.Close()
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	q.Id = 100
	if _, err := r.Resolve(context.Background(), model.ResolveRequest{Message: q}); err != nil {
		t.Fatal(err)
	}
	r.ConfigurationChanged("resource updated", true)
	q.Id = 200
	trace := &model.ResolutionTrace{}
	done := make(chan *dns.Msg, 1)
	go func() {
		reply, _ := r.Resolve(context.Background(), model.ResolveRequest{Message: q, Trace: trace})
		done <- reply
	}()
	select {
	case reply := <-done:
		if reply == nil || reply.Id != 200 || reply.Answer[0].(*dns.A).A.String() != "203.0.113.1" || !trace.CacheHit {
			t.Fatal("cached response not returned immediately")
		}
	case <-time.After(time.Second):
		t.Fatal("response waited for refresh")
	}
	<-entered
	for i := 0; i < 32; i++ {
		r.Resolve(context.Background(), model.ResolveRequest{Message: q})
	}
	if tr.Calls.Load() != 2 {
		t.Fatal("duplicate response refreshes", tr.Calls.Load())
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.Cache.Stats().StaleEntries == 0 {
			reply, err := r.Resolve(context.Background(), model.ResolveRequest{Message: q})
			if err != nil || reply.Answer[0].(*dns.A).A.String() != "203.0.113.2" {
				t.Fatal("new response missing")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("response refresh did not replace cache")
}

func TestOptimisticResponseKeepsOldAnswerOnUpstreamFailure(t *testing.T) {
	for _, rcode := range []int{-1, dns.RcodeServerFailure} {
		first := true
		tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
			if first {
				first = false
				return mocks.AddressReply(q, "203.0.113.1"), nil
			}
			if rcode < 0 {
				return nil, errors.New("upstream unavailable")
			}
			m := new(dns.Msg)
			m.SetRcode(q, rcode)
			return m, nil
		}}
		r, _ := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10, resolver.OptimisticOptions{Enabled: true})
		q := new(dns.Msg)
		q.SetQuestion("example.test.", dns.TypeA)
		r.Resolve(context.Background(), model.ResolveRequest{Message: q})
		r.ConfigurationChanged("resource updated", true)
		r.Resolve(context.Background(), model.ResolveRequest{Message: q})
		deadline := time.Now().Add(time.Second)
		for r.Stats()[0].Failures == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if r.Stats()[0].Failures != 1 || r.Cache.Stats().Entries != 1 || r.Cache.Stats().StaleEntries != 1 {
			t.Fatal("failed refresh discarded cached response", r.Cache.Stats())
		}
		reply, err := r.Resolve(context.Background(), model.ResolveRequest{Message: q})
		if err != nil || reply.Rcode != dns.RcodeSuccess || reply.Answer[0].(*dns.A).A.String() != "203.0.113.1" {
			t.Fatal("failure replaced previous answer")
		}
		r.Close()
	}
}

func TestOptimisticResponseRefreshCannotResurrectManualFlush(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	first := true
	tr := &mocks.Transport{Func: func(ctx context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		if first {
			first = false
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
	r, _ := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10, resolver.OptimisticOptions{Enabled: true})
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	r.Resolve(context.Background(), model.ResolveRequest{Message: q})
	r.ConfigurationChanged("resource updated", true)
	r.Resolve(context.Background(), model.ResolveRequest{Message: q})
	<-entered
	r.Cache.Flush()
	close(release)
	r.Close()
	if r.Cache.Stats().Entries != 0 {
		t.Fatal("background response resurrected manually flushed cache")
	}

}

func TestOptimisticResponseCanDisableIdleRefresh(t *testing.T) {
	tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.1"), nil
	}}
	r, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10, resolver.OptimisticOptions{Enabled: true, IdleRefresh: false})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	if _, err := r.Resolve(context.Background(), model.ResolveRequest{Message: q}); err != nil {
		t.Fatal(err)
	}
	r.ConfigurationChanged("resource updated", true)
	time.Sleep(1250 * time.Millisecond)
	if tr.Calls.Load() != 1 || r.Cache.Stats().StaleEntries != 1 {
		t.Fatal("disabled idle refresh queried upstream or discarded stale response", tr.Calls.Load(), r.Cache.Stats())
	}
}

func TestOptimisticResponseRefreshesWhileIdle(t *testing.T) {
	var calls atomic.Int32
	tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		if calls.Add(1) == 1 {
			return mocks.AddressReply(q, "203.0.113.1"), nil
		}
		return mocks.AddressReply(q, "203.0.113.2"), nil
	}}
	r, err := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10, resolver.OptimisticOptions{Enabled: true, IdleRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	q := new(dns.Msg)
	q.SetQuestion("idle.example.test.", dns.TypeA)
	if _, err := r.Resolve(context.Background(), model.ResolveRequest{Message: q}); err != nil {
		t.Fatal(err)
	}
	r.ConfigurationChanged("resource updated", true)
	deadline := time.Now().Add(3 * time.Second)
	for r.Cache.Stats().StaleEntries > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 2 || r.Cache.Stats().StaleEntries != 0 {
		t.Fatal("idle query cache refresh did not replace its stale response", calls.Load(), r.Cache.Stats())
	}
}

func TestRestoredOptimisticResponsesRefreshWhileIdle(t *testing.T) {
	var calls atomic.Int32
	tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		if calls.Add(1) == 1 {
			return mocks.AddressReply(q, "203.0.113.1"), nil
		}
		return mocks.AddressReply(q, "203.0.113.2"), nil
	}}
	endpoints := []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}
	before, err := resolver.NewReal(tr, endpoints, false, 10, resolver.OptimisticOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	q := new(dns.Msg)
	q.SetQuestion("restored-idle.example.test.", dns.TypeA)
	if _, err := before.Resolve(context.Background(), model.ResolveRequest{Message: q}); err != nil {
		t.Fatal(err)
	}
	snapshot := before.ResponseSnapshot()
	before.Close()

	after, err := resolver.NewReal(tr, endpoints, false, 10, resolver.OptimisticOptions{Enabled: true, IdleRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if restored := after.RestoreResponses(snapshot); restored != 1 {
		t.Fatalf("restored %d responses, want 1", restored)
	}
	deadline := time.Now().Add(3 * time.Second)
	for after.Cache.Stats().StaleEntries > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 2 || after.Cache.Stats().StaleEntries != 0 {
		t.Fatal("restored response did not refresh while idle", calls.Load(), after.Cache.Stats())
	}
}
