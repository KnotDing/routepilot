package app

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"surge-controller/internal/dnsserver"
	"time"
)

const statisticsInterval = 5 * time.Second

func (a *App) saveDNSStatistics(snapshot dnsserver.CounterSnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.store.SaveStatistics(ctx, "dns", snapshot); err != nil {
		return fmt.Errorf("persist DNS statistics: %w", err)
	}
	return nil
}

func (a *App) saveDNSMonitoring(snapshot dnsserver.DNSMonitoringSnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.store.SaveStatistics(ctx, "dns_monitoring", snapshot); err != nil {
		return fmt.Errorf("persist DNS monitoring: %w", err)
	}
	return nil
}

func sortedMonitoringKeys(values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (a *App) persistStatistics(ctx context.Context, last dnsserver.CounterSnapshot) {
	defer close(a.statsDone)
	ticker := time.NewTicker(statisticsInterval)
	defer ticker.Stop()
	var lastMonitoring dnsserver.DNSMonitoringSnapshot
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := a.dnsCounters.Snapshot()
			if snapshot != last {
				if err := a.saveDNSStatistics(snapshot); err != nil {
					slog.Error("DNS statistics persistence failed", "error", err)
				} else {
					last = snapshot
				}
			}
			monitoring := a.dnsMonitoring.Snapshot()
			if monitoring.Received != lastMonitoring.Received || monitoring.Completed != lastMonitoring.Completed || monitoring.StartedAt != lastMonitoring.StartedAt {
				if err := a.saveDNSMonitoring(monitoring); err != nil {
					slog.Error("DNS monitoring persistence failed", "error", err)
				} else {
					lastMonitoring = monitoring
				}
			}
			if err := a.saveDecisionCache(); err != nil {
				slog.Error("Decision cache persistence failed", "error", err)
			}
			if err := a.saveResponseCache(); err != nil {
				slog.Error("Response cache persistence failed", "error", err)
			}
		}
	}
}
