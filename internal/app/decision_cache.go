package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"surge-controller/internal/cache"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"surge-controller/internal/surge"
	"time"
)

type decisionRecord = cache.Record[classifier.Key, model.DecisionResult]

type decisionCacheSnapshot struct {
	Version   int              `json:"version"`
	Scope     string           `json:"scope"`
	Digest    string           `json:"digest"`
	Selection string           `json:"selection"`
	Entries   []decisionRecord `json:"entries"`
}

// Only classification inputs belong in this fingerprint. Management addresses,
// credentials, route destinations and UI preferences do not change decisions.
func decisionCacheScope(d config.Document) string {
	c, s := d.Settings.Cache, d.Settings.Surge
	raw, _ := json.Marshal([]any{d.Settings.General.Mode, d.Overrides, d.Hosts,
		s.TrustDirect, s.CLIPath, s.UserName, c.ExpireDecisions,
		c.DirectTTLSeconds, c.SurgeTTLSeconds, c.InvalidateOnReadFailure,
		c.InvalidateDirectOnSelectionChange, c.InvalidateRejectOnSelectionChange})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func retainedDecisions(records []decisionRecord, timed bool) []decisionRecord {
	out := make([]decisionRecord, 0, len(records))
	now := time.Now()
	for _, record := range records {
		key, value := record.Key, record.Value
		if value.Decision != model.Direct && value.Decision != model.Surge && value.Decision != model.Reject {
			continue
		}
		domain, err := model.NormalizeDomain(key.Domain)
		if err != nil || domain != key.Domain || value.Domain != key.Domain ||
			!value.ClientIP.IsValid() || key.ClientGroup != value.ClientIP.String() ||
			key.QueryClass != fmt.Sprintf("%d/1", value.QueryType) || value.QueryType == 0 {
			continue
		}
		if timed {
			if value.Persistent || record.ExpiresAt.IsZero() || !now.Before(record.ExpiresAt) {
				continue
			}
		} else if !value.Persistent || !record.ExpiresAt.IsZero() {
			continue
		}
		out = append(out, record)
	}
	return out
}

// The monitor owns revision updates; the persistence writer uses the same lock
// so a newly observed digest can never be saved alongside pre-invalidation rows.
func (rt *Runtime) observeDecisionRevision(snapshot surge.Snapshot, err error) (string, int) {
	rt.decisionMu.Lock()
	defer rt.decisionMu.Unlock()
	pending := rt.pendingDecisions
	confirmed := err == nil && snapshot.Digest != ""
	if pending != nil && confirmed {
		rt.decisionRevision = decisionRevision{digest: pending.Digest, selection: pending.Selection}
	}
	previousDigest := rt.decisionRevision.digest
	reason, selection := rt.decisionRevision.observe(snapshot.Digest, snapshot.SelectionDigest, err, rt.settings.Settings.Cache)
	c := rt.settings.Settings.Cache
	configurationChanged := confirmed && previousDigest != "" && snapshot.Digest != previousDigest
	optimistic := c.Optimistic && configurationChanged
	optimisticResponses := c.OptimisticResponses && configurationChanged
	selected := func(_ classifier.Key, value model.DecisionResult) bool {
		return value.Decision == model.Direct && c.InvalidateDirectOnSelectionChange || value.Decision == model.Reject && c.InvalidateRejectOnSelectionChange
	}
	if reason != "" {
		if selection {
			rt.engine.Cache.FlushValuesWhere(selected, reason)
		} else {
			if !optimistic {
				rt.engine.Cache.FlushReason(reason)
			}
			if rt.real != nil {
				rt.real.ConfigurationChanged(reason, optimisticResponses)
			}
			for _, upstream := range rt.policyResolvers {
				upstream.ConfigurationChanged(reason, optimisticResponses)
			}
		}
	}
	restored := 0
	if pending != nil && confirmed {
		// Optimistic updates restore saved decisions as stale. Otherwise a full
		// update rejects them; selection updates follow the configured policy.
		if reason == "" || selection || optimistic {
			records := retainedDecisions(pending.Entries, c.ExpireDecisions)
			if selection || !c.Optimistic {
				kept := records[:0]
				for _, record := range records {
					if (!selection || !selected(record.Key, record.Value)) && (c.Optimistic || !record.Stale) {
						kept = append(kept, record)
					}
				}
				records = kept
			}
			restored = rt.engine.Cache.Restore(records)
		}
		rt.pendingDecisions = nil
	} else if reason != "" && !selection {
		// An explicit invalidate-on-read-failure setting also discards startup rows.
		rt.pendingDecisions = nil
	}
	if optimistic {
		rt.engine.Cache.MarkStale(func(value model.DecisionResult) bool {
			return value.Decision == model.Direct || value.Decision == model.Surge || value.Decision == model.Reject
		})
		reason += "；乐观缓存保留旧决策，按需后台刷新"
	}
	if optimisticResponses {
		reason += "；乐观响应缓存保留旧响应，按需后台刷新"
	}
	return reason, restored
}

func (rt *Runtime) decisionSnapshot() decisionCacheSnapshot {
	rt.decisionMu.Lock()
	defer rt.decisionMu.Unlock()
	c := rt.settings.Settings.Cache
	if pending := rt.pendingDecisions; pending != nil {
		// Keep the saved snapshot while Surge is unavailable; never serve these rows
		// until a current revision is confirmed, nor overwrite them with an empty cache.
		snapshot := *pending
		snapshot.Entries = retainedDecisions(pending.Entries, c.ExpireDecisions)
		return snapshot
	}
	snapshot := decisionCacheSnapshot{Version: 2, Scope: rt.decisionScope,
		Digest: rt.decisionRevision.digest, Selection: rt.decisionRevision.selection}
	if snapshot.Digest != "" {
		snapshot.Entries = retainedDecisions(rt.engine.Cache.Snapshot(), c.ExpireDecisions)
	}
	return snapshot
}

func (a *App) saveDecisionCache() error {
	// Snapshot after taking the writer lock: an older periodic write must not
	// overwrite a successful manual flush or configuration invalidation.
	a.cachePersistMu.Lock()
	defer a.cachePersistMu.Unlock()
	a.mu.RLock()
	rt := a.runtime
	if rt == nil {
		a.mu.RUnlock()
		return nil
	}
	snapshot := rt.decisionSnapshot()
	a.mu.RUnlock()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode decision cache: %w", err)
	}
	digest := sha256.Sum256(raw)
	if digest == a.savedCacheDigest {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = a.store.SaveDecisionCache(ctx, raw); err != nil {
		return fmt.Errorf("persist decision cache: %w", err)
	}
	a.savedCacheDigest = digest
	return nil
}
