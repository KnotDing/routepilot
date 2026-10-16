package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"surge-controller/internal/routing"
	"testing"
	"time"
)

func TestDetectedNextHopRemainsAutomaticAndBindsStaticConfirmation(t *testing.T) {
	a := fixtureApp(t)
	d := a.Configuration()
	raw, err := os.ReadFile(d.Settings.Surge.CLIPath)
	if err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(filepath.Dir(d.Settings.Surge.CLIPath), "vmnet-cli-mock")
	script := strings.Replace(string(raw), "case \"$2 $3\" in", "case \"$2 $3\" in\n \"vmnet status\") printf '%s' '{\"running\":1,\"self-ip\":\"192.0.2.141\",\"router-ip\":\"192.0.2.1\"}' ;;", 1)
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	d.Settings.Surge.CLIPath = cli
	d.Settings.Routing.Mode = "static"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Apply(ctx, d); err != nil {
		t.Fatal(err)
	}
	waitAddress := func(address string) {
		t.Helper()
		for end := time.Now().Add(4 * time.Second); time.Now().Before(end); {
			view, err := a.View(ctx, "surge/status")
			if err != nil {
				t.Fatal(err)
			}
			status := view.(map[string]any)["next_hop4"].(routing.NextHop4Status)
			if status.Address == address && status.Automatic && status.Error == "" {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("automatic next hop was not applied", address)
	}
	waitAddress("192.0.2.141")
	a.mu.RLock()
	rt, manager := a.runtime, a.runtime.routes
	a.mu.RUnlock()
	d = a.Configuration()
	d.Settings.Routing.StaticFakeIPConfigured = true
	if err := a.Apply(ctx, d); err != nil {
		t.Fatal(err)
	}
	if a.Configuration().Settings.Routing.StaticFakeIPNextHop4 != "192.0.2.141" || a.Configuration().Settings.Surge.NextHop4 != "" {
		t.Fatal("detection replaced automatic setting or confirmation address missing")
	}
	rt.routingMu.Lock()
	manager.ObserveNextHop4("192.0.2.142", nil, time.Now())
	rt.routingMu.Unlock()
	waitAddress("192.0.2.142")
	d = a.Configuration()
	d.Settings.Logging.QueryHistory = !d.Settings.Logging.QueryHistory
	if err := a.Apply(ctx, d); err != nil {
		t.Fatal(err)
	}
	if a.Configuration().Settings.Routing.StaticFakeIPNextHop4 != "192.0.2.141" || manager.Status().FakeIPReady {
		t.Fatal("unrelated save silently confirmed changed address")
	}
	d = a.Configuration()
	d.Settings.Routing.StaticFakeIPNextHop4 = "" // explicit checkbox confirmation
	if err := a.Apply(ctx, d); err != nil {
		t.Fatal(err)
	}
	saved, err := a.store.Load(ctx)
	if err != nil || saved.Settings.Routing.StaticFakeIPNextHop4 != "192.0.2.142" || saved.Settings.Surge.NextHop4 != "" {
		t.Fatal("automatic mode/confirmation not persisted", err)
	}
	a.mu.RLock()
	if a.runtime != rt || a.runtime.routes != manager {
		t.Error("next-hop update replaced DNS or routing manager")
	}
	a.mu.RUnlock()
}
