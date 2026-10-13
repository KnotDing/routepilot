package notifications

import (
	"testing"
	"time"

	"surge-controller/internal/cache"
	"surge-controller/internal/config"
)

func BenchmarkAnomalySampling(b *testing.B) {
	for _, mode := range []string{"disabled", "defaults", "hour-window"} {
		b.Run(mode, func(b *testing.B) {
			s := config.Default().Settings.Notifications
			s.Enabled = mode != "disabled"
			if mode == "hour-window" {
				s.Anomalies.CacheWindowSeconds = 3600
				s.Anomalies.BurstWindowSeconds = 3600
			}
			var monitor AnomalyMonitor
			at := time.Unix(1000000, 0)
			sample := func(n int) AnomalySample {
				return AnomalySample{At: at.Add(time.Duration(n) * time.Second), Queries: uint64(n) * 1000, DecisionEnabled: true, ResponseEnabled: true, Decision: cache.OptimisticStats{Compared: uint64(n) * 100, Mismatches: uint64(n) * 10}, Response: cache.OptimisticStats{Compared: uint64(n) * 100, Mismatches: uint64(n) * 10}}
			}
			for i := 0; i <= 3600; i++ {
				monitor.Observe(s, sample(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				monitor.Observe(s, sample(i+3601))
			}
		})
	}
}
