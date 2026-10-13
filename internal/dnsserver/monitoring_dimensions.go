package dnsserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Bound retained request correlations independently of query-history logging.
// New combinations beyond this limit become an explicit unfilterable row.
const monitoringObservationCapacity = 4096

type DNSObservationKey struct {
	Source       string `json:"source"`
	Domain       string `json:"domain"`
	Ingress      string `json:"ingress"`
	Destination  string `json:"destination"`
	Policy       string `json:"policy"`
	FinalPolicy  string `json:"final_policy"`
	Decision     string `json:"decision"`
	ResponseCode string `json:"response_code"`
	Other        bool   `json:"other,omitempty"`
}
type DNSObservation struct {
	DNSObservationKey
	Queries   uint64  `json:"queries"`
	LatencyMS float64 `json:"latency_ms_sum"`
}
type DNSDomainCount struct {
	Domain  string `json:"domain"`
	Other   bool   `json:"other,omitempty"`
	Queries uint64 `json:"queries"`
}
type MonitoringPolicyOption struct {
	Value string `json:"value"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
}
type MonitoringFilterOptions struct {
	Destinations  []string                 `json:"destinations"`
	Policies      []MonitoringPolicyOption `json:"policies"`
	FinalPolicies []MonitoringPolicyOption `json:"final_policies"`
	Domains       []string                 `json:"domains"`
	ResponseCodes []string                 `json:"response_codes"`
}
type MonitoringFilter struct{ Source, Destination, Policy, FinalPolicy, Domain, ResponseCode string }

func (f MonitoringFilter) HasDimensions() bool {
	return f.Destination != "" || f.Policy != "" || f.FinalPolicy != "" || f.Domain != "" || f.ResponseCode != ""
}
func MonitoringDomain(domain string) string {
	if strings.TrimSpace(domain) == "." {
		return "."
	}
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
}
func MonitoringPolicyValue(name string) string {
	kind := "named"
	if name == "" {
		kind = "unnamed"
	}
	raw, _ := json.Marshal([2]string{kind, name})
	return string(raw)
}
func (f MonitoringFilter) Matches(row DNSObservation) bool {
	if row.Other {
		return f.Source == "" && !f.HasDimensions()
	}
	return (f.Source == "" || row.Source == f.Source) && (f.Destination == "" || row.Destination == f.Destination) && (f.Policy == "" || MonitoringPolicyValue(row.Policy) == f.Policy) && (f.FinalPolicy == "" || MonitoringPolicyValue(row.FinalPolicy) == f.FinalPolicy) && (f.Domain == "" || row.Domain == f.Domain) && (f.ResponseCode == "" || row.ResponseCode == f.ResponseCode)
}
func observationRows(counts map[DNSObservationKey]DNSObservation) []DNSObservation {
	rows := make([]DNSObservation, 0, len(counts))
	for _, row := range counts {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].DNSObservationKey, rows[j].DNSObservationKey
		av := [8]string{a.Source, a.Domain, a.Ingress, a.Destination, a.Policy, a.FinalPolicy, a.Decision, a.ResponseCode}
		bv := [8]string{b.Source, b.Domain, b.Ingress, b.Destination, b.Policy, b.FinalPolicy, b.Decision, b.ResponseCode}
		for k := range av {
			if av[k] != bv[k] {
				return av[k] < bv[k]
			}
		}
		return !a.Other && b.Other
	})
	return rows
}
func differenceObservations(current, previous []DNSObservation) ([]DNSObservation, error) {
	old := map[DNSObservationKey]DNSObservation{}
	now := map[DNSObservationKey]DNSObservation{}
	for _, row := range previous {
		old[row.DNSObservationKey] = row
	}
	for _, row := range current {
		now[row.DNSObservationKey] = row
	}
	for key, row := range old {
		if now[key].Queries < row.Queries || now[key].LatencyMS < row.LatencyMS {
			return nil, fmt.Errorf("monitoring request dimensions regressed")
		}
	}
	delta := map[DNSObservationKey]DNSObservation{}
	for key, row := range now {
		prior := old[key]
		row.Queries -= prior.Queries
		row.LatencyMS -= prior.LatencyMS
		if row.Queries > 0 {
			delta[key] = row
		}
	}
	return observationRows(delta), nil
}
func addObservations(dst, src []DNSObservation) []DNSObservation {
	counts := map[DNSObservationKey]DNSObservation{}
	for _, list := range [][]DNSObservation{dst, src} {
		for _, row := range list {
			key := row.DNSObservationKey
			if _, ok := counts[key]; !ok && len(counts) >= monitoringObservationCapacity {
				key = DNSObservationKey{Other: true}
			}
			n := counts[key]
			n.DNSObservationKey = key
			n.Queries += row.Queries
			n.LatencyMS += row.LatencyMS
			counts[key] = n
		}
	}
	return observationRows(counts)
}
func ObservationOptions(rows []DNSObservation) MonitoringFilterOptions {
	options := MonitoringFilterOptions{Destinations: []string{}, Policies: []MonitoringPolicyOption{}, FinalPolicies: []MonitoringPolicyOption{}, Domains: []string{}, ResponseCodes: []string{}}
	destinations, domains, codes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	policies, finals := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		if row.Other {
			continue
		}
		destinations[row.Destination] = true
		if row.Domain != "" {
			domains[row.Domain] = true
		}
		codes[row.ResponseCode] = true
		policies[row.Policy] = true
		finals[row.FinalPolicy] = true
	}
	for key := range destinations {
		options.Destinations = append(options.Destinations, key)
	}
	for key := range domains {
		options.Domains = append(options.Domains, key)
	}
	for key := range codes {
		options.ResponseCodes = append(options.ResponseCodes, key)
	}
	for _, pair := range []struct {
		values map[string]bool
		out    *[]MonitoringPolicyOption
	}{{policies, &options.Policies}, {finals, &options.FinalPolicies}} {
		for name := range pair.values {
			kind := "named"
			if name == "" {
				kind = "unnamed"
			}
			*pair.out = append(*pair.out, MonitoringPolicyOption{MonitoringPolicyValue(name), name, kind})
		}
		sort.Slice(*pair.out, func(i, j int) bool { return (*pair.out)[i].Value < (*pair.out)[j].Value })
	}
	sort.Strings(options.Destinations)
	sort.Strings(options.Domains)
	sort.Strings(options.ResponseCodes)
	return options
}
func AttachMonitoringDomains(view *DNSMonitoringSnapshot, rows []DNSObservation, filter MonitoringFilter) {
	type domainKey struct {
		domain string
		other  bool
	}
	counts := map[domainKey]uint64{}
	view.Domains = []DNSDomainCount{}
	view.DimensionCompleted = 0
	view.DimensionOverflow = 0
	for _, row := range rows {
		if !filter.Matches(row) {
			continue
		}
		counts[domainKey{row.Domain, row.Other}] += row.Queries
		view.DimensionCompleted += row.Queries
		if row.Other {
			view.DimensionOverflow += row.Queries
		}
	}
	for key, n := range counts {
		view.Domains = append(view.Domains, DNSDomainCount{key.domain, key.other, n})
	}
	sort.Slice(view.Domains, func(i, j int) bool {
		a, b := view.Domains[i], view.Domains[j]
		if a.Queries != b.Queries {
			return a.Queries > b.Queries
		}
		return a.Domain < b.Domain
	})
}

// All predicates apply to the same observed request. No filtering of marginal
// totals, sampling from the query-history ring, or inferred associations.
func FilterMonitoringCounts(snapshot DNSMonitoringSnapshot, filter MonitoringFilter) DNSMonitoringSnapshot {
	out := EmptyMonitoringCounts()
	out.StartedAt = snapshot.StartedAt
	out.SessionStartedAt = snapshot.SessionStartedAt
	out.ObservedAt = snapshot.ObservedAt
	out.DimensionsStartedAt = snapshot.DimensionsStartedAt
	out.DimensionsSupported = snapshot.DimensionsSupported
	out.CompletedOnly = true
	sources := map[string]DNSMonitoringSnapshot{}
	flows := map[flowKey]uint64{}
	policies := map[policyKey]uint64{}
	for _, row := range snapshot.Observations {
		if !filter.Matches(row) || row.Other {
			continue
		}
		out.Completed += row.Queries
		out.LatencyMS += row.LatencyMS
		out.Decisions[row.Decision] += row.Queries
		out.ResponseCodes[row.ResponseCode] += row.Queries
		flows[flowKey{row.Ingress, row.Destination}] += row.Queries
		policies[policyKey{policy: row.Policy, final: row.FinalPolicy, destination: row.Destination}] += row.Queries
		child := sources[row.Source]
		if child.Decisions == nil {
			child = EmptyMonitoringCounts()
		}
		child.Received += row.Queries
		child.Completed += row.Queries
		child.LatencyMS += row.LatencyMS
		child.Decisions[row.Decision] += row.Queries
		child.ResponseCodes[row.ResponseCode] += row.Queries
		child.Flows = append(child.Flows, DNSFlow{row.Ingress, row.Destination, row.Queries})
		child.Policies = append(child.Policies, DNSPolicyCount{Policy: row.Policy, FinalPolicy: row.FinalPolicy, Destination: row.Destination, Queries: row.Queries})
		sources[row.Source] = child
	}
	out.Received = out.Completed
	for key, n := range flows {
		out.Flows = append(out.Flows, DNSFlow{key.ingress, key.destination, n})
	}
	for key, n := range policies {
		out.Policies = append(out.Policies, DNSPolicyCount{Policy: key.policy, FinalPolicy: key.final, Destination: key.destination, Queries: n})
	}
	for source, counts := range sources { // Merge repeated paths before exposing source charts.
		merged := EmptyMonitoringCounts()
		AddMonitoringCounts(&merged, counts)
		out.Sources = append(out.Sources, DNSSourceCount{source, merged})
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].Source < out.Sources[j].Source })
	out.SourcePolicyFlows = SourcePolicyFlows(out.Sources)
	AttachMonitoringDomains(&out, snapshot.Observations, filter)
	return out
}

func MonitoringDimensionStart(snapshot *DNSMonitoringSnapshot, at time.Time) {
	if snapshot.DimensionsStartedAt.IsZero() {
		snapshot.DimensionsStartedAt = at
	}
	snapshot.DimensionsSupported = true
}
