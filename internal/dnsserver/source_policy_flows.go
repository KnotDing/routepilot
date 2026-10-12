package dnsserver

import "sort"

type DNSSourcePolicyFlow struct {
	Source  string `json:"source"`
	Policy  string `json:"policy"`
	Kind    string `json:"kind"`
	Queries uint64 `json:"queries"`
}

// Display at most 11 sources x 11 policies. Group observed pairs after choosing
// the top nodes; multiplying marginal shares would invent flow relationships.
func SourcePolicyFlows(sources []DNSSourceCount) []DNSSourcePolicyFlow {
	type policyID struct{ name, kind string }
	counts := map[policyID]uint64{}
	visible := make([]DNSSourceCount, 0, len(sources))
	idOf := func(p DNSPolicyCount) policyID {
		kind := "named"
		if p.Other {
			kind = "overflow"
		} else if p.Policy == "" {
			kind = "unnamed"
		}
		return policyID{p.Policy, kind}
	}
	for _, src := range sources {
		if src.Source == "legacy" || src.Counts.Completed == 0 {
			continue
		}
		visible = append(visible, src)
		for _, p := range src.Counts.Policies {
			counts[idOf(p)] += p.Queries
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		if visible[i].Counts.Completed != visible[j].Counts.Completed {
			return visible[i].Counts.Completed > visible[j].Counts.Completed
		}
		return visible[i].Source < visible[j].Source
	})
	keys := make([]policyID, 0, len(counts))
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
	top := map[policyID]bool{}
	for i, key := range keys {
		if i >= 10 {
			break
		}
		top[key] = true
	}
	type pair struct {
		source string
		policy policyID
	}
	pairs := map[pair]uint64{}
	for i, src := range visible {
		source := src.Source
		if i >= 10 {
			source = "remaining-sources"
		}
		for _, p := range src.Counts.Policies {
			key := idOf(p)
			if !top[key] {
				key = policyID{"", "rest"}
			}
			pairs[pair{source, key}] += p.Queries
		}
	}
	rows := make([]DNSSourcePolicyFlow, 0, len(pairs))
	for key, n := range pairs {
		if n > 0 {
			rows = append(rows, DNSSourcePolicyFlow{key.source, key.policy.name, key.policy.kind, n})
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
		return a.Policy < b.Policy
	})
	return rows
}
