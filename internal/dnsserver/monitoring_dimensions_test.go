package dnsserver

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"surge-controller/internal/model"
	"testing"
	"time"
)

func recordDimension(m *Monitoring, source, domain, destination, policy, final, code string) {
	ip := netip.MustParseAddr(source)
	m.BeginSource(ip)
	m.FinishQuerySource(ip, domain, false, model.DecisionResult{Decision: model.Surge, Policy: policy, FinalPolicy: final}, destination, code, 2*time.Millisecond)
}
func TestMonitoringDimensionsMatchActualRequestIntersectionsAndRestore(t *testing.T) {
	m := NewMonitoring(DNSMonitoringSnapshot{})
	recordDimension(m, "192.0.2.1", "EXAMPLE.COM.", "cache", "Proxy", "Node A", "NOERROR")
	recordDimension(m, "192.0.2.1", "example.com", "real", "Proxy", "Node B", "NXDOMAIN")
	recordDimension(m, "192.0.2.1", "other.test", "cache", "Proxy", "Node A", "NOERROR")
	recordDimension(m, "192.0.2.2", "example.com", "cache", "Proxy", "Node A", "NOERROR")
	m.BeginSource(netip.MustParseAddr("192.0.2.1"))
	saved := m.Snapshot()
	filter := MonitoringFilter{Source: "192.0.2.1", Domain: "example.com", Destination: "cache", Policy: MonitoringPolicyValue("Proxy"), FinalPolicy: MonitoringPolicyValue("Node A"), ResponseCode: "NOERROR"}
	view := FilterMonitoringCounts(saved, filter)
	if view.Completed != 1 || view.Received != 1 || view.InFlight != 0 || view.LatencyMS != 2 || len(view.Sources) != 1 || len(view.Flows) != 1 || view.Flows[0].Destination != "cache" || view.SourcePolicyFlows[0].Queries != 1 || view.Domains[0].Queries != 1 {
		t.Fatalf("crossed marginal dimensions: %+v", view)
	}
	filter.FinalPolicy = MonitoringPolicyValue("Node B")
	if FilterMonitoringCounts(saved, filter).Completed != 0 {
		t.Fatal("fabricated intersection")
	}
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint DNSMonitoringSnapshot
	if err = json.Unmarshal(raw, &checkpoint); err != nil {
		t.Fatal(err)
	}
	restored := NewMonitoring(checkpoint).Snapshot()
	if len(restored.Observations) != 4 || restored.Unconfirmed != 1 || !restored.DimensionsStartedAt.Equal(saved.DimensionsStartedAt) {
		t.Fatalf("lost persistent dimensions: %+v", restored)
	}
	domainView := saved
	AttachMonitoringDomains(&domainView, saved.Observations, MonitoringFilter{})
	if domainView.DimensionCompleted != 4 || domainView.Domains[0].Domain != "example.com" || domainView.Domains[0].Queries != 3 {
		t.Fatal(domainView.Domains)
	}
	options := ObservationOptions(saved.Observations)
	if len(options.Domains) != 2 || len(options.ResponseCodes) != 2 || len(options.FinalPolicies) != 2 {
		t.Fatal(options)
	}
	if MonitoringDomain(".") != "." {
		t.Fatal("root domain lost")
	}
	if len(m.View().Observations) != 0 {
		t.Fatal("large request correlations leaked into status polling")
	}
}
func TestMonitoringDimensionsCapacityOverflowConservesAndDoesNotMatchFilters(t *testing.T) {
	m := NewMonitoring(DNSMonitoringSnapshot{})
	for i := 0; i < monitoringObservationCapacity+7; i++ {
		recordDimension(m, "192.0.2.1", fmt.Sprintf("host-%d.test", i), "real", "DIRECT", "DIRECT", "NOERROR")
	}
	before := m.Snapshot()
	recordDimension(m, "192.0.2.1", "host-0.test", "real", "DIRECT", "DIRECT", "NOERROR")
	after := m.Snapshot()
	if len(after.Observations) != monitoringObservationCapacity+1 {
		t.Fatal(len(after.Observations))
	}
	AttachMonitoringDomains(&after, after.Observations, MonitoringFilter{})
	if after.DimensionCompleted != after.Completed || after.DimensionOverflow != 7 {
		t.Fatalf("lost overflow: %d %d", after.DimensionCompleted, after.DimensionOverflow)
	}
	if FilterMonitoringCounts(after, MonitoringFilter{Domain: "host-0.test"}).Completed != 2 || FilterMonitoringCounts(after, MonitoringFilter{Domain: "host-4096.test"}).Completed != 0 {
		t.Fatal("overflow matched a specific domain")
	}
	delta, err := MonitoringDifference(after, before)
	if err != nil || len(delta.Observations) != 1 || delta.Observations[0].Queries != 1 {
		t.Fatal(delta, err)
	}
	after.Observations = nil
	if _, err = MonitoringDifference(after, before); err == nil {
		t.Fatal("regressed observations were accepted")
	}
}
