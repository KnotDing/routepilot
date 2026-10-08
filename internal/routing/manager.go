// Package routing is the only routing-dependent contract used by DNS health
// and state control. Static routing is a declaration, not a router API.
package routing

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"surge-controller/internal/bgp"
	"surge-controller/internal/config"
	"sync"
	"time"
)

type Backend interface {
	Reconcile(context.Context, []bgp.Advertisement) error
	Refresh(context.Context) (bool, error)
	Status() bgp.Status
	Close(context.Context) error
}
type Route struct {
	bgp.Advertisement
	Installed         *bool         `json:"installed"` // nil means external installation is unknown
	Backend           string        `json:"backend"`
	Managed           bool          `json:"managed"`
	Declared          bool          `json:"declared"`
	OperatorConfirmed bool          `json:"operator_confirmed"`
	Verified          bool          `json:"verified"`
	State             string        `json:"route_state"`
	Guidance          string        `json:"guidance,omitempty"`
	Connectivity      *Verification `json:"connectivity,omitempty"`
}
type Verification struct {
	Prefix    string    `json:"prefix"`
	Target    string    `json:"target"`
	Port      int       `json:"port"`
	Success   bool      `json:"success"`
	CheckedAt time.Time `json:"checked_at"`
	Scope     string    `json:"scope"`
}
type Status struct {
	BGPRequested         bool             `json:"bgp_requested"`
	Mode                 string           `json:"mode"`
	FakeIPBackend        string           `json:"fake_ip_backend"`
	DNSBackend           string           `json:"dns_backend"`
	Enabled              bool             `json:"enabled"` // BGP engine, not DNS availability
	Advertised           int              `json:"advertised"`
	Limit                int              `json:"limit"`
	Peers                []bgp.PeerStatus `json:"peers"`
	Routes               []Route          `json:"routes"`
	LastError            string           `json:"last_error"`
	FakeIPReady          bool             `json:"fake_ip_ready"`
	DNSInterceptionReady bool             `json:"dns_interception_ready"`
	Limitations          []string         `json:"limitations"`
}
type Manager struct {
	mu            sync.RWMutex
	document      config.Document
	bgp           Backend
	startupError  string
	verifications map[string]Verification
	snapshot      Status
	dial          func(context.Context, string, string) (net.Conn, error)
}

func New(ctx context.Context, d config.Document, notify func(string, string)) *Manager {
	var backend Backend
	failure := ""
	if d.Settings.Routing.UsesBGP() && d.Settings.BGP.Enabled {
		speaker, e := bgp.New(ctx, d)
		if e != nil {
			failure = "BGP startup failed; DNS service remains available"
			if notify != nil {
				notify("ERROR", failure)
			}
		} else {
			backend = bgp.NewManager(speaker, d.Settings.BGP.RouteLimit, notify)
		}
	}
	return newManager(d, backend, failure)
}
func newManager(d config.Document, backend Backend, failure string) *Manager {
	m := &Manager{document: d.Clone(), bgp: backend, startupError: failure, verifications: map[string]Verification{}}
	m.dial = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	m.snapshot = m.makeStatus(bgp.Status{Limit: d.Settings.BGP.RouteLimit, Peers: []bgp.PeerStatus{}, Routes: []bgp.Advertisement{}})
	return m
}
func Definitions(d config.Document) []bgp.Advertisement {
	rows := []bgp.Advertisement{}
	for _, prefix := range d.Settings.Surge.FakePrefixes {
		next := d.Settings.Surge.NextHop4
		if netip.MustParsePrefix(prefix).Addr().Is6() {
			next = d.Settings.Surge.NextHop6
		}
		rows = append(rows, bgp.Advertisement{Prefix: prefix, NextHop: next, Kind: "fake-ip", Binding: "surge", LocalPreference: 100})
	}
	for _, r := range d.Interception {
		if !r.Enabled {
			continue
		}
		next := d.Settings.DNS.Source4
		if netip.MustParsePrefix(r.Prefix).Addr().Is6() {
			next = d.Settings.DNS.Source6
		}
		rows = append(rows, bgp.Advertisement{Prefix: r.Prefix, NextHop: next, Kind: "dns-interception", Binding: "controller", LocalPreference: 100})
	}
	for _, r := range d.Routes {
		if r.Enabled {
			rows = append(rows, bgp.Advertisement{Prefix: r.Prefix, NextHop: r.NextHop, Kind: "manual", Binding: r.Binding, MED: r.MED, LocalPreference: r.LocalPreference, Communities: r.Communities})
		}
	}
	return rows
}
func (m *Manager) Desired(controller, dns, surge bool) []bgp.Advertisement {
	s := m.document.Settings
	r := s.Routing.Effective()
	out := []bgp.Advertisement{}
	if m.bgp == nil {
		return out
	}
	for _, row := range Definitions(m.document) {
		switch row.Kind {
		case "fake-ip":
			if r.FakeIPBackend != "bgp" || !surge {
				continue
			}
		case "dns-interception":
			if r.DNSBackend != "bgp" || !s.DNS.InterceptionEnabled || !controller {
				continue
			}
		case "manual":
			if row.Binding == "controller" && !controller || row.Binding == "dns" && !dns || row.Binding == "surge" && !surge {
				continue
			}
		}
		out = append(out, row)
	}
	return out
}
func (m *Manager) Reconcile(ctx context.Context, desired []bgp.Advertisement) error {
	if m.bgp == nil {
		return nil
	}
	r := m.document.Settings.Routing.Effective()
	for _, row := range desired {
		if row.Kind == "fake-ip" && r.FakeIPBackend != "bgp" || row.Kind == "dns-interception" && r.DNSBackend != "bgp" {
			return errors.New("external static routes cannot be sent to BGP")
		}
	}
	return m.bgp.Reconcile(ctx, desired)
}

// SyncPeers updates only BGP neighbor configuration in place and leaves the
// local RIB untouched. The caller serializes this with routing refreshes.
func (m *Manager) SyncPeers(ctx context.Context, d config.Document) error {
	if m.bgp != nil {
		updater, ok := m.bgp.(interface {
			SyncPeers(context.Context, config.Document) error
		})
		if !ok {
			return errors.New("BGP backend does not support in-place peer updates")
		}
		if err := updater.SyncPeers(ctx, d); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.document = d.Clone()
	m.mu.Unlock()
	return nil
}

// UpdateDocument replaces route declarations without recreating the speaker.
// The app holds routingMu while calling this and the next reconciliation adds
// or withdraws only the changed paths from the existing BGP session.
func (m *Manager) UpdateDocument(d config.Document) {
	m.document = d.Clone()
}

func (m *Manager) Refresh(ctx context.Context) (bool, error) {
	var err error
	s := bgp.Status{Limit: m.document.Settings.BGP.RouteLimit, Peers: []bgp.PeerStatus{}, Routes: []bgp.Advertisement{}}
	if m.bgp != nil {
		_, err = m.bgp.Refresh(ctx)
		s = m.bgp.Status()
		if err != nil {
			s.LastError = "BGP inspection failed"
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot = m.makeStatus(s)
	return m.snapshot.FakeIPReady, err
}
func (m *Manager) makeStatus(s bgp.Status) Status {
	cfg := m.document.Settings
	r := cfg.Routing.Effective()
	v := Status{BGPRequested: r.UsesBGP() && cfg.BGP.Enabled, Mode: r.Mode, FakeIPBackend: r.FakeIPBackend, DNSBackend: r.DNSBackend, Enabled: s.Enabled, Advertised: s.Advertised, Limit: cfg.BGP.RouteLimit, Peers: s.Peers, Routes: []Route{}, LastError: s.LastError, Limitations: []string{}}
	if m.startupError != "" {
		v.LastError = m.startupError
	}
	present := map[string]bool{}
	peersReady := len(s.Peers) > 0 && s.LastError == "" && m.startupError == ""
	for _, p := range s.Peers {
		peersReady = peersReady && p.State == "ESTABLISHED"
	}
	for _, a := range s.Routes {
		installed := a.Installed
		state := "inactive"
		if installed {
			state = "local-rib"
		}
		if a.Announced {
			state = "advertised"
		}
		v.Routes = append(v.Routes, Route{Advertisement: a, Installed: &installed, Backend: "bgp", Managed: true, State: state})
		present[a.Prefix] = true
	}
	fakeReady := len(cfg.Surge.FakePrefixes) > 0
	dnsReady := true
	dnsCount := 0
	for _, a := range Definitions(m.document) {
		backend := r.FakeIPBackend
		if a.Kind == "dns-interception" {
			backend = r.DNSBackend
		}
		if a.Kind == "manual" {
			backend = "bgp"
		}
		if backend == "static" {
			confirmed := r.StaticFakeIPConfigured
			if a.Kind == "dns-interception" {
				confirmed = r.StaticDNSInterception && r.StaticDNSConfigured
			}
			state := "declared"
			if confirmed {
				state = "operator-confirmed"
			}
			row := Route{Advertisement: a, Backend: "static", Managed: false, Declared: true, OperatorConfirmed: confirmed, State: state, Guidance: "Manually configure " + a.Prefix + " via " + a.NextHop + " on the gateway; RoutePilot does not change or inspect that router."}
			if a.NextHop == "" {
				row.Guidance = "Configure the " + a.Kind + " next hop for this address family first, then manually route " + a.Prefix + " to it on the gateway. RoutePilot does not change the router."
			}
			if probe, ok := m.verifications[a.Prefix]; ok {
				copy := probe
				row.Connectivity = &copy
			}
			v.Routes = append(v.Routes, row)
			if a.Kind == "fake-ip" {
				fakeReady = fakeReady && confirmed && a.NextHop != ""
			} else {
				dnsCount++
				dnsReady = dnsReady && confirmed
			}
		} else {
			announced := false
			for _, row := range v.Routes {
				if row.Prefix == a.Prefix {
					announced = row.Announced
				}
			}
			if a.Kind == "fake-ip" {
				fakeReady = fakeReady && peersReady && announced
			}
			if a.Kind == "dns-interception" {
				dnsCount++
				dnsReady = dnsReady && peersReady && announced
			}
			if !present[a.Prefix] {
				inactive := false
				v.Routes = append(v.Routes, Route{Advertisement: a, Installed: &inactive, Backend: "bgp", Managed: s.Enabled, State: "inactive"})
			}
		}
	}
	v.FakeIPReady = fakeReady
	v.DNSInterceptionReady = !cfg.DNS.InterceptionEnabled || dnsCount > 0 && dnsReady
	if r.FakeIPBackend == "static" || r.DNSBackend == "static" {
		v.Limitations = append(v.Limitations, "External static routes are operator declarations; installation is unknown and RoutePilot never installs or withdraws them.", "Operator confirmation permits Fake-IP issuance; optional Mac connectivity probes do not prove gateway or LAN route installation.", "Static Fake-IP routes remain after Surge or RoutePilot stops.")
		if r.DNSBackend == "static" {
			v.Limitations = append(v.Limitations, "Persistent DNS interception can cause DNS outages if RoutePilot stops. It is disabled unless explicitly opted in; external failover or manual removal is required.")
		}
	}
	v.Limitations = append(v.Limitations, "BGP advertised status confirms local RIB/Adj-RIB-Out, not the remote gateway FIB.")
	sort.Slice(v.Routes, func(i, j int) bool { return v.Routes[i].Prefix < v.Routes[j].Prefix })
	return v
}

// Status includes all declarations. ActiveStatus retains the old installed-only
// view for lifecycle tests and accounting; static declarations remain visible.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v := m.snapshot
	v.Peers = append([]bgp.PeerStatus{}, v.Peers...)
	v.Routes = append([]Route{}, v.Routes...)
	for i := range v.Routes {
		v.Routes[i].Communities = append([]string{}, v.Routes[i].Communities...)
		if v.Routes[i].Connectivity != nil {
			copy := *v.Routes[i].Connectivity
			v.Routes[i].Connectivity = &copy
		}
	}
	v.Limitations = append([]string{}, v.Limitations...)
	return v
}
func (m *Manager) ActiveStatus() Status {
	v := m.Status()
	rows := []Route{}
	for _, r := range v.Routes {
		if r.Backend == "static" || r.Installed != nil && *r.Installed {
			rows = append(rows, r)
		}
	}
	v.Routes = rows
	return v
}
func (m *Manager) Close(ctx context.Context) error {
	if m.bgp != nil {
		return m.bgp.Close(ctx)
	}
	return nil
}
func (m *Manager) Verify(ctx context.Context, prefix, target string, port int) (Verification, error) {
	var route *Route
	for _, r := range m.Status().Routes {
		if r.Prefix == prefix && r.Backend == "static" && r.Kind == "fake-ip" {
			copy := r
			route = &copy
			break
		}
	}
	ip, e := netip.ParseAddr(target)
	p, pe := netip.ParsePrefix(prefix)
	if route == nil || e != nil || pe != nil || !p.Contains(ip) || port < 1 || port > 65535 {
		return Verification{}, errors.New("connectivity probe requires a literal address inside a declared static Fake-IP prefix and TCP port")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := m.dial(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if conn != nil {
		conn.Close()
	}
	v := Verification{Prefix: prefix, Target: target, Port: port, Success: err == nil, CheckedAt: time.Now(), Scope: "Mac TCP connectivity only; does not verify external route installation, LAN reachability, or automatic failover"}
	m.mu.Lock()
	m.verifications[prefix] = v
	m.mu.Unlock()
	_, _ = m.Refresh(ctx)
	return v, nil
}
