// Package cache provides a bounded LRU with constant-time generation invalidation.
package cache

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

// Forever preserves an entry until explicit invalidation or capacity eviction.
const Forever time.Duration = -1

func validExpiry(now, expiry time.Time) bool { return expiry.IsZero() || now.Before(expiry) }

type entry[K comparable, V any] struct {
	key                K
	value              V
	generation         uint64
	expiry             time.Time
	stale              bool
	optimisticRequests uint64
}

// Record preserves the key and absolute expiry, in addition to the cached value.
type Record[K comparable, V any] struct {
	Key       K         `json:"key"`
	Value     V         `json:"value"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Stale     bool      `json:"stale,omitempty"`
}
type Stats struct {
	StaleEntries           int       `json:"stale_entries"`
	Entries                int       `json:"entries"`
	Generation             uint64    `json:"generation"`
	Hits                   uint64    `json:"hits"`
	Misses                 uint64    `json:"misses"`
	OptimisticHits         uint64    `json:"optimistic_hits"`
	OptimisticCompared     uint64    `json:"optimistic_compared"`
	OptimisticMismatches   uint64    `json:"optimistic_mismatches"`
	Evictions              uint64    `json:"evictions"`
	Invalidations          uint64    `json:"invalidations"`
	LastInvalidationAt     time.Time `json:"last_invalidation_at"`
	LastInvalidationReason string    `json:"last_invalidation_reason"`
}

// OptimisticStats tracks reliability samples independently from cache entries.
// Callers can persist it across cache recreation and restore it until a user
// explicitly clears the cache.
type OptimisticStats struct {
	Hits       uint64 `json:"optimistic_hits"`
	Compared   uint64 `json:"optimistic_compared"`
	Mismatches uint64 `json:"optimistic_mismatches"`
}
type Options struct {
	// RetainExpired keeps bounded entries past expiry for stale-while-revalidate.
	// Expired entries are reported as stale; their original deadlines are kept.
	RetainExpired bool
}
type Cache[K comparable, V any] struct {
	mu                                                       sync.Mutex
	items                                                    map[K]*list.Element
	order                                                    *list.List
	capacity                                                 int
	generation                                               atomic.Uint64
	hits, misses, evictions                                  atomic.Uint64
	now                                                      func() time.Time
	invalidations                                            uint64
	lastInvalidationAt                                       time.Time
	lastInvalidationReason                                   string
	retainExpired                                            bool
	optimisticHits, optimisticCompared, optimisticMismatches uint64
}

func New[K comparable, V any](capacity int, options ...Options) *Cache[K, V] {
	if capacity < 1 {
		panic("cache capacity must be positive")
	}
	c := &Cache[K, V]{items: make(map[K]*list.Element), order: list.New(), capacity: capacity, now: time.Now}
	if len(options) > 0 {
		c.retainExpired = options[0].RetainExpired
	}
	c.generation.Store(1)
	return c
}
func (c *Cache[K, V]) retained(now, expiry time.Time) bool {
	return c.retainExpired || validExpiry(now, expiry)
}
func (c *Cache[K, V]) Generation() uint64 { return c.generation.Load() }
func (c *Cache[K, V]) Get(k K) (V, bool) {
	v, ok, _, _ := c.GetState(k)
	return v, ok
}

// GetState reads freshness and the generation together with the value.
func (c *Cache[K, V]) GetState(k K) (V, bool, bool, uint64) {
	return c.getState(k, true, false)
}

// GetOptimisticState records each request served a stale value atomically with
// lookup. Requests arriving during the same refresh are counted individually.
func (c *Cache[K, V]) GetOptimisticState(k K) (V, bool, bool, uint64) {
	return c.getState(k, true, true)
}

// PeekState checks a maintenance candidate without affecting LRU or counters.
func (c *Cache[K, V]) PeekState(k K) (V, bool, bool, uint64) {
	return c.getState(k, false, false)
}
func (c *Cache[K, V]) getState(k K, touch, optimistic bool) (V, bool, bool, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		v := e.Value.(entry[K, V])
		now := c.now()
		if v.generation == c.Generation() && c.retained(now, v.expiry) {
			stale := v.stale || !validExpiry(now, v.expiry)
			if optimistic && stale {
				v.optimisticRequests++
				e.Value = v
				c.optimisticHits++
			}
			if touch {
				c.order.MoveToFront(e)
				c.hits.Add(1)
			}
			return v.value, true, stale, v.generation
		}
		c.remove(e)
	}
	if touch {
		c.misses.Add(1)
	}
	var zero V
	return zero, false, false, c.Generation()
}

// StaleRecords returns a bounded maintenance page in LRU order, together with
// its generation. Reading it does not count as a hit or promote any entries.
func (c *Cache[K, V]) StaleRecords(offset, limit int) ([]Record[K, V], uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now, generation := c.now(), c.Generation()
	var out []Record[K, V]
	if offset < 0 || limit < 1 {
		return out, generation
	}
	for e := c.order.Front(); e != nil; e = e.Next() {
		v := e.Value.(entry[K, V])
		if v.generation != generation || !c.retained(now, v.expiry) || !(v.stale || !validExpiry(now, v.expiry)) {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		out = append(out, Record[K, V]{v.key, v.value, v.expiry, true})
		if len(out) == limit {
			break
		}
	}
	return out, generation
}

// Put rejects a result evaluated before an invalidation, including an in-flight miss.
func (c *Cache[K, V]) Put(k K, v V, g uint64, ttl time.Duration) bool {
	return c.PutCompared(k, v, g, ttl, nil)
}

// PutCompared settles outstanding stale requests only for an accepted result
// in the same generation. Failure, idle reads and rejected writes are not samples.
// same must be a fast, pure comparison; it runs under the cache lock.
func (c *Cache[K, V]) PutCompared(k K, v V, g uint64, ttl time.Duration, same func(V, V) bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g != c.Generation() || (ttl <= 0 && ttl != Forever) {
		return false
	}
	if e, ok := c.items[k]; ok {
		c.compare(e, v, same)
		c.remove(e)
	}
	var expiry time.Time
	if ttl != Forever {
		expiry = c.now().Add(ttl)
	}
	e := c.order.PushFront(entry[K, V]{key: k, value: v, generation: g, expiry: expiry})
	c.items[k] = e
	if c.order.Len() > c.capacity {
		c.remove(c.order.Back())
		c.evictions.Add(1)
	}
	return true
}

func (c *Cache[K, V]) compare(e *list.Element, value V, same func(V, V) bool) {
	old := e.Value.(entry[K, V])
	if same != nil && old.generation == c.Generation() && old.optimisticRequests > 0 {
		c.optimisticCompared += old.optimisticRequests
		if !same(old.value, value) {
			c.optimisticMismatches += old.optimisticRequests
		}
	}
}

// Delete rejects a removal requested by an older evaluation generation.
func (c *Cache[K, V]) Delete(k K, generation uint64) bool {
	var zero V
	return c.DeleteCompared(k, zero, generation, nil)
}

// DeleteCompared also verifies a successful refresh whose TTL forbids caching.
func (c *Cache[K, V]) DeleteCompared(k K, value V, generation uint64, same func(V, V) bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.Generation() {
		return false
	}
	if e, ok := c.items[k]; ok {
		c.compare(e, value, same)
		c.remove(e)
	}
	return true
}

func (c *Cache[K, V]) remove(e *list.Element) {
	delete(c.items, e.Value.(entry[K, V]).key)
	c.order.Remove(e)
}
func (c *Cache[K, V]) Flush() uint64 { return c.FlushReason("缓存已清理") }
func (c *Cache[K, V]) invalidate(reason string) uint64 {
	c.invalidations++
	c.lastInvalidationAt = c.now()
	c.lastInvalidationReason = reason
	return c.generation.Add(1)
}
func (c *Cache[K, V]) FlushReason(reason string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[K]*list.Element)
	c.order.Init()
	return c.invalidate(reason)
}

// MarkStale retains stable decisions for optimistic responses while advancing
// the generation, so an evaluation started before this update cannot overwrite them.
func (c *Cache[K, V]) MarkStale(keep func(V) bool) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.Generation()
	generation := c.generation.Add(1)
	for _, e := range c.items {
		v := e.Value.(entry[K, V])
		if v.generation != old || !c.retained(c.now(), v.expiry) || !keep(v.value) {
			c.remove(e)
		} else {
			v.generation, v.stale = generation, true
			e.Value = v
		}
	}
	return generation
}

// A targeted flush also advances the generation to reject in-flight writes.
func (c *Cache[K, V]) FlushWhere(match func(K) bool) {
	c.FlushValuesWhere(func(k K, _ V) bool { return match(k) }, "按域名清理缓存")
}
func (c *Cache[K, V]) FlushValuesWhere(match func(K, V) bool, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.Generation()
	g := c.invalidate(reason)
	for k, e := range c.items {
		v := e.Value.(entry[K, V])
		if v.generation != old || !c.retained(c.now(), v.expiry) || match(k, v.value) {
			c.remove(e)
		} else {
			v.generation = g
			e.Value = v
		}
	}
}
func (c *Cache[K, V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, stale := 0, 0
	now := c.now()
	for _, e := range c.items {
		v := e.Value.(entry[K, V])
		if v.generation == c.Generation() && c.retained(now, v.expiry) {
			n++
			if v.stale || !validExpiry(now, v.expiry) {
				stale++
			}
		}
	}
	return Stats{Entries: n, StaleEntries: stale, Generation: c.Generation(), Hits: c.hits.Load(), Misses: c.misses.Load(), OptimisticHits: c.optimisticHits, OptimisticCompared: c.optimisticCompared, OptimisticMismatches: c.optimisticMismatches, Evictions: c.evictions.Load(), Invalidations: c.invalidations, LastInvalidationAt: c.lastInvalidationAt, LastInvalidationReason: c.lastInvalidationReason}
}

func (c *Cache[K, V]) OptimisticStats() OptimisticStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return OptimisticStats{Hits: c.optimisticHits, Compared: c.optimisticCompared, Mismatches: c.optimisticMismatches}
}

func (c *Cache[K, V]) RestoreOptimisticStats(stats OptimisticStats) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.optimisticHits = stats.Hits
	c.optimisticCompared = stats.Compared
	c.optimisticMismatches = stats.Mismatches
}

func (c *Cache[K, V]) ResetOptimisticStats() {
	c.RestoreOptimisticStats(OptimisticStats{})
}

// Values exposes a bounded snapshot without affecting LRU order or hit counters.
func (c *Cache[K, V]) Values(limit int) []V {
	values, _ := c.Page(0, limit, nil)
	return values
}

// Snapshot returns retained entries from least to most recently used.
func (c *Cache[K, V]) Snapshot() []Record[K, V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Record[K, V], 0, c.order.Len())
	now, generation := c.now(), c.Generation()
	for e := c.order.Back(); e != nil; e = e.Prev() {
		v := e.Value.(entry[K, V])
		if v.generation == generation && c.retained(now, v.expiry) {
			out = append(out, Record[K, V]{v.key, v.value, v.expiry, v.stale || !validExpiry(now, v.expiry)})
		}
	}
	return out
}

// Restore keeps original deadlines and LRU priority. Fresh entries already in
// this process take precedence; capacity reduction drops the oldest records.
func (c *Cache[K, V]) Restore(records []Record[K, V]) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now, generation := c.now(), c.Generation()
	for _, e := range c.items {
		v := e.Value.(entry[K, V])
		if v.generation != generation || !c.retained(now, v.expiry) {
			c.remove(e)
		}
	}
	restored := 0
	for i := len(records) - 1; i >= 0 && c.order.Len() < c.capacity; i-- {
		r := records[i]
		if !c.retained(now, r.ExpiresAt) {
			continue
		}
		if _, exists := c.items[r.Key]; exists {
			continue
		}
		e := c.order.PushBack(entry[K, V]{key: r.Key, value: r.Value, generation: generation, expiry: r.ExpiresAt, stale: r.Stale})
		c.items[r.Key] = e
		restored++
	}
	return restored
}
func (c *Cache[K, V]) Page(offset, limit int, match func(V) bool) ([]V, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []V{}
	now := c.now()
	total := 0
	for e := c.order.Front(); e != nil; e = e.Next() {
		v := e.Value.(entry[K, V])
		if v.generation == c.Generation() && c.retained(now, v.expiry) && (match == nil || match(v.value)) {
			if total >= offset && len(out) < limit {
				out = append(out, v.value)
			}
			total++
		}
	}
	return out, total
}
