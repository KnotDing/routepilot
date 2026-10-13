package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/notifications"
	"surge-controller/internal/resolver"
	"surge-controller/tests/mocks"
)

// Cached fixtures exercise the actual response cache, with no external service.
func performanceDispatcher(t testing.TB, mode string) (*dnsserver.Dispatcher, *mocks.Surge, *mocks.Transport) {
	t.Helper()
	adapter := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{Policy: "benchmark-policy", FinalPolicy: "DIRECT", ContextSafe: true}, nil
	}}
	engine, err := classifier.New(context.Background(), adapter, classifier.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	upstream := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
		answer := mocks.AddressReply(q, "203.0.113.7")
		answer.Answer[0].Header().Ttl = 3600
		return answer, nil
	}}
	real, err := resolver.NewReal(upstream, []resolver.ResolverEndpoint{{Protocol: "udp", Address: "192.0.2.53:53", Enabled: true}}, false, 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(real.Close)
	d := &dnsserver.Dispatcher{Classifier: engine, Real: real, Surge: adapter, Gate: &mocks.Gate{State: dnsserver.SafetySnapshot{AllowFakeIP: true, Generation: 1}}}
	if mode == "history-disabled" {
		d.History = dnsserver.NewHistory(1000, false)
	}
	if mode == "history" || mode == "full" {
		d.History = dnsserver.NewHistory(1000, true)
	}
	if mode == "monitoring" || mode == "full" {
		d.Monitoring = dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
		d.ClientStatistics = dnsserver.NewClientStatistics(4096)
	}
	return d, adapter, upstream
}

// Resource usage at LAN rates, with 10k warm cache entries and 100 sources.
// This includes this test process, management snapshots and SQLite checkpoints;
// external health probes, BGP, the native App and miss latency are excluded.
func TestDNSLANResourceUsage(t *testing.T) {
	if os.Getenv("ROUTEPILOT_STRESS") != "1" {
		t.Skip("set ROUTEPILOT_STRESS=1 for LAN resource measurements")
	}
	for _, qps := range []int{0, 50, 200} {
		t.Run(fmt.Sprintf("qps-%d", qps), func(t *testing.T) {
			d, _, _ := performanceDispatcher(t, "full")
			queries := make([]*dns.Msg, 10000)
			for i := range queries {
				q := new(dns.Msg)
				q.SetQuestion(fmt.Sprintf("lan-%d.test.", i), dns.TypeA)
				queries[i] = q
				if d.Dispatch(context.Background(), performanceClient(uint32(i%100+1)), q).Rcode != dns.RcodeSuccess {
					t.Fatal("cache warmup failed")
				}
			}
			directory := filepath.Join(t.TempDir(), "private")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			store, err := database.Open(filepath.Join(directory, "performance.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			settings := config.Default().Settings.Notifications
			settings.Enabled = true
			var alerts notifications.AnomalyMonitor
			var before, after runtime.MemStats
			cpuSeconds := func() float64 {
				var usage syscall.Rusage
				if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
					t.Fatal(err)
				}
				return float64(usage.Utime.Sec+usage.Stime.Sec) + float64(usage.Utime.Usec+usage.Stime.Usec)/1e6
			}
			runtime.GC()
			runtime.ReadMemStats(&before)
			cpuStart := cpuSeconds()
			started := time.Now()
			stop := time.NewTimer(10 * time.Second)
			defer stop.Stop()
			sampler := time.NewTicker(time.Second)
			defer sampler.Stop()
			checkpoint := time.NewTicker(5 * time.Second)
			defer checkpoint.Stop()
			var requests <-chan time.Time
			if qps > 0 {
				ticker := time.NewTicker(time.Second / time.Duration(qps))
				defer ticker.Stop()
				requests = ticker.C
			}
			count := 0
			var snapshots uint64
		loop:
			for {
				select {
				case <-stop.C:
					break loop
				case at := <-requests:
					index := count % len(queries)
					client := performanceClient(uint32(index%100 + 1))
					client.ReceivedAt = at
					answer := d.Dispatch(context.Background(), client, queries[index])
					if answer.Rcode != dns.RcodeSuccess {
						t.Fatal("query failed")
					}
					count++
				case at := <-sampler.C:
					// Model an open management window's one-second status refresh.
					real := d.Real.(*resolver.Real)
					if os.Getenv("ROUTEPILOT_RESOURCE_NO_STATUS") != "1" {
						d.Classifier.(*classifier.Engine).Cache.Stats()
						real.Cache.Stats()
						d.ClientStatistics.Snapshot()
						d.Monitoring.View()
						snapshots++
					}
					// Notification sampling uses cumulative reliability counters only.
					response := real.Cache.OptimisticCounters()
					if os.Getenv("ROUTEPILOT_SAMPLE_FULL_SCAN") == "1" {
						stats := real.Cache.Stats()
						response.Compared, response.Mismatches = stats.OptimisticCompared, stats.OptimisticMismatches
					}
					alerts.Observe(settings, notifications.AnomalySample{At: at, Queries: d.Statistics().Snapshot().Queries, Decision: d.Classifier.(*classifier.Engine).Cache.OptimisticCounters(), Response: response, DecisionEnabled: true, ResponseEnabled: true})
				case <-checkpoint.C:
					if err := store.SaveDNSMonitoring(context.Background(), d.Monitoring.Snapshot()); err != nil {
						t.Fatal(err)
					}
					if err := store.SaveStatistics(context.Background(), "dns_clients", d.ClientStatistics.Snapshot()); err != nil {
						t.Fatal(err)
					}
				}
			}
			elapsed := time.Since(started).Seconds()
			cpu := cpuSeconds() - cpuStart
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc
			runtime.GC()
			var retained runtime.MemStats
			runtime.ReadMemStats(&retained)
			raw, err := json.Marshal(map[string]any{"target_qps": qps, "completed": count, "actual_qps": float64(count) / elapsed, "seconds": elapsed, "cpu_seconds": cpu, "cpu_one_core_percent": cpu / elapsed * 100, "allocated_bytes_per_second": float64(allocated) / elapsed, "gc_cycles": after.NumGC - before.NumGC, "heap_before_mib": float64(before.HeapAlloc) / (1 << 20), "heap_after_gc_mib": float64(retained.HeapAlloc) / (1 << 20), "heap_sys_mib": float64(retained.HeapSys) / (1 << 20), "goroutines": runtime.NumGoroutine(), "status_refreshes": snapshots})
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(raw))
			runtime.KeepAlive(queries)
			runtime.KeepAlive(d)
		})
	}
}

func performanceClient(i uint32) model.DNSQueryContext {
	return model.DNSQueryContext{ClientIP: netip.AddrFrom4([4]byte{192, 0, byte(i >> 8), byte(i)})}
}

func BenchmarkDNSProductionPaths(b *testing.B) {
	for _, mode := range []string{"bare", "history-disabled", "history", "monitoring", "full"} {
		b.Run(mode, func(b *testing.B) {
			d, adapter, upstream := performanceDispatcher(b, mode)
			q := new(dns.Msg)
			q.SetQuestion("performance.test.", dns.TypeA)
			for i := uint32(1); i <= 256; i++ {
				d.Dispatch(context.Background(), performanceClient(i), q)
			}
			rules, exchanges := adapter.Calls.Load(), upstream.Calls.Load()
			var worker atomic.Uint32
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				client := performanceClient((worker.Add(1)-1)%256 + 1)
				for pb.Next() {
					answer := d.Dispatch(context.Background(), client, q)
					if answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
						b.Error("invalid cached answer")
						return
					}
				}
			})
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/s")
			if adapter.Calls.Load() != rules || upstream.Calls.Load() != exchanges {
				b.Fatal("hot path performed external work")
			}
		})
	}
}

// Opt in to sustained real-socket stress; routine regressions remain short.
// Percentiles sample every 16th request, bounding latency storage per client.
func TestDNSProductionStress(t *testing.T) {
	if os.Getenv("ROUTEPILOT_STRESS") != "1" {
		t.Skip("set ROUTEPILOT_STRESS=1 for sustained UDP load")
	}
	duration := 10 * time.Second
	if raw := os.Getenv("ROUTEPILOT_STRESS_DURATION"); raw != "" {
		var err error
		duration, err = time.ParseDuration(raw)
		if err != nil || duration < time.Second || duration > time.Minute {
			t.Fatal("stress duration must be 1s..1m")
		}
	}
	for _, workers := range []int{16, 64, 256} {
		t.Run(fmt.Sprintf("workers-%d", workers), func(t *testing.T) {
			d, adapter, upstream := performanceDispatcher(t, "full")
			server, err := dnsserver.Listen(context.Background(), []dnsserver.Ingress{{Bind: netip.MustParseAddrPort("127.0.0.1:0")}}, dnsserver.DefaultServerConfig(), d.Dispatch)
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			target := server.Addresses()[0].String()
			q := new(dns.Msg)
			q.SetQuestion("performance.test.", dns.TypeA)
			if _, _, err = (&dns.Client{Timeout: time.Second}).Exchange(q, target); err != nil {
				t.Fatal(err)
			}
			rules, exchanges := adapter.Calls.Load(), upstream.Calls.Load()
			var wg sync.WaitGroup
			var completed, failed atomic.Uint64
			latencies := make([][]time.Duration, workers)
			ready := make(chan struct{})
			started := time.Now()
			stop := started.Add(duration)
			for worker := 0; worker < workers; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					conn, err := net.Dial("udp", target)
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					wire := &dns.Conn{Conn: conn}
					msg := q.Copy()
					<-ready
					for count := 0; time.Now().Before(stop); count++ {
						msg.Id++
						begin := time.Now()
						if err = conn.SetDeadline(begin.Add(time.Second)); err != nil {
							t.Error(err)
							return
						}
						if err = wire.WriteMsg(msg); err != nil {
							failed.Add(1)
							continue
						}
						answer, err := wire.ReadMsg()
						if err != nil || answer.Id != msg.Id || answer.Rcode != dns.RcodeSuccess || len(answer.Answer) != 1 {
							failed.Add(1)
							continue
						}
						completed.Add(1)
						if count%16 == 0 {
							latencies[worker] = append(latencies[worker], time.Since(begin))
						}
					}
				}(worker)
			}
			close(ready)
			wg.Wait()
			elapsed := time.Since(started)
			var samples []time.Duration
			for _, batch := range latencies {
				samples = append(samples, batch...)
			}
			if len(samples) == 0 {
				t.Fatal("no successful queries")
			}
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			report := map[string]any{"workers": workers, "completed": completed.Load(), "failed": failed.Load(), "server_dropped": server.Dropped.Load(), "seconds": elapsed.Seconds(), "qps": float64(completed.Load()) / elapsed.Seconds(), "p50_ms": float64(samples[len(samples)/2]) / float64(time.Millisecond), "p99_ms": float64(samples[len(samples)*99/100]) / float64(time.Millisecond), "latency_samples": len(samples), "rule_evaluations": adapter.Calls.Load() - rules, "upstream_exchanges": upstream.Calls.Load() - exchanges}
			raw, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(raw))
			if failed.Load() != 0 || server.Dropped.Load() != 0 {
				t.Error("load incurred failed or dropped queries")
			}
			if adapter.Calls.Load() != rules || upstream.Calls.Load() != exchanges {
				t.Error("cached load invoked external work")
			}
			if d.Monitoring.Snapshot().Completed != completed.Load()+1 {
				t.Error("monitoring lost request counts")
			}
		})
	}
}
