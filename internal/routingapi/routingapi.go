// Package routingapi publishes RoutePilot routes to third-party routers
// through their HTTP APIs. OpenWrt and UniFi OS are built-in presets; a
// custom preset renders operator-provided request templates. Published only
// means the router accepted the configuration; it never claims the forwarding
// plane installed the path.
package routingapi

import (
	"context"
	"errors"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"surge-controller/internal/bgp"
	"surge-controller/internal/config"
)

// Status is the readonly snapshot exposed through the routing status view.
// BaseURL never carries credentials; secrets stay in the encrypted store.
type Status struct {
	Enabled   bool         `json:"enabled"`
	Preset    string       `json:"preset"`
	AuthMode  string       `json:"auth_mode"`
	BaseURL   string       `json:"base_url"`
	Published int          `json:"published"`
	LastError string       `json:"last_error"`
	LastCheck time.Time    `json:"last_check"`
	Routes    []RouteState `json:"routes"`
}

// RouteState reports one route as observed on the router. Present means the
// desired route exists and matches; Managed marks router entries created by
// RoutePilot that are no longer desired and therefore pending withdrawal.
type RouteState struct {
	Name    string `json:"name"`
	Prefix  string `json:"prefix"`
	NextHop string `json:"next_hop"`
	ID      string `json:"id,omitempty"`
	Present bool   `json:"present"`
	Managed bool   `json:"managed,omitempty"`
}

type desiredRoute struct {
	adv  bgp.Advertisement
	name string
}

type backend interface {
	peek(ctx context.Context, desired []desiredRoute) ([]RouteState, error)
	sync(ctx context.Context, desired []desiredRoute) ([]RouteState, error)
	close(ctx context.Context)
}

type Manager struct {
	mu      sync.Mutex
	impl    backend
	cfg     config.RoutingAPI
	desired []desiredRoute
	status  Status
	notify  func(severity, message string)
}

// New builds a router API backend. It validates the configuration shape only;
// reachability is left to the periodic reconciliation so an offline router
// never blocks DNS startup.
func New(cfg config.RoutingAPI, notify func(severity, message string)) (*Manager, error) {
	cfg = cfg.Effective()
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	factory := map[string]func(config.RoutingAPI, func(string, string)) backend{
		"openwrt": func(c config.RoutingAPI, n func(string, string)) backend { return newOpenWrt(c, n) },
		"unifi":   func(c config.RoutingAPI, n func(string, string)) backend { return newUniFi(c, n) },
		"custom":  func(c config.RoutingAPI, n func(string, string)) backend { return newCustom(c, n) },
	}[cfg.Preset]
	if factory == nil {
		return nil, errors.New("routing API preset must be openwrt, unifi or custom")
	}
	return &Manager{impl: factory(cfg, notify), cfg: cfg, notify: notify, status: Status{Enabled: true, Preset: cfg.Preset, AuthMode: cfg.Auth, BaseURL: cfg.BaseURL, Routes: []RouteState{}}}, nil
}

// RouteName derives the stable identifier used on the router. Custom templates
// must keep {{name}} as the route name so leftovers can still be recognized.
func RouteName(kind, prefix string) string {
	short := map[string]string{"fake-ip": "fakeip", "dns-interception": "dns", "manual": "manual"}[kind]
	if short == "" {
		short = kind
	}
	normalized := strings.NewReplacer("/", "-", ":", "-", ".", ".").Replace(prefix)
	return "routepilot-" + short + "-" + normalized
}

func desiredRoutes(desired []bgp.Advertisement) []desiredRoute {
	out := make([]desiredRoute, 0, len(desired))
	seen := map[string]bool{}
	for _, a := range desired {
		name := RouteName(a.Kind, a.Prefix)
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, desiredRoute{adv: a, name: name})
	}
	return out
}

// Steady-state polling interval. Reconciliation and inspection fall back to
// the cached snapshot while the desired set is unchanged, which keeps the
// health loop from hammering the router once per second. Desired-set changes
// and shutdown withdrawals bypass the interval immediately.
const steadyInterval = 10 * time.Second

func sameDesired(a, b []desiredRoute) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(d desiredRoute) string { return d.name + "|" + d.adv.Prefix + "|" + d.adv.NextHop }
	index := make(map[string]bool, len(a))
	for _, d := range a {
		index[key(d)] = true
	}
	for _, d := range b {
		if !index[key(d)] {
			return false
		}
	}
	return true
}

func (m *Manager) Reconcile(ctx context.Context, desired []bgp.Advertisement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := desiredRoutes(desired)
	if sameDesired(m.desired, next) && !m.status.LastCheck.IsZero() && time.Since(m.status.LastCheck) < steadyInterval {
		if m.status.LastError != "" {
			return errors.New(m.status.LastError)
		}
		return nil
	}
	m.desired = next
	states, err := m.impl.sync(ctx, m.desired)
	m.record(states, err)
	return err
}

// Refresh re-reads the router without changing it. The boolean reports whether
// every desired route is present, mirroring the BGP readiness contract.
func (m *Manager) Refresh(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.status.LastCheck.IsZero() && time.Since(m.status.LastCheck) < steadyInterval {
		return m.cachedReady(), nil
	}
	states, err := m.impl.peek(ctx, m.desired)
	m.record(states, err)
	if err != nil {
		return false, err
	}
	return m.cachedReady(), nil
}

func (m *Manager) cachedReady() bool {
	present := map[string]bool{}
	for _, s := range m.status.Routes {
		if s.Present {
			present[s.Name] = true
		}
	}
	for _, d := range m.desired {
		if !present[d.name] {
			return false
		}
	}
	return true
}

func (m *Manager) record(states []RouteState, err error) {
	m.status.LastCheck = time.Now()
	m.status.LastError = ""
	if err != nil {
		m.status.LastError = err.Error()
	}
	m.status.Routes = append([]RouteState{}, states...)
	sort.Slice(m.status.Routes, func(i, j int) bool { return m.status.Routes[i].Name < m.status.Routes[j].Name })
	m.status.Published = 0
	for _, s := range m.status.Routes {
		if s.Present {
			m.status.Published++
		}
	}
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.status
	v.Routes = append([]RouteState{}, v.Routes...)
	return v
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	impl := m.impl
	m.mu.Unlock()
	if impl != nil {
		impl.close(ctx)
	}
	return nil
}

// routeFamilies splits a prefix into the pieces reused by every template.
type routeFacts struct {
	routeType string
	target    string
	netmask   string
	values    string
}

func routeFactsFor(prefix, nextHop string) routeFacts {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return routeFacts{}
	}
	if p.Addr().Is4() {
		bits := p.Bits()
		var m [4]byte
		for i := 0; i < bits; i++ {
			m[i/8] |= 1 << (7 - uint(i%8))
		}
		netmask := strings.Join([]string{itoa(int(m[0])), itoa(int(m[1])), itoa(int(m[2])), itoa(int(m[3]))}, ".")
		return routeFacts{
			routeType: "route",
			target:    p.Addr().String(),
			netmask:   netmask,
			values:    `"target":"` + p.Addr().String() + `","netmask":"` + netmask + `","gateway":"` + nextHop + `"`,
		}
	}
	return routeFacts{
		routeType: "route6",
		target:    p.String(),
		netmask:   "",
		values:    `"target":"` + p.String() + `","gateway":"` + nextHop + `"`,
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
