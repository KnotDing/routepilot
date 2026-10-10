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
