package dnsserver_test

import (
	"context"
	"github.com/miekg/dns"
	"net/netip"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/tests/mocks"
	"testing"
)

func TestClientForcedPoliciesPrecedeRulesAndPreserveFakeIPGate(t *testing.T) {
	for _, decision := range []model.Decision{model.Direct, model.Surge, model.Reject} {
		for _, kind := range []uint16{dns.TypeA, dns.TypeTXT} {
			for _, address := range []string{"::ffff:192.0.2.10", "2001:db8::10"} {
				t.Run(string(decision)+dns.TypeToString[kind]+address, func(t *testing.T) {
					gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, DirectFallback: true, Generation: 1}}
					realCalls, surgeCalls := 0, 0
					d := &dnsserver.Dispatcher{Gate: gate, FakeIssuer: gate, Local: monitorLocal{}, Monitoring: dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{}), History: dnsserver.NewHistory(10, true), FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, ClientPolicies: map[netip.Addr]model.Decision{netip.MustParseAddr(address).Unmap(): decision}, Real: &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
						realCalls++
						return mocks.AddressReply(r.Message, "203.0.113.1"), nil
					}}, Surge: &mocks.Surge{Fake: func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
						surgeCalls++
						return mocks.AddressReply(q, "198.18.1.1"), nil
					}}}
					q := new(dns.Msg)
					q.SetQuestion("client.test.", kind)
					reply := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr(address)}, q)
					record := d.History.Records()[0]
					if record.Result.Source != "client-policy" || record.Result.Decision != decision || d.Monitoring.Snapshot().Completed != 1 {
						t.Fatal(record)
					}
					if decision == model.Reject && (reply.Rcode != dns.RcodeRefused || realCalls+surgeCalls != 0) {
						t.Fatal("reject bypassed", reply)
					}
					if decision == model.Direct && (realCalls != 1 || surgeCalls != 0) {
						t.Fatal("direct not forced")
					}
					if decision == model.Surge && (surgeCalls != 1 || realCalls != 0) {
						t.Fatal("surge not forced")
					}
					if decision == model.Surge {
						gate.State.AllowFakeIP = false
						reply = d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr(address)}, q)
						if reply.Rcode != dns.RcodeServerFailure || surgeCalls != 1 || realCalls != 0 {
							t.Fatal("forced policy bypassed safety gate")
						}
					}
				})
			}
		}
	}
}

func TestClientPoliciesDoNotAffectOtherClientsOrBypassANYRefusal(t *testing.T) {
	d := &dnsserver.Dispatcher{Gate: &mocks.Gate{}, Local: monitorLocal{}, ClientPolicies: map[netip.Addr]model.Decision{netip.MustParseAddr("192.0.2.10"): model.Reject}, History: dnsserver.NewHistory(10, true)}
	q := new(dns.Msg)
	q.SetQuestion("client.test.", dns.TypeA)
	reply := d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.20")}, q)
	if reply.Rcode != dns.RcodeSuccess || d.History.Records()[0].Result.Source != "local-dns" {
		t.Fatal("other client inherited forced policy")
	}
	d.ClientPolicies[netip.MustParseAddr("192.0.2.10")] = model.Direct
	q.SetQuestion("client.test.", dns.TypeANY)
	if reply = d.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10")}, q); reply.Rcode != dns.RcodeRefused {
		t.Fatal("client override bypassed ANY refusal")
	}
}
