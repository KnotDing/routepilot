package cache

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestGroupedVerificationUsesServedValueAndRejectedWritesNeverSettle(t *testing.T) {
	c := New[string, string](10)
	c.ConfigureGroups(func(_ string, v string) string { return v }, nil)
	c.Put("a", "DIRECT", c.Generation(), Forever)
	g := c.MarkStale(func(string) bool { return true })
	for i := 0; i < 3; i++ {
		c.GetOptimisticState("a")
	}
	c.PeekState("a")
	c.StaleRecords(0, 1)
	c.Stats()
	if c.PutCompared("a", "SURGE", g-1, Forever, func(a, b string) bool { return a == b }) {
		t.Fatal("old generation accepted")
	}
	if c.OptimisticCounters().Compared != 0 {
		t.Fatal("rejected write verified requests")
	}
	c.PutCompared("a", "SURGE", g, Forever, func(a, b string) bool { return a == b })
	s := c.Stats()
	if got := s.Groups["DIRECT"]; got.OptimisticHits != 3 || got.OptimisticCompared != 3 || got.OptimisticMismatches != 3 || got.Entries != 0 {
		t.Fatal("incorrect served-decision attribution", got)
	}
	if got := s.Groups["SURGE"]; got.Entries != 1 || got.OptimisticCompared != 0 {
		t.Fatal("new result received old samples", got)
	}
	saved := c.OptimisticStats()
	next := New[string, string](10)
	next.ConfigureGroups(func(_ string, v string) string { return v }, nil)
	next.RestoreOptimisticStats(saved)
	if !reflect.DeepEqual(next.OptimisticStats(), saved) {
		t.Fatal("group counters lost on restore")
	}
	next.ResetOptimisticStats()
	if !reflect.DeepEqual(next.OptimisticCounters(), OptimisticStats{}) {
		t.Fatal("manual reset retained counters")
	}
}

func TestSelectiveLookupAndExpiryDoNotCountDisallowedStaleHits(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New[string, string](10, Options{RetainExpired: true})
	c.now = func() time.Time { return now }
	c.ConfigureGroups(func(_ string, v string) string { return v }, func(v string) bool { return v == "optimistic" })
	for _, v := range []string{"ordinary", "optimistic"} {
		c.Put(v, v, c.Generation(), time.Second)
	}
	now = now.Add(2 * time.Second)
	if _, ok, _, _ := c.GetAllowedState("ordinary", true, func(string, bool) bool { return true }); ok {
		t.Fatal("ordinary endpoint served expired value")
	}
	if _, ok, _, _ := c.GetAllowedState("optimistic", true, func(string, bool) bool { return false }); ok {
		t.Fatal("disabled endpoint served stale value")
	}
	if c.Stats().OptimisticHits != 0 || c.Stats().Hits != 0 {
		t.Fatal("rejected lookups counted as hits")
	}
}

func TestGroupStatisticsAreBoundedAndSnapshotsDoNotMutateCounters(t *testing.T) {
	c := New[string, string](1100)
	c.ConfigureGroups(func(k, v string) string { return v }, nil)
	for i := 0; i < 1100; i++ {
		key := fmt.Sprint(i)
		c.Put(key, key, c.Generation(), Forever)
	}
	before := c.OptimisticStats()
	stats := c.Stats()
	if len(stats.Groups) > 1025 || stats.Groups["other"].Entries != 76 {
		t.Fatal("unbounded or incomplete groups", len(stats.Groups), stats.Groups["other"])
	}
	if !reflect.DeepEqual(before, c.OptimisticStats()) {
		t.Fatal("stats snapshot changed reliability")
	}
}
