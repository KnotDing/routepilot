package surge

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
)

type dnsRuleRunner struct {
	*runner
	explanations []model.RuleMatchResult
	lookupErr    error
	lookup       func(context.Context) error
	calls        [][]string
}

func (r *dnsRuleRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "rule" && args[1] == "explain" {
		r.calls = append(r.calls, append([]string(nil), args...))
		if len(r.explanations) == 0 {
			return nil, errors.New("unexpected evaluation retry")
		}
		m := r.explanations[0]
		r.explanations = r.explanations[1:]
		return json.Marshal(m)
	}
	if len(args) == 3 && args[0] == "dns" && args[1] == "lookup" {
		r.calls = append(r.calls, append([]string(nil), args...))
		if r.lookup != nil {
			return nil, r.lookup(ctx)
		}
		return []byte(`{"answers":[]}`), r.lookupErr
	}
	return r.runner.Run(ctx, args...)
}

func dnsExplanation(policy string, notes ...string) model.RuleMatchResult {
	return model.RuleMatchResult{MatchedRule: "FINAL," + policy, Policy: policy, FinalPolicy: policy, FinalType: "direct", Notes: notes}
}

func newDNSRuleAdapter(t *testing.T, r *dnsRuleRunner) *Adapter {
	t.Helper()
	r.runner = &runner{rules: []string{"FINAL,DIRECT"}}
	a, err := New(r, transport{}, resolver.ResolverEndpoint{Protocol: "udp", Address: "198.18.0.2:53"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return a
}

var dnsRuleRequest = model.RuleMatchRequest{Domain: "Example.test.", ClientIP: netip.MustParseAddr("192.0.2.10")}

func TestRuleDNSCompletedDuringEvaluationDoesNotLookupAgain(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP", "DNS completed, IPv4 address for matching: 192.0.2.30, IPv6 address for matching: N/A")}}
	a := newDNSRuleAdapter(t, r)
	m, err := a.ExplainRule(context.Background(), dnsRuleRequest)
	if err != nil || !m.ContextSafe || m.RuleDNS == nil || m.RuleDNS.State != "completed" || m.RuleDNS.LookupPerformed || len(r.calls) != 1 {
		t.Fatalf("completed DNS was treated as a limitation: %+v, %v, calls=%v", m, err, r.calls)
	}
}

func TestRuleDNSCompletedAfterTransientFailureDoesNotLookupAgain(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP", "DNS lookup failed from the first server", "DNS completed, IPv4 address for matching: 192.0.2.30, IPv6 address for matching: N/A")}}
	a := newDNSRuleAdapter(t, r)
	m, err := a.ExplainRule(context.Background(), dnsRuleRequest)
	if err != nil || !m.ContextSafe || m.RuleDNS == nil || m.RuleDNS.State != "completed" || m.RuleDNS.LookupPerformed || len(r.calls) != 1 {
		t.Fatalf("a recovered DNS failure still prevented the final result: %+v %v", m, err)
	}
}

func TestRuleDNSPendingLookupAndReEvaluationPreserveContext(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{
		dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP"),
		dnsExplanation("Proxy", "Rule evaluating requires DNS lookup for rule: GEOIP", "DNS completed, IPv4 address for matching: 192.0.2.30, IPv6 address for matching: N/A"),
	}}
	a := newDNSRuleAdapter(t, r)
	m, err := a.ExplainRule(context.Background(), dnsRuleRequest)
	if err != nil || m.FinalPolicy != "Proxy" || m.RuleDNS == nil || m.RuleDNS.State != "completed" || !m.RuleDNS.LookupPerformed || !m.RuleDNS.ReEvaluated {
		t.Fatalf("re-evaluation did not replace the initial policy: %+v, %v", m, err)
	}
	if len(r.calls) != 3 || !reflect.DeepEqual(r.calls[0], r.calls[2]) || !reflect.DeepEqual(r.calls[1], []string{"dns", "lookup", "example.test"}) {
		t.Fatalf("wrong resolution/evaluation sequence: %v", r.calls)
	}
}

func TestRuleDNSCanCompleteAnExplanationWithoutInitialPolicy(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{{Notes: []string{"Rule evaluating requires DNS lookup for rule: GEOIP"}}, dnsExplanation("DIRECT", "DNS completed, IPv4 address for matching: 192.0.2.30")}}
	a := newDNSRuleAdapter(t, r)
	m, err := a.ExplainRule(context.Background(), dnsRuleRequest)
	if err != nil || m.FinalPolicy != "DIRECT" || m.RuleDNS == nil || !m.RuleDNS.ReEvaluated {
		t.Fatalf("pending evaluation rejected before resolution could finish: %+v %v", m, err)
	}
}

func TestRuleDNSRetryCannotHideFailureOrLoop(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lookupErr error
		second    model.RuleMatchResult
		wantCalls int
		state     string
	}{
		{"lookup-failure", errors.New("DNS unavailable"), model.RuleMatchResult{}, 2, "failed"},
		{"still-pending", nil, dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP"), 3, "incomplete"},
		{"still-failing", nil, dnsExplanation("DIRECT", "DNS lookup failed: no response"), 3, "failed"},
		{"no-addresses", nil, dnsExplanation("DIRECT", "DNS completed, IPv4 address for matching: N/A, IPv6 address for matching: N/A"), 3, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &dnsRuleRunner{lookupErr: tc.lookupErr, explanations: []model.RuleMatchResult{dnsExplanation("DIRECT", "DNS lookup failed: timeout"), tc.second}}
			a := newDNSRuleAdapter(t, r)
			m, err := a.ExplainRule(context.Background(), dnsRuleRequest)
			if err == nil || m.ContextSafe || m.RuleDNS == nil || m.RuleDNS.State != tc.state || m.RuleDNS.Reason == "" || len(r.calls) != tc.wantCalls {
				t.Fatalf("failure accepted or retried without a bound: %+v, %v, calls=%v", m, err, r.calls)
			}
		})
	}
}

func TestRuleDNSLookupHonorsEvaluationDeadline(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP")}, lookup: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
	a := newDNSRuleAdapter(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	m, err := a.ExplainRule(ctx, dnsRuleRequest)
	if !errors.Is(err, context.DeadlineExceeded) || m.RuleDNS == nil || m.RuleDNS.State != "failed" || len(r.calls) != 2 {
		t.Fatalf("deadline did not stop DNS re-evaluation: %+v %v", m, err)
	}
}

func TestCompletedRuleDNSKeepsOtherContextLimitations(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP", "DNS completed, IPv4 address for matching: 192.0.2.30", "Missing process path for rule matching")}}
	a := newDNSRuleAdapter(t, r)
	m, err := a.ExplainRule(context.Background(), dnsRuleRequest)
	if err != nil || m.ContextSafe || !strings.Contains(m.SafetyReason, "Missing process path") || m.RuleDNS.State != "completed" {
		t.Fatalf("DNS success hid a missing connection field: %+v %v", m, err)
	}
}

func TestConnectionTestUsesSameDNSRetryWithExplicitMetadata(t *testing.T) {
	r := &dnsRuleRunner{explanations: []model.RuleMatchResult{dnsExplanation("DIRECT", "Rule evaluating requires DNS lookup for rule: GEOIP"), dnsExplanation("DIRECT")}}
	a := newDNSRuleAdapter(t, r)
	m, err := a.ExplainConnection(context.Background(), "example.test", dnsRuleRequest.ClientIP, "tcp", 8443)
	if err != nil || m.RuleDNS == nil || !m.RuleDNS.ReEvaluated || len(r.calls) != 3 || !reflect.DeepEqual(r.calls[0], r.calls[2]) {
		t.Fatalf("connection retry lost context: %+v %v %v", m, err, r.calls)
	}
	for _, want := range []string{"dest-port=8443", "protocol=tcp", "source-address=192.0.2.10"} {
		if !strings.Contains(strings.Join(r.calls[2], " "), want) {
			t.Fatalf("missing connection metadata: %s", want)
		}
	}
}
