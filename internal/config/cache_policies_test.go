package config

import (
	"encoding/json"
	"surge-controller/internal/cache"
	"testing"
)

func TestCachePoliciesDefaultsValidationAndClone(t *testing.T) {
	d := Default()
	off := false
	d.Settings.Cache.Decisions = map[string]cache.Policy{"DIRECT": {Optimistic: &off}, "SURGE": {Optimistic: &off}}
	d.Settings.Cache.Upstreams = map[string]cache.Policy{"udp://192.0.2.53:53": {Optimistic: &off}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := d.Clone()
	*copy.Settings.Cache.Decisions["DIRECT"].Optimistic = true
	copy.Settings.Cache.Upstreams["udp://192.0.2.53:53"] = cache.Policy{}
	if d.Settings.Cache.Decisions["DIRECT"].OptimisticEnabled() || d.Settings.Cache.Upstreams["udp://192.0.2.53:53"].OptimisticEnabled() {
		t.Fatal("clone mutated active cache policies")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var restored Document
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Settings.Cache.Decisions["DIRECT"].OptimisticEnabled() || restored.Settings.Cache.Decisions["SURGE"].OptimisticEnabled() {
		t.Fatal("policy disabled value lost in roundtrip")
	}
	if !restored.Settings.Cache.Decisions["REJECT"].OptimisticEnabled() {
		t.Fatal("missing override changed legacy behavior")
	}
	invalid := d.Clone()
	invalid.Settings.Cache.Decisions["BAD"] = cache.Policy{}
	if invalid.Validate() == nil {
		t.Fatal("unsupported decision accepted")
	}
	invalid = d.Clone()
	invalid.Settings.Cache.Upstreams["bad\nidentity"] = cache.Policy{}
	if invalid.Validate() == nil {
		t.Fatal("invalid upstream identity accepted")
	}
}
