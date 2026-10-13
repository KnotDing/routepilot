package notifications

import (
	"strings"
	"testing"
	"time"

	"surge-controller/internal/cache"
	"surge-controller/internal/config"
	"surge-controller/internal/state"
)

func anomalySettings() config.Notifications {
	s := notificationSettings()
	s.Anomalies = config.NotificationAnomalies{CacheMismatchPercent: 25, CacheWindowSeconds: 3, CacheMinimumCompared: 4, BurstQPS: 10, BurstWindowSeconds: 2, SustainedQPS: 5, SustainedSeconds: 3}
	return s
}
func anomalySample(second int) AnomalySample {
	return AnomalySample{At: time.Unix(1000000+int64(second), 0), Queries: 1000000 + uint64(second*10), DecisionEnabled: true, ResponseEnabled: true, Decision: cache.OptimisticStats{Hits: 1000000 + uint64(second*1000), Compared: 1000 + uint64(second*4), Mismatches: 500 + uint64(second*2)}, Response: cache.OptimisticStats{Compared: 1000 + uint64(second*4), Mismatches: 500}}
}
func alertState(values []state.Component, name string) string {
	for _, v := range values {
		if v.Name == name {
			return v.State
		}
	}
	return "missing"
}

func TestAnomaliesUseOnlyNewVerifiedSamplesAndSeparateCaches(t *testing.T) {
	var monitor AnomalyMonitor
	s := anomalySettings()
	first := monitor.Observe(s, anomalySample(0))
	if alertState(first, DecisionMismatch) != "UNKNOWN" || alertState(first, RequestBurst) != "UNKNOWN" {
		t.Fatal("persisted totals became fresh samples", first)
	}
	for i := 1; i <= 3; i++ {
		monitor.Observe(s, anomalySample(i))
	}
	values := monitor.Snapshot()
	if alertState(values, DecisionMismatch) != "DOWN" || alertState(values, ResponseMismatch) != "UP" {
		t.Fatal("cache results were merged or used stale/unverified requests", values)
	}
	if !strings.Contains(values[0].LastError, "6 / 12") {
		t.Fatal("wrong request denominator", values[0])
	}
	m := New(func(string, string, string) {})
	m.Observe(s, values)
	payload := queued(t, m).payload
	if !strings.Contains(payload.Body, "6 / 12") || payload.Details[DecisionMismatch] == "" {
		t.Fatal("alert metrics omitted", payload)
	}
	if len(m.queue) != 0 {
		t.Fatal("unexpected extra notification")
	}
	m.Observe(s, values)
	if len(m.queue) != 0 {
		t.Fatal("duplicate alarm ignored cooldown")
	}
}

func TestAnomaliesRequireEnoughEvidenceAndResetOnCounterClear(t *testing.T) {
	var monitor AnomalyMonitor
	s := anomalySettings()
	s.Anomalies.CacheMinimumCompared = 20
	for i := 0; i <= 3; i++ {
		monitor.Observe(s, anomalySample(i))
	}
	if alertState(monitor.Snapshot(), DecisionMismatch) != "UNKNOWN" {
		t.Fatal("too few comparisons caused an alert")
	}
	s.Anomalies.CacheMinimumCompared = 4
	for i := 4; i <= 7; i++ {
		monitor.Observe(s, anomalySample(i))
	}
	if alertState(monitor.Snapshot(), DecisionMismatch) != "DOWN" {
		t.Fatal("threshold changes did not resample")
	}
	reset := anomalySample(8)
	reset.Decision = cache.OptimisticStats{}
	if alertState(monitor.Observe(s, reset), DecisionMismatch) != "UNKNOWN" {
		t.Fatal("cleared counters underflowed or falsely recovered")
	}
	reset.At = anomalySample(9).At
	reset.Decision = cache.OptimisticStats{Compared: 5, Mismatches: 6}
	if alertState(monitor.Observe(s, reset), DecisionMismatch) != "UNKNOWN" {
		t.Fatal("invalid counters treated as evidence")
	}
}

func TestRequestBurstUsesWindowAverageAndSustainedRequiresContinuity(t *testing.T) {
	var monitor AnomalyMonitor
	s := anomalySettings()
	for i := 0; i <= 2; i++ {
		monitor.Observe(s, anomalySample(i))
	}
	if alertState(monitor.Snapshot(), RequestBurst) != "DOWN" || alertState(monitor.Snapshot(), RequestSustained) != "UNKNOWN" {
		t.Fatal("short spike counted as sustained load", monitor.Snapshot())
	}
	monitor.Observe(s, anomalySample(3))
	if alertState(monitor.Snapshot(), RequestSustained) != "DOWN" {
		t.Fatal("continuous high load did not alert")
	}
	idle := anomalySample(4)
	idle.Queries = anomalySample(3).Queries
	if alertState(monitor.Observe(s, idle), RequestSustained) != "UP" {
		t.Fatal("idle interval did not clear sustained load")
	}
	gap := anomalySample(20)
	values := monitor.Observe(s, gap)
	if alertState(values, RequestBurst) != "UNKNOWN" || alertState(values, RequestSustained) != "UNKNOWN" {
		t.Fatal("sleep gap manufactured traffic continuity", values)
	}
	for i := 21; i <= 22; i++ {
		monitor.Observe(s, anomalySample(i))
	}
	if alertState(monitor.Snapshot(), RequestSustained) != "UNKNOWN" {
		t.Fatal("sustained timer survived gap")
	}
}

func TestAnomaliesRespectSelectionsDisabledCachesAndReenableWarmup(t *testing.T) {
	var monitor AnomalyMonitor
	s := anomalySettings()
	s.Errors = []string{"cache_mismatch"}
	for i := 0; i <= 3; i++ {
		sample := anomalySample(i)
		sample.ResponseEnabled = false
		monitor.Observe(s, sample)
	}
	values := monitor.Snapshot()
	if alertState(values, DecisionMismatch) != "DOWN" || alertState(values, ResponseMismatch) != "DISABLED" || alertState(values, RequestBurst) != "DISABLED" {
		t.Fatal("selection or cache enable flag ignored", values)
	}
	s.Enabled = false
	monitor.Observe(s, anomalySample(4))
	s.Enabled = true
	if alertState(monitor.Observe(s, anomalySample(5)), DecisionMismatch) != "UNKNOWN" {
		t.Fatal("disabled period became evidence")
	}
}

func TestManualCacheResetStartsNewEvidenceEvenIfCountersQuicklyExceedOldTotals(t *testing.T) {
	var monitor AnomalyMonitor
	s := anomalySettings()
	for i := 0; i <= 3; i++ {
		monitor.Observe(s, anomalySample(i))
	}
	monitor.ResetCaches()
	if alertState(monitor.Snapshot(), DecisionMismatch) != "UNKNOWN" {
		t.Fatal("old cache alert retained after manual clear")
	}
	sample := anomalySample(4)
	sample.Decision = cache.OptimisticStats{Compared: 10000, Mismatches: 5000}
	values := monitor.Observe(s, sample)
	if alertState(values, DecisionMismatch) != "UNKNOWN" || alertState(values, RequestBurst) != "DOWN" {
		t.Fatal("cache reset mixed old evidence or reset unrelated traffic", values)
	}
}
