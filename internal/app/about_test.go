package app

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"surge-controller/internal/buildinfo"
	"surge-controller/internal/config"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/state"
)

func TestAboutStatusExposesBackendRelease(t *testing.T) {
	d := config.Default()
	a := &App{config: d, runtime: &Runtime{machine: state.New(d.Settings.Health, "auto"), dispatcher: &dnsserver.Dispatcher{}, started: time.Now()}}
	view, err := a.View(context.Background(), "status")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Software buildinfo.Info `json:"software"`
		State    string         `json:"state"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatal(err)
	}
	if status.Software.Version == "" || status.Software.Build == "" || status.Software.Name != "RoutePilot" || status.Software.OS != runtime.GOOS || status.Software.Arch != runtime.GOARCH || status.State != state.Starting {
		t.Fatalf("incorrect backend identity or status: %+v", status)
	}
}
