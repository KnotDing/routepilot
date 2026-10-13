package cache

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkCacheStatisticsCost(b *testing.B) {
	for _, capacity := range []int{10000, 100000, 1000000} {
		for _, full := range []bool{false, true} {
			mode := "optimistic-counters"
			if full {
				mode = "full-scan"
			}
			b.Run(fmt.Sprintf("%d/%s", capacity, mode), func(b *testing.B) {
				c := New[int, int](capacity)
				for i := 0; i < capacity; i++ {
					c.Put(i, i, c.Generation(), time.Hour)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if full {
						c.Stats()
					} else {
						c.OptimisticCounters()
					}
				}
			})
		}
	}
}
