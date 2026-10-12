package dnsserver

import (
	"sort"
	"sync"
	"time"

	"surge-controller/internal/model"
)

const monitoringPolicyCapacity = 512

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
}

type policyKey struct {
	policy, final, destination string
	other                      bool
}
type flowKey struct{ ingress, destination string }

// Monitoring is shared across runtime rebuilds and persisted independently of
// query history. Each request contributes exactly one flow and policy pair.
type Monitoring struct {
	mu       sync.Mutex
	state    DNSMonitoringSnapshot
	flows    map[flowKey]uint64
	policies map[policyKey]uint64
}

func NewMonitoring(saved DNSMonitoringSnapshot) *Monitoring {
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
	if saved.ResponseCodes == nil {
		saved.ResponseCodes = make(map[string]uint64)
	}
	if saved.Decisions == nil {
		saved.Decisions = make(map[string]uint64)
	}
	m := &Monitoring{state: saved, flows: make(map[flowKey]uint64), policies: make(map[policyKey]uint64)}
	for _, f := range saved.Flows {
		m.flows[flowKey{f.Ingress, f.Destination}] += f.Queries
	}
	for _, p := range saved.Policies {
		m.policies[policyKey{p.Policy, p.FinalPolicy, p.Destination, p.Other}] += p.Queries
	}
	m.state.Flows, m.state.Policies = nil, nil
	return m
}

func (m *Monitoring) Begin() {
	m.mu.Lock()
	m.state.Received++
	m.state.InFlight++
	m.mu.Unlock()
}

func (m *Monitoring) Finish(intercepted bool, result model.DecisionResult, destination, rcode string, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	if _, exists := m.policies[key]; !exists && len(m.policies) >= monitoringPolicyCapacity {
		key = policyKey{other: true}
	}
	m.policies[key]++
}

func (m *Monitoring) Snapshot() DNSMonitoringSnapshot {
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
	return s
}
