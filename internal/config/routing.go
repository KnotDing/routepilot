package config

import (
	"errors"
	"net/netip"
)

// Routing separates DNS policy from the mechanism used to provide its paths.
// Configured means an explicit operator declaration, never an automatic probe.
type Routing struct {
	Mode                   string `json:"mode"`
	FakeIPBackend          string `json:"fake_ip_backend"`
	DNSBackend             string `json:"dns_backend"`
	StaticFakeIPNextHop4   string `json:"static_fake_ip_next_hop4,omitempty"`
	StaticFakeIPConfigured bool   `json:"static_fake_ip_configured"`
	StaticDNSInterception  bool   `json:"static_dns_interception"`
	StaticDNSConfigured    bool   `json:"static_dns_configured"`
}

func (r Routing) Effective() Routing {
	if r.Mode == "" {
		r.Mode = "bgp"
	}
	switch r.Mode {
	case "bgp":
		r.FakeIPBackend = "bgp"
		r.DNSBackend = "bgp"
	case "static":
		r.FakeIPBackend = "static"
		r.DNSBackend = "static"
	}
	return r
}
func (r Routing) Validate() error {
	r = r.Effective()
	if r.StaticFakeIPNextHop4 != "" {
		ip, err := netip.ParseAddr(r.StaticFakeIPNextHop4)
		if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsLoopback() {
			return errors.New("invalid confirmed static Fake-IP IPv4 next hop")
		}
	}
	if r.Mode != "bgp" && r.Mode != "static" && r.Mode != "hybrid" {
		return errors.New("routing mode must be bgp, static or hybrid")
	}
	for _, b := range []string{r.FakeIPBackend, r.DNSBackend} {
		if b != "bgp" && b != "static" {
			return errors.New("routing backend must be bgp or static")
		}
	}
	return nil
}
func (r Routing) UsesBGP() bool {
	r = r.Effective()
	return r.Mode != "static" && (r.FakeIPBackend == "bgp" || r.DNSBackend == "bgp")
}
