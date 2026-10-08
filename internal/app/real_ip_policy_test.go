package app

import (
	"context"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
)

type realIPPolicyMatcher struct{}

func (realIPPolicyMatcher) MatchAlwaysRealIP(name string) (string, bool) {
	return "*.test", name == "example.test."
}

func TestAlwaysRealIPPolicySkipsRuleEngineAndSurgeMode(t *testing.T) {
	// A nil engine makes accidental Surge evaluation fail immediately. The
	// explicit real-IP directive precedes mode and DNS steering overrides.
	p := &policy{mode: "force-surge", realIPDomains: realIPPolicyMatcher{}, overrides: []config.Override{{Domain: "example.test", Match: "exact", Decision: model.Surge, Enabled: true}}}
	d, err := p.Classify(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), Question: dns.Question{Name: "example.test", Qtype: dns.TypeA, Qclass: dns.ClassINET}})
	if err != nil || d.Decision != model.Direct || d.Source != "surge-always-real-ip" || d.MatchedRule != "always-real-ip = *.test" {
		t.Fatalf("rule-test DNS behavior lost always-real-ip directive: %+v %v", d, err)
	}
}
