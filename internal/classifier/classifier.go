package classifier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
	"surge-controller/internal/cache"
	"surge-controller/internal/model"
)

var ErrBusy = errors.New("rule evaluation capacity exhausted")

type Key struct{ Domain, ClientGroup, QueryClass string }
type Config struct {
	Capacity, Workers, QueueSize                       int
	Timeout, DirectTTL, SurgeTTL, UnknownTTL, ErrorTTL time.Duration
	TrustDirect                                        bool
	ExpireDecisions                                    bool
	Optimistic                                         bool
}

func DefaultConfig() Config {
	return Config{Capacity: 10000, Workers: 8, QueueSize: 256, Timeout: 500 * time.Millisecond, DirectTTL: 5 * time.Minute, SurgeTTL: 5 * time.Minute, UnknownTTL: 10 * time.Second, ErrorTTL: time.Second}
}

type job struct {
	ctx    context.Context
	query  model.DNSQueryContext
	result chan outcome
}
type outcome struct {
	decision model.DecisionResult
	err      error
}
type Engine struct {
	RuleQueries, RuleErrors, RuleDurationNS atomic.Uint64
	adapter                                 model.SurgeAdapter
	config                                  Config
	Cache                                   *cache.Cache[Key, model.DecisionResult]
	flight                                  singleflight.Group
	jobs                                    chan job
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	workers                                 sync.WaitGroup
	refreshMu                               sync.Mutex
	refreshing                              map[Key]uint64
	closed                                  bool
	refreshWorkers                          sync.WaitGroup
}

func New(parent context.Context, a model.SurgeAdapter, c Config) (*Engine, error) {
	if c.Capacity < 1 || c.Workers < 1 || c.Workers > 256 || c.QueueSize < 1 || c.QueueSize > 65536 || c.Timeout <= 0 {
		return nil, errors.New("invalid classifier limits")
	}
	for _, ttl := range []time.Duration{c.DirectTTL, c.SurgeTTL, c.UnknownTTL, c.ErrorTTL} {
		if ttl < 0 {
			return nil, errors.New("negative decision TTL")
		}
	}
	ctx, cancel := context.WithCancel(parent)
	e := &Engine{adapter: a, config: c, Cache: cache.New[Key, model.DecisionResult](c.Capacity), jobs: make(chan job, c.QueueSize), ctx: ctx, cancel: cancel, refreshing: make(map[Key]uint64)}
	for i := 0; i < c.Workers; i++ {
		e.workers.Add(1)
		go e.worker()
	}
	return e, nil
}
func (e *Engine) Close() {
	e.refreshMu.Lock()
	e.closed = true
	e.cancel()
	e.refreshMu.Unlock()
	e.refreshWorkers.Wait()
	e.workers.Wait()
}
func (e *Engine) worker() {
	defer e.workers.Done()
	for {
		select {
		case <-e.ctx.Done():
			return
		case j := <-e.jobs:
			var out outcome
			if j.ctx.Err() != nil {
				out = outcome{e.unknown(j.query, "evaluation deadline exceeded"), j.ctx.Err()}
			} else {
				started := time.Now()
				e.RuleQueries.Add(1)
				match, err := e.adapter.ExplainRule(j.ctx, model.RuleMatchRequest{Domain: j.query.Question.Name, ClientIP: j.query.ClientIP})
				e.RuleDurationNS.Add(uint64(time.Since(started)))
				if err != nil {
					e.RuleErrors.Add(1)
				}
				d := e.unknown(j.query, "insufficient connection context")
				d.RuleNotes = append([]string(nil), match.Notes...)
				d.RuleDNS = match.RuleDNS
				if err != nil {
					d.Reason = "Surge evaluation failed"
					if match.RuleDNS != nil && match.RuleDNS.Reason != "" {
						d.Reason = match.RuleDNS.Reason
					}
				} else {
					d.MatchedRule = match.MatchedRule
					d.Policy = match.Policy
					d.FinalPolicy = match.FinalPolicy
					d.Reason = match.SafetyReason
					switch {
					case match.FinalPolicy == "DIRECT" && match.ContextSafe && len(match.UnderlyingChain) == 0:
						d.Decision = model.Direct
						d.Confidence = model.Certain
					case e.config.TrustDirect && match.FinalPolicy == "DIRECT" && strings.EqualFold(match.FinalType, "direct") && len(match.UnderlyingChain) == 0:
						d.Decision = model.Direct
						d.Confidence = model.Configured
						d.Reason = "按配置采用 Surge DIRECT 结果，跳过连接上下文保守判断；原始原因：" + match.SafetyReason
					case strings.HasPrefix(match.FinalPolicy, "REJECT") && match.ContextSafe:
						d.Decision = model.Reject
						d.Confidence = model.Certain
					case match.FinalPolicy != "" && match.FinalPolicy != "DIRECT":
						d.Decision = model.Surge
					}
				}
				out = outcome{d, err}
			}
			j.result <- out
		}
	}
}
func (e *Engine) unknown(q model.DNSQueryContext, reason string) model.DecisionResult {
	return model.DecisionResult{QueryType: q.Question.Qtype, Decision: model.Unknown, Confidence: model.Uncertain, Domain: q.Question.Name, ClientIP: q.ClientIP, Source: "surge", Reason: reason, CreatedAt: time.Now()}
}
func (e *Engine) Classify(ctx context.Context, q model.DNSQueryContext) (model.DecisionResult, error) {
	domain, err := model.NormalizeDomain(q.Question.Name)
	if err != nil {
		return e.unknown(q, "invalid domain"), err
	}
	q.Question.Name = domain
	// Per-client isolation is the safe default for Surge source-address rules.
	key := Key{domain, q.ClientIP.String(), fmt.Sprint(q.Question.Qtype, "/", q.Question.Qclass)}
	d, ok, stale, gen := e.Cache.GetState(key)
	if ok && (!stale || e.config.Optimistic) {
		if stale {
			e.refresh(q, key, gen)
		}
		d.Source = "cache"
		return d, nil
	}
	ch := e.evaluate(q, key, gen, false)
	select {
	case <-ctx.Done():
		return e.unknown(q, "request canceled"), ctx.Err()
	case r := <-ch:
		if r.Err != nil {
			return e.unknown(q, "evaluation failure"), r.Err
		}
		out := r.Val.(outcome)
		if gen != e.Cache.Generation() {
			return e.unknown(q, "Surge generation changed during evaluation"), nil
		}
		return out.decision, out.err
	}
}

// Refresh only queried stale entries, with one task per key and bounded concurrency.
// Failed or uncertain evaluations retain the old decision and retry on a later hit.
func (e *Engine) refresh(q model.DNSQueryContext, key Key, gen uint64) {
	e.refreshMu.Lock()
	if _, running := e.refreshing[key]; e.closed || running || len(e.refreshing) >= e.config.Workers {
		e.refreshMu.Unlock()
		return
	}
	e.refreshing[key] = gen
	e.refreshWorkers.Add(1)
	e.refreshMu.Unlock()
	go func() {
		defer e.refreshWorkers.Done()
		defer func() { e.refreshMu.Lock(); delete(e.refreshing, key); e.refreshMu.Unlock() }()
		ch := e.evaluate(q, key, gen, true)
		select {
		case <-e.ctx.Done():
			return
		case r := <-ch:
			out, _ := r.Val.(outcome)
			if r.Err == nil && out.err == nil && out.decision.Decision != model.Unknown {
				return
			}
		}
		// A short backoff also bounds retries while Surge is unavailable.
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-e.ctx.Done():
		case <-timer.C:
		}
	}()
}

func (e *Engine) evaluate(q model.DNSQueryContext, key Key, gen uint64, refresh bool) <-chan singleflight.Result {
	flightKey := fmt.Sprintf("%d|%s|%s|%s", gen, key.Domain, key.ClientGroup, key.QueryClass)
	return e.flight.DoChan(flightKey, func() (any, error) {
		evaluation, cancel := context.WithTimeout(e.ctx, e.config.Timeout)
		defer cancel()
		j := job{evaluation, q, make(chan outcome, 1)}
		select {
		case <-evaluation.Done():
			return outcome{e.unknown(q, "evaluation stopped"), evaluation.Err()}, nil
		case e.jobs <- j:
		default:
			return outcome{e.unknown(q, "evaluation queue full"), ErrBusy}, nil
		}
		var out outcome
		select {
		case out = <-j.result:
		case <-evaluation.Done():
			out = outcome{e.unknown(q, "evaluation timed out"), evaluation.Err()}
		}
		ttl := e.config.UnknownTTL
		switch out.decision.Decision {
		case model.Direct:
			ttl = e.config.DirectTTL
		case model.Surge, model.Reject:
			ttl = e.config.SurgeTTL
		}
		if out.err != nil {
			ttl = e.config.ErrorTTL
		}
		if out.err == nil && out.decision.Decision != model.Unknown && !e.config.ExpireDecisions {
			ttl = cache.Forever
			out.decision.Persistent = true
		} else {
			out.decision.ExpiresAt = out.decision.CreatedAt.Add(ttl)
		}
		if !refresh || out.err == nil && out.decision.Decision != model.Unknown {
			e.Cache.Put(key, out.decision, gen, ttl)
		}
		return out, nil
	})
}

func (e *Engine) FlushDomain(domain string) error {
	d, err := model.NormalizeDomain(domain)
	if err != nil {
		return err
	}
	e.Cache.FlushWhere(func(k Key) bool { return k.Domain == d })
	return nil
}
