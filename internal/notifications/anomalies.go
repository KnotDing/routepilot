package notifications

import (
	"fmt"
	"sync"
	"time"

	"surge-controller/internal/cache"
	"surge-controller/internal/config"
	"surge-controller/internal/state"
)

const (
	DecisionMismatch = "DNS cache mismatch decision"
	ResponseMismatch = "DNS cache mismatch response"
	RequestBurst     = "DNS request burst"
	RequestSustained = "DNS sustained requests"
)

func IsAnomaly(name string) bool {
	return name == DecisionMismatch || name == ResponseMismatch || name == RequestBurst || name == RequestSustained
}

type AnomalySample struct {
	At                               time.Time
	Queries                          uint64
	Decision, Response               cache.OptimisticStats
	DecisionEnabled, ResponseEnabled bool
}
type counterPoint struct {
	at                time.Time
	total, mismatches uint64
}
type counterWindow struct{ points []counterPoint }

func (w *counterWindow) observe(p counterPoint, window time.Duration) (uint64, uint64, time.Duration, bool) {
	if n := len(w.points); n > 0 {
		last := w.points[n-1]
		if !p.at.After(last.at) || p.at.Sub(last.at) > 5*time.Second || p.total < last.total || p.mismatches < last.mismatches {
			w.points = nil
		}
	}
	if p.mismatches > p.total {
		w.points = nil
		return 0, 0, 0, false
	}
	w.points = append(w.points, p)
	cutoff := p.at.Add(-window)
	for len(w.points) > 1 && !w.points[1].at.After(cutoff) {
		w.points = w.points[1:]
	}
	if len(w.points) > 7202 {
		w.points = []counterPoint{p}
	}
	first := w.points[0]
	elapsed := p.at.Sub(first.at)
	total, mismatches := p.total-first.total, p.mismatches-first.mismatches
	if mismatches > total {
		w.points = []counterPoint{p}
		return 0, 0, 0, false
	}
	return total, mismatches, elapsed, elapsed >= window
}

// Samples are process-local deltas. Persisted totals never become startup traffic.
// This detector only produces notification states; it does not drive routing health.
type AnomalyMonitor struct {
	mu                        sync.Mutex
	settings                  config.NotificationAnomalies
	decision, response, burst counterWindow
	previous                  AnomalySample
	sustainedAt               time.Time
	values                    []state.Component
}

func (m *AnomalyMonitor) Snapshot() []state.Component {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]state.Component(nil), m.values...)
}

func (m *AnomalyMonitor) ResetCaches() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decision = counterWindow{}
	m.response = counterWindow{}
	for i := range m.values {
		if (m.values[i].Name == DecisionMismatch || m.values[i].Name == ResponseMismatch) && m.values[i].State != "DISABLED" {
			m.values[i].State = "UNKNOWN"
			m.values[i].LastError = ""
		}
	}
}

func (m *AnomalyMonitor) Observe(s config.Notifications, sample AnomalySample) []state.Component {
	m.mu.Lock()
	defer m.mu.Unlock()
	options := s.Anomalies.Effective()
	if options != m.settings {
		m.decision = counterWindow{}
		m.response = counterWindow{}
		m.burst = counterWindow{}
		m.previous = AnomalySample{}
		m.sustainedAt = time.Time{}
		m.settings = options
	}
	cacheAlert := func(name string, enabled bool, stats cache.OptimisticStats, w *counterWindow) state.Component {
		c := state.Component{Name: name, State: "DISABLED"}
		if !s.Enabled || !s.Selected(name) || !enabled {
			w.points = nil
			return c
		}
		c.State = "UNKNOWN"
		compared, mismatches, elapsed, ready := w.observe(counterPoint{sample.At, stats.Compared, stats.Mismatches}, time.Duration(options.CacheWindowSeconds)*time.Second)
		if !ready || compared < uint64(options.CacheMinimumCompared) {
			return c
		}
		c.State = "UP"
		percent := float64(mismatches) / float64(compared) * 100
		if percent >= options.CacheMismatchPercent {
			c.State = "DOWN"
			c.LastError = fmt.Sprintf("%.2f%% ≥ %.2f%% · %d / %d · %.0fs", percent, options.CacheMismatchPercent, mismatches, compared, elapsed.Seconds())
		}
		return c
	}
	values := []state.Component{cacheAlert(DecisionMismatch, sample.DecisionEnabled, sample.Decision, &m.decision), cacheAlert(ResponseMismatch, sample.ResponseEnabled, sample.Response, &m.response)}
	burst := state.Component{Name: RequestBurst, State: "DISABLED"}
	if s.Enabled && s.Selected(RequestBurst) {
		burst.State = "UNKNOWN"
		count, _, elapsed, ready := m.burst.observe(counterPoint{at: sample.At, total: sample.Queries}, time.Duration(options.BurstWindowSeconds)*time.Second)
		if ready {
			burst.State = "UP"
			qps := float64(count) / elapsed.Seconds()
			if qps >= float64(options.BurstQPS) {
				burst.State = "DOWN"
				burst.LastError = fmt.Sprintf("%.1f QPS ≥ %d QPS · %.0fs", qps, options.BurstQPS, elapsed.Seconds())
			}
		}
	} else {
		m.burst.points = nil
	}
	sustained := state.Component{Name: RequestSustained, State: "DISABLED"}
	if s.Enabled && s.Selected(RequestSustained) {
		sustained.State = "UNKNOWN"
		elapsed := sample.At.Sub(m.previous.At)
		if !m.previous.At.IsZero() && elapsed > 0 && elapsed <= 5*time.Second && sample.Queries >= m.previous.Queries {
			qps := float64(sample.Queries-m.previous.Queries) / elapsed.Seconds()
			sustained.State = "UP"
			if qps >= float64(options.SustainedQPS) {
				sustained.State = "UNKNOWN"
				if m.sustainedAt.IsZero() {
					m.sustainedAt = m.previous.At
				}
				if duration := sample.At.Sub(m.sustainedAt); duration >= time.Duration(options.SustainedSeconds)*time.Second {
					sustained.State = "DOWN"
					sustained.LastError = fmt.Sprintf("%.1f QPS ≥ %d QPS · %.0fs", qps, options.SustainedQPS, duration.Seconds())
				}
			} else {
				m.sustainedAt = time.Time{}
			}
		} else {
			m.sustainedAt = time.Time{}
		}
	} else {
		m.sustainedAt = time.Time{}
	}
	if s.Enabled && s.Selected(RequestSustained) {
		m.previous = sample
	} else {
		m.previous = AnomalySample{}
	}
	m.values = append(values, burst, sustained)
	return append([]state.Component(nil), m.values...)
}
