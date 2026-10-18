package dhcpserver

import (
	"context"
	"errors"
	"net"
	"surge-controller/internal/config"
	"testing"
)

func TestAddressModeAndHardwareInterfaceMapping(t *testing.T) {
	order := "(1) USB LAN\n(Hardware Port: Ethernet, Device: en7)\n(2) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n"
	if networkService(order, "en0") != "Wi-Fi" || networkService(order, "en7") != "USB LAN" || networkService(order, "en1") != "" {
		t.Fatal("wrong network service selected")
	}
	for input, want := range map[string]string{"Manual Configuration\nIP address: 192.0.2.10": "static", "DHCP Configuration\nIP address: 192.0.2.10": "dhcp", "unknown configuration": "unknown"} {
		if got := parseAddressMode(input); got != want {
			t.Fatalf("%q %q", got, want)
		}
	}
}
func TestPreflightWarnsWithoutClaimingProbeFailureIsNoServer(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	name := ""
	for _, i := range interfaces {
		if i.Flags&net.FlagUp != 0 && i.Flags&net.FlagBroadcast != 0 {
			name = i.Name
			break
		}
	}
	if name == "" {
		t.Skip("no broadcast interface")
	}
	s := NewService(nil)
	s.addressCheck = func(context.Context, string) (string, error) { return "dhcp", nil }
	s.discoverCheck = func(context.Context, *net.Interface, string) ([]DiscoveredServer, error) {
		return []DiscoveredServer{{Address: "192.0.2.1"}}, nil
	}
	r, err := s.Check(context.Background(), configForInterface(name))
	if err != nil || len(r.Warnings) != 2 || len(r.Servers) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	s.discoverCheck = func(context.Context, *net.Interface, string) ([]DiscoveredServer, error) {
		return []DiscoveredServer{}, errors.New("port unavailable")
	}
	r, err = s.Check(context.Background(), configForInterface(name))
	if err != nil || r.ProbeError == "" || len(r.Warnings) != 2 {
		t.Fatalf("%+v %v", r, err)
	}
}
func configForInterface(name string) config.DHCP { d := testConfig(); d.Interface = name; return d }
