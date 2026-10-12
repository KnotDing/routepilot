package api

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/dnsserver"
)

type monitorService struct {
	routeService
	source string
}

func (s *monitorService) DNSMonitoringView(_ context.Context, source string) (dnsserver.DNSMonitoringSnapshot, error) {
	s.source = source
	return dnsserver.EmptyMonitoringCounts(), nil
}

func TestMonitoringReadAPIValidatesRangesSourcesAndNormalizesClientIP(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "private", "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := &monitorService{routeService: routeService{doc: config.Default()}}
	a := &API{Service: service, Store: store}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"dns/monitoring?source=::ffff:192.0.2.10", 200},
		{"dns/monitoring?source=not-an-ip", 400},
		{"dns/monitoring/history?from=2026-10-12T00:00:00Z&to=2026-10-12T01:00:00Z", 200},
		{"dns/monitoring/history?from=2026-10-12T00:00:01Z&to=2026-10-12T01:00:00Z", 400},
		{"dns/monitoring/history?from=2026-10-12T01:00:00Z&to=2026-10-12T00:00:00Z", 400},
		{"dns/monitoring/history?from=2026-10-01T00:00:00Z&to=2026-10-12T00:00:00Z", 400},
		{"dns/monitoring/history?from=bad&to=2026-10-12T00:00:00Z", 400},
	} {
		r := httptest.NewRecorder()
		a.Handler().ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/"+tc.path, nil))
		if r.Code != tc.status {
			t.Fatalf("%s: got %d %s", tc.path, r.Code, r.Body.String())
		}
	}
	if service.source != "192.0.2.10" {
		t.Fatal("mapped IP was not normalized")
	}
}
