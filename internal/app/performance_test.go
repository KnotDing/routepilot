package app

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
)

func BenchmarkConfigurationStatusView(b *testing.B) {
	for _, clients := range []int{100, 4096} {
		b.Run(fmt.Sprintf("clients-%d", clients), func(b *testing.B) {
			stats := dnsserver.NewClientStatistics(4096)
			for i := 0; i < clients; i++ {
				stats.Record(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}), model.Direct, false, time.Now())
			}
			a := &App{runtime: &Runtime{dispatcher: &dnsserver.Dispatcher{ClientStatistics: stats}}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := a.View(context.Background(), "configuration/status"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
