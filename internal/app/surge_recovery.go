package app

import (
	"context"
	"math"
	"surge-controller/internal/config"
	"surge-controller/internal/surge"
	"time"
)

type recoveryTarget struct{ name, policy string }

func appendTargets(s config.SurgeRestart) []recoveryTarget {
	var result []recoveryTarget
	for _, name := range s.Groups {
		result = append(result, recoveryTarget{"Surge group " + name, name})
	}
	for _, name := range s.Nodes {
		result = append(result, recoveryTarget{"Surge node " + name, name})
	}
	return result
}
func (a *App) observeSurgeRecovery(ctx context.Context, rt *Runtime, s config.SurgeRestart, raw map[string]bool) {
	if ctx.Err() != nil {
		return
	}
	checks := map[string]bool{}
	if s.API {
		checks["Surge API"] = raw["Surge API"]
	}
	if s.DNS {
		checks["Surge DNS"] = raw["Surge DNS"]
	}
	for _, target := range appendTargets(s) {
		checks[target.name] = raw[target.name]
	}
	reason := a.surgeRecovery.Take(time.Now(), rt.started, s, checks)
	if reason == "" || ctx.Err() != nil {
		return
	}
	a.event("WARN", "surge_restart", "Surge 持续异常，正在重启引擎："+reason)
	go func() {
		restartCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		err := surge.RestartEngine(restartCtx, rt.adapter.Runner)
		a.surgeRecovery.Complete(err)
		if err != nil {
			a.event("ERROR", "surge_restart", "Surge 引擎重启失败："+err.Error())
		} else {
			a.event("INFO", "surge_restart", "Surge 引擎重启请求已完成，等待健康检查恢复")
		}
	}()
}

// Actual elapsed time avoids exaggerated QPS after sleep. Lifetime counters may
// be restored before startup; the initial baseline never counts persisted data.
func queryRate(current, previous uint64, elapsed time.Duration) float64 {
	if current < previous || elapsed <= 0 {
		return 0
	}
	return float64(current-previous) / elapsed.Seconds()
}
func (a *App) sampleQueryRate(ctx context.Context) {
	defer close(a.queryRateDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	previous, at := a.dnsCounters.Snapshot().Queries, time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			current := a.dnsCounters.Snapshot().Queries
			a.queryRateBits.Store(math.Float64bits(queryRate(current, previous, now.Sub(at))))
			previous, at = current, now
		}
	}
}
