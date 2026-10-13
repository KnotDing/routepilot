package app

import (
	"context"
	"reflect"
	"testing"

	"surge-controller/internal/cache"
	"surge-controller/internal/config"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

func TestAnomalyThresholdsApplyWithoutDNSRuntimeRestart(t *testing.T) {
	a := fixtureApp(t)
	a.mu.RLock()
	rt := a.runtime
	a.mu.RUnlock()
	d := a.Configuration()
	d.Settings.Notifications.Errors = []string{"cache_mismatch", "dns_request_burst", "dns_request_sustained"}
	d.Settings.Notifications.Anomalies = config.NotificationAnomalies{CacheMismatchPercent: 2.5, CacheWindowSeconds: 30, CacheMinimumCompared: 50, BurstQPS: 200, BurstWindowSeconds: 5, SustainedQPS: 50, SustainedSeconds: 120}
	if err := a.SaveConfiguration(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	waitConfiguration(t, a, "succeeded")
	a.mu.RLock()
	unchanged := a.runtime == rt
	a.mu.RUnlock()
	if !unchanged || a.activeConfiguration().Settings.Notifications.Anomalies != d.Settings.Notifications.Anomalies {
		t.Fatal("notification thresholds rebuilt DNS runtime or did not apply")
	}
	saved, err := a.store.Load(context.Background())
	if err != nil || saved.Settings.Notifications.Anomalies != d.Settings.Notifications.Anomalies {
		t.Fatal("thresholds not persisted", err)
	}
	view, err := a.View(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	status := view.(map[string]any)
	if status["notification_anomaly_alerts"] != true {
		t.Fatal("capability missing")
	}
	if _, ok := status["notification_alerts"]; !ok {
		t.Fatal("alert snapshot missing")
	}
}

func TestResponseAnomalyCountersIncludeCarriedTotalsSurgeAndPolicyPools(t *testing.T) {
	pool := func(stats cache.OptimisticStats) *resolver.Real {
		r, err := resolver.NewReal(&mocks.Transport{}, []resolver.ResolverEndpoint{{Protocol: "udp", Address: "192.0.2.53:53", Enabled: true}}, false, 100)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(r.Close)
		r.Cache.RestoreOptimisticStats(stats)
		return r
	}
	rt := &Runtime{responseOptimisticStats: cache.OptimisticStats{Hits: 100, Compared: 40, Mismatches: 4}, real: pool(cache.OptimisticStats{Hits: 20, Compared: 10, Mismatches: 2}), surgeResponses: pool(cache.OptimisticStats{Hits: 11, Compared: 5, Mismatches: 1}), policyResolvers: []*resolver.Real{nil, pool(cache.OptimisticStats{Hits: 30, Compared: 15, Mismatches: 3})}}
	before := rt.real.Cache.Stats()
	if got := responseOptimisticCounters(rt); !reflect.DeepEqual(got, cache.OptimisticStats{Hits: 161, Compared: 70, Mismatches: 10}) {
		t.Fatal("lost cumulative or policy-specific evidence", got)
	}
	if after := rt.real.Cache.Stats(); !reflect.DeepEqual(after, before) {
		t.Fatal("sampling changed cache statistics", before, after)
	}
}
