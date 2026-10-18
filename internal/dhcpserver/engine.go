package dhcpserver

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"surge-controller/internal/config"
)

type Lease struct {
	Address   string    `json:"address"`
	MAC       string    `json:"mac"`
	ClientID  string    `json:"client_id"`
	Hostname  string    `json:"hostname"`
	ExpiresAt time.Time `json:"expires_at"`
	State     string    `json:"state"`
}
type State struct {
	Subnet string  `json:"subnet"`
	Leases []Lease `json:"leases"`
}
type offer struct {
	ip      string
	expires time.Time
}
type Engine struct {
	mu       sync.Mutex
	config   config.DHCP
	leases   map[string]Lease // IP -> committed lease or declined quarantine
	offers   map[string]offer
	reserved map[string]string
	held     map[string]string
	save     func(State) error
	now      func() time.Time
}

func New(d config.DHCP, saved State, save func(State) error) (*Engine, error) {
	if !d.Enabled {
		return nil, fmt.Errorf("DHCP engine requires an enabled configuration")
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	e := &Engine{config: d.Clone(), leases: map[string]Lease{}, offers: map[string]offer{}, reserved: map[string]string{}, held: map[string]string{}, save: save, now: time.Now}
	for _, v := range d.Devices {
		if v.Address != "" {
			m, _ := net.ParseMAC(v.MAC)
			e.reserved[v.Address] = m.String()
		}
	}
	if saved.Subnet == d.Subnet {
		for _, v := range saved.Leases {
			if v.State != "bound" && v.State != "declined" {
				return nil, fmt.Errorf("invalid persisted DHCP lease state")
			}
			a, err := config.DHCPv4(v.Address)
			if err != nil {
				return nil, err
			}
			if !netip.MustParsePrefix(d.Subnet).Contains(a) {
				return nil, fmt.Errorf("persisted DHCP lease is outside subnet")
			}
			if !v.ExpiresAt.After(e.now()) {
				continue
			}
			if _, exists := e.leases[v.Address]; exists {
				return nil, fmt.Errorf("duplicate persisted DHCP address")
			}
			e.leases[v.Address] = v
		}
	}
	return e, nil
}
func (e *Engine) stateLocked() State {
	s := State{Subnet: e.config.Subnet, Leases: []Lease{}}
	for _, v := range e.leases {
		if v.ExpiresAt.After(e.now()) {
			s.Leases = append(s.Leases, v)
		}
	}
	sort.Slice(s.Leases, func(i, j int) bool {
		return netip.MustParseAddr(s.Leases[i].Address).Less(netip.MustParseAddr(s.Leases[j].Address))
	})
	return s
}
func (e *Engine) Snapshot() State { e.mu.Lock(); defer e.mu.Unlock(); return e.stateLocked() }
func (e *Engine) available(ip string, p packet, fixed string) bool {
	a, err := config.DHCPv4(ip)
	if err != nil || ip == e.config.ServerIP {
		return false
	}
	if !netip.MustParsePrefix(e.config.Subnet).Contains(a) {
		return false
	}
	if fixed != "" {
		if ip != fixed {
			return false
		}
	} else if a.Compare(netip.MustParseAddr(e.config.PoolStart)) < 0 || a.Compare(netip.MustParseAddr(e.config.PoolEnd)) > 0 {
		return false
	}
	if mac := e.reserved[ip]; mac != "" && mac != p.mac {
		return false
	}
	if v, ok := e.leases[ip]; ok && v.ExpiresAt.After(e.now()) && (v.State == "declined" || v.ClientID != p.key || v.MAC != p.mac) {
		return false
	}
	if key := e.held[ip]; key != "" && key != p.key {
		return false
	}
	return true
}
func (e *Engine) selectAddress(p packet, fixed string) string {
	if fixed != "" {
		if e.available(fixed, p, fixed) {
			return fixed
		}
		return ""
	}
	for ip, v := range e.leases {
		if v.ClientID == p.key && v.MAC == p.mac && e.available(ip, p, fixed) {
			return ip
		}
	}
	if v, ok := e.offers[p.key]; ok && e.available(v.ip, p, fixed) {
		return v.ip
	}
	if a := optIP(p.options[50]); a.IsValid() && e.available(a.String(), p, fixed) {
		return a.String()
	}
	end := netip.MustParseAddr(e.config.PoolEnd)
	for a := netip.MustParseAddr(e.config.PoolStart); a.Compare(end) <= 0; a = a.Next() {
		if e.available(a.String(), p, fixed) {
			return a.String()
		}
	}
	return ""
}

// Handle commits a lease before acknowledging it. Source address validation and
// interface selection are performed by the transport; no request spawns work.
func (e *Engine) Handle(raw []byte) (response []byte, destination *net.UDPAddr, resultErr error) {
	p, err := decode(raw)
	if err != nil {
		return nil, nil, nil
	}
	defer func() {
		// RFC 2131 includes the IP/UDP headers in the minimum 576-byte
		// receive capability. Never truncate options into a success reply.
		if err := replySizeError(p, response); err != nil {
			response = nil
			destination = nil
			resultErr = err
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	for k, v := range e.offers {
		if !v.expires.After(now) {
			e.forgetOffer(k)
		}
	}
	for k, v := range e.leases {
		if !v.ExpiresAt.After(now) {
			delete(e.leases, k)
		}
	}
	o, fixed := e.config.ForDevice(p.mac)
	server := netip.MustParseAddr(e.config.ServerIP)
	selected := optIP(p.options[54])
	if selected.IsValid() && selected != server {
		e.forgetOffer(p.key)
		return nil, nil, nil
	}
	dest := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	if !p.ci.IsUnspecified() && p.kind != 1 {
		if !netip.MustParsePrefix(e.config.Subnet).Contains(p.ci) {
			return nil, nil, nil
		}
		dest.IP = net.IP(p.ci.AsSlice())
	}
	save := func() error {
		if e.save == nil {
			return nil
		}
		return e.save(e.stateLocked())
	}
	switch p.kind {
	case 1: // DISCOVER: only an expiring in-memory reservation, never a grant.
		if len(e.offers) >= 1024 {
			if _, ok := e.offers[p.key]; !ok {
				return nil, nil, nil
			}
		}
		ip := e.selectAddress(p, fixed)
		if ip == "" {
			return nil, nil, nil
		}
		e.forgetOffer(p.key)
		e.offers[p.key] = offer{ip, now.Add(60 * time.Second)}
		e.held[ip] = p.key
		return reply(p, 2, ip, e.config, o), dest, nil
	case 3: // SELECTING, INIT-REBOOT, RENEWING and REBINDING.
		requested := optIP(p.options[50])
		if !p.ci.IsUnspecified() {
			if requested.IsValid() {
				return nil, nil, nil
			}
			requested = p.ci
		}
		if !requested.IsValid() {
			return nil, nil, nil
		}
		ip := requested.String()
		if !e.available(ip, p, fixed) {
			// Do not NAK unknown INIT-REBOOT clients on a shared segment.
			if selected != server {
				return nil, nil, nil
			}
			dest.IP = net.IPv4bcast
			return reply(p, 6, "", e.config, o), dest, nil
		}
		answer := reply(p, 5, ip, e.config, o)
		if err := replySizeError(p, answer); err != nil {
			return nil, nil, err
		}
		previous, existed := e.leases[ip]
		name := strings.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return -1
			}
			return r
		}, string(p.options[12]))
		if len(name) > 128 {
			name = name[:128]
		}
		e.leases[ip] = Lease{Address: ip, MAC: p.mac, ClientID: p.key, Hostname: name, ExpiresAt: now.Add(time.Duration(o.LeaseSeconds) * time.Second), State: "bound"}
		if err := save(); err != nil {
			if existed {
				e.leases[ip] = previous
			} else {
				delete(e.leases, ip)
			}
			return nil, nil, fmt.Errorf("persist DHCP lease: %w", err)
		}
		e.forgetOffer(p.key)
		return answer, dest, nil
	case 4, 7: // DECLINE / RELEASE: authenticate ownership of the address.
		if selected != server {
			return nil, nil, nil
		}
		ip := p.ci.String()
		if p.kind == 4 {
			a := optIP(p.options[50])
			if !a.IsValid() {
				return nil, nil, nil
			}
			ip = a.String()
		}
		v, ok := e.leases[ip]
		offered, hasOffer := e.offers[p.key]
		if (!ok || v.ClientID != p.key || v.MAC != p.mac) && !(p.kind == 4 && hasOffer && offered.ip == ip) {
			return nil, nil, nil
		}
		if p.kind == 4 {
			e.leases[ip] = Lease{Address: ip, MAC: p.mac, ClientID: p.key, ExpiresAt: now.Add(10 * time.Minute), State: "declined"}
		} else {
			delete(e.leases, ip)
		}
		if err := save(); err != nil {
			if ok {
				e.leases[ip] = v
			} else {
				delete(e.leases, ip)
			}
			return nil, nil, fmt.Errorf("persist DHCP release/decline: %w", err)
		}
		e.forgetOffer(p.key)
		return nil, nil, nil
	case 8: // INFORM supplies options without allocating or renewing a lease.
		if p.ci.IsUnspecified() {
			return nil, nil, nil
		}
		return reply(p, 5, "", e.config, o), dest, nil
	}
	return nil, nil, nil
}

func (e *Engine) forgetOffer(key string) {
	if v, ok := e.offers[key]; ok {
		delete(e.held, v.ip)
		delete(e.offers, key)
	}
}

func replySizeError(p packet, b []byte) error {
	limit := 576 - 28
	if v := p.options[57]; len(v) == 2 {
		limit = int(binary.BigEndian.Uint16(v)) - 28
	}
	if len(b) > limit {
		return fmt.Errorf("DHCP options exceed the client's maximum message size; reduce option lengths")
	}
	return nil
}
