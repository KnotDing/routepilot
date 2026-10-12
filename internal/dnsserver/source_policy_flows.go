package dnsserver

import "sort"

type DNSSourcePolicyFlow struct {
	Source      string `json:"source"`
	Policy      string `json:"policy"`
	Kind        string `json:"kind"`
	FinalPolicy string `json:"final_policy"`
	FinalKind   string `json:"final_kind"`
	Queries     uint64 `json:"queries"`
}

type monitoringPolicyID struct{ name, kind string }

func monitoringPolicyKey(name string, other bool) monitoringPolicyID {
	kind := "named"
	if other {
		kind = "overflow"
	} else if name == "" {
		kind = "unnamed"
	}
	return monitoringPolicyID{name, kind}
}

func topMonitoringPolicies(counts map[monitoringPolicyID]uint64) map[monitoringPolicyID]bool {
	keys := make([]monitoringPolicyID, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if counts[a] != counts[b] {
			return counts[a] > counts[b]
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.name < b.name
	})
	top := map[monitoringPolicyID]bool{}
	for i, key := range keys {
		if i >= 10 {
			break
		}
		top[key] = true
	}
	return top
}

// Bound each of three columns to ten main nodes plus a remainder. Group only
// observed source/policy/final-policy triples; never multiply marginal shares.
func SourcePolicyFlows(sources []DNSSourceCount) []DNSSourcePolicyFlow {
	policies, finals := map[monitoringPolicyID]uint64{}, map[monitoringPolicyID]uint64{}
	visible := make([]DNSSourceCount, 0, len(sources))
	for _, src := range sources {
		if src.Source == "legacy" || src.Counts.Completed == 0 {
			continue
		}
		visible = append(visible, src)
		for _, p := range src.Counts.Policies {
			policies[monitoringPolicyKey(p.Policy, p.Other)] += p.Queries
			finals[monitoringPolicyKey(p.FinalPolicy, p.Other)] += p.Queries
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		if visible[i].Counts.Completed != visible[j].Counts.Completed {
			return visible[i].Counts.Completed > visible[j].Counts.Completed
		}
		return visible[i].Source < visible[j].Source
	})
	topPolicies, topFinals := topMonitoringPolicies(policies), topMonitoringPolicies(finals)
	type triple struct {
		source        string
		policy, final monitoringPolicyID
	}
	counts := map[triple]uint64{}
	for i, src := range visible {
		source := src.Source
		if i >= 10 {
			source = "remaining-sources"
		}
		for _, p := range src.Counts.Policies {
			policy, final := monitoringPolicyKey(p.Policy, p.Other), monitoringPolicyKey(p.FinalPolicy, p.Other)
			if !topPolicies[policy] {
				policy = monitoringPolicyID{"", "rest"}
			}
			if !topFinals[final] {
				final = monitoringPolicyID{"", "rest"}
			}
			counts[triple{source, policy, final}] += p.Queries
		}
	}
	rows := make([]DNSSourcePolicyFlow, 0, len(counts))
	for key, n := range counts {
		if n > 0 {
			rows = append(rows, DNSSourcePolicyFlow{key.source, key.policy.name, key.policy.kind, key.final.name, key.final.kind, n})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Policy != b.Policy {
			return a.Policy < b.Policy
		}
		if a.FinalKind != b.FinalKind {
			return a.FinalKind < b.FinalKind
		}
		return a.FinalPolicy < b.FinalPolicy
	})
	return rows
}
