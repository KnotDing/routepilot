package config

import (
	"encoding/json"
	"testing"
)

func TestInterceptionProtectsListenersAndIPv6SurgeDNS(t *testing.T) {
	for _, prefix := range []string{"192.168.20.2/32", "fd00:20::2/128"} {
		d := Default()
		d.Settings.DNS.Listeners = []string{"192.168.20.2:1053"}
		d.Settings.Surge.DNS = "[fd00:20::2]:53"
		d.Interception = []Interception{{ID: "test", Prefix: prefix, Enabled: true}}
		if d.Validate() == nil {
			t.Fatal("interception covered a listener or internal Surge DNS", prefix)
		}
	}
}

func TestLocalDNSValidationProtectsFakeIPAndDuplicates(t *testing.T) {
	d := Default()
	d.Hosts = []Host{{ID: "h", Domain: "nas.test", Match: "exact", Addresses: []string{"192.0.2.53", "2001:db8::53"}, TTL: 60, Source: "manual", Enabled: true}}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, address := range []string{"198.18.1.1", "invalid", "ff02::1"} {
		bad := d.Clone()
		bad.Hosts[0].Addresses = []string{address}
		if bad.Validate() == nil {
			t.Fatal("unsafe mapping accepted", address)
		}
	}
	for _, name := range []string{"", ".", "*.test"} {
		bad := d.Clone()
		bad.Hosts[0].Domain = name
		if bad.Validate() == nil {
			t.Fatal("invalid exact name accepted", name)
		}
	}
	bad := d.Clone()
	h := bad.Hosts[0]
	h.ID = "h2"
	h.Domain = "NAS.TEST."
	bad.Hosts = append(bad.Hosts, h)
	if bad.Validate() == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestStaticDNSInterceptionRequiresIndependentOptIn(t *testing.T) {
	d := Default()
	d.Settings.Routing = Routing{Mode: "static"}
	d.Settings.DNS.InterceptionEnabled = true
	d.Settings.DNS.Transparent = []string{"192.168.20.2:1054"}
	d.Settings.DNS.EgressInterface = "en0"
	d.Settings.DNS.IngressInterfaces = []string{"en0"}
	d.Settings.DNS.Source4 = "192.168.20.2"
	d.Settings.DNS.BypassVerified = true
	if d.Validate() == nil {
		t.Fatal("persistent static interception enabled implicitly")
	}
	d.Settings.Routing.StaticDNSInterception = true
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	d.Settings.Routing = Routing{Mode: "hybrid", FakeIPBackend: "static", DNSBackend: "bgp"}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestTrustDirectIsOptInAcrossOldAndNewConfiguration(t *testing.T) {
	if Default().Settings.Surge.TrustDirect {
		t.Fatal("default opted out of conservative classification")
	}
	var old Surge
	if err := json.Unmarshal([]byte(`{"dns":"198.18.0.2:53"}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.TrustDirect {
		t.Fatal("old configuration changed behavior")
	}
	d := Default()
	d.Settings.Surge.TrustDirect = true
	if !d.Clone().Settings.Surge.TrustDirect {
		t.Fatal("configuration clone lost opt-in")
	}
}

func TestRemoteBackendOriginsAndClone(t *testing.T) {
	for _, raw := range []string{"https://192.168.20.10:1055", "https://[fd00:20::10]:1055", "http://127.0.0.1:1088/"} {
		if _, err := BackendURL(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"file:///etc/passwd", "https://user:password@192.168.20.10", "https://192.168.20.10/path", "https://192.168.20.10?token=secret", "https://192.168.20.10#fragment", "https://192.168.20.10:99999", "https://untrusted.example", "http://0.0.0.0:1055"} {
		if _, err := BackendURL(raw); err == nil {
			t.Fatal("invalid backend accepted", raw)
		}
	}
	d := Default()
	d.Settings.RemoteBackends = []Backend{{ID: "remote", Name: "Remote", URL: "https://192.168.20.10:1055"}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := d.Clone()
	copy.Settings.RemoteBackends[0].URL = "http://127.0.0.1:1"
	if d.Settings.RemoteBackends[0].URL == copy.Settings.RemoteBackends[0].URL {
		t.Fatal("backend draft mutated live configuration")
	}
}
