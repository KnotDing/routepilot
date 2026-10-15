package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"surge-controller/internal/config"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"testing"
	"time"
)

func TestClientHostnameProbeCachesSkipsRemarksAndEvicts(t *testing.T) {
	now := time.Now()
	calls := map[string]int{}
	h := newClientHostnames([]config.Client{{IP: "192.0.2.1", Name: " Manual "}}, func(ctx context.Context, ip string) ([]string, error) {
		calls[ip]++
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > hostnameProbeTimeout {
			t.Fatal("missing bounded lookup deadline")
		}
		if ip == "192.0.2.3" {
			return nil, errors.New("lookup failed")
		}
		return []string{"bad\nname", "Desktop.LAN."}, nil
	})
	rows := []dnsserver.ClientStats{{ClientIP: "192.0.2.1"}, {ClientIP: "::ffff:192.0.2.2"}, {ClientIP: "192.0.2.3"}, {ClientIP: "::"}, {ClientIP: "ff02::1"}, {ClientIP: "invalid"}}
	h.probe(context.Background(), rows, now)
	if !reflect.DeepEqual(calls, map[string]int{"192.0.2.2": 1, "192.0.2.3": 1}) {
		t.Fatalf("unexpected queries: %v", calls)
	}
	snapshot := dnsserver.ClientStatsSnapshot{Clients: []dnsserver.ClientStats{{ClientIP: "192.0.2.2"}, {ClientIP: "192.0.2.3"}}}
	h.decorate(&snapshot)
	if snapshot.Clients[0].Hostname != "desktop.lan" || snapshot.Clients[1].Hostname != "" || snapshot.Clients[1].HostnameError != "lookup failed" {
		t.Fatalf("bad display hints: %+v", snapshot)
	}
	h.probe(context.Background(), rows, now.Add(time.Minute))
	if calls["192.0.2.2"] != 1 || calls["192.0.2.3"] != 1 {
		t.Fatal("retried before expiry")
	}
	h.probe(context.Background(), rows, now.Add(hostnameFailureTTL))
	if calls["192.0.2.2"] != 1 || calls["192.0.2.3"] != 2 {
		t.Fatal("incorrect positive/negative cache lifetimes")
	}
	h.probe(context.Background(), rows, now.Add(hostnameSuccessTTL))
	if calls["192.0.2.2"] != 2 {
		t.Fatal("expired hostname was not refreshed")
	}
	h.probe(context.Background(), nil, now)
	if len(h.entries) != 0 {
		t.Fatal("evicted clients retained hostname cache")
	}
}

func TestClientHostnameProbeFairBoundAndCancellation(t *testing.T) {
	rows := make([]dnsserver.ClientStats, hostnameProbeBatch+2)
	seen := map[string]bool{}
	h := newClientHostnames(nil, func(_ context.Context, ip string) ([]string, error) { seen[ip] = true; return nil, nil })
	for i := range rows {
		rows[i].ClientIP = fmt.Sprintf("192.0.2.%d", i+1)
	}
	now := time.Now()
	h.probe(context.Background(), rows, now)
	if len(seen) != hostnameProbeBatch {
		t.Fatal("probe batch is not bounded")
	}
	h.probe(context.Background(), rows, now.Add(hostnameFailureTTL))
	if len(seen) != len(rows) {
		t.Fatal("retries starved unprobed clients")
	}
	if h.entries[rows[0].ClientIP].error != "No usable PTR record" {
		t.Fatal("empty PTR result was treated as success")
	}

	started := make(chan struct{})
	h = newClientHostnames(nil, func(ctx context.Context, _ string) ([]string, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	clients := dnsserver.NewClientStatistics(1)
	clients.Record(netip.MustParseAddr("192.0.2.1"), model.Direct, false, now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.run(ctx, clients)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	// Reading display hints stays nonblocking while the lookup is in progress.
	snapshot := clients.Snapshot()
	h.decorate(&snapshot)
	cancel()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop on runtime cancellation")
	}
	if len(h.entries) != 0 {
		t.Fatal("cancelled runtime published a lookup result")
	}
}

func TestClientHostnameOptionRuntimeLifecycle(t *testing.T) {
	a := fixtureApp(t)
	if a.runtime.clientHostnames != nil {
		t.Fatal("probing enabled by default")
	}
	doc := a.Configuration()
	doc.Settings.Logging.ProbeHostnames = true
	if err := a.activateConfiguration(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	detector := a.runtime.clientHostnames
	if detector == nil {
		t.Fatal("option did not start detector")
	}
	doc.Settings.Logging.ProbeHostnames = false
	if err := a.activateConfiguration(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	select {
	case <-detector.done:
	default:
		t.Fatal("disabled detector still running")
	}
	if a.runtime.clientHostnames != nil {
		t.Fatal("disable retained detector")
	}
}

func TestClientHostnameViewsDoNotChangeStatistics(t *testing.T) {
	a := fixtureApp(t)
	detector := newClientHostnames(nil, nil)
	close(detector.done)
	a.runtime.clientHostnames = detector
	// Install a deterministic result without any external DNS traffic.
	detector.mu.Lock()
	detector.entries["192.0.2.9"] = clientHostname{name: "desktop.lan", retryAt: time.Now().Add(time.Hour)}
	detector.mu.Unlock()
	a.clientStatistics.Record(netip.MustParseAddr("192.0.2.9"), model.Direct, false, time.Now())
	for _, key := range []string{"status", "dns/clients"} {
		view, err := a.View(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if key == "status" {
			view = view.(map[string]any)["client_statistics"]
		}
		if view.(dnsserver.ClientStatsSnapshot).Clients[0].Hostname != "desktop.lan" {
			t.Fatal("hostname missing from", key)
		}
	}
	if a.clientStatistics.Snapshot().Clients[0].Hostname != "" {
		t.Fatal("display hint changed persisted client statistics")
	}
}
