package state

import (
	"github.com/miekg/dns"
	"surge-controller/internal/config"
	"surge-controller/internal/dnsserver"
	"sync"
	"time"
)

const (
	Starting       = "STARTING"
	Recovering     = "RECOVERING"
	Healthy        = "HEALTHY"
	SurgeDegraded  = "SURGE_DEGRADED"
	DirectFallback = "DIRECT_FALLBACK"
	Stopping       = "STOPPING"
)

type Input struct{ Controller, DNS, Surge, FakeIP, Routes bool }
type Plan struct {
	State           string    `json:"state"`
	AllowFakeIP     bool      `json:"allow_fake_ip"`
	DirectFallback  bool      `json:"direct_fallback"`
	KeepSurgeRoutes bool      `json:"keep_surge_routes"`
	Generation      uint64    `json:"generation"`
	DrainUntil      time.Time `json:"drain_until"`
}
type Machine struct {
	mu                                 sync.Mutex
	response                           sync.RWMutex
	config                             config.Health
	mode                               string
	plan                               Plan
	failedAt, recoveredAt, issuedUntil time.Time
	active                             int
	stopping                           bool
}

func New(c config.Health, mode string) *Machine {
	return &Machine{config: c, mode: mode, plan: Plan{State: Starting, Generation: 1}}
}
func (m *Machine) disable() {
	if m.plan.AllowFakeIP {
		m.plan.AllowFakeIP = false
		m.plan.Generation++
	}
}
func (m *Machine) Step(now time.Time, i Input) Plan {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping || !i.Controller {
		m.disable()
		m.plan.State = Stopping
		m.plan.DirectFallback = false
		m.plan.KeepSurgeRoutes = false
		return m.plan
	}
	live := i.DNS && i.Surge && i.FakeIP && m.mode != "force-direct"
	if !live {
		m.disable()
		m.recoveredAt = time.Time{}
		m.plan.DirectFallback = true
		if m.failedAt.IsZero() {
			m.failedAt = now
		}
		deadline := m.failedAt.Add(time.Duration(m.config.GraceSeconds) * time.Second)
		if m.issuedUntil.After(deadline) {
			deadline = m.issuedUntil
		}
		m.plan.DrainUntil = deadline
		if m.plan.KeepSurgeRoutes && (now.Before(deadline) || m.active > 0) {
			m.plan.State = DirectFallback
		} else {
			m.plan.KeepSurgeRoutes = false
			m.plan.State = DirectFallback
		}
		return m.plan
	}
	m.failedAt = time.Time{}
	m.plan.DirectFallback = false
	m.plan.KeepSurgeRoutes = true
	if !i.Routes {
		m.disable()
		m.recoveredAt = time.Time{}
		m.plan.State = Recovering
		return m.plan
	}
	if m.recoveredAt.IsZero() {
		m.recoveredAt = now
	}
	if now.Sub(m.recoveredAt) < time.Duration(m.config.StabilizeSeconds)*time.Second {
		m.disable()
		m.plan.State = Recovering
	} else {
		if !m.plan.AllowFakeIP {
			m.plan.Generation++
		}
		m.plan.AllowFakeIP = true
		m.plan.State = Healthy
	}
	return m.plan
}
func (m *Machine) Plan() Plan { m.mu.Lock(); defer m.mu.Unlock(); return m.plan }
func (m *Machine) SetMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = mode
	m.disable()
	m.recoveredAt = time.Time{}
	if mode == "force-direct" {
		m.plan.DirectFallback = true
		m.failedAt = time.Now()
		if m.plan.KeepSurgeRoutes {
			m.plan.State = DirectFallback
		} else {
			m.plan.State = DirectFallback
		}
	} else {
		m.plan.DirectFallback = false
	}
}

// SuspendRouting invalidates issuance during backend replacement while DNS
// listeners and policy/real resolvers keep running. Existing TTLs still drain.
func (m *Machine) SuspendRouting() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disable()
	m.recoveredAt = time.Time{}
	m.plan.State = Recovering
	return m.issuedUntil
}

func (m *Machine) Snapshot() dnsserver.SafetySnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return dnsserver.SafetySnapshot{AllowFakeIP: m.plan.AllowFakeIP, DirectFallback: m.plan.DirectFallback, Generation: m.plan.Generation}
}
func (m *Machine) Stop() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopping = true
	m.disable()
	m.plan.State = Stopping
	m.plan.DirectFallback = false
	return m.issuedUntil
}

// Every DNS response holds this barrier through socket writing. Route
// withdrawal can only pass DrainResponses after all pre-existing writes finish.
func (m *Machine) BeginResponse()  { m.response.RLock() }
func (m *Machine) EndResponse()    { m.response.RUnlock() }
func (m *Machine) DrainResponses() { m.response.Lock(); m.response.Unlock() }

type lease struct {
	machine    *Machine
	generation uint64
	once       sync.Once
}

func (m *Machine) AcquireFakeIP() (dnsserver.IssueLease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.plan.AllowFakeIP || m.stopping {
		return nil, false
	}
	m.active++
	return &lease{machine: m, generation: m.plan.Generation}, true
}
func (l *lease) Valid() bool {
	m := l.machine
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plan.AllowFakeIP && !m.stopping && m.plan.Generation == l.generation
}
func (l *lease) Observe(reply *dns.Msg) {
	m := l.machine
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, rr := range reply.Answer {
		if rr.Header().Rrtype == dns.TypeA || rr.Header().Rrtype == dns.TypeAAAA {
			expiry := now.Add(time.Duration(rr.Header().Ttl) * time.Second)
			if expiry.After(m.issuedUntil) {
				m.issuedUntil = expiry
			}
		}
	}
}
func (l *lease) Release() {
	l.once.Do(func() { m := l.machine; m.mu.Lock(); m.active--; m.mu.Unlock() })
}

type Component struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	LastSuccess time.Time `json:"last_success"`
	LastFailure time.Time `json:"last_failure"`
	LatencyMS   float64   `json:"latency_ms"`
	Successes   int       `json:"successes"`
	Failures    int       `json:"failures"`
	LastError   string    `json:"last_error"`
}
type Health struct {
	mu                                 sync.Mutex
	components                         map[string]Component
	successThreshold, failureThreshold int
}

func NewHealth(successes, failures int) *Health {
	return &Health{components: map[string]Component{}, successThreshold: successes, failureThreshold: failures}
}
func (h *Health) Observe(name string, ok bool, latency time.Duration, message string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.components[name]
	c.Name = name
	if c.State == "" {
		c.State = "UNKNOWN"
	}
	c.LatencyMS = float64(latency) / float64(time.Millisecond)
	if ok {
		c.Successes++
		c.Failures = 0
		c.LastSuccess = time.Now()
		c.LastError = ""
		if c.Successes >= h.successThreshold {
			c.State = "UP"
		}
	} else {
		c.Failures++
		c.Successes = 0
		c.LastFailure = time.Now()
		c.LastError = message
		if c.Failures >= h.failureThreshold {
			c.State = "DOWN"
		}
	}
	h.components[name] = c
	return c.State == "UP"
}
func (h *Health) Values() []Component {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Component{}
	for _, v := range h.components {
		out = append(out, v)
	}
	return out
}
func (h *Health) Disabled(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.components[name] = Component{Name: name, State: "DISABLED"}
}

// Remove checks that no longer exist in the configuration. Optional policy
// probes can otherwise accumulate indefinitely as names are edited.
func (h *Health) Remove(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.components, name)
}
