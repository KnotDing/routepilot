package app

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"surge-controller/internal/buildinfo"
	"surge-controller/internal/cache"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/state"
)

func TestAboutStatusExposesBackendRelease(t *testing.T) {
	d := config.Default()
	a := &App{dnsMonitoring: dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{}), config: d, runtime: &Runtime{machine: state.New(d.Settings.Health, "auto"), dispatcher: &dnsserver.Dispatcher{}, engine: &classifier.Engine{Cache: cache.New[classifier.Key, model.DecisionResult](d.Settings.Cache.Capacity)}, settings: d, started: time.Now()}}
	view, err := a.View(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Software buildinfo.Info  `json:"software"`
		State    string          `json:"state"`
		Name     *string         `json:"name"`
		Cache    map[string]any  `json:"cache"`
		Go       goRuntimeStatus `json:"go_runtime"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatal(err)
	}
	if status.Software.Version == "" || status.Software.Build == "" || status.Software.Name != "RoutePilot" || status.Software.OS != runtime.GOOS || status.Software.Arch != runtime.GOARCH || status.State != state.Starting {
		t.Fatalf("incorrect backend identity or status: %+v", status)
	}
	if status.Name != nil {
		t.Fatalf("status must not expose a backend name: %s", *status.Name)
	}
	if status.Cache["decision"] == nil || status.Cache["response"] == nil {
		t.Fatalf("cache metrics missing from status: %+v", status.Cache)
	}
	if status.Go.Goroutines == 0 || status.Go.MaxProcs != runtime.GOMAXPROCS(0) || status.Go.MemoryTotalBytes < status.Go.HeapObjectsBytes {
		t.Fatalf("invalid Go runtime metrics: %+v", status.Go)
	}
	if status.Go.ProcessRSSBytes == 0 || status.Go.ProcessRSSError != "" {
		t.Fatalf("invalid process RSS metrics: %+v", status.Go)
	}
}
