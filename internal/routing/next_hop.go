package routing

import (
	"net/netip"
	"surge-controller/internal/config"
	"time"
)

type NextHop4Status struct {
	Automatic       bool      `json:"automatic"`
	Address         string    `json:"address"`
	DetectedAddress string    `json:"detected_address,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
	Error           string    `json:"error,omitempty"`
	Pending         bool      `json:"pending"`
}

func (m *Manager) automaticNextHop4() bool {
	if m.document.Settings.Surge.NextHop4 != "" {
		return false
	}
	for _, prefix := range m.document.Settings.Surge.FakePrefixes {
		if p, err := netip.ParsePrefix(prefix); err == nil && p.Addr().Is4() {
			return true
		}
	}
	return false
}

// Probe at most once per 30 seconds, using the existing health loop. Explicit
// addresses bypass the CLI entirely; unrelated configuration saves keep state.
func (m *Manager) NextHop4ProbeDue(now time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.automaticNextHop4() && (m.nextHop4.CheckedAt.IsZero() || now.Sub(m.nextHop4.CheckedAt) >= 30*time.Second)
}

func (m *Manager) ObserveNextHop4(address string, failure error, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.automaticNextHop4() {
		return
	}
	v := NextHop4Status{Automatic: true, DetectedAddress: address, CheckedAt: now}
	if failure != nil {
		v.Error = failure.Error()
	} else if !validNextHop4(m.document, address) {
		v.Error = "Detected Surge VM IPv4 next hop failed routing safety validation"
	}
	if v.Error != "" {
		v.DetectedAddress = ""
	}
	m.nextHop4 = v
}

func validNextHop4(d config.Document, address string) bool {
	d.Settings.Surge.NextHop4 = address
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range d.Settings.Surge.FakePrefixes {
		if p, err := netip.ParsePrefix(prefix); err == nil && p.Contains(ip) {
			return false
		}
	}
	return d.Validate() == nil
}

func (m *Manager) nextHop4Ready() bool {
	return !m.automaticNextHop4() || m.nextHop4.Error == "" && m.nextHop4.DetectedAddress != "" && m.nextHop4.DetectedAddress == m.autoNextHop4
}

func (m *Manager) NextHop4Ready() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nextHop4Ready()
}

// Called only after Fake-IP issuance is suspended, response writes have drained
// and existing answer TTLs have expired. It never changes the saved setting.
func (m *Manager) ApplyNextHop4() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.automaticNextHop4() {
		m.autoNextHop4 = m.nextHop4.DetectedAddress
	}
}

func (m *Manager) NextHop4() NextHop4Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nextHop4Status()
}

func (m *Manager) nextHop4Status() NextHop4Status {
	if m.document.Settings.Surge.NextHop4 != "" {
		return NextHop4Status{Address: m.document.Settings.Surge.NextHop4}
	}
	v := m.nextHop4
	v.Automatic = true
	v.Address = m.autoNextHop4
	v.Pending = v.DetectedAddress != "" && v.DetectedAddress != v.Address
	return v
}

func (m *Manager) effectiveDocument() config.Document {
	d := m.document
	if m.automaticNextHop4() {
		d.Settings.Surge.NextHop4 = m.autoNextHop4
		// A static declaration is bound to the address the operator confirmed.
		// Detection never installs routes or confirms a changed gateway address.
		if m.autoNextHop4 == "" || d.Settings.Routing.StaticFakeIPNextHop4 != m.autoNextHop4 {
			d.Settings.Routing.StaticFakeIPConfigured = false
		}
	}
	return d
}

func (m *Manager) updateDocument(d config.Document) {
	old, next := m.document.Settings.Surge, d.Settings.Surge
	if old.NextHop4 != next.NextHop4 || old.CLIPath != next.CLIPath || old.UserName != next.UserName {
		m.nextHop4 = NextHop4Status{}
		m.autoNextHop4 = ""
	}
	m.document = d.Clone()
	if m.automaticNextHop4() && m.nextHop4.DetectedAddress != "" && !validNextHop4(d, m.nextHop4.DetectedAddress) {
		m.nextHop4 = NextHop4Status{}
	}
}
