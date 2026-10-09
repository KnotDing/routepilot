package frr

import (
	"strings"
	"surge-controller/internal/config"
	"testing"
)

func fixture() (config.Document, Request) {
	d := config.Default()
	d.Settings.DNS.Source4 = "192.168.20.2"
	d.Settings.Surge.NextHop4 = "192.168.20.141"
	d.Peers = []config.Peer{{ID: "ucg", Address: "192.168.20.1", ASN: 65001, Enabled: true, HoldSeconds: 60, KeepaliveSeconds: 20}}
	d.Interception = []config.Interception{{ID: "dns", Prefix: "8.8.8.8/32", Enabled: true}, {ID: "disabled", Prefix: "9.9.9.9/32", Enabled: false}}
	return d, Request{PeerID: "ucg", GatewayASN: 65001, GatewayRouterID: "192.168.20.1", ControllerAddress: "192.168.20.2"}
}
func TestGatewayASNDirectionDynamicImportsAndNoRouteExports(t *testing.T) {
	d, r := fixture()
	e, err := Generate(d, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"router bgp 65001", "neighbor 192.168.20.2 remote-as 64512", "neighbor 192.168.20.2 timers 20 60", "address-family ipv4 unicast", "address-family ipv6 unicast", "route-map RP-IN-V4 permit 10", "route-map RP-IN-V6 permit 10", "route-map RP-OUT deny 10", "maximum-prefix 128"} {
		if !strings.Contains(e.Content, text) {
			t.Fatalf("missing %q", text)
		}
	}
	for _, text := range []string{"prefix-list RP-PREFIXES", "redistribute", "default-originate", "set ip next-hop", "permit 0.0.0.0/0"} {
		if strings.Contains(e.Content, text) {
			t.Fatalf("unsafe output %q", text)
		}
	}
	if strings.Index(e.Content, "route-map RP-IN-V4 permit") < strings.Index(e.Content, "exit-address-family") {
		t.Fatal("UniFi requires route maps after router section")
	}
}
func TestStaticAndHybridKeepDynamicInboundPolicy(t *testing.T) {
	d, r := fixture()
	d.Settings.Routing = config.Routing{Mode: "hybrid", FakeIPBackend: "static", DNSBackend: "bgp"}
	e, err := Generate(d, r)
	if err != nil || strings.Contains(e.Content, "permit 198.18.0.0/15") || !strings.Contains(e.Content, "route-map RP-IN-V4 permit 10") || strings.Contains(e.Content, "ip prefix-list") {
		t.Fatal(e, err)
	}
	d.Settings.Routing = config.Routing{Mode: "static"}
	e, err = Generate(d, r)
	if err != nil || strings.Contains(e.Content, "seq 10 permit") || !strings.Contains(e.Content, "route-map RP-IN-V4 permit 10") || !strings.Contains(e.Content, "route-map RP-OUT deny 10") {
		t.Fatal(e, err)
	}
}
func TestExportRejectsMismatchAndInjectionAndSupportsIPv6(t *testing.T) {
	d, r := fixture()
	for _, change := range []func(*Request){func(r *Request) { r.GatewayASN = 64513 }, func(r *Request) { r.ControllerAddress = "192.168.20.3" }, func(r *Request) { r.GatewayRouterID = "192.168.20.1\nnetwork 0.0.0.0/0" }, func(r *Request) { r.PeerID = "missing" }} {
		bad := r
		change(&bad)
		if _, err := Generate(d, bad); err == nil {
			t.Fatal("invalid export accepted", bad)
		}
	}
	d.Settings.DNS.Source6 = "fd00:20::2"
	d.Settings.Surge.FakePrefixes = append(d.Settings.Surge.FakePrefixes, "fd00:18::/64")
	e, err := Generate(d, r)
	if err != nil || !strings.Contains(e.Content, "address-family ipv6 unicast") || !strings.Contains(e.Content, "route-map RP-IN-V6 permit 10") || strings.Contains(e.Content, "ipv6 prefix-list") {
		t.Fatal(e, err)
	}
	if _, err := Generate(d, Request{GatewayASN: 65001, GatewayRouterID: "192.168.20.1", ControllerAddress: "fd00:20::2"}); err != nil {
		t.Fatal(err)
	}
}
