package dnsserver_test

import (
	"fmt"
	"surge-controller/internal/dnsserver"
	"testing"
)

func TestSourcePolicyFlowsGroupObservedPairsWithoutLegacyOrLostRequests(t *testing.T) {
	var sources []dnsserver.DNSSourceCount
	for i := 0; i < 12; i++ {
		sources = append(sources, dnsserver.DNSSourceCount{Source: fmt.Sprintf("192.0.2.%d", i+1), Counts: dnsserver.DNSMonitoringSnapshot{Completed: 12, Policies: []dnsserver.DNSPolicyCount{{Policy: fmt.Sprintf("Policy %02d", i), Queries: 10}, {Policy: "Shared", Queries: 2}}}})
	}
	sources = append(sources, dnsserver.DNSSourceCount{Source: "legacy", Counts: dnsserver.DNSMonitoringSnapshot{Completed: 999, Policies: []dnsserver.DNSPolicyCount{{Policy: "Old", Queries: 999}}}})
	rows := dnsserver.SourcePolicyFlows(sources)
	var total uint64
	clients, policies := map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		total += r.Queries
		clients[r.Source] = true
		policies[r.Kind+":"+r.Policy] = true
		if r.Source == "legacy" || r.Policy == "Old" {
			t.Fatal("legacy data included", r)
		}
	}
	if total != 144 || len(clients) != 11 || len(policies) > 11 || len(rows) > 121 {
		t.Fatal(total, len(clients), len(policies), len(rows))
	}
	for _, r := range rows {
		if r.Source != "remaining-sources" && r.Kind == "named" && r.Policy != "Shared" {
			var found bool
			for _, s := range sources {
				for _, p := range s.Counts.Policies {
					if s.Source == r.Source && p.Policy == r.Policy && p.Queries == r.Queries {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("invented source/policy relationship", r)
			}
		}
	}
}

func TestSourcePolicyFlowsPreservesObservedFinalPolicyTriples(t *testing.T) {
	sources := []dnsserver.DNSSourceCount{
		{Source: "192.0.2.1", Counts: dnsserver.DNSMonitoringSnapshot{Completed: 10, Policies: []dnsserver.DNSPolicyCount{{Policy: "Proxy", FinalPolicy: "Node A", Queries: 7}, {Policy: "Proxy", FinalPolicy: "Node B", Queries: 3}}}},
		{Source: "192.0.2.2", Counts: dnsserver.DNSMonitoringSnapshot{Completed: 20, Policies: []dnsserver.DNSPolicyCount{{Policy: "Proxy", FinalPolicy: "Node B", Queries: 20}}}},
	}
	rows := dnsserver.SourcePolicyFlows(sources)
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row.FinalKind != "named" || row.Source == "192.0.2.2" && row.FinalPolicy != "Node B" {
			t.Fatal("invented final policy", row)
		}
		if row.Source == "192.0.2.1" && row.FinalPolicy == "Node A" && row.Queries != 7 {
			t.Fatal(row)
		}
	}
}

func TestSourcePolicyFlowsBoundsAllThreeColumnsWithoutDroppingCounts(t *testing.T) {
	var sources []dnsserver.DNSSourceCount
	for s := 0; s < 12; s++ {
		src := dnsserver.DNSSourceCount{Source: fmt.Sprintf("192.0.2.%d", s+1), Counts: dnsserver.DNSMonitoringSnapshot{Completed: 144}}
		for p := 0; p < 12; p++ {
			for f := 0; f < 12; f++ {
				src.Counts.Policies = append(src.Counts.Policies, dnsserver.DNSPolicyCount{Policy: fmt.Sprintf("Policy%02d", p), FinalPolicy: fmt.Sprintf("Final%02d", f), Queries: 1})
			}
		}
		sources = append(sources, src)
	}
	rows := dnsserver.SourcePolicyFlows(sources)
	var total uint64
	for _, row := range rows {
		total += row.Queries
	}
	if len(rows) != 1331 || total != 1728 {
		t.Fatal(len(rows), total)
	}
}
