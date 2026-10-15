package cache

// Policy omits overrides by default to preserve existing cache behavior.
type Policy struct {
	Enabled    *bool `json:"enabled,omitempty"`
	Optimistic *bool `json:"optimistic,omitempty"`
}

func (p Policy) CacheEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}
func (p Policy) OptimisticEnabled() bool {
	return p.CacheEnabled() && (p.Optimistic == nil || *p.Optimistic)
}
func ClonePolicies(policies map[string]Policy) map[string]Policy {
	if policies == nil {
		return nil
	}
	out := make(map[string]Policy, len(policies))
	for key, p := range policies {
		if p.Enabled != nil {
			v := *p.Enabled
			p.Enabled = &v
		}
		if p.Optimistic != nil {
			v := *p.Optimistic
			p.Optimistic = &v
		}
		out[key] = p
	}
	return out
}

type ReliabilityStats struct {
	Hits       uint64 `json:"optimistic_hits"`
	Compared   uint64 `json:"optimistic_compared"`
	Mismatches uint64 `json:"optimistic_mismatches"`
}
type GroupStats struct {
	Entries              int    `json:"entries"`
	StaleEntries         int    `json:"stale_entries"`
	Hits                 uint64 `json:"hits"`
	Misses               uint64 `json:"misses"`
	Evictions            uint64 `json:"evictions"`
	OptimisticHits       uint64 `json:"optimistic_hits"`
	OptimisticCompared   uint64 `json:"optimistic_compared"`
	OptimisticMismatches uint64 `json:"optimistic_mismatches"`
}

func (s *GroupStats) Add(next GroupStats) {
	s.Entries += next.Entries
	s.StaleEntries += next.StaleEntries
	s.Hits += next.Hits
	s.Misses += next.Misses
	s.Evictions += next.Evictions
	s.OptimisticHits += next.OptimisticHits
	s.OptimisticCompared += next.OptimisticCompared
	s.OptimisticMismatches += next.OptimisticMismatches
}
func (s *OptimisticStats) Add(next OptimisticStats) {
	s.Hits += next.Hits
	s.Compared += next.Compared
	s.Mismatches += next.Mismatches
	if len(next.ByGroup) > 0 && s.ByGroup == nil {
		s.ByGroup = make(map[string]ReliabilityStats)
	}
	for key, v := range next.ByGroup {
		key = boundedGroupKey(s.ByGroup, key)
		old := s.ByGroup[key]
		old.Hits += v.Hits
		old.Compared += v.Compared
		old.Mismatches += v.Mismatches
		s.ByGroup[key] = old
	}
}

// ConfigureGroups is called before a cache is exposed to requests. Grouping
// attributes verification to the old value that was actually served.
func (c *Cache[K, V]) ConfigureGroups(group func(K, V) string, retainExpired func(V) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.group = group
	c.retainValue = retainExpired
	c.groups = make(map[string]GroupStats)
}
func (c *Cache[K, V]) groupHit(k K, v V, stale bool) {
	if c.group == nil {
		return
	}
	key := c.groupName(k, v)
	s := c.groups[key]
	s.Hits++
	if stale {
		s.OptimisticHits++
	}
	c.groups[key] = s
}
func (c *Cache[K, V]) groupCompare(k K, v V, count uint64, mismatch bool) {
	if c.group == nil {
		return
	}
	key := c.groupName(k, v)
	s := c.groups[key]
	s.OptimisticCompared += count
	if mismatch {
		s.OptimisticMismatches += count
	}
	c.groups[key] = s
}

// GetAllowedState checks selective cache policy and records accepted requests
// atomically, so disabled stale values never inflate reliability counters.
func (c *Cache[K, V]) GetAllowedState(k K, optimistic bool, allow func(V, bool) bool) (V, bool, bool, uint64) {
	return c.getState(k, true, optimistic, allow)
}

func (s OptimisticStats) Clone() OptimisticStats {
	if s.ByGroup != nil {
		groups := make(map[string]ReliabilityStats, len(s.ByGroup))
		for k, v := range s.ByGroup {
			groups[k] = v
		}
		s.ByGroup = groups
	}
	return s
}
func (c *Cache[K, V]) groupName(k K, v V) string {
	key := c.group(k, v)
	if _, ok := c.groups[key]; !ok {
		key = boundedGroupKey(c.groups, key)
		c.groups[key] = GroupStats{}
	}
	return key
}

// RecordMissGroup attributes a completed foreground lookup to its actual
// result. Failed/cancelled lookups without a known result stay aggregate-only.
func (c *Cache[K, V]) RecordMissGroup(k K, v V, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.group == nil || generation != c.Generation() {
		return
	}
	key := c.groupName(k, v)
	s := c.groups[key]
	s.Misses++
	c.groups[key] = s
}
func (c *Cache[K, V]) groupEviction(v entry[K, V]) {
	if c.group == nil {
		return
	}
	key := c.groupName(v.key, v.value)
	s := c.groups[key]
	s.Evictions++
	c.groups[key] = s
}

func boundedGroupKey[T any](groups map[string]T, key string) string {
	if key == "other" {
		return key
	}
	count := len(groups)
	if _, ok := groups["other"]; ok {
		count--
	}
	if _, ok := groups[key]; !ok && count >= 1024 {
		return "other"
	}
	return key
}
func AddGroupStats(groups map[string]GroupStats, key string, stats GroupStats) {
	key = boundedGroupKey(groups, key)
	old := groups[key]
	old.Add(stats)
	groups[key] = old
}
