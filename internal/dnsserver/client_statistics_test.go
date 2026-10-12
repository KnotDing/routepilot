package dnsserver

import (
	"net/netip"
	"testing"
	"time"

	"surge-controller/internal/model"
)

func TestClientStatisticsAggregatesAndBoundsClientList(t *testing.T) {
	stats := NewClientStatistics(2)
	start := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	stats.Record(netip.MustParseAddr("192.0.2.1"), model.Direct, false, start)
	stats.Record(netip.MustParseAddr("::ffff:192.0.2.1"), model.Surge, true, start.Add(time.Second))
	stats.Record(netip.MustParseAddr("192.0.2.2"), model.Reject, false, start.Add(2*time.Second))
	stats.Record(netip.MustParseAddr("192.0.2.3"), model.Unknown, false, start.Add(3*time.Second))

	snapshot := stats.Snapshot()
	if snapshot.Capacity != 2 || snapshot.EvictedClients != 1 || len(snapshot.Clients) != 2 {
		t.Fatalf("client list did not enforce its bound: %+v", snapshot)
	}
	if snapshot.Stats.Clients != 2 || snapshot.Stats.Queries != 4 ||
		snapshot.Stats.Direct != 1 || snapshot.Stats.Surge != 1 ||
		snapshot.Stats.Reject != 1 || snapshot.Stats.Unknown != 1 ||
		snapshot.Stats.Errors != 1 {
		t.Fatalf("unexpected aggregate client statistics: %+v", snapshot.Stats)
	}
	if snapshot.Clients[0].ClientIP != "192.0.2.3" || snapshot.Clients[1].ClientIP != "192.0.2.2" {
		t.Fatalf("clients are not ordered by most recent activity: %+v", snapshot.Clients)
	}
}

func TestClientStatisticsIgnoresInvalidClientAddress(t *testing.T) {
	stats := NewClientStatistics(1)
	stats.Record(netip.Addr{}, model.Direct, false, time.Now())
	snapshot := stats.Snapshot()
	if len(snapshot.Clients) != 0 || snapshot.Stats.Queries != 0 {
		t.Fatalf("invalid client address was recorded: %+v", snapshot)
	}
}

func TestClientStatisticsRestorePreservesEvictionsAndRejectsInvalidRowsAtomically(t *testing.T) {
	original := NewClientStatistics(2)
	for i := 1; i <= 3; i++ {
		original.Record(netip.AddrFrom4([4]byte{192, 0, 2, byte(i)}), model.Direct, false, time.Now())
	}
	saved := original.Snapshot()
	restored := NewClientStatistics(2)
	if err := restored.Restore(saved); err != nil {
		t.Fatal(err)
	}
	got := restored.Snapshot()
	if got.Stats.Queries != 3 || got.EvictedClients != 1 || len(got.Clients) != 2 {
		t.Fatal(got)
	}
	saved.Clients[0].ClientIP = "invalid"
	if err := restored.Restore(saved); err == nil {
		t.Fatal("invalid restore accepted")
	}
	if restored.Snapshot().Clients[0].ClientIP != got.Clients[0].ClientIP {
		t.Fatal("failed restore mutated data")
	}
}

func TestClientStatisticsPreservesScopedIPv6StatisticsAcrossRestore(t *testing.T) {
	original := NewClientStatistics(2)
	original.Record(netip.MustParseAddr("fe80::10%en0"), model.Direct, false, time.Now())
	restored := NewClientStatistics(2)
	if err := restored.Restore(original.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if got := restored.Snapshot(); got.Clients[0].ClientIP != "fe80::10%en0" || got.Stats.Queries != 1 {
		t.Fatal(got)
	}
}
