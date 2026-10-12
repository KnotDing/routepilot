package config

import (
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
