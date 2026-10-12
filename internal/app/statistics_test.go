package app

import (
	"context"
	"path/filepath"
	"strings"
	"surge-controller/internal/cache"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"testing"
	"time"
)

func TestDNSStatisticsSurviveShutdownAndContinueAcrossRestarts(t *testing.T) {
	a := fixtureApp(t)
	want := dnsserver.CounterSnapshot{Queries: 100, Direct: 60, Surge: 30, Reject: 7, Unknown: 3, Intercepted: 70, Addressed: 30, Errors: 2}
	a.dnsCounters.Restore(want)
	path := filepath.Join(a.dir, "c.sqlite")
	for restart := 0; restart < 3; restart++ {
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		var err error
		a, err = Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.dnsCounters.Snapshot(); got != want {
			_ = a.Close()
			t.Fatalf("restart %d lost cumulative counters: got %+v want %+v", restart, got, want)
		}
		a.dnsCounters.Queries.Add(1)
		want.Queries++
		a.dnsCounters.Direct.Add(1)
		want.Direct++
		a.dnsCounters.Addressed.Add(1)
		want.Addressed++
	}
	t.Cleanup(func() { _ = a.Close() })
}

func TestDNSMonitoringSurvivesRuntimeRebuildRestartAndExposesStatusAndMetrics(t *testing.T) {
	a := fixtureApp(t)
	a.dnsMonitoring.Begin()
	a.dnsMonitoring.Finish(true, model.DecisionResult{Decision: model.Surge, Policy: "Proxy", FinalPolicy: "Node A"}, "real", "NOERROR", 2*time.Millisecond)
	previous := a.runtime.dispatcher
	document := a.Configuration()
	document.Settings.DNS.TimeoutMS += 100
	if err := a.activateConfiguration(context.Background(), document); err != nil {
		t.Fatal(err)
	}
	if a.runtime.dispatcher == previous {
		t.Fatal("test did not exercise a DNS runtime rebuild")
	}
	if a.runtime.dispatcher.Monitoring != a.dnsMonitoring {
		t.Fatal("reconfiguration replaced cumulative monitoring")
	}
	path := filepath.Join(a.dir, "c.sqlite")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	view, err := restarted.View(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	s := view.(map[string]any)["dns_monitoring"].(dnsserver.DNSMonitoringSnapshot)
	if s.Received != 1 || s.Completed != 1 || s.Decisions["SURGE"] != 1 || s.Policies[0].Policy != "Proxy" {
		t.Fatalf("monitoring was not restored: %+v", s)
	}
	from := s.ObservedAt.Truncate(time.Minute).Add(-time.Minute)
	history, err := restarted.store.DNSMonitoringHistory(context.Background(), from, from.Add(2*time.Minute), "unknown")
	if err != nil {
		t.Fatal(err)
	}
	if history.Summary.Completed != 1 || len(history.Summary.Policies) != 1 || history.Summary.Policies[0].Policy != "Proxy" || history.Summary.Flows[0].Destination != "real" {
		t.Fatalf("source history lost across restart: %+v", history)
	}
	metrics := restarted.Metrics()
	for _, line := range []string{"dns_monitor_completed_total 1", `dns_monitor_flow_total{ingress="intercepted",destination="real"} 1`, `dns_monitor_policy_total{policy="Proxy",final_policy="Node A",destination="real",other="false"} 1`} {
		if !strings.Contains(metrics, line) {
			t.Fatalf("missing metric %q", line)
		}
	}
}

func TestCacheReliabilityStatisticsSurviveServiceRestart(t *testing.T) {
	a := fixtureApp(t)
	path := filepath.Join(a.dir, "c.sqlite")
	want := cache.OptimisticStats{Hits: 17, Compared: 12, Mismatches: 3}
	a.runtime.engine.Cache.RestoreOptimisticStats(want)
	a.runtime.responseOptimisticStats = want
	if err := a.saveDecisionCache(); err != nil {
		t.Fatal("save decision reliability stats:", err)
	}
	if err := a.saveResponseCache(); err != nil {
		t.Fatal("save response reliability stats:", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if got := restarted.runtime.engine.Cache.OptimisticStats(); got != want {
		t.Fatalf("decision reliability stats after restart=%+v want=%+v", got, want)
	}
	if got := restarted.runtime.responseOptimisticStats; got != want {
		t.Fatalf("response reliability stats after restart=%+v want=%+v", got, want)
	}
}
