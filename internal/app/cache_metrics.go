package app

import (
	"fmt"
	"sort"
	"strings"
	"surge-controller/internal/cache"
)

func writeCacheGroupMetrics(b *strings.Builder, prefix, label string, groups map[string]cache.GroupStats) {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		s := groups[key]
		for _, metric := range []struct {
			name  string
			value uint64
		}{
			{"entries", uint64(s.Entries)}, {"stale_entries", uint64(s.StaleEntries)}, {"hits_total", s.Hits}, {"misses_total", s.Misses}, {"evictions_total", s.Evictions},
			{"optimistic_hits_total", s.OptimisticHits}, {"optimistic_compared_total", s.OptimisticCompared}, {"optimistic_mismatches_total", s.OptimisticMismatches},
		} {
			fmt.Fprintf(b, "%s_by_%s_%s{%s=%q} %d\n", prefix, label, metric.name, label, key, metric.value)
		}
	}
}
