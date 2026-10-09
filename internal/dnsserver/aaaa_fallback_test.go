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

type ipv6Disabled bool

func (s ipv6Disabled) IPv6Disabled() bool { return bool(s) }

type fallbackLocal struct{ calls int }

func (s *fallbackLocal) ResolveLocal(_ context.Context, _ model.DNSQueryContext, q *dns.Msg) (*dns.Msg, bool) {
	s.calls++
	return mocks.AddressReply(q, "2001:db8::2"), true
}

func TestAAAAFallbackPrecedesHostsAndAlwaysRealIP(t *testing.T) {
	for _, mode := range []string{"block", "upstream", "rules"} {
		for _, disabled := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/ipv6-on", true: "/ipv6-off"}[disabled], func(t *testing.T) {
				local := &fallbackLocal{}
				wire := &mocks.Transport{Func: func(_ context.Context, ep resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
					if ep.Address != "192.0.2.53:53" {
						t.Fatal("AAAA fallback used the original destination")
					}
					return mocks.AddressReply(q, "2001:db8::7"), nil
				}}
				real, err := resolver.NewReal(wire, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, true, 10)
				if err != nil {
					t.Fatal(err)
				}
				defer real.Close()
				history := dnsserver.NewHistory(10, true)
				d := &dnsserver.Dispatcher{AAAAFallback: mode, IPv6Status: ipv6Disabled(disabled), Local: local, RealIPDomains: realIPDomains{}, Real: real, Classifier: noRuleQueries{t}, Gate: &mocks.Gate{}, History: history}
				q := new(dns.Msg)
				q.SetQuestion("example.test.", dns.TypeAAAA)
				q.SetEdns0(4096, true)
				reply := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), Intercepted: true, OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53}, q)
				active := disabled && mode != "rules"
				if reply.Rcode != dns.RcodeSuccess || local.calls != map[bool]int{true: 0, false: 1}[active] {
					t.Fatal("incorrect priority", reply, local.calls)
				}
				rows := history.Records()
				if len(rows) != 1 || active && rows[0].Result.Source != "surge-ipv6-fallback" || !active && rows[0].Result.Source != "local-dns" {
					t.Fatal("incorrect query history", rows)
				}
				if active && mode == "block" {
					if len(reply.Answer) != 0 || wire.Calls.Load() != 0 || reply.IsEdns0() == nil || reply.IsEdns0().UDPSize() != 1232 || reply.IsEdns0().Do() || reply.AuthenticatedData || d.Counters.Reject.Load() != 1 {
						t.Fatal("AAAA blocking did external work or returned an incorrect NODATA response", reply)
					}
				} else if active && wire.Calls.Load() != 1 {
					t.Fatal("upstream fallback did not use the configured pool")
				} else if !active && wire.Calls.Load() != 0 {
					t.Fatal("inactive fallback queried upstream")
				}
			})
		}
	}
}

func TestAAAAFallbackHotUpdateAndUnknownIPv6State(t *testing.T) {
	local := &fallbackLocal{}
	d := &dnsserver.Dispatcher{AAAAFallback: "block", Local: local, Gate: &mocks.Gate{}}
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeAAAA)
	query := model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}
	if len(d.Dispatch(context.Background(), query, q).Answer) != 1 {
		t.Fatal("missing IPv6 state enabled fallback")
	}
	d.IPv6Status = ipv6Disabled(true)
	if len(d.Dispatch(context.Background(), query, q).Answer) != 0 {
		t.Fatal("disabled IPv6 did not block")
	}
	d.SetAAAAFallback("rules")
	if len(d.Dispatch(context.Background(), query, q).Answer) != 1 {
		t.Fatal("hot update did not restore ordinary behavior")
	}
	d.SetAAAAFallback("block")
	q.SetQuestion("example.test.", dns.TypeA)
	if len(d.Dispatch(context.Background(), query, q).Answer) != 1 {
		t.Fatal("AAAA fallback intercepted A")
	}
}

func TestAAAAUpstreamFallbackDoesNotLeakFakeAddressesOrUseSurgeAfterFailure(t *testing.T) {
	for _, address := range []string{"fd00:198:18::1", "error"} {
		t.Run(address, func(t *testing.T) {
			real := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
				if address == "error" {
					return nil, errors.New("upstream failed")
				}
				return mocks.AddressReply(r.Message, address), nil
			}}
			d := &dnsserver.Dispatcher{AAAAFallback: "upstream", IPv6Status: ipv6Disabled(true), Real: real, Gate: &mocks.Gate{}, Classifier: noRuleQueries{t}, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("fd00:198:18::/64")}, Surge: &mocks.Surge{Fake: func(context.Context, *dns.Msg) (*dns.Msg, error) {
				t.Fatal("failed AAAA fallback queried Surge")
				return nil, nil
			}}}
			q := new(dns.Msg)
			q.SetQuestion("example.test.", dns.TypeAAAA)
			reply := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q)
			if reply.Rcode != dns.RcodeServerFailure || len(reply.Answer) != 0 || real.Calls.Load() != 1 || d.Counters.Errors.Load() != 1 {
				t.Fatal(reply)
			}
		})
	}
}
