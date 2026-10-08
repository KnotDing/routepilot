package dnsserver_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

type realIPDomains struct{}

func (realIPDomains) MatchAlwaysRealIP(name string) (string, bool) {
	return "*.test", name == "example.test."
}

type noRuleQueries struct{ t *testing.T }

func (c noRuleQueries) Classify(context.Context, model.DNSQueryContext) (model.DecisionResult, error) {
	c.t.Fatal("always-real-ip queried the rule classifier")
	return model.DecisionResult{}, nil
}

func TestAlwaysRealIPUsesConfiguredUpstreamWithoutSurge(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeHTTPS, dns.TypeSVCB} {
		wire := &mocks.Transport{Func: func(_ context.Context, ep resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
			if ep.Address != "192.0.2.53:53" {
				t.Fatalf("queried original resolver instead of configured upstream: %+v", ep)
			}
			m := new(dns.Msg)
			m.SetReply(q)
			switch q.Question[0].Qtype {
			case dns.TypeA:
				return mocks.AddressReply(q, "203.0.113.7"), nil
			case dns.TypeAAAA:
				return mocks.AddressReply(q, "2001:db8::7"), nil
			default:
				rr, err := dns.NewRR("example.test. 60 IN " + dns.TypeToString[q.Question[0].Qtype] + " 1 svc.example.test. ipv4hint=203.0.113.7")
				if err != nil {
					t.Fatal(err)
				}
				m.Answer = []dns.RR{rr}
			}
			return m, nil
		}}
		real, err := resolver.NewReal(wire, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, true, 10)
		if err != nil {
			t.Fatal(err)
		}
		surge := &mocks.Surge{Fake: func(context.Context, *dns.Msg) (*dns.Msg, error) {
			t.Fatal("always-real-ip queried Surge Fake-IP DNS")
			return nil, nil
		}}
		history := dnsserver.NewHistory(10, true)
		d := &dnsserver.Dispatcher{RealIPDomains: realIPDomains{}, Real: real, Classifier: noRuleQueries{t}, Surge: surge, Gate: &mocks.Gate{}, History: history, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}}
		q := new(dns.Msg)
		q.SetQuestion("EXAMPLE.test.", qtype)
		queryContext := model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53, Transport: model.UDP, Intercepted: true}
		reply := d.Dispatch(context.Background(), queryContext, q)
		rows := history.Records()
		if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 1 || wire.Calls.Load() != 1 || len(rows) != 1 || rows[0].Result.Source != "surge-always-real-ip" || rows[0].Result.Decision != model.Direct {
			t.Fatalf("DNS type %d lost bypass or history: reply=%+v rows=%+v", qtype, reply, rows)
		}
	}
}

func TestAlwaysRealIPDoesNotUseSurgeWhenRealUpstreamReturnsFakeIP(t *testing.T) {
	real := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		return mocks.AddressReply(r.Message, "198.18.1.2"), nil
	}}
	surge := &mocks.Surge{Fake: func(context.Context, *dns.Msg) (*dns.Msg, error) {
		t.Fatal("queried Surge after unsafe upstream result")
		return nil, nil
	}}
	d := &dnsserver.Dispatcher{RealIPDomains: realIPDomains{}, Real: real, Classifier: noRuleQueries{t}, Surge: surge, Gate: &mocks.Gate{}, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}}
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	reply := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
	if reply.Rcode != dns.RcodeServerFailure || len(reply.Answer) != 0 {
		t.Fatal("unsafe real-upstream response was exposed", reply)
	}
}
