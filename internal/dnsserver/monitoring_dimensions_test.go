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

func TestMonitoringDimensionsFullCheckpointSurvivesRepeatedRestore(t *testing.T) {
	m := NewMonitoring(DNSMonitoringSnapshot{})
	for i := 0; i < monitoringObservationCapacity+7; i++ {
		recordDimension(m, "192.0.2.1", fmt.Sprintf("host-%d.test", i), "real", "DIRECT", "DIRECT", "NOERROR")
	}
	before := m.Snapshot()
	last := before.Observations[len(before.Observations)-1].Domain
	for restart := 0; restart < 3; restart++ {
		raw, err := json.Marshal(before)
		if err != nil {
			t.Fatal(err)
		}
		var checkpoint DNSMonitoringSnapshot
		if err = json.Unmarshal(raw, &checkpoint); err != nil {
			t.Fatal(err)
		}
		m = NewMonitoring(checkpoint)
		restored := m.Snapshot()
		if _, err = MonitoringDifference(restored, before); err != nil {
			t.Fatalf("restart %d: %v", restart, err)
		}
		recordDimension(m, "192.0.2.1", last, "real", "DIRECT", "DIRECT", "NOERROR")
		recordDimension(m, "192.0.2.1", "new-overflow.test", "real", "DIRECT", "DIRECT", "NOERROR")
		after := m.Snapshot()
		delta, err := MonitoringDifference(after, before)
		if err != nil || delta.Completed != 2 {
			t.Fatalf("restart %d: delta=%d, err=%v", restart, delta.Completed, err)
		}
		if got := FilterMonitoringCounts(after, MonitoringFilter{Domain: last}).Completed; got != uint64(restart+2) {
			t.Fatal("retained detail was replaced by overflow", last, got)
		}
		AttachMonitoringDomains(&after, after.Observations, MonitoringFilter{})
		if after.DimensionCompleted != after.Completed || after.DimensionOverflow != uint64(8+restart) || len(after.Observations) != monitoringObservationCapacity+1 {
			t.Fatal("restore changed observation capacity or counts")
		}
		before = after
	}
}

func TestMonitoringObservationMergeDoesNotChargeOverflowAgainstDetailCapacity(t *testing.T) {
	overflow := DNSObservation{DNSObservationKey: DNSObservationKey{Other: true}, Queries: 7, LatencyMS: 14}
	detailed := make([]DNSObservation, monitoringObservationCapacity)
	for i := range detailed {
		detailed[i] = DNSObservation{DNSObservationKey: DNSObservationKey{Domain: fmt.Sprintf("host-%d.test", i)}, Queries: 1, LatencyMS: 2}
	}
	for _, order := range []bool{false, true} {
		rows := addObservations([]DNSObservation{overflow}, detailed)
		if order {
			rows = addObservations(detailed, []DNSObservation{overflow})
		}
		merged := addObservations(nil, rows)
		var details int
		var total uint64
		for _, row := range merged {
			total += row.Queries
			if !row.Other {
				details++
			} else if row.Queries != 7 {
				t.Fatal("overflow absorbed an existing detail", row.Queries)
			}
		}
		if details != monitoringObservationCapacity || total != monitoringObservationCapacity+7 {
			t.Fatal("capacity changed on merge", details, total)
		}
	}
}
