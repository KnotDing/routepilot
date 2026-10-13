package api

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"surge-controller/internal/model"
	"testing"
	"time"

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
		{"dns/monitoring?destination=cache", 501},
		{"dns/monitoring?destination=invalid", 400},
		{"dns/monitoring?domain=a..test", 400},
		{"dns/monitoring?policy=Proxy", 400},
		{"dns/monitoring?response_code=invalid", 400},
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

type dimensionMonitorService struct {
	monitorService
	filter dnsserver.MonitoringFilter
}

func (s *dimensionMonitorService) DNSMonitoringFilteredView(_ context.Context, filter dnsserver.MonitoringFilter) (dnsserver.DNSMonitoringSnapshot, error) {
	s.filter = filter
	v := dnsserver.EmptyMonitoringCounts()
	v.DimensionsSupported = true
	return v, nil
}
func TestMonitoringAPIForwardsNormalizedCombinedFilters(t *testing.T) {
	service := &dimensionMonitorService{monitorService: monitorService{routeService: routeService{doc: config.Default()}}}
	a := &API{Service: service}
	query := url.Values{"source": {"::ffff:192.0.2.1"}, "domain": {"EXAMPLE.TEST."}, "destination": {"cache"}, "policy": {"[\"named\", \"Proxy\"]"}, "final_policy": {"[\"unnamed\", \"\"]"}, "response_code": {"nxdomain"}}
	recorder := httptest.NewRecorder()
	a.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/dns/monitoring?"+query.Encode(), nil))
	if recorder.Code != 200 {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
	f := service.filter
	if f.Source != "192.0.2.1" || f.Domain != "example.test" || f.Destination != "cache" || f.Policy != dnsserver.MonitoringPolicyValue("Proxy") || f.FinalPolicy != dnsserver.MonitoringPolicyValue("") || f.ResponseCode != "NXDOMAIN" {
		t.Fatalf("lost predicates: %+v", f)
	}
}

func TestMonitoringHistoricalFiltersRejectUnrecordedLegacyDimensions(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "private", "legacy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Minute)
	m := dnsserver.NewMonitoring(dnsserver.DNSMonitoringSnapshot{})
	strip := func(s dnsserver.DNSMonitoringSnapshot) dnsserver.DNSMonitoringSnapshot {
		s.Observations = nil
		s.DimensionsSupported = false
		s.DimensionsStartedAt = time.Time{}
		return s
	}
	baseline := strip(m.Snapshot())
	baseline.ObservedAt = at
	if err = store.SaveDNSMonitoring(ctx, baseline); err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("192.0.2.1")
	m.BeginSource(ip)
	m.FinishSource(ip, false, model.DecisionResult{Decision: model.Direct}, "real", "NOERROR", time.Millisecond)
	current := strip(m.Snapshot())
	current.ObservedAt = at.Add(5 * time.Second)
	if err = store.SaveDNSMonitoring(ctx, current); err != nil {
		t.Fatal(err)
	}
	a := &API{Store: store, Service: &monitorService{routeService: routeService{doc: config.Default()}}}
	query := url.Values{"from": {at.Format(time.RFC3339)}, "to": {at.Add(time.Minute).Format(time.RFC3339)}, "domain": {"example.test"}}
	recorder := httptest.NewRecorder()
	a.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/api/v1/dns/monitoring/history?"+query.Encode(), nil))
	if recorder.Code != 422 {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
}
