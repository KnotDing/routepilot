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
	d.Settings.Cache.Upstreams = map[string]cache.Policy{"udp://192.0.2.53:53": {Optimistic: &off}, "surge://127.0.0.1:6153": {Optimistic: &off}}
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

func TestOrdinaryCacheSwitchesRoundTripCloneAndValidation(t *testing.T) {
	off, on := false, true
	d := Default()
	if !d.Settings.Cache.DecisionCacheEnabled() || !d.Settings.Cache.ResponseCacheEnabled() {
		t.Fatal("legacy caches disabled by default")
	}
	d.Settings.Cache.DecisionsEnabled, d.Settings.Cache.ResponsesEnabled = &off, &off
	d.Settings.Cache.Decisions = map[string]cache.Policy{"DIRECT": {Enabled: &off}}
	d.Settings.Cache.Upstreams = map[string]cache.Policy{"udp://192.0.2.53:53": {Enabled: &off}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := d.Clone()
	*cloned.Settings.Cache.DecisionsEnabled = true
	*cloned.Settings.Cache.ResponsesEnabled = true
	*cloned.Settings.Cache.Decisions["DIRECT"].Enabled = true
	*cloned.Settings.Cache.Upstreams["udp://192.0.2.53:53"].Enabled = true
	if d.Settings.Cache.DecisionCacheEnabled() || d.Settings.Cache.ResponseCacheEnabled() || d.Settings.Cache.Decisions["DIRECT"].CacheEnabled() || d.Settings.Cache.Upstreams["udp://192.0.2.53:53"].CacheEnabled() {
		t.Fatal("clone shares cache switches")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var restored Document
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Settings.Cache.DecisionCacheEnabled() || restored.Settings.Cache.ResponseCacheEnabled() || restored.Settings.Cache.Decisions["DIRECT"].CacheEnabled() || restored.Settings.Cache.Upstreams["udp://192.0.2.53:53"].CacheEnabled() {
		t.Fatal("explicit false lost in roundtrip")
	}
	for _, change := range []func(*Cache){
		func(c *Cache) { c.Optimistic = true },
		func(c *Cache) { c.OptimisticResponses = true },
		func(c *Cache) { c.Decisions["DIRECT"] = cache.Policy{Enabled: &off, Optimistic: &on} },
		func(c *Cache) { c.Upstreams["udp://192.0.2.53:53"] = cache.Policy{Enabled: &off, Optimistic: &on} },
	} {
		invalid := d.Clone()
		change(&invalid.Settings.Cache)
		if invalid.Validate() == nil {
			t.Fatal("optimistic cache enabled without ordinary cache")
		}
	}
}
