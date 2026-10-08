package dnsserver

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

const AnchorName = "com.apple/surge-controller"

type PFConfig struct {
	IngressInterfaces               []string
	Prefixes                        []netip.Prefix
	Listener4, Listener6            netip.AddrPort
	EgressInterface                 string
	Source4, Source6                netip.Addr
	Gateway4, Gateway6              netip.Addr
	SourcePortFirst, SourcePortLast uint16
}

var interfaceName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,14}$`)

func (c PFConfig) Rules() (string, error) {
	if len(c.IngressInterfaces) == 0 || len(c.Prefixes) == 0 || len(c.Prefixes) > 128 || !interfaceName.MatchString(c.EgressInterface) {
		return "", errors.New("PF requires valid interfaces and 1..128 interception prefixes")
	}
	for _, i := range c.IngressInterfaces {
		if !interfaceName.MatchString(i) {
			return "", errors.New("invalid PF ingress interface")
		}
	}
	if c.SourcePortFirst < 1024 || c.SourcePortLast < c.SourcePortFirst || c.SourcePortLast >= 49152 || int(c.SourcePortLast)-int(c.SourcePortFirst) > 4095 {
		return "", errors.New("invalid reserved source port range")
	}
	var b strings.Builder
	b.WriteString("# Generated; load only into " + AnchorName + ". Never replace /etc/pf.conf.\n")
	interfaces := "{ " + strings.Join(c.IngressInterfaces, ", ") + " }"
	has4, has6 := false, false
	for _, p := range c.Prefixes {
		if !p.IsValid() || p != p.Masked() || p.Bits() == 0 || p.Addr().IsMulticast() || p.Addr().IsLoopback() {
			return "", errors.New("invalid interception prefix")
		}
		if p.Addr().Is4() {
			has4 = true
		} else {
			has6 = true
		}
	}
	type family struct {
		enabled         bool
		name            string
		listener        netip.AddrPort
		source, gateway netip.Addr
		ipv4            bool
	}
	families := []family{{has4, "inet", c.Listener4, c.Source4, c.Gateway4, true}, {has6, "inet6", c.Listener6, c.Source6, c.Gateway6, false}}
	for _, f := range families {
		if !f.enabled {
			continue
		}
		if !f.listener.IsValid() || f.listener.Port() == 0 || f.listener.Addr().IsUnspecified() || !f.source.IsValid() || !f.gateway.IsValid() || f.source.IsUnspecified() || f.gateway.IsUnspecified() || f.listener.Addr().Is4() != f.ipv4 || f.source.Is4() != f.ipv4 || f.gateway.Is4() != f.ipv4 {
			return "", errors.New("each intercepted family requires listener, pinned source, and gateway")
		}
		var prefixes []string
		for _, p := range c.Prefixes {
			if p.Addr().Is4() == f.ipv4 {
				prefixes = append(prefixes, p.String())
			}
		}
		destinations := "{ " + strings.Join(prefixes, ", ") + " }"
		// Redirect ingress only, explicitly exclude locally originated DNS. PF state
		// reverse translation restores the resolver source address on replies.
		fmt.Fprintf(&b, "rdr on %s %s proto { udp, tcp } from ! %s to %s port 53 -> %s port %d\n", interfaces, f.name, f.source, destinations, f.listener.Addr(), f.listener.Port())
	}
	// Filtering/routing follows all translation rules in PF grammar.
	for _, f := range families {
		if !f.enabled {
			continue
		}
		// A missing/changed router PBR must fail closed instead of recursively
		// processing our own bounced query. Match both original and translated ports.
		fmt.Fprintf(&b, "block drop in quick on %s %s proto { udp, tcp } from %s to any port { 53, %d }\n", interfaces, f.name, f.source, f.listener.Port())
		fmt.Fprintf(&b, "pass in quick on %s %s proto { udp, tcp } from any to %s port %d keep state\n", interfaces, f.name, f.listener.Addr(), f.listener.Port())
		// Do not let an upstream keep-state fast path bypass the inbound self-source
		// drop. Route each outbound packet explicitly; permit replies to reserved
		// ports separately. Connected sockets still validate their remote endpoint.
		fmt.Fprintf(&b, "pass in quick on %s %s proto { udp, tcp } from any port { 53, 853, 443 } to %s port %d:%d no state\n", interfaces, f.name, f.source, c.SourcePortFirst, c.SourcePortLast)
		fmt.Fprintf(&b, "pass out quick route-to ( %s %s ) %s proto { udp, tcp } from %s port %d:%d to any port { 53, 853, 443 } no state\n", c.EgressInterface, f.gateway, f.name, f.source, c.SourcePortFirst, c.SourcePortLast)
	}
	return b.String(), nil
}
