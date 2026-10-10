package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"surge-controller/internal/config"
	"surge-controller/internal/resolver"
	"time"
)

type responseCacheSnapshot struct {
	Version int                              `json:"version"`
	Scope   string                           `json:"scope"`
	Pools   [][]resolver.ResponseCacheRecord `json:"pools,omitempty"`
}

func responseCacheScope(d config.Document) string {
	s := d.Settings.DNS
	// Empty lists have the same meaning whether decoded as null or []. Saving
	// an unrelated setting must not discard responses because of that encoding.
	s.Upstreams = append([]resolver.ResolverEndpoint{}, s.Upstreams...)
	s.PolicyUpstreams = append([]config.PolicyUpstream{}, s.PolicyUpstreams...)
	for i := range s.PolicyUpstreams {
		s.PolicyUpstreams[i].Upstreams = append([]resolver.ResolverEndpoint{}, s.PolicyUpstreams[i].Upstreams...)
		s.PolicyUpstreams[i].FallbackUpstreams = append([]resolver.ResolverEndpoint{}, s.PolicyUpstreams[i].FallbackUpstreams...)
	}
	raw, _ := json.Marshal([]any{s.Upstreams, s.PolicyUpstreams, s.PreserveOriginal,
		s.EgressInterface, s.Source4, s.Source6, s.Gateway4, s.Gateway6})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (rt *Runtime) responsePools() []*resolver.Real {
	var pools []*resolver.Real
	if rt.real != nil {
		pools = append(pools, rt.real)
	}
	return append(pools, rt.policyResolvers...)
}

func (rt *Runtime) responseSnapshot() responseCacheSnapshot {
	rt.decisionMu.Lock()
	defer rt.decisionMu.Unlock()
	snapshot := responseCacheSnapshot{Version: 1, Scope: rt.responseScope}
	if rt.settings.Settings.Cache.OptimisticResponses {
		for _, pool := range rt.responsePools() {
			snapshot.Pools = append(snapshot.Pools, pool.ResponseSnapshot())
		}
	}
	return snapshot
}

func (rt *Runtime) restoreResponses(snapshot *responseCacheSnapshot) {
	if snapshot == nil || snapshot.Version != 1 || snapshot.Scope != rt.responseScope || !rt.settings.Settings.Cache.OptimisticResponses {
		return
	}
	pools := rt.responsePools()
	if len(pools) != len(snapshot.Pools) {
		return
	}
	for i, pool := range pools {
		pool.RestoreResponses(snapshot.Pools[i])
	}
}

func (a *App) saveResponseCache() error {
	a.responsePersistMu.Lock()
	defer a.responsePersistMu.Unlock()
	a.mu.RLock()
	rt := a.runtime
	a.mu.RUnlock()
	if rt == nil {
		return nil
	}
	snapshot := rt.responseSnapshot()
	a.mu.RLock()
	current := rt == a.runtime
	a.mu.RUnlock()
	if !current {
		return nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode response cache: %w", err)
	}
	digest := sha256.Sum256(raw)
	if digest == a.savedResponseDigest {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.store.SaveStatistics(ctx, "response_cache", snapshot); err != nil {
		return fmt.Errorf("persist response cache: %w", err)
	}
	a.savedResponseDigest = digest
	return nil
}
