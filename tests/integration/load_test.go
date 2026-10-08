package integration

import (
	"context"
	"encoding/json"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"sort"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
	"sync"
	"testing"
	"time"
)

func TestCachedDNSLoad(t *testing.T) {
	d, _, surge := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	})
	d.History = dnsserver.NewHistory(16, false)
	upstream := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.7"), nil
	}}
	real, e := resolver.NewReal(upstream, []resolver.ResolverEndpoint{{Protocol: "udp", Address: "192.0.2.53:53", Enabled: true}}, false, 100)
	if e != nil {
		t.Fatal(e)
	}
	d.Real = real
	server, e := dnsserver.Listen(context.Background(), []dnsserver.Ingress{{Bind: netip.MustParseAddrPort("127.0.0.1:0")}}, dnsserver.DefaultServerConfig(), d.Dispatch)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	target := server.Addresses()[0].String()
	q := new(dns.Msg)
	q.SetQuestion("cached.test.", dns.TypeA)
	client := &dns.Client{Timeout: time.Second}
	if _, _, e = client.Exchange(q, target); e != nil {
		t.Fatal(e)
	}
	const workers = 16
	const each = 1250
	latencies := make([]time.Duration, workers*each)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	started := time.Now()
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			conn, e := net.Dial("udp", target)
			if e != nil {
				errs <- e
				return
			}
			defer conn.Close()
			wire := &dns.Conn{Conn: conn}
			msg := new(dns.Msg)
			msg.SetQuestion("cached.test.", dns.TypeA)
			for i := 0; i < each; i++ {
				msg.Id = uint16(worker*each + i)
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				begin := time.Now()
				if e = wire.WriteMsg(msg); e != nil {
					errs <- e
					return
				}
				answer, e := wire.ReadMsg()
				if e != nil {
					errs <- e
					return
				}
				if answer.Id != msg.Id || answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
					errs <- context.Canceled
					return
				}
				latencies[worker*each+i] = time.Since(begin)
			}
		}(worker)
	}
	wg.Wait()
	elapsed := time.Since(started)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if upstream.Calls.Load() != 1 || surge.Calls.Load() != 1 {
		t.Fatalf("cache hits executed external work: upstream=%d rule=%d", upstream.Calls.Load(), surge.Calls.Load())
	}
	if server.Dropped.Load() != 0 {
		t.Fatal("load dropped queries")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	report := map[string]any{"cached_dns_queries": len(latencies), "workers": workers, "qps": float64(len(latencies)) / elapsed.Seconds(), "p50_ms": float64(latencies[len(latencies)/2]) / float64(time.Millisecond), "p99_ms": float64(latencies[len(latencies)*99/100]) / float64(time.Millisecond), "external_rule_evaluations": surge.Calls.Load(), "external_resolver_exchanges": upstream.Calls.Load(), "scope": "actual UDP sockets + cached classification/response; excludes external upstream latency"}
	raw, _ := json.Marshal(report)
	t.Log(string(raw))
}
