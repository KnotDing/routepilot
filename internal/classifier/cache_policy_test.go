package classifier_test

import (
	"context"
	"fmt"
	"surge-controller/internal/cache"
	"surge-controller/internal/classifier"
	"surge-controller/internal/model"
	"surge-controller/tests/mocks"
	"sync/atomic"
	"testing"
	"time"
)

func TestPerDecisionOptimisticDisablePreservesOrdinaryCaching(t *testing.T) {
	for _, policy := range []string{"DIRECT", "Proxy", "REJECT", ""} {
		t.Run(policy, func(t *testing.T) {
			decision := map[string]model.Decision{"DIRECT": model.Direct, "Proxy": model.Surge, "REJECT": model.Reject, "": model.Unknown}[policy]
			off := false
			cfg := classifier.DefaultConfig()
			cfg.Optimistic = true
			cfg.OptimisticIdleRefresh = false
			cfg.Decisions = map[string]cache.Policy{string(decision): {Optimistic: &off}}
			mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
				return model.RuleMatchResult{FinalPolicy: policy, ContextSafe: true}, nil
			}}
			e, err := classifier.New(context.Background(), mock, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			for i := 0; i < 2; i++ {
				if _, err := e.Classify(context.Background(), query()); err != nil {
					t.Fatal(err)
				}
			}
			if mock.Calls.Load() != 1 || e.Cache.Stats().Entries != 1 {
				t.Fatal("ordinary caching was disabled", e.Cache.Stats(), mock.Calls.Load())
			}
			e.Cache.MarkStale(func(model.DecisionResult) bool { return true })
			if _, err := e.Classify(context.Background(), query()); err != nil {
				t.Fatal(err)
			}
			if mock.Calls.Load() != 2 || e.Cache.Stats().OptimisticHits != 0 {
				t.Fatal("disabled optimistic decision served stale", e.Cache.Stats())
			}
		})
	}
}

func TestDecisionRefreshIntoNonOptimisticDecisionVerifiesOldBucketAndCachesFreshResult(t *testing.T) {
	var phase atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	mock := &mocks.Surge{Match: func(ctx context.Context, _ model.RuleMatchRequest) (model.RuleMatchResult, error) {
		if phase.Load() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return model.RuleMatchResult{}, ctx.Err()
			}
			return model.RuleMatchResult{FinalPolicy: "Proxy", ContextSafe: true}, nil
		}
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	}}
	off := false
	cfg := classifier.DefaultConfig()
	cfg.Optimistic = true
	cfg.Decisions = map[string]cache.Policy{"SURGE": {Optimistic: &off}}
	e, err := classifier.New(context.Background(), mock, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.Classify(context.Background(), query())
	phase.Store(true)
	e.Cache.MarkStale(func(model.DecisionResult) bool { return true })
	for i := 0; i < 4; i++ {
		e.Classify(context.Background(), query())
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("refresh missing")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for e.Cache.Stats().OptimisticCompared < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	s := e.Cache.Stats()
	old := s.Groups["DIRECT"]
	if s.Entries != 1 || s.Groups["SURGE"].Entries != 1 || old.OptimisticHits != 4 || old.OptimisticCompared != 4 || old.OptimisticMismatches != 4 {
		t.Fatal("refresh did not retain ordinary result or lost old requests", s)
	}
	e.Cache.MarkStale(func(d model.DecisionResult) bool { return e.AllowsOptimistic(d.Decision) })
	if e.Cache.Stats().Entries != 0 {
		t.Fatal("non-optimistic refreshed decision retained after invalidation")
	}

}

func TestDisabledDecisionCacheBypassesFreshAndStaleEntries(t *testing.T) {
	for _, global := range []bool{false, true} {
		for _, policy := range []string{"DIRECT", "Proxy", "REJECT", ""} {
			t.Run(fmt.Sprintf("global=%v/%s", global, policy), func(t *testing.T) {
				off := false
				decision := map[string]model.Decision{"DIRECT": model.Direct, "Proxy": model.Surge, "REJECT": model.Reject, "": model.Unknown}[policy]
				cfg := classifier.DefaultConfig()
				cfg.Optimistic = true
				cfg.OptimisticIdleRefresh = false
				if global {
					cfg.CacheEnabled = &off
				} else {
					cfg.Decisions = map[string]cache.Policy{string(decision): {Enabled: &off}}
				}
				mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
					return model.RuleMatchResult{FinalPolicy: policy, ContextSafe: true}, nil
				}}
				e, err := classifier.New(context.Background(), mock, cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer e.Close()
				q := query()
				key := classifier.Key{Domain: "example.com.", ClientGroup: q.ClientIP.String(), QueryClass: "1/1"}
				for _, stale := range []bool{false, true} {
					e.Cache.Put(key, model.DecisionResult{Decision: decision}, e.Cache.Generation(), cache.Forever)
					if stale {
						e.Cache.MarkStale(func(model.DecisionResult) bool { return true })
					}
					before := mock.Calls.Load()
					for i := 0; i < 2; i++ {
						got, err := e.Classify(context.Background(), q)
						if err != nil || got.Decision != decision || got.Source == "cache" {
							t.Fatal("disabled cache changed result", got, err)
						}
					}
					if mock.Calls.Load() != before+2 || e.Cache.Stats().Entries != 0 || e.Cache.Stats().OptimisticHits != 0 || e.AllowsOptimistic(decision) {
						t.Fatal("disabled cache reused or stored result", e.Cache.Stats())
					}
				}
			})
		}
	}
}

func TestDecisionRefreshIntoDisabledCacheStillVerifiesOldRequests(t *testing.T) {
	var changed atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	mock := &mocks.Surge{Match: func(ctx context.Context, _ model.RuleMatchRequest) (model.RuleMatchResult, error) {
		if changed.Load() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return model.RuleMatchResult{}, ctx.Err()
			}
			return model.RuleMatchResult{FinalPolicy: "Proxy", ContextSafe: true}, nil
		}
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	}}
	off := false
	cfg := classifier.DefaultConfig()
	cfg.Optimistic = true
	cfg.OptimisticIdleRefresh = false
	cfg.Decisions = map[string]cache.Policy{"SURGE": {Enabled: &off}}
	e, err := classifier.New(context.Background(), mock, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err = e.Classify(context.Background(), query()); err != nil {
		t.Fatal(err)
	}
	changed.Store(true)
	e.Cache.MarkStale(func(model.DecisionResult) bool { return true })
	for i := 0; i < 4; i++ {
		if _, err = e.Classify(context.Background(), query()); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for e.Cache.Stats().OptimisticCompared < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	s := e.Cache.Stats()
	if s.Entries != 0 || s.Groups["DIRECT"].OptimisticCompared != 4 || s.Groups["DIRECT"].OptimisticMismatches != 4 {
		t.Fatal("disabled new result retained or old requests lost", s)
	}
}
