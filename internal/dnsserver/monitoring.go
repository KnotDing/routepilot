package dnsserver

import (
	"net/netip"
	"sort"
	"sync"
	"time"

	"surge-controller/internal/model"
)

const monitoringPolicyCapacity = 512
const monitoringSourceCapacity = 512
const monitoringSourcePolicyCapacity = 64

// Flow counts completed client requests by their final DNS handling path.
// It is not a count of wire exchanges, retries, or application traffic.
type DNSFlow struct {
	Ingress     string `json:"ingress"`
	Destination string `json:"destination"`
	Queries     uint64 `json:"queries"`
}

type DNSPolicyCount struct {
	Policy      string `json:"policy"`
	FinalPolicy string `json:"final_policy"`
	Destination string `json:"destination"`
	Other       bool   `json:"other,omitempty"`
	Queries     uint64 `json:"queries"`
}

type DNSSourceCount struct {
	Source string                `json:"source"`
	Counts DNSMonitoringSnapshot `json:"counts"`
}

type DNSMonitoringSnapshot struct {
	StartedAt        time.Time         `json:"started_at"`
	SessionStartedAt time.Time         `json:"session_started_at"`
	ObservedAt       time.Time         `json:"observed_at"`
	Received         uint64            `json:"received"`
	Completed        uint64            `json:"completed"`
	Unconfirmed      uint64            `json:"unconfirmed"`
	InFlight         uint64            `json:"in_flight"`
	ResponseCodes    map[string]uint64 `json:"response_codes"`
	Decisions        map[string]uint64 `json:"decisions"`
	LatencyMS        float64           `json:"latency_ms_sum"`
	Flows            []DNSFlow         `json:"flows"`
	Policies         []DNSPolicyCount  `json:"policies"`
	Sources          []DNSSourceCount  `json:"sources,omitempty"`
}

type policyKey struct {
	policy, final, destination string
	other                      bool
}
type flowKey struct{ ingress, destination string }

// Monitoring is shared across runtime rebuilds and persisted independently of
// query history. Each request contributes exactly one flow and policy pair.
type Monitoring struct {
	mu             sync.Mutex
	state          DNSMonitoringSnapshot
	flows          map[flowKey]uint64
	policies       map[policyKey]uint64
	sources        map[string]*Monitoring
	policyCapacity int
}

func NewMonitoring(saved DNSMonitoringSnapshot) *Monitoring {
	return newMonitoring(saved, true, monitoringPolicyCapacity)
}

func newMonitoring(saved DNSMonitoringSnapshot, trackSources bool, policyCapacity int) *Monitoring {
	now := time.Now().UTC()
	if saved.StartedAt.IsZero() {
		saved.StartedAt = now
	}
	saved.SessionStartedAt = now
	// In-flight requests at the last flush have no persisted completion. They
	// may have finished before the exit; do not report them as proven failures.
	if saved.Received > saved.Completed+saved.Unconfirmed {
		saved.Unconfirmed = saved.Received - saved.Completed
	}
	saved.InFlight = 0
	// Restored global totals and the legacy source must never share mutable
	// maps. A new global request must not change a source that completed none.
	codes, decisions := make(map[string]uint64), make(map[string]uint64)
	for k, v := range saved.ResponseCodes {
		codes[k] = v
	}
	for k, v := range saved.Decisions {
		decisions[k] = v
	}
	saved.ResponseCodes, saved.Decisions = codes, decisions
	m := &Monitoring{state: saved, flows: make(map[flowKey]uint64), policies: make(map[policyKey]uint64), policyCapacity: policyCapacity}
	if trackSources {
		m.sources = map[string]*Monitoring{}
		if len(saved.Sources) == 0 && saved.Received > 0 {
			legacy := saved
			legacy.Sources = nil
			m.sources["legacy"] = newMonitoring(legacy, false, monitoringSourcePolicyCapacity)
		}
		for _, src := range saved.Sources {
			m.sources[src.Source] = newMonitoring(src.Counts, false, monitoringSourcePolicyCapacity)
		}
	}
	for _, f := range saved.Flows {
		m.flows[flowKey{f.Ingress, f.Destination}] += f.Queries
	}
	for _, p := range saved.Policies {
		m.policies[policyKey{p.Policy, p.FinalPolicy, p.Destination, p.Other}] += p.Queries
	}
	m.state.Flows, m.state.Policies, m.state.Sources = nil, nil, nil
	return m
}

func MonitoringSource(ip netip.Addr) string {
	if !ip.IsValid() {
		return "unknown"
	}
	return ip.Unmap().String()
}

func (m *Monitoring) source(ip netip.Addr) *Monitoring {
	key := MonitoringSource(ip)
	if child := m.sources[key]; child != nil {
		return child
	}
	if len(m.sources) >= monitoringSourceCapacity {
		key = "other"
		if child := m.sources[key]; child != nil {
			return child
		}
	}
	child := newMonitoring(DNSMonitoringSnapshot{StartedAt: m.state.StartedAt}, false, monitoringSourcePolicyCapacity)
	m.sources[key] = child
	return child
}
func (m *Monitoring) Begin() { m.BeginSource(netip.Addr{}) }
func (m *Monitoring) BeginSource(ip netip.Addr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Received++
	m.state.InFlight++
	if m.sources != nil {
		child := m.source(ip)
		child.mu.Lock()
		child.state.Received++
		child.state.InFlight++
		child.mu.Unlock()
	}
}

func (m *Monitoring) Finish(intercepted bool, result model.DecisionResult, destination, rcode string, latency time.Duration) {
	m.FinishSource(netip.Addr{}, intercepted, result, destination, rcode, latency)
}

func (m *Monitoring) FinishSource(ip netip.Addr, intercepted bool, result model.DecisionResult, destination, rcode string, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finish(intercepted, result, destination, rcode, latency)
	if m.sources != nil {
		child := m.source(ip)
		child.mu.Lock()
		child.finish(intercepted, result, destination, rcode, latency)
		child.mu.Unlock()
	}
}

func (m *Monitoring) finish(intercepted bool, result model.DecisionResult, destination, rcode string, latency time.Duration) {
	m.state.Completed++
	m.state.InFlight--
	m.state.ResponseCodes[rcode]++
	decision := result.Decision
	switch decision {
	case model.Direct, model.Surge, model.Reject, model.Unknown:
	default:
		decision = model.Unknown
	}
	m.state.Decisions[string(decision)]++
	m.state.LatencyMS += float64(latency) / float64(time.Millisecond)
	ingress := "addressed"
	if intercepted {
		ingress = "intercepted"
	}
	m.flows[flowKey{ingress, destination}]++
	key := policyKey{policy: result.Policy, final: result.FinalPolicy, destination: destination}
	if _, exists := m.policies[key]; !exists && len(m.policies) >= m.policyCapacity {
		key = policyKey{other: true}
	}
	m.policies[key]++
}

func (m *Monitoring) Snapshot() DNSMonitoringSnapshot { return m.snapshot(true) }
func (m *Monitoring) View() DNSMonitoringSnapshot     { return m.snapshot(false) }
func (m *Monitoring) snapshot(details bool) DNSMonitoringSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.ObservedAt = time.Now().UTC()
	s.ResponseCodes, s.Decisions = make(map[string]uint64), make(map[string]uint64)
	for k, v := range m.state.ResponseCodes {
		s.ResponseCodes[k] = v
	}
	for k, v := range m.state.Decisions {
		s.Decisions[k] = v
	}
	s.Flows, s.Policies = []DNSFlow{}, []DNSPolicyCount{}
	for k, v := range m.flows {
		s.Flows = append(s.Flows, DNSFlow{k.ingress, k.destination, v})
	}
	for k, v := range m.policies {
		s.Policies = append(s.Policies, DNSPolicyCount{k.policy, k.final, k.destination, k.other, v})
	}
	sort.Slice(s.Flows, func(i, j int) bool {
		a, b := s.Flows[i], s.Flows[j]
		if a.Ingress != b.Ingress {
			return a.Ingress < b.Ingress
		}
		return a.Destination < b.Destination
	})
	sort.Slice(s.Policies, func(i, j int) bool {
		a, b := s.Policies[i], s.Policies[j]
		if a.Queries != b.Queries {
			return a.Queries > b.Queries
		}
		if a.Other != b.Other {
			return !a.Other
		}
		if a.Policy != b.Policy {
			return a.Policy < b.Policy
		}
		if a.FinalPolicy != b.FinalPolicy {
			return a.FinalPolicy < b.FinalPolicy
		}
		return a.Destination < b.Destination
	})

	for source, child := range m.sources {
		var counts DNSMonitoringSnapshot
		if details {
			counts = child.Snapshot()
		} else {
			child.mu.Lock()
			counts = EmptyMonitoringCounts()
			counts.Received = child.state.Received
			counts.Completed = child.state.Completed
			counts.InFlight = child.state.InFlight
			counts.Unconfirmed = child.state.Unconfirmed
			counts.LatencyMS = child.state.LatencyMS
			for k, v := range child.flows {
				counts.Flows = append(counts.Flows, DNSFlow{k.ingress, k.destination, v})
			}
			child.mu.Unlock()
		}
		counts.StartedAt, counts.ObservedAt, counts.SessionStartedAt = s.StartedAt, s.ObservedAt, s.SessionStartedAt
		s.Sources = append(s.Sources, DNSSourceCount{source, counts})
	}
	sort.Slice(s.Sources, func(i, j int) bool {
		a, b := s.Sources[i], s.Sources[j]
		if a.Counts.Received != b.Counts.Received {
			return a.Counts.Received > b.Counts.Received
		}
		return a.Source < b.Source
	})
	return s
}

func (m *Monitoring) SourceSnapshot(source string) (DNSMonitoringSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	child := m.sources[source]
	if child == nil {
		return EmptyMonitoringCounts(), false
	}
	s := child.Snapshot()
	s.StartedAt, s.ObservedAt, s.SessionStartedAt = m.state.StartedAt, time.Now().UTC(), m.state.SessionStartedAt
	return s, true
}
