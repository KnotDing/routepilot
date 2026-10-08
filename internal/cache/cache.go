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
	key        K
	value      V
	generation uint64
	expiry     time.Time
}
type Stats struct {
	Entries                int       `json:"entries"`
	Generation             uint64    `json:"generation"`
	Hits                   uint64    `json:"hits"`
	Misses                 uint64    `json:"misses"`
	Evictions              uint64    `json:"evictions"`
	Invalidations          uint64    `json:"invalidations"`
	LastInvalidationAt     time.Time `json:"last_invalidation_at"`
	LastInvalidationReason string    `json:"last_invalidation_reason"`
}
type Cache[K comparable, V any] struct {
	mu                      sync.Mutex
	items                   map[K]*list.Element
	order                   *list.List
	capacity                int
	generation              atomic.Uint64
	hits, misses, evictions atomic.Uint64
	now                     func() time.Time
	invalidations           uint64
	lastInvalidationAt      time.Time
	lastInvalidationReason  string
}

func New[K comparable, V any](capacity int) *Cache[K, V] {
	if capacity < 1 {
		panic("cache capacity must be positive")
	}
	c := &Cache[K, V]{items: make(map[K]*list.Element), order: list.New(), capacity: capacity, now: time.Now}
	c.generation.Store(1)
	return c
}
func (c *Cache[K, V]) Generation() uint64 { return c.generation.Load() }
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		v := e.Value.(entry[K, V])
		if v.generation == c.Generation() && validExpiry(c.now(), v.expiry) {
			c.order.MoveToFront(e)
			c.hits.Add(1)
			return v.value, true
		}
		c.remove(e)
	}
	c.misses.Add(1)
	var zero V
	return zero, false
}

// Put rejects a result evaluated before an invalidation, including an in-flight miss.
func (c *Cache[K, V]) Put(k K, v V, g uint64, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g != c.Generation() || (ttl <= 0 && ttl != Forever) {
		return false
	}
	if e, ok := c.items[k]; ok {
		c.remove(e)
	}
	var expiry time.Time
	if ttl != Forever {
		expiry = c.now().Add(ttl)
	}
	e := c.order.PushFront(entry[K, V]{k, v, g, expiry})
	c.items[k] = e
	if c.order.Len() > c.capacity {
		c.remove(c.order.Back())
		c.evictions.Add(1)
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
		if v.generation != old || !validExpiry(c.now(), v.expiry) || match(k, v.value) {
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
	n := 0
	now := c.now()
	for _, e := range c.items {
		v := e.Value.(entry[K, V])
		if v.generation == c.Generation() && validExpiry(now, v.expiry) {
			n++
		}
	}
	return Stats{Entries: n, Generation: c.Generation(), Hits: c.hits.Load(), Misses: c.misses.Load(), Evictions: c.evictions.Load(), Invalidations: c.invalidations, LastInvalidationAt: c.lastInvalidationAt, LastInvalidationReason: c.lastInvalidationReason}
}

// Values exposes a bounded snapshot without affecting LRU order or hit counters.
func (c *Cache[K, V]) Values(limit int) []V {
	values, _ := c.Page(0, limit, nil)
	return values
}
func (c *Cache[K, V]) Page(offset, limit int, match func(V) bool) ([]V, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []V{}
	now := c.now()
	total := 0
	for e := c.order.Front(); e != nil; e = e.Next() {
		v := e.Value.(entry[K, V])
		if v.generation == c.Generation() && validExpiry(now, v.expiry) && (match == nil || match(v.value)) {
			if total >= offset && len(out) < limit {
				out = append(out, v.value)
			}
			total++
		}
	}
	return out, total
}
