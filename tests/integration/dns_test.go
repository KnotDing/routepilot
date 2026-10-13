package integration

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
	"surge-controller/internal/classifier"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/tests/mocks"
)

func setup(t *testing.T, match func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error)) (*dnsserver.Dispatcher, *mocks.Gate, *mocks.Surge) {
	t.Helper()
	surge := &mocks.Surge{Match: match, Fake: func(_ context.Context, q *dns.Msg) (*dns.Msg, error) { return mocks.AddressReply(q, "198.18.1.2"), nil }}
	real := &mocks.Resolver{Func: func(_ context.Context, q model.ResolveRequest) (*dns.Msg, error) {
		return mocks.AddressReply(q.Message, "203.0.113.7"), nil
	}}
	engine, err := classifier.New(context.Background(), surge, classifier.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}
	d := &dnsserver.Dispatcher{Classifier: engine, Real: real, Surge: surge, Gate: gate, FakeIssuer: gate, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, History: dnsserver.NewHistory(32, true)}
	return d, gate, surge
}
func TestClassificationPaths(t *testing.T) {
	d, _, _ := setup(t, func(_ context.Context, r model.RuleMatchRequest) (model.RuleMatchResult, error) {
		switch r.Domain {
		case "direct.test.":
			return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
		case "ambiguous.test.":
			return model.RuleMatchResult{FinalPolicy: "DIRECT"}, nil
		case "failed.test.":
			return model.RuleMatchResult{}, errors.New("unavailable")
		case "proxy.test.":
			return model.RuleMatchResult{FinalPolicy: "Proxy"}, nil
		default:
			return model.RuleMatchResult{}, nil
		}
	})
	for _, tc := range []struct{ name, address string }{{"direct.test", "203.0.113.7"}, {"proxy.test", "198.18.1.2"}, {"unknown.test", "198.18.1.2"}, {"ambiguous.test", "198.18.1.2"}, {"failed.test", "198.18.1.2"}} {
		t.Run(tc.name, func(t *testing.T) {
			q := new(dns.Msg)
			q.SetQuestion(dns.Fqdn(tc.name), dns.TypeA)
			r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
			if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != tc.address {
				t.Fatalf("unexpected answer %v", r)
			}
		})
	}
	// Shared real DNS addresses never influence policy, and there is no BGP
	// dependency in the DNS classifier or dispatcher.
}

func TestQueryHistoryRecordsResponseSectionsAndFailureCause(t *testing.T) {
	d, _, surge := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "Proxy"}, nil
	})
	q := new(dns.Msg)
	q.SetQuestion("result.test.", dns.TypeA)
	q.Id = 2216
	surge.Fake = func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
		r := mocks.AddressReply(q, "198.18.1.2")
		rr, _ := dns.NewRR("result.test. 60 IN NS ns.result.test.")
		r.Ns = []dns.RR{rr}
		r.SetEdns0(1232, false)
		return r, nil
	}
	d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
	r := d.History.Records()[0]
	if r.Response.ID != 2216 || r.Response.Status != "NOERROR" || len(r.Response.Answers) != 1 || len(r.Response.Authority) != 1 || len(r.Response.Additional) != 1 {
		t.Fatal("response sections not recorded", r.Response)
	}
	if len(r.Answers) != 1 || r.Answers[0] != r.Response.Answers[0] {
		t.Fatal("history answers differ from response detail", r)
	}
	r.Answers[0] = "redacted"
	if r.Response.Answers[0] == "redacted" {
		t.Fatal("editing history answers changed response detail")
	}
	surge.Fake = func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.7"), nil
	}
	d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
	r = d.History.Records()[0]
	if r.Response.Status != "SERVFAIL" || len(r.Response.Answers) != 0 || r.ResolutionError == "" {
		t.Fatal("unsafe address failure not explained", r)
	}
}
func TestServiceBindingAndRouteGates(t *testing.T) {
	d, gate, surge := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "Proxy"}, nil
	})
	for _, kind := range []uint16{dns.TypeHTTPS, dns.TypeSVCB} {
		q := new(dns.Msg)
		q.SetQuestion("proxy.test.", kind)
		q.SetEdns0(1232, true)
		r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
		if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 0 || r.AuthenticatedData {
			t.Fatal("SURGE service binding leaked data")
		}
		raw, err := r.Pack()
		if err != nil {
			t.Fatal(err)
		}
		parsed := new(dns.Msg)
		if err = parsed.Unpack(raw); err != nil {
			t.Fatal(err)
		}
	}
	q := new(dns.Msg)
	q.SetQuestion("proxy.test.", dns.TypeA)
	gate.Set(dnsserver.SafetySnapshot{Generation: 2})
	if r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q); r.Rcode != dns.RcodeServerFailure {
		t.Fatal("Fake-IP issued before route ready")
	}
	gate.Set(dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 3})
	surge.Fake = func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
		gate.Set(dnsserver.SafetySnapshot{Generation: 4})
		return mocks.AddressReply(q, "198.18.1.2"), nil
	}
	if r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q); r.Rcode != dns.RcodeServerFailure {
		t.Fatal("Fake-IP escaped after readiness loss")
	}
	gate.Set(dnsserver.SafetySnapshot{DirectFallback: true, Generation: 5})
	r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
	if r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatal("explicit state fallback failed")
	}
}
func TestDirectAndTransparentSockets(t *testing.T) {
	for _, transparent := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "transparent"}[transparent], func(t *testing.T) {
			lookup := &mocks.Lookup{Destination: netip.MustParseAddrPort("8.8.8.8:53")}
			port := uint16(0)
			if transparent { // Reserve ephemeral fixed port for this test.
				reserve, err := dnsserver.Listen(context.Background(), []dnsserver.Ingress{{Bind: netip.MustParseAddrPort("127.0.0.1:0")}}, dnsserver.DefaultServerConfig(), func(context.Context, model.DNSQueryContext, *dns.Msg) *dns.Msg { return nil })
				if err != nil {
					t.Fatal(err)
				}
				port = reserve.Addresses()[0].Port()
				reserve.Close()
			}
			contexts := make(chan model.DNSQueryContext, 2)
			server, err := dnsserver.Listen(context.Background(), []dnsserver.Ingress{{Bind: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port), Intercepted: transparent, Lookup: lookup}}, dnsserver.DefaultServerConfig(), func(_ context.Context, q model.DNSQueryContext, m *dns.Msg) *dns.Msg {
				contexts <- q
				return mocks.AddressReply(m, "203.0.113.7")
			})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			for _, protocol := range []string{"udp", "tcp"} {
				q := new(dns.Msg)
				q.SetQuestion("example.test.", dns.TypeA)
				client := &dns.Client{Net: protocol}
				r, _, err := client.Exchange(q, server.Addresses()[0].String())
				if err != nil || r.Rcode != 0 {
					t.Fatalf("%s: %v", protocol, err)
				}
				got := <-contexts
				if got.Intercepted != transparent || got.Transport != model.DNSProtocol(protocol) || got.ClientIP.String() != "127.0.0.1" {
					t.Fatalf("context lost: %+v", got)
				}
				if transparent && got.OriginalDestinationIP.String() != "8.8.8.8" {
					t.Fatal("original destination lost")
				}
			}
		})
	}
}
func TestSelfOriginatedLoopRejected(t *testing.T) {
	d, _, _ := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		t.Fatal("loop reached classifier")
		return model.RuleMatchResult{}, nil
	})
	d.ForbiddenClients = []netip.Addr{netip.MustParseAddr("192.0.2.2")}
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.2"), OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53, Intercepted: true}, q)
	if r.Rcode != dns.RcodeRefused || d.Real.(*mocks.Resolver).Calls.Load() != 0 {
		t.Fatal("upstream request re-entered forwarding pipeline")
	}
}

func TestRealResolverCannotLeakFakeAddresses(t *testing.T) {
	d, gate, _ := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	})
	d.FakePrefixes = append(d.FakePrefixes, netip.MustParsePrefix("fd00:198:18::/48"))
	for _, fallback := range []bool{false, true} {
		gate.Set(dnsserver.SafetySnapshot{DirectFallback: fallback, Generation: 1})
		for _, section := range []string{"answer", "authority", "additional"} {
			for _, address := range []string{"198.18.1.2", "fd00:198:18::2"} {
				t.Run(section+address, func(t *testing.T) {
					q := new(dns.Msg)
					kind := uint16(dns.TypeA)
					if netip.MustParseAddr(address).Is6() {
						kind = dns.TypeAAAA
					}
					q.SetQuestion("leak.test.", kind)
					d.Real = &mocks.Resolver{Func: func(_ context.Context, request model.ResolveRequest) (*dns.Msg, error) {
						r := mocks.AddressReply(request.Message, address)
						if section == "authority" {
							r.Ns, r.Answer = r.Answer, nil
						}
						if section == "additional" {
							r.Extra, r.Answer = r.Answer, nil
						}
						return r, nil
					}}
					if r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q); r.Rcode != dns.RcodeServerFailure || len(r.Answer)+len(r.Ns)+len(r.Extra) != 0 {
						t.Fatal("real forwarding leaked a Fake-IP outside readiness gate", r)
					}
				})
			}
		}
	}
}

func TestIPv6SurgeFakeAnswerTTLAndDNSSEC(t *testing.T) {
	d, _, surge := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "Proxy"}, nil
	})
	d.FakePrefixes = append(d.FakePrefixes, netip.MustParsePrefix("fd00:198:18::/48"))
	maximum := uint32(10)
	d.FakeTTLMax = &maximum
	surge.Fake = func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
		r := mocks.AddressReply(q, "fd00:198:18::2")
		r.AuthenticatedData = true
		return r, nil
	}
	q := new(dns.Msg)
	q.SetQuestion("v6.test.", dns.TypeAAAA)
	r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 || r.Answer[0].Header().Ttl != maximum || r.AuthenticatedData {
		t.Fatal("IPv6 Fake-IP/TTL/DNSSEC contract", r)
	}
}

func TestRealServiceBindingHintCannotLeakFakeIP(t *testing.T) {
	d, _, _ := setup(t, func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	})
	d.Real = &mocks.Resolver{Func: func(_ context.Context, q model.ResolveRequest) (*dns.Msg, error) {
		r := new(dns.Msg)
		r.SetReply(q.Message)
		rr, e := dns.NewRR("hint.test. 60 IN HTTPS 1 . ipv4hint=198.18.1.2")
		if e != nil {
			t.Fatal(e)
		}
		r.Answer = []dns.RR{rr}
		return r, nil
	}}
	q := new(dns.Msg)
	q.SetQuestion("hint.test.", dns.TypeHTTPS)
	if r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q); r.Rcode != dns.RcodeServerFailure {
		t.Fatal("real service binding leaked fake hint", r)
	}
}

func TestConfiguredDirectUsesRealDNSAndRecordsConfiguredConfidence(t *testing.T) {
	surge := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct", SafetyReason: "requires dns"}, nil
	}, Fake: func(context.Context, *dns.Msg) (*dns.Msg, error) {
		t.Error("configured DIRECT sent to Surge Fake-IP DNS")
		return nil, errors.New("unexpected Fake-IP call")
	}}
	cfg := classifier.DefaultConfig()
	cfg.TrustDirect = true
	engine, err := classifier.New(context.Background(), surge, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	real := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		return mocks.AddressReply(r.Message, "203.0.113.7"), nil
	}}
	gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}
	d := &dnsserver.Dispatcher{Classifier: engine, Real: real, Surge: surge, Gate: gate, FakeIssuer: gate, History: dnsserver.NewHistory(8, true)}
	q := new(dns.Msg)
	q.SetQuestion("configured.test.", dns.TypeA)
	r := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatalf("unexpected response: %v", r)
	}
	records := d.History.Records()
	if len(records) != 1 || records[0].Result.Confidence != model.Configured || records[0].Result.Decision != model.Direct {
		t.Fatalf("wrong classification provenance: %+v", records)
	}
}
