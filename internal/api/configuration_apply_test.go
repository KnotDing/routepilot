package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"surge-controller/internal/config"
	"surge-controller/internal/model"
)

type durableService struct {
	routeService
	saves int
}

func (s *durableService) Apply(context.Context, config.Document) error {
	panic("HTTP configuration submission waited for runtime activation")
}
func (s *durableService) SaveConfiguration(_ context.Context, d config.Document) error {
	s.saves++
	s.doc = d.Clone()
	return nil
}

func TestEveryConfigurationSubmissionUsesDurableSave(t *testing.T) {
	d := config.Default()
	d.Peers = []config.Peer{{ID: "peer", Address: "192.0.2.2", ASN: 64513, Port: 179, HoldSeconds: 30, KeepaliveSeconds: 10}}
	d.Routes = []config.Route{{ID: "route", Prefix: "203.0.113.0/24", NextHop: "192.0.2.1", Binding: "none"}}
	d.Hosts = []config.Host{{ID: "host", Domain: "example.test", Match: "exact", Addresses: []string{"192.0.2.1"}, TTL: 60, Source: "manual", Enabled: true}}
	d.Interception = []config.Interception{{ID: "intercept", Prefix: "192.0.2.53/32"}}
	d.Overrides = []config.Override{{ID: "override", Domain: "example.test", Match: "exact", Decision: model.Direct}}
	d.RouteFeeds = []config.RouteFeed{{ID: "feed", URL: "https://example.com/routes.txt", IntervalMinutes: 60, SyncIPv4: true, NextHop4: "192.0.2.1", Binding: "none"}}
	d.Settings.RemoteBackends = []config.Backend{{ID: "backend", Name: "Remote", URL: "http://192.0.2.5:1055"}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	type submission struct {
		method, path string
		body         any
	}
	cases := []submission{
		{"PUT", "settings", d.Settings},
		{"PUT", "mode", map[string]string{"mode": "force-direct"}},
		{"POST", "configuration/import", d},
		{"POST", "configuration/retry", nil},
		{"POST", "resources/bulk-delete", map[string]any{"resource": "dns/hosts", "ids": []string{"host"}}},
		{"POST", "bgp/routes/import", map[string]any{"prefixes": []string{"203.0.114.0/24"}, "next_hop4": "192.0.2.1", "enabled": true}},
	}
	for _, resource := range []struct {
		path, id string
		body     any
	}{
		{"bgp/peers", "peer", d.Peers[0]},
		{"bgp/routes", "route", d.Routes[0]},
		{"dns/hosts", "host", d.Hosts[0]},
		{"dns/interception", "intercept", d.Interception[0]},
		{"overrides", "override", d.Overrides[0]},
		{"bgp/routes/feeds", "feed", d.RouteFeeds[0]},
		{"management/backends", "backend", d.Settings.RemoteBackends[0]},
	} {
		cases = append(cases, submission{"PUT", resource.path + "/" + resource.id, resource.body}, submission{"DELETE", resource.path + "/" + resource.id, nil})
		// Distinct domain/address/prefix values avoid duplicate-resource validation.
		fresh := d.Clone()
		fresh.Peers = nil
		fresh.Routes = nil
		fresh.Hosts = nil
		fresh.Interception = nil
		fresh.Overrides = nil
		fresh.RouteFeeds = nil
		fresh.Settings.RemoteBackends = nil
		t.Run("POST "+resource.path, func(t *testing.T) {
			assertDurableSubmission(t, fresh, submission{"POST", resource.path, resource.body})
		})
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) { assertDurableSubmission(t, d, tc) })
	}
}

func assertDurableSubmission(t *testing.T, d config.Document, tc struct {
	method, path string
	body         any
}) {
	t.Helper()
	s := &durableService{routeService: routeService{doc: d.Clone()}}
	a := &API{Service: s}
	raw, err := json.Marshal(tc.body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest(tc.method, "/api/v1/"+tc.path, bytes.NewReader(raw)))
	if w.Code < 200 || w.Code >= 300 || s.saves != 1 {
		t.Fatalf("status=%d saves=%d body=%s", w.Code, s.saves, w.Body.String())
	}
}

func TestBackgroundOperationAcknowledgesBeforeWorkCompletes(t *testing.T) {
	a := &API{}
	finish := make(chan struct{})
	defer close(finish)
	w := httptest.NewRecorder()
	a.startOperation(w, "test", func(context.Context) (any, error) { <-finish; return map[string]bool{"ok": true}, nil })
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	var response struct {
		Operation operation `json:"operation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Operation.ID == "" || response.Operation.State != "running" {
		t.Fatal("task was not accepted immediately")
	}
}
