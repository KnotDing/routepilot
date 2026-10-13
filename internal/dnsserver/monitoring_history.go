package dnsserver

import (
	"fmt"
	"sort"
	"time"
)

// MonitoringBucket stores interval deltas, not repeated cumulative snapshots.
// Seconds measures observed time; gaps and process boundaries have no rate.
type MonitoringBucket struct {
	From      time.Time             `json:"from"`
	To        time.Time             `json:"to"`
	Seconds   float64               `json:"seconds"`
	RateValid bool                  `json:"rate_valid"`
	Counts    DNSMonitoringSnapshot `json:"counts"`
}

type MonitoringHistory struct {
	From              time.Time             `json:"from"`
	To                time.Time             `json:"to"`
	RetentionDays     int                   `json:"retention_days"`
	ResolutionSeconds int64                 `json:"resolution_seconds"`
	Points            []MonitoringBucket    `json:"points"`
	Summary           DNSMonitoringSnapshot `json:"summary"`
	Sources           []DNSSourceCount      `json:"sources"`
	SourcePolicyFlows []DNSSourcePolicyFlow `json:"source_policy_flows"`
	TotalReceived     uint64                `json:"total_received"`
}

func EmptyMonitoringCounts() DNSMonitoringSnapshot {
	return DNSMonitoringSnapshot{Decisions: map[string]uint64{}, ResponseCodes: map[string]uint64{}, Flows: []DNSFlow{}, Policies: []DNSPolicyCount{}}
}

func MonitoringDifference(current, previous DNSMonitoringSnapshot) (DNSMonitoringSnapshot, error) {
	d := EmptyMonitoringCounts()
	d.DimensionsSupported = current.DimensionsSupported
	d.DimensionsStartedAt = current.DimensionsStartedAt
	var dimensionErr error
	d.Observations, dimensionErr = differenceObservations(current.Observations, previous.Observations)
	if dimensionErr != nil {
		return d, dimensionErr
	}
	if current.Received < previous.Received || current.Completed < previous.Completed || current.Unconfirmed < previous.Unconfirmed || current.LatencyMS < previous.LatencyMS {
		return d, fmt.Errorf("monitoring counters regressed")
	}
	d.Received, d.Completed, d.Unconfirmed = current.Received-previous.Received, current.Completed-previous.Completed, current.Unconfirmed-previous.Unconfirmed
	var observationsTotal uint64
	for _, row := range d.Observations {
		observationsTotal += row.Queries
	}
	if observationsTotal > d.Completed {
		return d, fmt.Errorf("monitoring request dimensions exceed completed requests")
	}
	d.InFlight, d.LatencyMS = current.InFlight, current.LatencyMS-previous.LatencyMS
	for _, pair := range []struct{ current, previous, dst map[string]uint64 }{{current.Decisions, previous.Decisions, d.Decisions}, {current.ResponseCodes, previous.ResponseCodes, d.ResponseCodes}} {
		for k, v := range pair.previous {
			if pair.current[k] < v {
				return d, fmt.Errorf("monitoring dimension regressed")
			}
		}
		for k, v := range pair.current {
			if n := v - pair.previous[k]; n > 0 {
				pair.dst[k] = n
			}
		}
	}
	flows := map[flowKey]uint64{}
	for _, f := range previous.Flows {
		flows[flowKey{f.Ingress, f.Destination}] = f.Queries
	}
	for _, f := range current.Flows {
		old := flows[flowKey{f.Ingress, f.Destination}]
		if f.Queries < old {
			return d, fmt.Errorf("monitoring flow regressed")
		}
		if n := f.Queries - old; n > 0 {
			f.Queries = n
			d.Flows = append(d.Flows, f)
		}
	}
	policies := map[policyKey]uint64{}
	for _, p := range previous.Policies {
		policies[policyKey{p.Policy, p.FinalPolicy, p.Destination, p.Other}] = p.Queries
	}
	for _, p := range current.Policies {
		old := policies[policyKey{p.Policy, p.FinalPolicy, p.Destination, p.Other}]
		if p.Queries < old {
			return d, fmt.Errorf("monitoring policy regressed")
		}
		if n := p.Queries - old; n > 0 {
			p.Queries = n
			d.Policies = append(d.Policies, p)
		}
	}

	var flowsTotal, policiesTotal, decisionsTotal, codesTotal uint64
	for _, f := range d.Flows {
		flowsTotal += f.Queries
	}
	for _, p := range d.Policies {
		policiesTotal += p.Queries
	}
	for _, n := range d.Decisions {
		decisionsTotal += n
	}
	for _, n := range d.ResponseCodes {
		codesTotal += n
	}
	if flowsTotal != d.Completed || policiesTotal != d.Completed || decisionsTotal != d.Completed || codesTotal != d.Completed {
		return d, fmt.Errorf("monitoring dimensions do not conserve completed requests")
	}
	previousSources := map[string]DNSMonitoringSnapshot{}
	for _, src := range previous.Sources {
		previousSources[src.Source] = src.Counts
	}
	for _, src := range current.Sources {
		counts, err := MonitoringDifference(src.Counts, previousSources[src.Source])
		if err != nil {
			return d, err
		}
		if counts.Received > 0 || counts.Completed > 0 || counts.Unconfirmed > 0 || counts.InFlight > 0 || src.Counts.InFlight != previousSources[src.Source].InFlight {
			d.Sources = append(d.Sources, DNSSourceCount{src.Source, counts})
		}
	}
	return d, nil
}

func AddMonitoringCounts(dst *DNSMonitoringSnapshot, src DNSMonitoringSnapshot) {
	dst.CompletedOnly = dst.CompletedOnly || src.CompletedOnly
	dst.DimensionCompleted += src.DimensionCompleted
	dst.DimensionOverflow += src.DimensionOverflow
	dst.Observations = addObservations(dst.Observations, src.Observations)
	dst.DimensionsSupported = dst.DimensionsSupported || src.DimensionsSupported
	if !src.DimensionsStartedAt.IsZero() && (dst.DimensionsStartedAt.IsZero() || src.DimensionsStartedAt.Before(dst.DimensionsStartedAt)) {
		dst.DimensionsStartedAt = src.DimensionsStartedAt
	}
	dst.Received += src.Received
	dst.Completed += src.Completed
	dst.Unconfirmed += src.Unconfirmed
	dst.LatencyMS += src.LatencyMS
	dst.InFlight = src.InFlight
	if dst.Decisions == nil {
		dst.Decisions = map[string]uint64{}
	}
	if dst.ResponseCodes == nil {
		dst.ResponseCodes = map[string]uint64{}
	}
	for k, v := range src.Decisions {
		dst.Decisions[k] += v
	}
	for k, v := range src.ResponseCodes {
		dst.ResponseCodes[k] += v
	}
	flows := map[flowKey]uint64{}
	for _, list := range [][]DNSFlow{dst.Flows, src.Flows} {
		for _, f := range list {
			flows[flowKey{f.Ingress, f.Destination}] += f.Queries
		}
	}
	dst.Flows = []DNSFlow{}
	for k, v := range flows {
		dst.Flows = append(dst.Flows, DNSFlow{k.ingress, k.destination, v})
	}
	sort.Slice(dst.Flows, func(i, j int) bool {
		a, b := dst.Flows[i], dst.Flows[j]
		if a.Ingress != b.Ingress {
			return a.Ingress < b.Ingress
		}
		return a.Destination < b.Destination
	})
	policies := map[policyKey]uint64{}
	for _, list := range [][]DNSPolicyCount{dst.Policies, src.Policies} {
		for _, p := range list {
			policies[policyKey{p.Policy, p.FinalPolicy, p.Destination, p.Other}] += p.Queries
		}
	}
	dst.Policies = []DNSPolicyCount{}
	for k, v := range policies {
		dst.Policies = append(dst.Policies, DNSPolicyCount{k.policy, k.final, k.destination, k.other, v})
	}
	sort.Slice(dst.Policies, func(i, j int) bool {
		a, b := dst.Policies[i], dst.Policies[j]
		if a.Queries != b.Queries {
			return a.Queries > b.Queries
		}
		if a.Policy != b.Policy {
			return a.Policy < b.Policy
		}
		if a.FinalPolicy != b.FinalPolicy {
			return a.FinalPolicy < b.FinalPolicy
		}
		if a.Destination != b.Destination {
			return a.Destination < b.Destination
		}
		return !a.Other && b.Other
	})
	if len(src.Sources) > 0 {
		sources := map[string]DNSMonitoringSnapshot{}
		for _, src := range dst.Sources {
			sources[src.Source] = src.Counts
		}
		for _, src := range src.Sources {
			counts := sources[src.Source]
			AddMonitoringCounts(&counts, src.Counts)
			sources[src.Source] = counts
		}
		dst.Sources = nil
		for source, counts := range sources {
			dst.Sources = append(dst.Sources, DNSSourceCount{source, counts})
		}
		sort.Slice(dst.Sources, func(i, j int) bool {
			a, b := dst.Sources[i], dst.Sources[j]
			if a.Counts.Received != b.Counts.Received {
				return a.Counts.Received > b.Counts.Received
			}
			return a.Source < b.Source
		})

	}
}

func MergeMonitoringBucket(dst *MonitoringBucket, src MonitoringBucket) {
	if dst.From.IsZero() {
		*dst = src
		return
	}
	if src.From.Sub(dst.To) > time.Second {
		dst.RateValid = false
	}
	dst.RateValid = dst.RateValid && src.RateValid
	dst.Seconds += src.Seconds
	if src.From.Before(dst.From) {
		dst.From = src.From
	}
	if src.To.After(dst.To) {
		dst.To = src.To
	}
	AddMonitoringCounts(&dst.Counts, src.Counts)
}
