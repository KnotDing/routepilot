package app

import (
	"context"
	"path/filepath"
	"surge-controller/internal/dnsserver"
	"testing"
)

func TestDNSStatisticsSurviveShutdownAndContinueAcrossRestarts(t *testing.T) {
	a := fixtureApp(t)
	want := dnsserver.CounterSnapshot{Queries: 100, Direct: 60, Surge: 30, Reject: 7, Unknown: 3, Intercepted: 70, Addressed: 30, Errors: 2}
	a.dnsCounters.Restore(want)
	path := filepath.Join(a.dir, "c.sqlite")
	for restart := 0; restart < 3; restart++ {
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		var err error
		a, err = Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.dnsCounters.Snapshot(); got != want {
			_ = a.Close()
			t.Fatalf("restart %d lost cumulative counters: got %+v want %+v", restart, got, want)
		}
		a.dnsCounters.Queries.Add(1)
		want.Queries++
		a.dnsCounters.Direct.Add(1)
		want.Direct++
		a.dnsCounters.Addressed.Add(1)
		want.Addressed++
	}
	t.Cleanup(func() { _ = a.Close() })
}
