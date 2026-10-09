package cache

import (
	"testing"
	"time"
)

func TestRestorePreservesExpiryLRUAndFreshValues(t *testing.T) {
	now := time.Unix(100, 0)
	old := New[string, string](4)
	old.now = func() time.Time { return now }
	old.Put("expired", "old", old.Generation(), time.Second)
	old.Put("oldest", "old", old.Generation(), time.Minute)
	old.Put("newer", "old", old.Generation(), time.Minute)
	old.Put("newest", "old", old.Generation(), time.Minute)
	old.MarkStale(func(string) bool { return true })
	saved := old.Snapshot()
	now = now.Add(2 * time.Second)
	next := New[string, string](3)
	next.now = old.now
	next.Put("newer", "fresh", next.Generation(), time.Hour)
	if n := next.Restore(saved); n != 2 {
		t.Fatal("unexpected restore count", n)
	}
	if value, ok, stale, _ := next.GetState("newer"); !ok || value != "fresh" || stale {
		t.Fatal("snapshot overwrote current data")
	}
	if _, ok := next.Get("expired"); ok {
		t.Fatal("expired record was restored")
	}
	if _, ok, stale, _ := next.GetState("newest"); !ok || !stale {
		t.Fatal("pending refresh lost")
	}
	now = now.Add(58 * time.Second)
	if _, ok := next.Get("newest"); ok {
		t.Fatal("restart extended original TTL")
	}
	if _, ok := next.Get("newer"); !ok {
		t.Fatal("restoration changed fresh value TTL")
	}
	// Reducing capacity keeps the most recently used saved rows.
	small := New[string, string](1)
	small.now = func() time.Time { return time.Unix(102, 0) }
	if n := small.Restore(saved); n != 1 {
		t.Fatal(n)
	}
	if _, ok := small.Get("newest"); !ok {
		t.Fatal("capacity reduction retained the wrong LRU record")
	}
}
