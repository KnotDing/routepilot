package dnsserver

import (
	"fmt"
	"net/netip"
)

// Restore validates the complete snapshot before replacing any in-memory data.
// Aggregate totals include evicted clients, whose individual rows are absent.
func (s *ClientStatistics) Restore(snapshot ClientStatsSnapshot) error {
	validCounts := func(queries, direct, surge, reject, unknown, failures uint64) bool {
		if failures > queries {
			return false
		}
		remaining := queries
		for _, count := range []uint64{direct, surge, reject, unknown} {
			if count > remaining {
				return false
			}
			remaining -= count
		}
		return remaining == 0
	}
	all := snapshot.Stats
	if !validCounts(all.Queries, all.Direct, all.Surge, all.Reject, all.Unknown, all.Errors) {
		return fmt.Errorf("invalid persisted client aggregate statistics")
	}
	if len(snapshot.Clients) > s.capacity {
		return fmt.Errorf("client statistics exceed capacity %d", s.capacity)
	}
	clients := make(map[string]ClientStats, len(snapshot.Clients))
	var totals ClientStatsTotals
	for _, row := range snapshot.Clients {
		ip, err := netip.ParseAddr(row.ClientIP)
		if err != nil || row.LastSeen.IsZero() {
			return fmt.Errorf("invalid persisted client statistics row")
		}
		row.ClientIP = ip.Unmap().String()
		if _, duplicate := clients[row.ClientIP]; duplicate || !validCounts(row.Queries, row.Direct, row.Surge, row.Reject, row.Unknown, row.Errors) || row.Queries > all.Queries-totals.Queries {
			return fmt.Errorf("invalid or duplicate persisted client statistics")
		}
		clients[row.ClientIP] = row
		totals.Queries += row.Queries
		totals.Direct += row.Direct
		totals.Surge += row.Surge
		totals.Reject += row.Reject
		totals.Unknown += row.Unknown
		totals.Errors += row.Errors
	}
	if totals.Queries > snapshot.Stats.Queries || totals.Direct > snapshot.Stats.Direct || totals.Surge > snapshot.Stats.Surge || totals.Reject > snapshot.Stats.Reject || totals.Unknown > snapshot.Stats.Unknown || totals.Errors > snapshot.Stats.Errors {
		return fmt.Errorf("persisted client totals do not cover client rows")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients, s.stats, s.evicted = clients, snapshot.Stats, snapshot.EvictedClients
	return nil
}
