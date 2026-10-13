package dnsserver

import (
	"net/netip"
	"testing"
	"time"

	"surge-controller/internal/model"
)

func benchmarkIP(n uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{10, byte(n >> 16), byte(n >> 8), byte(n)})
}

func BenchmarkClientStatisticsCapacity(b *testing.B) {
	for _, mode := range []string{"existing", "churn"} {
		b.Run(mode, func(b *testing.B) {
			s := NewClientStatistics(4096)
			at := time.Now()
			for i := uint32(1); i <= 4096; i++ {
				s.Record(benchmarkIP(i), model.Direct, false, at.Add(time.Duration(i)))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				index := uint32(i%4096 + 1)
				if mode == "churn" {
					index = uint32(i%1000000 + 4097)
				}
				s.Record(benchmarkIP(index), model.Direct, false, at.Add(time.Second+time.Duration(i)))
			}
		})
	}
}

func BenchmarkMonitoringSnapshot(b *testing.B) {
	for _, detailed := range []bool{false, true} {
		name := "view"
		if detailed {
			name = "checkpoint"
		}
		b.Run(name, func(b *testing.B) {
			m := NewMonitoring(DNSMonitoringSnapshot{})
			for i := uint32(1); i <= 256; i++ {
				for j := uint32(1); j <= 64; j++ {
					ip := benchmarkIP(i)
					m.BeginSource(ip)
					policy := benchmarkIP(j).String()
					m.FinishSource(ip, false, model.DecisionResult{Decision: model.Direct, Policy: policy, FinalPolicy: policy}, "cache", "NOERROR", time.Millisecond)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if detailed {
					m.Snapshot()
				} else {
					m.View()
				}
			}
		})
	}
}
