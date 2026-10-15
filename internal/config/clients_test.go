package config

import (
	"encoding/json"
	"surge-controller/internal/model"
	"testing"
)

func TestClientSettingsValidateCloneAndNormalizedDuplicates(t *testing.T) {
	d := Default()
	d.Settings.DNS.Clients = []Client{{ID: "a", IP: "192.0.2.10", Name: "办公电脑", Decision: model.Surge}, {ID: "b", IP: "2001:db8::10", Decision: ""}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := d.Clone()
	copy.Settings.DNS.Clients[0].Name = "changed"
	if d.Settings.DNS.Clients[0].Name != "办公电脑" {
		t.Fatal("configuration aliases client settings")
	}
	for _, ip := range []string{"0.0.0.0", "ff02::1", "192.0.2.0/24", "bad", "fe80::1%en0"} {
		bad := d.Clone()
		bad.Settings.DNS.Clients[0].IP = ip
		if bad.Validate() == nil {
			t.Fatal("accepted invalid client", ip)
		}
	}
	duplicate := d.Clone()
	duplicate.Settings.DNS.Clients = append(duplicate.Settings.DNS.Clients, Client{ID: "c", IP: "::ffff:192.0.2.10"})
	if duplicate.Validate() == nil {
		t.Fatal("duplicate normalized IP accepted")
	}
	bad := d.Clone()
	bad.Settings.DNS.Clients[0].Decision = model.Unknown
	if bad.Validate() == nil {
		t.Fatal("unsupported forced policy accepted")
	}
}

func TestClientHostnameProbeIsOptIn(t *testing.T) {
	d := Default()
	if err := json.Unmarshal([]byte(`{"settings":{"logging":{"level":"info"}}}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Settings.Logging.ProbeHostnames || d.Settings.Logging.HostnameDeviceOnly {
		t.Fatal("hostname probing must default off")
	}
	d.Settings.Logging.ProbeHostnames = true
	d.Settings.Logging.HostnameDeviceOnly = true
	encoded, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var restored Document
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.Settings.Logging.ProbeHostnames || !restored.Settings.Logging.HostnameDeviceOnly {
		t.Fatal("round trip lost hostname setting")
	}
	if !d.Clone().Settings.Logging.ProbeHostnames || !d.Clone().Settings.Logging.HostnameDeviceOnly {
		t.Fatal("clone lost hostname setting")
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}
