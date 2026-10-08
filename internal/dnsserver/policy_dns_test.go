package dnsserver_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

type policyClassifier struct {
	result model.DecisionResult
	err    error
}

func (c policyClassifier) Classify(context.Context, model.DNSQueryContext) (model.DecisionResult, error) {
	return c.result, c.err
}

func TestPolicyDNSPriorityAndExistingPaths(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   model.DecisionResult
		fallback bool
		failed   bool
		want     string
	}{
		{"final wins", model.DecisionResult{Decision: model.Surge, Policy: "Proxy", FinalPolicy: "Node A"}, false, false, "final"},
		{"group fallback", model.DecisionResult{Decision: model.Surge, Policy: "Proxy", FinalPolicy: "Node B"}, false, false, "policy"},
		{"unmatched", model.DecisionResult{Decision: model.Surge, Policy: "Other", FinalPolicy: "Node B"}, false, false, "surge"},
		{"case sensitive", model.DecisionResult{Decision: model.Surge, Policy: "proxy", FinalPolicy: "node a"}, false, false, "surge"},
		{"direct preserved", model.DecisionResult{Decision: model.Direct, Confidence: model.Certain, Policy: "Proxy", FinalPolicy: "Node A"}, false, false, "direct"},
		{"reject preserved", model.DecisionResult{Decision: model.Reject, Confidence: model.Certain, Policy: "Proxy", FinalPolicy: "Node A"}, false, false, "reject"},
		{"unknown preserved", model.DecisionResult{Decision: model.Unknown, Policy: "Proxy", FinalPolicy: "Node A"}, false, false, "surge"},
		{"failed evaluation", model.DecisionResult{Decision: model.Surge, Policy: "Proxy", FinalPolicy: "Node A"}, false, true, "surge"},
		{"state fallback", model.DecisionResult{Decision: model.Surge, Policy: "Proxy", FinalPolicy: "Node A"}, true, false, "direct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			used := ""
			upstream := func(label string) model.Resolver {
				return &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
					used = label
					if label != "direct" && !r.UseConfiguredUpstreams {
						t.Fatal("policy DNS preserved original destination")
					}
					if r.Trace != nil {
						r.Trace.Endpoint = label
					}
					return mocks.AddressReply(r.Message, "203.0.113.7"), nil
				}}
			}
			gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, DirectFallback: tc.fallback, Generation: 1}}
			classificationErr := error(nil)
			if tc.failed {
				classificationErr = errors.New("evaluation failed")
			}
			d := &dnsserver.Dispatcher{Gate: gate, FakeIssuer: gate, Real: upstream("direct"), Classifier: policyClassifier{tc.result, classificationErr}, Surge: &mocks.Surge{Fake: func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
				used = "surge"
				return mocks.AddressReply(q, "198.18.1.1"), nil
			}}, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, History: dnsserver.NewHistory(10, true), PolicyDNS: []dnsserver.PolicyDNSBinding{{Field: "policy", Policy: "Proxy", Resolver: upstream("policy")}, {Field: "final_policy", Policy: "Node A", Resolver: upstream("final")}}}
			q := new(dns.Msg)
			q.SetQuestion("test.example.", dns.TypeA)
			r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
			if tc.want == "reject" {
				if r.Rcode != dns.RcodeRefused || used != "" {
					t.Fatal("reject escaped", r, used)
				}
				return
			}
			if r.Rcode != dns.RcodeSuccess || used != tc.want {
				t.Fatalf("wanted %s, got %s: %v", tc.want, used, r)
			}
			record := d.History.Records()[0]
			if tc.want == "final" || tc.want == "policy" {
				if record.Result.PolicyDNS == nil || record.Resolver != tc.want || record.Result.Decision != model.Surge {
					t.Fatalf("lost policy DNS provenance: %+v", record)
				}
			} else if record.Result.PolicyDNS != nil {
				t.Fatal("unexpected DNS override", record)
			}
		})
	}
}

func TestPolicyDNSAddressTypesPinnedUpstreamAndCache(t *testing.T) {
	for _, kind := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeHTTPS, dns.TypeSVCB} {
		wire := &mocks.Transport{Func: func(_ context.Context, ep resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
			if ep.Address != "192.0.2.53:53" {
				t.Fatal("wrong policy endpoint", ep)
			}
			if kind == dns.TypeA {
				return mocks.AddressReply(q, "203.0.113.7"), nil
			}
			if kind == dns.TypeAAAA {
				return mocks.AddressReply(q, "2001:db8::7"), nil
			}
			m := new(dns.Msg)
			m.SetReply(q)
			rr, _ := dns.NewRR("test.example. 60 IN " + dns.TypeToString[kind] + " 1 svc.example. ipv4hint=203.0.113.7")
			m.Answer = []dns.RR{rr}
			return m, nil
		}}
		real, e := resolver.NewReal(wire, []resolver.ResolverEndpoint{{Address: "192.0.2.53:53", Protocol: "udp", Enabled: true}}, false, 10)
		if e != nil {
			t.Fatal(e)
		}
		d := &dnsserver.Dispatcher{Gate: &mocks.Gate{}, Classifier: policyClassifier{result: model.DecisionResult{Decision: model.Surge, Policy: "Proxy"}}, PolicyDNS: []dnsserver.PolicyDNSBinding{{Field: "policy", Policy: "Proxy", Resolver: real}}, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, History: dnsserver.NewHistory(10, true)}
		q := new(dns.Msg)
		q.SetQuestion("test.example.", kind)
		qc := model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), Intercepted: true, OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53, Transport: model.UDP}
		for i := 0; i < 2; i++ {
			q.Id = uint16(100 + i)
			r := d.Dispatch(context.Background(), qc, q)
			if r.Rcode != 0 || len(r.Answer) != 1 || r.Id != q.Id {
				t.Fatalf("type %d lost real override answer: %v", kind, r)
			}
		}
		if wire.Calls.Load() != 1 || !d.History.Records()[0].ResponseCacheHit {
			t.Fatal("policy response cache missed or mixed", d.History.Records())
		}
	}
}

func TestPolicyDNSFailureAndFakeIPDoNotLeakOrFallBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		upstream := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
			if fail {
				return nil, errors.New("timeout")
			}
			return mocks.AddressReply(r.Message, "198.18.1.1"), nil
		}}
		gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}
		d := &dnsserver.Dispatcher{Gate: gate, FakeIssuer: gate, Surge: &mocks.Surge{Fake: func(context.Context, *dns.Msg) (*dns.Msg, error) {
			t.Fatal("failed policy DNS fell back to Surge")
			return nil, nil
		}}, Classifier: policyClassifier{result: model.DecisionResult{Decision: model.Surge, Policy: "Proxy"}}, PolicyDNS: []dnsserver.PolicyDNSBinding{{Field: "policy", Policy: "Proxy", Resolver: upstream}}, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, History: dnsserver.NewHistory(10, true)}
		q := new(dns.Msg)
		q.SetQuestion("test.example.", dns.TypeA)
		r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
		if r.Rcode != dns.RcodeServerFailure || len(r.Answer) != 0 || d.History.Records()[0].ResolutionError == "" {
			t.Fatal("unsafe/failed policy upstream exposed", r)
		}
	}
}
