package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"surge-controller/internal/database"
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

func (a *App) saveClientStatistics() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.store.SaveStatistics(ctx, "dns_clients", a.clientStatistics.Snapshot()); err != nil {
		message := err.Error()
		a.clientPersistError.Store(&message)
		return fmt.Errorf("persist client statistics: %w", err)
	}
	a.clientPersistError.Store(nil)
	return nil
}

func (a *App) saveDNSMonitoring(snapshot dnsserver.DNSMonitoringSnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.store.SaveDNSMonitoring(ctx, snapshot); err != nil {
		message := err.Error()
		a.monitorPersistError.Store(&message)
		return fmt.Errorf("persist DNS monitoring: %w", err)
	}
	a.monitorPersistError.Store(nil)
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
	var lastClients uint64
	ticker := time.NewTicker(statisticsInterval)
	defer ticker.Stop()
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
			clientQueries := a.clientStatistics.Snapshot().Stats.Queries
			if clientQueries != lastClients || a.clientPersistError.Load() != nil {
				if err := a.saveClientStatistics(); err != nil {
					slog.Error("Client statistics persistence failed", "error", err)
				} else {
					lastClients = clientQueries
				}
			}
			monitoring := a.dnsMonitoring.Snapshot()
			if err := a.saveDNSMonitoring(monitoring); err != nil {
				slog.Error("DNS monitoring persistence failed", "error", err)
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

func (a *App) monitoringHistoryStatus() map[string]any {
	message := ""
	if e := a.monitorPersistError.Load(); e != nil {
		message = *e
	}
	return map[string]any{"supported": true, "retention_days": database.MonitoringRetentionDays, "resolution_seconds": database.MonitoringResolutionSeconds, "storage_error": message}
}

func (a *App) DNSMonitoringView(_ context.Context, source string) (dnsserver.DNSMonitoringSnapshot, error) {
	if source == "" {
		return a.dnsMonitoring.View(), nil
	}
	snapshot, ok := a.dnsMonitoring.SourceSnapshot(source)
	if !ok {
		return snapshot, errors.New("monitoring source is unavailable")
	}
	return snapshot, nil
}

// Optional capability: older connected backends retain source-only monitoring.
func (a *App) DNSMonitoringFilteredView(_ context.Context, filter dnsserver.MonitoringFilter) (dnsserver.DNSMonitoringSnapshot, error) {
	full := a.dnsMonitoring.Snapshot()
	view := full
	if filter.Source != "" {
		found := false
		for _, source := range full.Sources {
			if source.Source == filter.Source {
				view = source.Counts
				view.StartedAt, view.ObservedAt, view.SessionStartedAt = full.StartedAt, full.ObservedAt, full.SessionStartedAt
				view.SourcePolicyFlows = dnsserver.SourcePolicyFlows([]dnsserver.DNSSourceCount{source})
				found = true
				break
			}
		}
		if !found {
			return dnsserver.EmptyMonitoringCounts(), errors.New("monitoring source is unavailable")
		}
	}
	if filter.HasDimensions() {
		view = dnsserver.FilterMonitoringCounts(full, filter)
	}
	options := dnsserver.ObservationOptions(full.Observations)
	view.FilterOptions = &options
	view.DimensionsSupported = true
	view.DimensionsStartedAt = full.DimensionsStartedAt
	dnsserver.AttachMonitoringDomains(&view, full.Observations, filter)
	view.Observations = nil
	for i := range view.Sources {
		view.Sources[i].Counts.Policies = nil
	}
	return view, nil
}
