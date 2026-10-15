package database

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"testing"
	"time"
)

func domainRequest(m *dnsserver.Monitoring, source, domain string) {
	ip := netip.MustParseAddr(source)
	m.BeginSource(ip)
	m.FinishQuerySource(ip, domain, false, model.DecisionResult{Decision: model.Direct, Policy: "DIRECT", FinalPolicy: "DIRECT"}, "real", "NOERROR", time.Millisecond)
}
func TestDomainRankingOutlivesCombinationCapacityAndPersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "domains.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	at := time.Now().UTC().Truncate(time.Minute)
	save := func(second int, retry bool) {
		t.Helper()
		snapshot := m.Snapshot()
		snapshot.ObservedAt = at.Add(time.Duration(second) * time.Second)
		batch := m.PendingDomains()
		if batch == nil {
			t.Fatal("missing batch")
		}
		batch.StartedAt = at
		batch.ObservedAt = snapshot.ObservedAt
		if err := s.SaveDNSMonitoring(ctx, snapshot, batch); err != nil {
			t.Fatal(err)
		}
		if retry {
			if err := s.SaveDNSMonitoring(ctx, snapshot, batch); err != nil {
				t.Fatal(err)
			}
		}
		m.CommitDomains(batch)
	}
	save(0, false)
	for batch := 1; batch <= 12; batch++ {
		for i := 0; i < 600; i++ {
			domainRequest(m, "192.0.2.1", fmt.Sprintf("host-%d.test", (batch-1)*600+i))
		}
		save(batch, false)
	}
	for i := 0; i < 2000; i++ {
		domainRequest(m, "192.0.2.1", "LATE-HOT.TEST.")
	}
	for i := 0; i < 9; i++ {
		domainRequest(m, "192.0.2.2", "late-hot.test")
	}
	// Reads include pending writes before sorting; a new popular domain is visible immediately.
	buffer := m.BufferedDomains()
	rows, stats, err := s.DNSMonitoringDomains(ctx, time.Time{}, time.Time{}, "", &buffer)
	if err != nil || stats == nil || stats.Completed != 9209 || stats.Unrecorded != 0 || rows[0].Domain != "late-hot.test" || rows[0].Queries != 2009 {
		t.Fatal(rows, stats, err)
	}
	var sum uint64
	for _, row := range rows {
		sum += row.Queries
	}
	if sum != 9209 || len(rows) != 101 || !rows[100].Rest {
		t.Fatal("top100 remainder does not conserve total", sum, len(rows))
	}
	sourceRows, sourceStats, err := s.DNSMonitoringDomains(ctx, time.Time{}, time.Time{}, "192.0.2.2", &buffer)
	if err != nil || sourceStats.Completed != 9 || len(sourceRows) != 1 || sourceRows[0].Queries != 9 {
		t.Fatal(sourceRows, sourceStats, err)
	}
	// The old correlated detail really is exhausted, while independent domains remain exact.
	dimensions := m.Snapshot()
	dnsserver.AttachMonitoringDomains(&dimensions, dimensions.Observations, dnsserver.MonitoringFilter{})
	if dimensions.DimensionOverflow == 0 || dnsserver.FilterMonitoringCounts(dimensions, dnsserver.MonitoringFilter{Domain: "late-hot.test"}).Completed != 0 {
		t.Fatal("fixture did not exhaust combinations")
	}
	save(13, true)
	rows, stats, err = s.DNSMonitoringDomains(ctx, time.Time{}, time.Time{}, "", nil)
	if err != nil || stats.Completed != 9209 || rows[0].Queries != 2009 {
		t.Fatal("retry duplicated batch", rows, stats, err)
	}
	history, err := s.DNSMonitoringHistory(ctx, at, at.Add(time.Minute), "")
	if err != nil || history.Summary.DomainStatistics == nil || history.Summary.DomainStatistics.Completed != 9209 || history.Summary.Domains[0].Queries != 2009 {
		t.Fatal(history.Summary.DomainStatistics, err)
	}
	sourceHistory, err := s.DNSMonitoringHistory(ctx, at, at.Add(time.Minute), "192.0.2.2")
	if err != nil || sourceHistory.Summary.Domains[0].Queries != 9 {
		t.Fatal(sourceHistory.Summary.Domains, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved dnsserver.DNSMonitoringSnapshot
	if err = s.LoadStatistics(ctx, "dns_monitoring", &saved); err != nil {
		t.Fatal(err)
	}
	m = dnsserver.NewMonitoring(saved)
	domainRequest(m, "192.0.2.1", "late-hot.test")
	buffer = m.BufferedDomains()
	rows, stats, err = s.DNSMonitoringDomains(ctx, time.Time{}, time.Time{}, "", &buffer)
	if err != nil || stats.Completed != 9210 || rows[0].Queries != 2010 || !stats.StartedAt.Equal(at) {
		t.Fatal("restart lost domain counts/start", stats, err)
	}
}

func TestDomainBatchRollsBackWithMonitoringAndKeepsRetry(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "private", "domains.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	first := m.Snapshot()
	batch := m.PendingDomains()
	if err = s.SaveDNSMonitoring(ctx, first, batch); err != nil {
		t.Fatal(err)
	}
	m.CommitDomains(batch)
	domainRequest(m, "192.0.2.1", "retry.test")
	valid := m.Snapshot()
	batch = m.PendingDomains()
	invalid := valid
	invalid.ObservedAt = first.ObservedAt
	if err = s.SaveDNSMonitoring(ctx, invalid, batch); err == nil {
		t.Fatal("invalid monitoring checkpoint accepted")
	}
	rows, stats, err := s.DNSMonitoringDomains(ctx, time.Time{}, time.Time{}, "", nil)
	if err != nil || stats.Completed != 0 || len(rows) != 0 {
		t.Fatal("domain writes escaped failed transaction", rows, stats, err)
	}
	if m.PendingDomains() != batch {
		t.Fatal("failed batch discarded")
	}
	if err = s.SaveDNSMonitoring(ctx, valid, batch); err != nil {
		t.Fatal(err)
	}
	m.CommitDomains(batch)
	rows, stats, err = s.DNSMonitoringDomains(ctx, time.Time{}, time.Time{}, "", nil)
	if err != nil || stats.Completed != 1 || rows[0].Queries != 1 {
		t.Fatal(rows, stats, err)
	}
}
