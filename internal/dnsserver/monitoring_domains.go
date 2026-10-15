package dnsserver

import "time"

// Only pending writes live in memory. A successful checkpoint releases these
// identities; lifetime domain cardinality is handled by the database.
const monitoringDomainBufferCapacity = 4096
const monitoringDomainFlushThreshold = monitoringDomainBufferCapacity / 2

type DomainCountKey struct {
	Source string `json:"source"`
	Domain string `json:"domain"`
	Other  bool   `json:"other,omitempty"`
}
type DomainCount struct {
	DomainCountKey
	Queries uint64 `json:"queries"`
}
type DomainBatch struct {
	Session    time.Time     `json:"session"`
	Sequence   uint64        `json:"sequence"`
	StartedAt  time.Time     `json:"started_at"`
	ObservedAt time.Time     `json:"observed_at"`
	Rows       []DomainCount `json:"rows"`
}
type DNSDomainStatistics struct {
	StartedAt  time.Time `json:"started_at"`
	Completed  uint64    `json:"completed"`
	Unrecorded uint64    `json:"unrecorded"`
}

func (m *Monitoring) recordDomain(source, domain string) {
	key := DomainCountKey{Source: source, Domain: domain}
	if _, ok := m.domainBuffer[key]; !ok && len(m.domainBuffer) >= monitoringDomainBufferCapacity {
		// Preserve the visible source and denominator if storage cannot keep up.
		key = DomainCountKey{Source: source, Other: true}
	}
	_, exists := m.domainBuffer[key]
	m.domainBuffer[key]++
	m.domainVersion++
	if !exists && len(m.domainBuffer) == monitoringDomainFlushThreshold {
		select {
		case m.domainWake <- struct{}{}:
		default:
		}
	}
}
func (m *Monitoring) DomainWake() <-chan struct{} { return m.domainWake }

// The frozen batch stays available for retry while requests use a fresh buffer.
func (m *Monitoring) PendingDomains() *DomainBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.domainPending == nil && (len(m.domainBuffer) > 0 || m.domainSequence == 0) {
		m.domainSequence++
		m.domainPending = &DomainBatch{Session: m.state.SessionStartedAt, Sequence: m.domainSequence, StartedAt: m.domainStartedAt, ObservedAt: time.Now().UTC()}
		for key, n := range m.domainBuffer {
			m.domainPending.Rows = append(m.domainPending.Rows, DomainCount{key, n})
		}
		m.domainBuffer = make(map[DomainCountKey]uint64)
	}
	return m.domainPending
}
func (m *Monitoring) CommitDomains(batch *DomainBatch) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if batch != nil && m.domainPending == batch {
		m.domainPending = nil
	}
	if len(m.domainBuffer) >= monitoringDomainFlushThreshold {
		select {
		case m.domainWake <- struct{}{}:
		default:
		}
	}
}

// Callers serialize database commit + acknowledgement with reads, so pending
// writes and committed rows can never be included twice or omitted during a save.
func (m *Monitoring) BufferedDomains() DomainBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	batch := DomainBatch{StartedAt: m.domainStartedAt, ObservedAt: time.Now().UTC()}
	if m.domainPending != nil {
		batch.Rows = append(batch.Rows, m.domainPending.Rows...)
	}
	for key, n := range m.domainBuffer {
		batch.Rows = append(batch.Rows, DomainCount{key, n})
	}
	return batch
}

func (m *Monitoring) DomainVersion() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.domainVersion
}
