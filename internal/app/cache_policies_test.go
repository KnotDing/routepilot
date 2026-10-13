package app

import (
	"context"
	"net/netip"
	"strings"
	"surge-controller/internal/cache"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"surge-controller/internal/surge"
	"surge-controller/tests/mocks"
	"testing"
	"time"
)

func TestSelectiveDecisionsRespectRevisionAndRestorePolicies(t *testing.T) {
	off := false
	d := config.Default()
	d.Settings.Cache.Optimistic = true
	d.Settings.Cache.Decisions = map[string]cache.Policy{"DIRECT": {Optimistic: &off}, "REJECT": {Optimistic: &off}}
	cfg := classifier.DefaultConfig()
	cfg.Optimistic = true
	cfg.OptimisticIdleRefresh = false
	cfg.Decisions = d.Settings.Cache.Decisions
	e, err := classifier.New(context.Background(), &mocks.Surge{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	rt := &Runtime{engine: e, settings: d, decisionRevision: decisionRevision{digest: "old", selection: "same"}}
	records := []decisionRecord{}
	for _, decision := range []model.Decision{model.Direct, model.Surge, model.Reject} {
		domain := strings.ToLower(string(decision)) + ".example."
		key := classifier.Key{Domain: domain, ClientGroup: "192.0.2.10", QueryClass: "1/1"}
		value := model.DecisionResult{Domain: domain, ClientIP: netip.MustParseAddr(key.ClientGroup), QueryType: 1, Decision: decision, Persistent: true, CreatedAt: time.Now()}
		e.Cache.Put(key, value, e.Cache.Generation(), cache.Forever)
		records = append(records, decisionRecord{Key: key, Value: value})
	}
	rt.observeDecisionRevision(surge.Snapshot{Digest: "new", SelectionDigest: "same"}, nil)
	if s := e.Cache.Stats(); s.Entries != 1 || s.Groups["SURGE"].StaleEntries != 1 {
		t.Fatal("revision retained disallowed decisions", s)
	}
	next, err := classifier.New(context.Background(), &mocks.Surge{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	rt.engine = next
	rt.pendingDecisions = &decisionCacheSnapshot{Digest: "new", Selection: "same", Entries: records}
	rt.observeDecisionRevision(surge.Snapshot{Digest: "new", SelectionDigest: "same"}, nil)
	if s := next.Cache.Stats(); s.Groups["DIRECT"].Entries != 1 || s.Groups["DIRECT"].StaleEntries != 0 || s.Groups["REJECT"].Entries != 1 || s.Groups["REJECT"].StaleEntries != 0 {
		t.Fatal("restore ignored selective decisions", s)
	}
}

func TestCacheGroupStatsExposeConsistentStatusCacheAPIAndMetrics(t *testing.T) {
	a := fixtureApp(t)
	decision := cache.OptimisticStats{Hits: 20, Compared: 10, Mismatches: 2, ByGroup: map[string]cache.ReliabilityStats{"DIRECT": {Hits: 20, Compared: 10, Mismatches: 2}}}
	a.runtime.engine.Cache.RestoreOptimisticStats(decision)
	a.runtime.responseOptimisticStats = cache.OptimisticStats{Hits: 12, Compared: 8, Mismatches: 1, ByGroup: map[string]cache.ReliabilityStats{"udp://192.0.2.53:53": {Hits: 12, Compared: 8, Mismatches: 1}}}
	status, err := a.View(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	view := status.(map[string]any)
	if view["selective_cache_policies"] != true || view["surge_response_cache"] != true {
		t.Fatal("selective capability missing")
	}
	parts := view["cache"].(map[string]any)
	if parts["decision"].(cache.Stats).Groups["DIRECT"].OptimisticMismatches != 2 || parts["response"].(map[string]any)["groups"].(map[string]cache.GroupStats)["udp://192.0.2.53:53"].OptimisticMismatches != 1 {
		t.Fatal("status lost cache group evidence")
	}
	cacheView, err := a.CacheView(context.Background(), 0, 50, "", "")
	if err != nil {
		t.Fatal(err)
	}
	api := cacheView.(map[string]any)
	if api["stats"].(cache.Stats).Groups["DIRECT"].OptimisticCompared != 10 || api["response_stats"].(map[string]any)["groups"].(map[string]cache.GroupStats)["udp://192.0.2.53:53"].OptimisticCompared != 8 {
		t.Fatal("cache API lost group evidence")
	}
	metrics := a.Metrics()
	for _, line := range []string{`decision_cache_by_decision_optimistic_compared_total{decision="DIRECT"} 10`, `response_cache_by_upstream_optimistic_mismatches_total{upstream="udp://192.0.2.53:53"} 1`} {
		if !strings.Contains(metrics, line) {
			t.Fatal("missing metric", line)
		}
	}
	if err := a.Flush(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if a.runtime.engine.Cache.OptimisticCounters().Hits != 0 || responseCacheStats(a.runtime)["groups"].(map[string]cache.GroupStats)["udp://192.0.2.53:53"].OptimisticHits != 0 {
		t.Fatal("manual clear retained group reliability")
	}
}
