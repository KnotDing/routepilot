package routing

import (
	"context"
	"errors"
	"testing"

	"surge-controller/internal/bgp"
	"surge-controller/internal/config"
	"surge-controller/internal/routingapi"
)

type fakeAPI struct {
	desired []bgp.Advertisement
	ready   bool
	err     error
	closed  bool
}

func (f *fakeAPI) Reconcile(_ context.Context, desired []bgp.Advertisement) error {
	f.desired = append([]bgp.Advertisement{}, desired...)
	return f.err
}
func (f *fakeAPI) Refresh(context.Context) (bool, error) { return f.ready, f.err }
func (f *fakeAPI) Status() routingapi.Status {
	states := []routingapi.RouteState{}
	for _, row := range f.desired {
		states = append(states, routingapi.RouteState{Name: routingapi.RouteName(row.Kind, row.Prefix), Prefix: row.Prefix, NextHop: row.NextHop, Present: f.ready})
	}
	status := routingapi.Status{Enabled: true, Routes: states}
	if f.err != nil {
		status.LastError = f.err.Error()
	}
	return status
}
func (f *fakeAPI) Close(context.Context) error { f.closed = true; return nil }

func apiDocument() config.Document {
	d := config.Default()
	d.Settings.Routing = config.Routing{Mode: "api"}
	d.Settings.RoutingAPI = config.RoutingAPI{Preset: "openwrt", BaseURL: "http://127.0.0.1:9", Username: "root", Password: "secret"}
	d.Settings.Surge.NextHop4 = "192.168.20.5"
	return d
}

func TestAPIModeSelectionAndDesired(t *testing.T) {
	d := apiDocument()
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	if !d.Settings.Routing.UsesAPI() || d.Settings.Routing.UsesBGP() {
		t.Fatal("api mode must select the API backend only")
	}
	m := New(context.Background(), d, nil)
	defer m.Close(context.Background())
	if m.bgp != nil {
		t.Fatal("api mode created a BGP speaker")
	}
	if m.api == nil {
		t.Fatal("api mode did not construct the router API backend")
	}
	if got := m.Desired(true, true, true); len(got) != 0 {
		t.Fatalf("bgp desired should stay empty without a speaker, got %v", got)
	}
	apiDesired := m.DesiredAPI(true, true, true)
	if len(apiDesired) != 1 || apiDesired[0].Prefix != "198.18.0.0/15" || apiDesired[0].Kind != "fake-ip" {
		t.Fatalf("unexpected api desired: %+v", apiDesired)
	}
	// Surge unhealthy keeps Fake-IP paths out of the router.
	if got := m.DesiredAPI(true, true, false); len(got) != 0 {
		t.Fatalf("fake-ip must not publish without surge health, got %+v", got)
	}
}

func TestAPIStatusAndReadiness(t *testing.T) {
	m := New(context.Background(), apiDocument(), nil)
	fake := &fakeAPI{ready: true}
	m.api = fake
	if e := m.ReconcileAll(context.Background(), true, true, true); e != nil {
		t.Fatal(e)
	}
	if len(fake.desired) != 1 || fake.desired[0].Kind != "fake-ip" {
		t.Fatalf("unexpected reconcile payload: %+v", fake.desired)
	}
	m.Refresh(context.Background())
	status := m.Status()
	if !status.APIRequested || !status.API.Enabled {
		t.Fatalf("api status missing: %+v", status)
	}
	for _, row := range status.Routes {
		if row.Backend != "api" || row.State != "api-published" || row.Installed == nil || !*row.Installed {
			t.Fatalf("unexpected api route row: %+v", row)
		}
	}
	if !status.FakeIPReady {
		t.Fatal("published api fake-ip should mark readiness")
	}
	fake.ready = false
	m.Refresh(context.Background())
	if m.Status().FakeIPReady {
		t.Fatal("unpublished api fake-ip must block readiness")
	}
	fake.err = errors.New("router down")
	if e := m.ReconcileAll(context.Background(), true, true, true); e == nil {
		t.Fatal("api failure must surface")
	}
	m.Refresh(context.Background())
	if m.Status().API.LastError == "" {
		t.Fatal("api failure must be visible in status")
	}
}

func TestReplaceAPISwapsBackendWithoutBGP(t *testing.T) {
	// Hybrid keeps DNS interception on BGP while Fake-IP goes through the
	// router API, so both backends run side by side.
	d := apiDocument()
	d.Settings.Routing = config.Routing{Mode: "hybrid", FakeIPBackend: "api", DNSBackend: "bgp"}
	d.Settings.BGP.Enabled = true
	d.Settings.BGP.RouterID = "192.168.20.5"
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	m := New(context.Background(), d, nil)
	if m.bgp == nil {
		t.Fatal("expected a BGP speaker beside the API backend")
	}
	fake := &fakeAPI{}
	m.api = fake
	m.ReplaceAPI(context.Background(), d)
	if !fake.closed {
		t.Fatal("previous API backend was not closed")
	}
	if m.api == nil {
		t.Fatal("replacement backend missing")
	}
	static := apiDocument()
	static.Settings.Routing = config.Routing{Mode: "static", StaticFakeIPConfigured: true}
	m.ReplaceAPI(context.Background(), static)
	if m.api != nil {
		t.Fatal("static mode must drop the API backend")
	}
	if m.bgp == nil {
		t.Fatal("API replacement must not disturb the BGP speaker")
	}
}

func TestHybridAPIBackendSelection(t *testing.T) {
	d := apiDocument()
	d.Settings.Routing = config.Routing{Mode: "hybrid", FakeIPBackend: "api", DNSBackend: "bgp"}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	// A fake BGP backend keeps this test off the shared speaker port.
	b := &backend{ready: true}
	m := newManager(d, b, "")
	fake := &fakeAPI{ready: true}
	m.api = fake
	if got := m.DesiredAPI(true, true, true); len(got) != 1 || got[0].Kind != "fake-ip" {
		t.Fatalf("hybrid must route only fake-ip through the API, got %+v", got)
	}
	// Without an explicit selection manual routes stay on BGP in hybrid mode.
	d.Routes = []config.Route{{ID: "r1", Prefix: "203.0.113.0/24", NextHop: "192.0.2.9", Enabled: true}}
	m.UpdateDocument(d)
	if got := m.DesiredAPI(true, true, true); len(got) != 1 {
		t.Fatalf("unselected manual routes must not join the api backend in hybrid, got %+v", got)
	}
	if got := m.Desired(true, true, true); len(got) != 1 || got[0].Kind != "manual" {
		t.Fatalf("unselected manual routes stay on BGP, got %+v", got)
	}
	// Selecting the api backend for manual routes moves them off BGP.
	d.Settings.Routing.ManualBackend = "api"
	m.UpdateDocument(d)
	if got := m.DesiredAPI(true, true, true); len(got) != 2 {
		t.Fatalf("manual routes with the api selection must join the api backend, got %+v", got)
	}
	if got := m.Desired(true, true, true); len(got) != 0 {
		t.Fatalf("manual routes must leave BGP once the api backend is selected, got %+v", got)
	}
	if e := m.Reconcile(context.Background(), []bgp.Advertisement{{Prefix: "203.0.113.0/24", NextHop: "192.0.2.9", Kind: "manual"}}); e == nil {
		t.Fatal("manual rows moved off BGP must not be reconciled into the speaker")
	}
	// Static selection hands manual routes to the operator instead.
	d.Settings.Routing.ManualBackend = "static"
	m.UpdateDocument(d)
	if got := m.DesiredAPI(true, true, true); len(got) != 1 {
		t.Fatalf("static manual routes must not join the api backend, got %+v", got)
	}
	if got := m.Desired(true, true, true); len(got) != 0 {
		t.Fatalf("static manual routes must not be announced over BGP, got %+v", got)
	}
	for _, row := range m.Status().Routes {
		if row.Kind == "manual" && (row.Backend != "static" || row.Managed || row.OperatorConfirmed) {
			t.Fatalf("static manual routes stay operator declarations, got %+v", row)
		}
	}
	// In the dedicated api mode they do without an explicit selection.
	d.Settings.Routing = config.Routing{Mode: "api"}
	m.UpdateDocument(d)
	if got := m.DesiredAPI(true, true, true); len(got) != 2 {
		t.Fatalf("api mode should publish manual routes too, got %+v", got)
	}
}
