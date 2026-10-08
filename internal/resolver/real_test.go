package resolver_test

import (
	"context"
	"errors"
	"net/netip"
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
