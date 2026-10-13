package cache

import (
	"reflect"
	"testing"
	"time"
)

func TestPermanentDecisionsStillObeyCapacityAndGeneration(t *testing.T) {
	c := New[string, string](2)
	now := time.Unix(1, 0)
	c.now = func() time.Time { return now }
	g := c.Generation()
	c.Put("direct", "DIRECT", g, Forever)
	c.Put("reject", "REJECT", g, Forever)
	now = now.AddDate(20, 0, 0)
	if _, ok := c.Get("direct"); !ok {
		t.Fatal("stable decision expired with time")
	}
	c.FlushValuesWhere(func(_ string, v string) bool { return v == "REJECT" }, "selection")
	if _, ok := c.Get("direct"); !ok {
		t.Fatal("selective invalidation dropped permanent DIRECT")
	}
	if _, ok := c.Get("reject"); ok {
		t.Fatal("permanent REJECT escaped invalidation")
	}
	if c.Put("stale", "DIRECT", g, Forever) {
		t.Fatal("old permanent in-flight decision accepted")
	}
	g = c.Generation()
	c.Put("surge", "SURGE", g, Forever)
	c.Put("new", "SURGE", g, Forever)
	if _, ok := c.Get("direct"); ok {
		t.Fatal("permanent cache bypassed capacity eviction")
	}
	c.Flush()
	if c.Stats().Entries != 0 {
		t.Fatal("permanent entries survived manual flush")
	}
}

func TestCacheLifecycle(t *testing.T) {
	c := New[string, int](2)
	g := c.Generation()
	now := time.Unix(1, 0)
	c.now = func() time.Time { return now }
	if _, ok := c.Get("a"); ok {
		t.Fatal("unexpected hit")
	}
	c.Put("a", 1, g, time.Minute)
	c.Put("b", 2, g, time.Minute)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatal("missing cache value")
	}
	c.Put("c", 3, g, time.Minute)
	if _, ok := c.Get("b"); ok {
		t.Fatal("LRU did not evict")
	}
	c.FlushWhere(func(k string) bool { return k == "a" })
	if _, ok := c.Get("a"); ok {
		t.Fatal("domain flush failed")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("targeted flush dropped another domain")
	}
	if c.Put("a", 4, g, time.Minute) {
		t.Fatal("in-flight result from old generation accepted")
	}
	c.Flush()
	if _, ok := c.Get("c"); ok {
		t.Fatal("generation invalidation failed")
	}
	g = c.Generation()
	c.Put("d", 4, g, time.Second)
	now = now.Add(time.Second)
	if _, ok := c.Get("d"); ok {
		t.Fatal("expired value accepted")
	}
	if c.Stats().Entries != 0 || c.Stats().Evictions == 0 {
		t.Fatal("cache stats inaccurate")
	}
}
func BenchmarkCacheHit(b *testing.B) {
	c := New[string, int](10000)
	c.Put("example.com|client|A", 1, c.Generation(), time.Hour)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, ok := c.Get("example.com|client|A"); !ok {
				b.Fatal("miss")
			}
		}
	})
}

func TestSelectiveInvalidationPreservesValidProxyDecisions(t *testing.T) {
	c := New[string, string](8)
	g := c.Generation()
	c.Put("old", "SURGE", g, time.Minute)
	c.Flush()
	g = c.Generation()
	c.Put("proxy", "SURGE", g, time.Minute)
	c.Put("direct", "DIRECT", g, time.Minute)
	c.FlushValuesWhere(func(_ string, v string) bool { return v == "DIRECT" }, "selection changed")
	if _, ok := c.Get("direct"); ok {
		t.Fatal("old DIRECT survived")
	}
	if _, ok := c.Get("proxy"); !ok {
		t.Fatal("proxy decision was unnecessarily discarded")
	}
	if _, ok := c.Get("old"); ok {
		t.Fatal("previously invalidated entry was resurrected")
	}
	if c.Put("flight", "DIRECT", g, time.Minute) {
		t.Fatal("in-flight DIRECT from previous selection accepted")
	}
	if c.Stats().LastInvalidationReason != "selection changed" {
		t.Fatal("missing invalidation explanation")
	}
}
func TestCachePageSearchesBeyondFirstPageAndSkipsExpired(t *testing.T) {
	c := New[int, int](1200)
	g := c.Generation()
	for i := 0; i < 1100; i++ {
		c.Put(i, i, g, time.Minute)
	}
	values, total := c.Page(1000, 100, nil)
	if len(values) != 100 || total != 1100 || values[0] != 99 {
		t.Fatal("incorrect pagination", len(values), total)
	}
	values, total = c.Page(0, 50, func(v int) bool { return v == 0 })
	if len(values) != 1 || total != 1 || values[0] != 0 {
		t.Fatal("search limited to visible page")
	}
}

func TestMarkStaleRetainsDecisionsAndRejectsOldWrites(t *testing.T) {
	c := New[string, string](4)
	now := time.Unix(1, 0)
	c.now = func() time.Time { return now }
	gen := c.Generation()
	c.Put("stable", "DIRECT", gen, time.Minute)
	c.Put("unknown", "UNKNOWN", gen, time.Minute)
	c.MarkStale(func(v string) bool { return v != "UNKNOWN" })
	_, ok, stale, current := c.GetState("stable")
	if !ok || !stale || current == gen || c.Stats().Invalidations != 0 || c.Stats().StaleEntries != 1 {
		t.Fatal("optimistic update discarded the entry or reported a flush")
	}
	if c.Put("stable", "SURGE", gen, Forever) {
		t.Fatal("old evaluation overwrote a stale decision")
	}
	if _, ok := c.Get("unknown"); ok {
		t.Fatal("UNKNOWN retained as an optimistic decision")
	}
	saved := c.Snapshot()
	restored := New[string, string](4)
	restored.now = c.now
	restored.Restore(saved)
	if _, ok, stale, _ := restored.GetState("stable"); !ok || !stale {
		t.Fatal("restoration lost pending refresh")
	}
	c.Put("stable", "REJECT", current, time.Minute)
	if _, _, stale, _ := c.GetState("stable"); stale {
		t.Fatal("successful replacement remains stale")
	}
	now = now.Add(time.Minute)
	if _, ok := c.Get("stable"); ok {
		t.Fatal("optimistic caching bypassed configured TTL")
	}
}

func TestOptimisticReliabilityCountsRequestsUntilAComparison(t *testing.T) {
	c := New[string, string](4, Options{RetainExpired: true})
	now := time.Unix(1, 0)
	c.now = func() time.Time { return now }
	generation := c.Generation()
	c.Put("answer", "old", generation, time.Minute)
	now = now.Add(time.Minute)
	for i := 0; i < 3; i++ {
		if value, ok, stale, _ := c.GetOptimisticState("answer"); !ok || !stale || value != "old" {
			t.Fatal("stale response was not served")
		}
	}
	c.StaleRecords(0, 10) // Background inspection is not a user request.
	c.PutCompared("answer", "old", generation, time.Minute, func(old, next string) bool { return old == next })
	if got, want := c.OptimisticStats(), (OptimisticStats{Hits: 3, Compared: 3}); !reflect.DeepEqual(got, want) {
		t.Fatalf("matching refresh stats=%+v want=%+v", got, want)
	}

	generation = c.MarkStale(func(string) bool { return true })
	for i := 0; i < 2; i++ {
		c.GetOptimisticState("answer")
	}
	c.PutCompared("answer", "changed", generation, time.Minute, func(old, next string) bool { return old == next })
	if got, want := c.OptimisticStats(), (OptimisticStats{Hits: 5, Compared: 5, Mismatches: 2}); !reflect.DeepEqual(got, want) {
		t.Fatalf("changed refresh stats=%+v want=%+v", got, want)
	}
	c.ResetOptimisticStats()
	if got := c.OptimisticStats(); !reflect.DeepEqual(got, OptimisticStats{}) {
		t.Fatalf("manual reset retained reliability stats: %+v", got)
	}
}
