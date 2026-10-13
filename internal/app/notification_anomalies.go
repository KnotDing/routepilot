package app

import (
	"strings"
	"time"

	"surge-controller/internal/cache"
	"surge-controller/internal/notifications"
	"surge-controller/internal/resolver"
)

func (a *App) observeNotificationAnomalies() {
	a.notificationSampleMu.Lock()
	defer a.notificationSampleMu.Unlock()
	a.mu.RLock()
	rt := a.runtime
	if rt == nil {
		a.mu.RUnlock()
		return
	}
	document := a.applied
	s := document.Settings
	response := responseOptimisticCounters(rt)
	sample := notifications.AnomalySample{At: time.Now(), Queries: a.dnsCounters.Snapshot().Queries, Decision: rt.engine.Cache.OptimisticCounters(), Response: response, DecisionEnabled: s.Cache.Optimistic, ResponseEnabled: s.Cache.OptimisticResponses}
	values := rt.health.Values()
	a.mu.RUnlock()
	activePeers := map[string]bool{}
	for _, peer := range document.Peers {
		if peer.Enabled && s.Routing.UsesBGP() && s.BGP.Enabled {
			activePeers["BGP peer "+peer.Address] = true
		}
	}
	filtered := values[:0]
	for _, value := range values {
		if strings.HasPrefix(value.Name, "BGP peer ") && !activePeers[value.Name] {
			continue
		}
		filtered = append(filtered, value)
	}
	alerts := a.notificationAnomalies.Observe(s.Notifications, sample)
	if a.notifications != nil {
		a.notifications.Observe(s.Notifications, append(filtered, alerts...))
	}
}

// Sampling needs cumulative reliability counters, never a scan of cache entries.
// Include persisted totals carried across runtime rebuilds and all policy pools.
func responseOptimisticCounters(rt *Runtime) cache.OptimisticStats {
	rt.decisionMu.Lock()
	saved := rt.responseOptimisticStats
	result := cache.OptimisticStats{Hits: saved.Hits, Compared: saved.Compared, Mismatches: saved.Mismatches}
	rt.decisionMu.Unlock()
	add := func(pool *resolver.Real) {
		if pool == nil {
			return
		}
		stats := pool.Cache.OptimisticCounters()
		result.Hits += stats.Hits
		result.Compared += stats.Compared
		result.Mismatches += stats.Mismatches
	}
	add(rt.real)
	add(rt.surgeResponses)
	for _, pool := range rt.policyResolvers {
		add(pool)
	}
	return result
}
