package bgp

import (
	"context"
	"encoding/json"
	"errors"
	"surge-controller/internal/config"
	"sync"
)

type installed struct {
	Route Advertisement
	UUID  []byte
}
type Status struct {
	Enabled    bool            `json:"enabled"`
	Advertised int             `json:"advertised"`
	Limit      int             `json:"limit"`
	Peers      []PeerStatus    `json:"peers"`
	Routes     []Advertisement `json:"routes"`
	LastError  string          `json:"last_error"`
}
type Manager struct {
	mu             sync.Mutex
	speaker        Speaker
	limit          int
	installed      map[string]installed
	notify         func(string, string)
	status         Status
	reconcileError string
}

func NewManager(s Speaker, limit int, notify func(string, string)) *Manager {
	return &Manager{speaker: s, limit: limit, installed: map[string]installed{}, notify: notify, status: Status{Enabled: s != nil, Limit: limit, Peers: []PeerStatus{}, Routes: []Advertisement{}}}
}

// SyncPeers keeps the current speaker and its installed routes while applying
// peer-only configuration changes. Speakers without this capability fall back
// to the normal runtime replacement path.
func (m *Manager) SyncPeers(ctx context.Context, d config.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.speaker == nil {
		return nil
	}
	updater, ok := m.speaker.(interface {
		SyncPeers(context.Context, config.Document) error
	})
	if !ok {
		return errors.New("BGP speaker does not support in-place peer updates")
	}
	return updater.SyncPeers(ctx, d)
}

func (m *Manager) Reconcile(ctx context.Context, desired []Advertisement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(desired) > m.limit {
		m.reconcileError = "hard BGP route limit exceeded"
		m.status.LastError = m.reconcileError
		if m.notify != nil {
			m.notify("CRITICAL", "Hard BGP route limit exceeded; advertisements rejected")
		}
		return errors.New("hard BGP route limit exceeded")
	}
	if m.speaker == nil {
		return nil
	}
	m.reconcileError = ""
	want := map[string]Advertisement{}
	for _, r := range desired {
		if _, ok := want[r.Prefix]; ok {
			m.reconcileError = "duplicate advertised prefix"
			m.status.LastError = m.reconcileError
			return errors.New(m.reconcileError)
		}
		want[r.Prefix] = r
	}
	for prefix, old := range m.installed {
		next, exists := want[prefix]
		a, _ := json.Marshal(old.Route)
		b, _ := json.Marshal(next)
		if !exists || string(a) != string(b) {
			if e := m.speaker.Delete(ctx, old.UUID); e != nil {
				m.reconcileError = "BGP withdrawal failed"
				m.status.LastError = m.reconcileError
				return e
			}
			delete(m.installed, prefix)
			if m.notify != nil {
				m.notify("INFO", "BGP advertisement withdrawn: "+prefix)
			}
		}
	}
	for prefix, r := range want {
		if _, exists := m.installed[prefix]; exists {
			continue
		}
		if len(m.installed) >= m.limit {
			m.reconcileError = "hard BGP route limit exceeded"
			m.status.LastError = m.reconcileError
			return errors.New(m.reconcileError)
		}
		id, e := m.speaker.Add(ctx, r)
		if e != nil {
			m.reconcileError = "BGP advertisement failed"
			m.status.LastError = m.reconcileError
			return e
		}
		m.installed[prefix] = installed{r, id}
		if m.notify != nil {
			m.notify("INFO", "BGP route added to local RIB: "+prefix)
		}
	}
	return nil
}
func (m *Manager) Refresh(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Routes = []Advertisement{}
	m.status.Advertised = 0
	if m.speaker == nil {
		return false, nil
	}
	peers, e := m.speaker.Peers(ctx)
	if e != nil {
		m.status.LastError = "BGP peer inspection failed"
		return false, e
	}
	ready := len(peers) > 0
	exported := []map[string]bool{}
	for i, p := range peers {
		if p.State != "ESTABLISHED" {
			ready = false
			continue
		}
		paths, e := m.speaker.Exported(ctx, p.Address)
		if e != nil {
			m.status.LastError = "BGP export inspection failed"
			return false, e
		}
		exported = append(exported, paths)
		peers[i].Prefixes = len(paths)
	}
	m.status.Peers = peers
	fakeCount := 0
	for prefix, r := range m.installed {
		row := r.Route
		row.Installed = true
		row.Announced = len(exported) > 0
		for _, p := range exported {
			if !p[prefix] {
				row.Announced = false
			}
		}
		if row.Announced {
			m.status.Advertised++
		}
		if row.Kind == "fake-ip" {
			fakeCount++
			ready = ready && row.Announced
		}
		if row.Kind == "dns-interception" {
			ready = ready && row.Announced
		}
		m.status.Routes = append(m.status.Routes, row)
	}
	m.status.LastError = m.reconcileError
	return ready && fakeCount > 0, nil
}
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.status
	v.Peers = append([]PeerStatus{}, v.Peers...)
	v.Routes = append([]Advertisement{}, v.Routes...)
	return v
}
func (m *Manager) Close(ctx context.Context) error {
	if e := m.Reconcile(ctx, nil); e != nil {
		if m.speaker != nil {
			_ = m.speaker.Close()
		}
		return e
	}
	if m.speaker != nil {
		return m.speaker.Close()
	}
	return nil
}
