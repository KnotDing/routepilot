package remote

import "testing"

func TestDHCPRelayAllowlist(t *testing.T) {
	for _, v := range []struct {
		path, method string
		want         bool
	}{{"dhcp/devices", "GET", true}, {"dhcp/devices", "PUT", true}, {"dhcp/devices", "DELETE", false}, {"dhcp/status", "GET", true}, {"dhcp/configuration", "GET", true}, {"dhcp/configuration", "PUT", true}, {"dhcp/check", "POST", true}, {"dhcp/check", "PUT", false}, {"dhcp/configuration", "DELETE", false}, {"dhcp/status/anything", "GET", false}} {
		if got := allowed(v.path, v.method); got != v.want {
			t.Fatalf("%s %s = %v", v.method, v.path, got)
		}
	}
}
