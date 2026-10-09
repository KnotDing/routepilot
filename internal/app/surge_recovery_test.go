package app

import (
	"surge-controller/internal/config"
	"testing"
	"time"
)

func TestQueryRateUsesDeltasAndElapsedTime(t *testing.T) {
	for _, tc := range []struct {
		current, previous uint64
		elapsed           time.Duration
		want              float64
	}{{105, 100, time.Second, 5}, {110, 100, 2 * time.Second, 5}, {100, 105, time.Second, 0}, {110, 100, 0, 0}, {110, 100, -time.Second, 0}} {
		if got := queryRate(tc.current, tc.previous, tc.elapsed); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
}
func TestRestartChangesDoNotChangeDecisionCacheScope(t *testing.T) {
	d := config.Default()
	scope := decisionCacheScope(d)
	d.Settings.Surge.Restart = config.SurgeRestart{Enabled: true, DNS: true}
	if decisionCacheScope(d) != scope {
		t.Fatal("restart preferences invalidated decisions")
	}
}
