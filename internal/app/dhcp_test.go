package app

import (
	"surge-controller/internal/config"
	"testing"
)

func TestDHCPChangesPreserveDNSAndBGPRuntime(t *testing.T) {
	old := config.Default().Clone()
	next := old.Clone()
	next.Settings.DHCP = config.DHCP{Interface: "en0", ServerIP: "192.0.2.10", Subnet: "192.0.2.0/24", PoolStart: "192.0.2.100", PoolEnd: "192.0.2.200", Options: config.DHCPOptions{LeaseSeconds: 3600}}
	impact := configurationChanges(old, next)
	if impact.DNS || impact.BGP || impact.Peers || impact.Routes || impact.Gate {
		t.Fatalf("DHCP-only edit restarted unrelated services: %+v", impact)
	}
}
