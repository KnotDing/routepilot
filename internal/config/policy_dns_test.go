package config

import (
	"encoding/json"
	"surge-controller/internal/resolver"
	"testing"
)

func policyDNSFixture() Document {
	d := Default()
	d.Settings.DNS.PolicyUpstreams = []PolicyUpstream{{Field: "policy", Policy: "Proxy", Enabled: true, Upstreams: []resolver.ResolverEndpoint{{Protocol: "udp", Address: "223.6.6.6:53", Enabled: true}}}}
	return d
}
func TestPolicyDNSValidationCloneAndPersistence(t *testing.T) {
	d := policyDNSFixture()
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(d)
	var restored Document
	if e := json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if e := restored.Validate(); e != nil {
		t.Fatal(e)
	}
	copy := d.Clone()
	copy.Settings.DNS.PolicyUpstreams[0].Upstreams[0].Address = "192.0.2.10:53"
	if d.Settings.DNS.PolicyUpstreams[0].Upstreams[0].Address != "223.6.6.6:53" {
		t.Fatal("nested endpoint mutation affected live configuration")
	}
	for name, change := range map[string]func(*Document){
		"field": func(d *Document) { d.Settings.DNS.PolicyUpstreams[0].Field = "source" },
		"empty": func(d *Document) { d.Settings.DNS.PolicyUpstreams[0].Policy = "" },
		"duplicate": func(d *Document) {
			d.Settings.DNS.PolicyUpstreams = append(d.Settings.DNS.PolicyUpstreams, d.Settings.DNS.PolicyUpstreams[0])
		},
		"disabled pool": func(d *Document) { d.Settings.DNS.PolicyUpstreams[0].Upstreams[0].Enabled = false },
		"fake endpoint": func(d *Document) { d.Settings.DNS.PolicyUpstreams[0].Upstreams[0].Address = "198.18.1.1:53" },
		"bad server":    func(d *Document) { d.Settings.DNS.PolicyUpstreams[0].Upstreams[0].Address = "dns.example" },
	} {
		t.Run(name, func(t *testing.T) {
			d := policyDNSFixture()
			change(&d)
			if d.Validate() == nil {
				t.Fatal("invalid policy DNS configuration accepted")
			}
		})
	}
	d.Settings.DNS.PolicyUpstreams[0].Enabled = false
	d.Settings.DNS.PolicyUpstreams[0].Upstreams[0].Enabled = false
	if e := d.Validate(); e != nil {
		t.Fatal("disabled mapping rejected", e)
	}
}

func TestPolicyDNSFallbackValidationAndClone(t *testing.T) {
	d := policyDNSFixture()
	rule := &d.Settings.DNS.PolicyUpstreams[0]
	rule.Fallback = "upstreams"
	rule.FallbackUpstreams = []resolver.ResolverEndpoint{{Protocol: "tcp", Address: "192.0.2.53:53", Enabled: true}}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(d)
	var restored Document
	if e := json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if restored.Settings.DNS.PolicyUpstreams[0].Fallback != "upstreams" || restored.Settings.DNS.PolicyUpstreams[0].FallbackUpstreams[0].Address != "192.0.2.53:53" {
		t.Fatal("backup not persisted")
	}
	copy := d.Clone()
	copy.Settings.DNS.PolicyUpstreams[0].FallbackUpstreams[0].Address = "192.0.2.54:53"
	if rule.FallbackUpstreams[0].Address != "192.0.2.53:53" {
		t.Fatal("backup edit mutated live configuration")
	}
	for _, mode := range []string{"", "none", "surge", "real", "upstreams"} {
		copy := d.Clone()
		copy.Settings.DNS.PolicyUpstreams[0].Fallback = mode
		if e := copy.Validate(); e != nil {
			t.Fatal(mode, e)
		}
	}
	for name, change := range map[string]func(*PolicyUpstream){
		"invalid mode":     func(r *PolicyUpstream) { r.Fallback = "proxy" },
		"empty pool":       func(r *PolicyUpstream) { r.FallbackUpstreams = nil },
		"disabled pool":    func(r *PolicyUpstream) { r.FallbackUpstreams[0].Enabled = false },
		"Fake-IP":          func(r *PolicyUpstream) { r.FallbackUpstreams[0].Address = "198.18.1.1:53" },
		"invalid endpoint": func(r *PolicyUpstream) { r.FallbackUpstreams[0].Address = "dns.example:53" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := d.Clone()
			change(&copy.Settings.DNS.PolicyUpstreams[0])
			if copy.Validate() == nil {
				t.Fatal("invalid backup accepted")
			}
		})
	}
}
