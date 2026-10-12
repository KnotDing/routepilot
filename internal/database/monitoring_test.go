package database

import (
	"context"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
)

func TestMonitoringHistoryPersistsFiltersSourcesAndAvoidsDuplicateIntervals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "history.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	sample := m.Snapshot()
	sample.ObservedAt = at
	if err = s.SaveDNSMonitoring(ctx, sample); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		ip := netip.MustParseAddr("192.0.2.10")
		decision := model.Surge
		policy, path := "Proxy", "surge"
		if i == 2 {
			ip = netip.MustParseAddr("192.0.2.20")
			decision = model.Direct
			policy, path = "DIRECT", "real"
		}
		m.BeginSource(ip)
		m.FinishSource(ip, i != 2, model.DecisionResult{Decision: decision, Policy: policy, FinalPolicy: policy}, path, "NOERROR", 2*time.Millisecond)
		sample = m.Snapshot()
		sample.ObservedAt = at.Add(time.Duration(i+1) * 5 * time.Second)
		if err = s.SaveDNSMonitoring(ctx, sample); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SaveDNSMonitoring(ctx, sample); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	all, err := s.DNSMonitoringHistory(ctx, at, at.Add(time.Minute), "")
	if err != nil {
		t.Fatal(err)
	}
	if all.Summary.Received != 3 || all.Summary.Completed != 3 || len(all.Points) != 1 || all.Points[0].Seconds != 15 || !all.Points[0].RateValid || len(all.Sources) != 2 {
		t.Fatalf("lost or duplicated history: %+v", all)
	}
	for source, want := range map[string]uint64{"192.0.2.10": 2, "192.0.2.20": 1} {
		h, err := s.DNSMonitoringHistory(ctx, at, at.Add(time.Minute), source)
		if err != nil {
			t.Fatal(err)
		}
		if h.Summary.Completed != want || h.Points[0].Counts.Completed != want || len(h.Summary.Policies) != 1 || len(h.Summary.Flows) != 1 || h.TotalReceived != 3 {
			t.Fatalf("source data leaked: %+v", h)
		}
	}
	// Failed transaction must not move the cumulative checkpoint.
	if _, err = s.DB.Exec("DROP TABLE dns_monitoring_history"); err != nil {
		t.Fatal(err)
	}
	m.BeginSource(netip.MustParseAddr("192.0.2.10"))
	m.FinishSource(netip.MustParseAddr("192.0.2.10"), false, model.DecisionResult{Decision: model.Direct}, "real", "NOERROR", time.Millisecond)
	sample = m.Snapshot()
	sample.ObservedAt = at.Add(20 * time.Second)
	if err = s.SaveDNSMonitoring(ctx, sample); err == nil {
		t.Fatal("history failure was hidden")
	}
	var saved dnsserver.DNSMonitoringSnapshot
	if err = s.LoadStatistics(ctx, "dns_monitoring", &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Completed != 3 {
		t.Fatal("failed transaction moved checkpoint")
	}
}

func TestMonitoringHistoryRetentionGapsAndMinuteRangeValidation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "private", "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	for _, offset := range []time.Duration{0, 5 * time.Second, 60 * time.Second, 65 * time.Second, 8 * 24 * time.Hour, 8*24*time.Hour + 5*time.Second} {
		sample := m.Snapshot()
		sample.ObservedAt = at.Add(offset)
		if err = s.SaveDNSMonitoring(ctx, sample); err != nil {
			t.Fatal(err)
		}
	}
	old, err := s.DNSMonitoringHistory(ctx, at, at.Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Points) != 0 {
		t.Fatal("expired history remained")
	}
	recent, err := s.DNSMonitoringHistory(ctx, at.Add(8*24*time.Hour), at.Add(8*24*time.Hour+time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Points) != 1 || recent.Points[0].RateValid {
		t.Fatal("long observation gap invented a rate")
	}
	for _, end := range []time.Time{at, at.Add(8 * 24 * time.Hour), at.Add(61 * time.Second)} {
		if _, err = s.DNSMonitoringHistory(ctx, at, end, ""); err == nil {
			t.Fatal("invalid range accepted")
		}
	}
}
