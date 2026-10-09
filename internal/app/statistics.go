package app

import (
	"context"
	"fmt"
	"log/slog"
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

func (a *App) persistStatistics(ctx context.Context, last dnsserver.CounterSnapshot) {
	defer close(a.statsDone)
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
			if err := a.saveDecisionCache(); err != nil {
				slog.Error("Decision cache persistence failed", "error", err)
			}
		}
	}
}
