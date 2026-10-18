package config

import (
	"encoding/json"
	"testing"
)

func dhcpFixture() DHCP {
	return DHCP{Enabled: true, Interface: "en0", ServerIP: "192.0.2.10", Subnet: "192.0.2.0/24", PoolStart: "192.0.2.100", PoolEnd: "192.0.2.200", Options: DHCPOptions{Gateway: "192.0.2.1", LeaseSeconds: 3600}}
}
func TestDHCPValidationRejectsUnsafeAllocationAndRoutes(t *testing.T) {
	if err := (DHCP{}).Validate(); err != nil {
		t.Fatal("old document no longer valid", err)
	}
	for name, change := range map[string]func(*DHCP){
		"network address":        func(d *DHCP) { d.PoolStart = "192.0.2.0" },
		"broadcast address":      func(d *DHCP) { d.PoolEnd = "192.0.2.255" },
		"server in pool":         func(d *DHCP) { d.ServerIP = "192.0.2.101" },
		"gateway in pool":        func(d *DHCP) { d.Options.Gateway = "192.0.2.102" },
		"gateway outside subnet": func(d *DHCP) { d.Options.Gateway = "198.51.100.1" },
		"route with host bits":   func(d *DHCP) { d.Options.Routes = []DHCPRoute{{Prefix: "91.105.0.0/13", Gateway: "192.0.2.2"}} },
		"duplicate MAC":          func(d *DHCP) { d.Devices = []DHCPDevice{{MAC: "02:aa:bb:cc:dd:ee"}, {MAC: "02:AA:BB:CC:DD:EE"}} },
		"reserved gateway":       func(d *DHCP) { d.Devices = []DHCPDevice{{MAC: "02:aa:bb:cc:dd:ee", Address: "192.0.2.1"}} },
		"override bad lease": func(d *DHCP) {
			n := 0
			d.Devices = []DHCPDevice{{MAC: "02:aa:bb:cc:dd:ee", Options: DHCPOverrides{LeaseSeconds: &n}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := dhcpFixture()
			change(&d)
			if err := d.Validate(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
func TestDHCPClonePreservesExplicitEmptyOverrides(t *testing.T) {
	d := dhcpFixture()
	list := []string{}
	gateway := "192.0.2.2"
	d.Devices = []DHCPDevice{{MAC: "02:aa:bb:cc:dd:ee", Options: DHCPOverrides{DNS: &list, Gateway: &gateway}}}
	d.Options.Routes = []DHCPRoute{{Prefix: "91.104.0.0/13", Gateway: "192.0.2.2"}}
	c := d.Clone()
	*c.Devices[0].Options.Gateway = "192.0.2.3"
	c.Options.Routes[0].Prefix = "10.0.0.0/8"
	if gateway != "192.0.2.2" || d.Options.Routes[0].Prefix != "91.104.0.0/13" {
		t.Fatal("clone shared options")
	}
	raw, _ := json.Marshal(c)
	var restored DHCP
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Devices[0].Options.DNS == nil || len(*restored.Devices[0].Options.DNS) != 0 {
		t.Fatal("explicit empty override became inheritance")
	}
}
