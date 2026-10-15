package app

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"reflect"
	"strings"
	"surge-controller/internal/cache"
	"surge-controller/internal/database"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"sync"
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
	a.dnsMonitoring.FinishQuerySource(netip.Addr{}, "persist.example", true, model.DecisionResult{Decision: model.Surge, Policy: "Proxy", FinalPolicy: "Node A"}, "real", "NOERROR", 2*time.Millisecond)
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
	filtered, err := restarted.DNSMonitoringFilteredView(context.Background(), dnsserver.MonitoringFilter{Domain: "persist.example", Destination: "real", Policy: dnsserver.MonitoringPolicyValue("Proxy")})
	if err != nil || filtered.Completed != 1 || len(filtered.Domains) != 1 || filtered.Domains[0].Domain != "persist.example" {
		t.Fatal(filtered, err)
	}
	metrics := restarted.Metrics()
	for _, line := range []string{`dns_monitor_domain_completed_total{domain="persist.example",other="false"} 1`, "dns_monitor_completed_total 1", `dns_monitor_flow_total{ingress="intercepted",destination="real"} 1`, `dns_monitor_policy_total{policy="Proxy",final_policy="Node A",destination="real",other="false"} 1`} {
		if !strings.Contains(metrics, line) {
			t.Fatalf("missing metric %q", line)
		}
	}
}

func TestCacheReliabilityStatisticsSurviveServiceRestart(t *testing.T) {
	a := fixtureApp(t)
	path := filepath.Join(a.dir, "c.sqlite")
	want := cache.OptimisticStats{Hits: 17, Compared: 12, Mismatches: 3, ByGroup: map[string]cache.ReliabilityStats{"DIRECT": {Hits: 17, Compared: 12, Mismatches: 3}}}
	responseWant := want.Clone()
	responseWant.ByGroup = map[string]cache.ReliabilityStats{"udp://192.0.2.53:53": {Hits: 17, Compared: 12, Mismatches: 3}}
	a.runtime.engine.Cache.RestoreOptimisticStats(want)
	a.runtime.responseOptimisticStats = responseWant
	d := a.Configuration()
	d.Settings.DNS.TimeoutMS++
	if err := a.activateConfiguration(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.runtime.engine.Cache.OptimisticStats(), want) || !reflect.DeepEqual(a.runtime.responseOptimisticStats, responseWant) {
		t.Fatal("runtime recreation lost cache group statistics")
	}
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
	if got := restarted.runtime.engine.Cache.OptimisticStats(); !reflect.DeepEqual(got, want) {
		t.Fatalf("decision reliability stats after restart=%+v want=%+v", got, want)
	}
	if got := restarted.runtime.responseOptimisticStats; !reflect.DeepEqual(got, responseWant) {
		t.Fatalf("response reliability stats after restart=%+v want=%+v", got, responseWant)
	}
}

func TestClientStatisticsPersistPeriodicallyAcrossRebuildAndRestart(t *testing.T) {
	a := fixtureApp(t)
	ip := netip.MustParseAddr("192.0.2.10")
	seen := time.Date(2026, 10, 12, 1, 2, 3, 0, time.UTC)
	a.clientStatistics.Record(ip, model.Direct, false, seen)
	a.clientStatistics.Record(netip.MustParseAddr("::ffff:192.0.2.10"), model.Surge, true, seen.Add(time.Second))
	deadline := time.Now().Add(8 * time.Second)
	for {
		var saved dnsserver.ClientStatsSnapshot
		if err := a.store.LoadStatistics(context.Background(), "dns_clients", &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Stats.Queries == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("periodic client checkpoint not written")
		}
		time.Sleep(25 * time.Millisecond)
	}
	previous := a.runtime.dispatcher
	d := a.Configuration()
	d.Settings.DNS.TimeoutMS++
	if err := a.activateConfiguration(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if previous == a.runtime.dispatcher || a.runtime.dispatcher.ClientStatistics != a.clientStatistics || a.clientStatistics.Snapshot().Stats.Queries != 2 {
		t.Fatal("rebuild lost client statistics")
	}
	path := filepath.Join(a.dir, "c.sqlite")
	a.clientStatistics.Record(netip.MustParseAddr("2001:db8::10"), model.Reject, false, seen)
	want := a.clientStatistics.Snapshot()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.clientStatistics.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("client checkpoint not restored: got %+v want %+v", got, want)
	}
	reopened.clientStatistics.Record(ip, model.Direct, false, seen.Add(2*time.Second))
	if got := reopened.clientStatistics.Snapshot(); got.Stats.Queries != 4 || got.Clients[0].Queries != 3 || got.Clients[0].Errors != 1 {
		t.Fatal(got)
	}
}

func TestClientPersistenceErrorsAreVisibleAndRetryPreservesCounts(t *testing.T) {
	a := fixtureApp(t)
	a.clientStatistics.Record(netip.MustParseAddr("192.0.2.10"), model.Reject, false, time.Now())
	if _, err := a.store.DB.Exec("CREATE TRIGGER fail_clients BEFORE INSERT ON statistics WHEN NEW.id='dns_clients' BEGIN SELECT RAISE(ABORT, 'test persistence failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := a.saveClientStatistics(); err == nil {
		t.Fatal("storage error hidden")
	}
	view, err := a.View(context.Background(), "dns/clients")
	if err != nil {
		t.Fatal(err)
	}
	if view.(dnsserver.ClientStatsSnapshot).StorageError == "" || !strings.Contains(a.Metrics(), "dns_client_statistics_storage_healthy 0") {
		t.Fatal("missing persistence error")
	}
	if _, err := a.store.DB.Exec("DROP TRIGGER fail_clients"); err != nil {
		t.Fatal(err)
	}
	if err := a.saveClientStatistics(); err != nil {
		t.Fatal(err)
	}
	view, _ = a.View(context.Background(), "dns/clients")
	if got := view.(dnsserver.ClientStatsSnapshot); got.StorageError != "" || got.Stats.Queries != 1 {
		t.Fatal(got)
	}
}

func TestIndependentDomainsRemainExactAcrossConcurrentSavesAndReads(t *testing.T) {
	ctx := context.Background()
	store, err := database.Open(filepath.Join(t.TempDir(), "private", "domains.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := &App{store: store, dnsMonitoring: dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})}
	if err = a.saveDNSMonitoring(a.dnsMonitoring.Snapshot()); err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("192.0.2.10")
	for i := 0; i < 100; i++ {
		a.dnsMonitoring.BeginSource(ip)
		a.dnsMonitoring.FinishQuerySource(ip, "concurrent.test", false, model.DecisionResult{Decision: model.Direct}, "real", "NOERROR", time.Millisecond)
	}
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 8; i++ {
			if err := a.saveDNSMonitoring(a.dnsMonitoring.Snapshot()); err != nil {
				failures <- err
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 8; i++ {
			view, err := a.DNSMonitoringFilteredView(ctx, dnsserver.MonitoringFilter{})
			if err != nil {
				failures <- err
				return
			}
			if view.DomainStatistics == nil || view.DomainStatistics.Completed != 100 || len(view.Domains) != 1 || view.Domains[0].Queries != 100 {
				failures <- fmt.Errorf("domain batch counted twice or omitted: %+v", view.DomainStatistics)
				return
			}
		}
	}()
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}
