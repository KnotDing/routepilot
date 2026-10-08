package dnsserver_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

func fallbackDispatcher(primary model.Resolver, fallback string, backup model.Resolver) (*dnsserver.Dispatcher, *mocks.Gate, *dns.Msg) {
	gate := &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}
	d := &dnsserver.Dispatcher{Gate: gate, FakeIssuer: gate, FakePrefixes: []netip.Prefix{netip.MustParsePrefix("198.18.0.0/15")}, History: dnsserver.NewHistory(10, true), Classifier: policyClassifier{result: model.DecisionResult{Decision: model.Surge, Policy: "Proxy"}}, PolicyDNS: []dnsserver.PolicyDNSBinding{{Field: "policy", Policy: "Proxy", Resolver: primary, Fallback: fallback, FallbackResolver: backup}}}
	q := new(dns.Msg)
	q.SetQuestion("test.example.", dns.TypeA)
	return d, gate, q
}
func tracedResolver(label, ip string) *mocks.Resolver {
	return &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		if !r.UseConfiguredUpstreams || !r.RetryDNSFailures {
			return nil, errors.New("backup did not select configured pool")
		}
		if r.Trace != nil {
			r.Trace.Endpoint = label
		}
		return mocks.AddressReply(r.Message, ip), nil
	}}
}

func TestPolicyDNSFallbackTargetsAndFailures(t *testing.T) {
	for _, target := range []string{"upstreams", "real", "surge"} {
		for _, failure := range []string{"timeout", "SERVFAIL", "REFUSED", "truncated", "fake-address", "mismatched-response"} {
			t.Run(target+"/"+failure, func(t *testing.T) {
				primary := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
					r.Trace.Endpoint = "primary"
					if failure == "timeout" {
						return nil, context.DeadlineExceeded
					}
					m := mocks.AddressReply(r.Message, "203.0.113.1")
					switch failure {
					case "SERVFAIL":
						m.Rcode = dns.RcodeServerFailure
					case "REFUSED":
						m.Rcode = dns.RcodeRefused
					case "truncated":
						m.Truncated = true
					case "fake-address":
						m = mocks.AddressReply(r.Message, "198.18.1.1")
					case "mismatched-response":
						m.Id++
					}
					return m, nil
				}}
				backup := tracedResolver("backup", "203.0.113.2")
				d, _, q := fallbackDispatcher(primary, target, backup)
				d.Real = backup
				surgeCalls := 0
				d.Surge = &mocks.Surge{Fake: func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
					surgeCalls++
					return mocks.AddressReply(q, "198.18.1.2"), nil
				}}
				client := model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), OriginalDestinationIP: netip.MustParseAddr("8.8.8.8"), OriginalDestinationPort: 53, Intercepted: true, Transport: model.UDP}
				response := d.Dispatch(context.Background(), client, q)
				if response.Rcode != 0 || len(response.Answer) != 1 {
					t.Fatal(response)
				}
				record := d.History.Records()[0]
				match := record.Result.PolicyDNS
				if match == nil || !match.FallbackUsed || len(match.Attempts) != 2 || match.Attempts[0].Error == "" || match.Attempts[1].Error != "" || record.ResolutionError != "" || record.Result.Decision != model.Surge {
					t.Fatalf("lost attempt trace: %+v %+v", record, match)
				}
				if target == "surge" {
					if surgeCalls != 1 || backup.Calls.Load() != 0 || record.Resolver != "Surge Fake-IP DNS" {
						t.Fatal("wrong Surge backup", record)
					}
				} else if backup.Calls.Load() != 1 || surgeCalls != 0 || record.Resolver != "backup" {
					t.Fatal("wrong real backup", record)
				}
			})
		}
	}
}
func TestPolicyDNSCompletedAnswersDoNotFallBack(t *testing.T) {
	for _, code := range []int{dns.RcodeSuccess, dns.RcodeNameError} {
		t.Run(dns.RcodeToString[code], func(t *testing.T) {
			primary := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
				m := new(dns.Msg)
				m.SetRcode(r.Message, code)
				return m, nil
			}}
			backup := tracedResolver("backup", "203.0.113.2")
			d, _, q := fallbackDispatcher(primary, "upstreams", backup)
			response := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
			if response.Rcode != code || backup.Calls.Load() != 0 || d.History.Records()[0].Result.PolicyDNS.FallbackUsed {
				t.Fatal("completed answer retried", response)
			}
		})
	}
}
func TestPolicyDNSPrimaryDeadlineLeavesBackupBudget(t *testing.T) {
	primary := &mocks.Resolver{Func: func(ctx context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	backup := tracedResolver("backup", "203.0.113.2")
	d, _, q := fallbackDispatcher(primary, "real", nil)
	d.Real = backup
	d.PolicyDNS[0].AttemptTimeout = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if r := d.Dispatch(ctx, model.DNSQueryContext{}, q); r.Rcode != 0 || backup.Calls.Load() != 1 || ctx.Err() != nil {
		t.Fatal("backup budget lost", r)
	}
}
func TestPolicyDNSCancellationStopsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	primary := &mocks.Resolver{Func: func(context.Context, model.ResolveRequest) (*dns.Msg, error) { cancel(); return nil, context.Canceled }}
	backup := tracedResolver("backup", "203.0.113.2")
	d, _, q := fallbackDispatcher(primary, "upstreams", backup)
	if r := d.Dispatch(ctx, model.DNSQueryContext{}, q); r.Rcode != dns.RcodeServerFailure || backup.Calls.Load() != 0 || d.History.Records()[0].Result.PolicyDNS.FallbackUsed {
		t.Fatal("canceled query triggered backup", r)
	}
}
func TestPolicyDNSBackupFailureIsTerminalAndNeverLeaksFakeIP(t *testing.T) {
	for _, fake := range []bool{true, false} {
		primary := &mocks.Resolver{Func: func(context.Context, model.ResolveRequest) (*dns.Msg, error) { return nil, errors.New("primary down") }}
		backup := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
			if fake {
				return mocks.AddressReply(r.Message, "198.18.1.2"), nil
			}
			return nil, errors.New("backup down")
		}}
		d, _, q := fallbackDispatcher(primary, "upstreams", backup)
		d.Real = &mocks.Resolver{Func: func(context.Context, model.ResolveRequest) (*dns.Msg, error) {
			t.Fatal("unexpected third fallback")
			return nil, nil
		}}
		r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
		record := d.History.Records()[0]
		if r.Rcode != dns.RcodeServerFailure || len(r.Answer) > 0 || len(record.Result.PolicyDNS.Attempts) != 2 || record.Result.PolicyDNS.Attempts[1].Error == "" || record.ResolutionError == "" {
			t.Fatal("backup failure leaked", record)
		}
	}
}
func TestPolicyDNSSurgeFallbackUsesIssuanceGateAndHTTPSBehavior(t *testing.T) {
	for _, mode := range []string{"not-ready", "generation-changed", "HTTPS", "SVCB"} {
		t.Run(mode, func(t *testing.T) {
			primary := &mocks.Resolver{Func: func(context.Context, model.ResolveRequest) (*dns.Msg, error) { return nil, errors.New("down") }}
			d, gate, q := fallbackDispatcher(primary, "surge", nil)
			calls := 0
			d.Surge = &mocks.Surge{Fake: func(_ context.Context, q *dns.Msg) (*dns.Msg, error) {
				calls++
				gate.Set(dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 2})
				return mocks.AddressReply(q, "198.18.1.2"), nil
			}}
			switch mode {
			case "not-ready":
				gate.Set(dnsserver.SafetySnapshot{Generation: 1})
			case "HTTPS":
				q.SetQuestion("test.example.", dns.TypeHTTPS)
			case "SVCB":
				q.SetQuestion("test.example.", dns.TypeSVCB)
			}
			r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
			if len(r.Answer) > 0 || gate.Leases != 0 {
				t.Fatal("unsafe Fake-IP or leaked lease", r)
			}
			if mode == "HTTPS" || mode == "SVCB" {
				if r.Rcode != 0 || calls != 0 {
					t.Fatal("address hint behavior changed", r)
				}
			} else if r.Rcode != dns.RcodeServerFailure {
				t.Fatal("unsafe issuance", r)
			}
		})
	}
}
func TestPolicyDNSBackupCacheDoesNotMaskPrimaryRecovery(t *testing.T) {
	recovered := false
	primary := &mocks.Resolver{Func: func(_ context.Context, r model.ResolveRequest) (*dns.Msg, error) {
		if !recovered {
			return nil, errors.New("down")
		}
		r.Trace.Endpoint = "primary"
		return mocks.AddressReply(r.Message, "203.0.113.1"), nil
	}}
	wire := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		return mocks.AddressReply(q, "203.0.113.2"), nil
	}}
	backup, err := resolver.NewReal(wire, []resolver.ResolverEndpoint{{Protocol: "udp", Address: "192.0.2.53:53", Enabled: true}}, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	d, _, q := fallbackDispatcher(primary, "upstreams", backup)
	for i := 0; i < 2; i++ {
		q.Id++
		if d.Dispatch(context.Background(), model.DNSQueryContext{}, q).Rcode != 0 {
			t.Fatal("backup failed")
		}
	}
	if wire.Calls.Load() != 1 || !d.History.Records()[0].ResponseCacheHit {
		t.Fatal("backup cache not isolated")
	}
	recovered = true
	q.Id++
	r := d.Dispatch(context.Background(), model.DNSQueryContext{}, q)
	record := d.History.Records()[0]
	if r.Rcode != 0 || record.Resolver != "primary" || record.Result.PolicyDNS.FallbackUsed || record.ResponseCacheHit || wire.Calls.Load() != 1 {
		t.Fatal("cached backup masked recovery", record)
	}
}
