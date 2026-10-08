package dnsserver

import (
	"net/netip"
	"strings"
	"testing"
)

func pfFixture() PFConfig {
	return PFConfig{IngressInterfaces: []string{"en0", "bridge100"}, Prefixes: []netip.Prefix{netip.MustParsePrefix("8.8.8.8/32")}, Listener4: netip.MustParseAddrPort("192.0.2.2:1054"), EgressInterface: "en0", Source4: netip.MustParseAddr("192.0.2.2"), Gateway4: netip.MustParseAddr("192.0.2.1"), SourcePortFirst: 20000, SourcePortLast: 20999}
}
func TestPFIngressAndEgressIsolation(t *testing.T) {
	c := pfFixture()
	rules, err := c.Rules()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"rdr on { en0, bridge100 } inet proto { udp, tcp } from ! 192.0.2.2", "to { 8.8.8.8/32 } port 53 -> 192.0.2.2 port 1054", "block drop in quick", "route-to ( en0 192.0.2.1 )", "from 192.0.2.2 port 20000:20999"} {
		if !strings.Contains(rules, s) {
			t.Fatalf("missing isolation rule %q", s)
		}
	}
	if strings.Contains(rules, "rdr pass") {
		t.Fatal("rdr pass bypassed loop guard")
	}
	c.IngressInterfaces = []string{"en0;evil"}
	if _, err = c.Rules(); err == nil {
		t.Fatal("PF interface injection accepted")
	}
}
func TestPFIPv6AndInvalidPrefixes(t *testing.T) {
	c := pfFixture()
	c.Prefixes = append(c.Prefixes, netip.MustParsePrefix("2001:4860:4860::8888/128"))
	if _, err := c.Rules(); err == nil {
		t.Fatal("missing IPv6 egress silently accepted")
	}
	c.Listener6 = netip.MustParseAddrPort("[2001:db8::2]:1054")
	c.Source6 = netip.MustParseAddr("2001:db8::2")
	c.Gateway6 = netip.MustParseAddr("2001:db8::1")
	rules, err := c.Rules()
	if err != nil || !strings.Contains(rules, "inet6") {
		t.Fatal("IPv6 rules absent")
	}
	c.Prefixes = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}
	if _, err = c.Rules(); err == nil {
		t.Fatal("default interception route accepted")
	}
}
