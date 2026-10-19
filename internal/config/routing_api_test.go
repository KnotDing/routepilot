package config

import "testing"

func TestRoutingAPIEffective(t *testing.T) {
	openwrt := RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1", Username: "root", Password: "x"}.Effective()
	if openwrt.Auth != "ubus" {
		t.Fatalf("openwrt preset must select ubus auth, got %s", openwrt.Auth)
	}
	unifi := RoutingAPI{Preset: "unifi", BaseURL: "https://192.168.1.1", HeaderValue: "k"}.Effective()
	if unifi.Auth != "header" || unifi.HeaderName != "X-API-KEY" {
		t.Fatalf("unifi preset must default to the X-API-KEY header, got %+v", unifi)
	}
	custom := RoutingAPI{Preset: "custom", Auth: "none", BaseURL: "http://192.168.1.1"}.Effective()
	if custom.Auth != "none" {
		t.Fatalf("custom preset must keep the chosen auth, got %s", custom.Auth)
	}
}

func validCustom() RoutingAPI {
	return RoutingAPI{
		Preset: "custom", Auth: "header", BaseURL: "http://192.168.1.1",
		HeaderName: "X-API-KEY", HeaderValue: "k",
		Add:    RoutingAPIRequest{Method: "POST", Path: "/routes", Body: `{"name":"{{name}}","network":"{{prefix}}"}`},
		Remove: RoutingAPIRequest{Method: "DELETE", Path: "/routes/{{id}}"},
		List:   RoutingAPIRequest{Method: "GET", Path: "/routes", IDField: "id", NameField: "name"},
	}
}

func TestRoutingAPIValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  RoutingAPI
		ok   bool
	}{
		{"openwrt complete", RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1", Username: "root", Password: "x"}, true},
		{"openwrt stored password", RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1", Username: "root", PasswordConfigured: true}, true},
		{"openwrt missing username", RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1", Password: "x"}, false},
		{"unifi complete", RoutingAPI{Preset: "unifi", BaseURL: "https://192.168.1.1", HeaderValue: "k"}, true},
		{"unifi stored key", RoutingAPI{Preset: "unifi", BaseURL: "https://192.168.1.1", HeaderConfigured: true}, true},
		{"custom complete", validCustom(), true},
		{"custom missing templates", RoutingAPI{Preset: "custom", Auth: "none", BaseURL: "http://192.168.1.1"}, false},
		{"custom unknown variable", func() RoutingAPI {
			c := validCustom()
			c.Add.Body = `{"x":"{{oops}}"}`
			return c
		}(), false},
		{"custom unterminated variable", func() RoutingAPI {
			c := validCustom()
			c.Add.Path = "/routes/{{id"
			return c
		}(), false},
		{"bad preset", RoutingAPI{Preset: "mikrotik", BaseURL: "http://192.168.1.1"}, false},
		{"missing url", RoutingAPI{Preset: "openwrt", Username: "root", Password: "x"}, false},
		{"userinfo url", RoutingAPI{Preset: "openwrt", BaseURL: "http://root:x@192.168.1.1", Username: "root", Password: "x"}, false},
		{"query url", RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1/?x=1", Username: "root", Password: "x"}, false},
	}
	for _, tc := range cases {
		if err := tc.cfg.Effective().Validate(); tc.ok && err != nil || !tc.ok && err == nil {
			t.Fatalf("%s: ok=%v err=%v", tc.name, tc.ok, err)
		}
	}
}

func TestRoutingModeAPI(t *testing.T) {
	api := Routing{Mode: "api"}.Effective()
	if api.FakeIPBackend != "api" || api.DNSBackend != "api" || api.ManualBackend != "api" {
		t.Fatalf("api mode must select the api backend: %+v", api)
	}
	if !api.UsesAPI() || api.UsesBGP() {
		t.Fatal("api mode must only use the API backend")
	}
	if e := api.Validate(); e != nil {
		t.Fatal(e)
	}
	hybrid := Routing{Mode: "hybrid", FakeIPBackend: "api", DNSBackend: "bgp"}
	if !hybrid.UsesAPI() || !hybrid.UsesBGP() {
		t.Fatal("hybrid must report both backends when mixed")
	}
	hybridManual := Routing{Mode: "hybrid", FakeIPBackend: "bgp", DNSBackend: "bgp", ManualBackend: "api"}
	if !hybridManual.UsesAPI() || !hybridManual.UsesBGP() {
		t.Fatal("hybrid manual api selection must report both backends")
	}
	legacyHybrid := Routing{Mode: "hybrid", FakeIPBackend: "static", DNSBackend: "bgp"}
	if legacyHybrid.Effective().ManualBackend != "bgp" {
		t.Fatal("older hybrid documents without a manual selection keep manual routes on BGP")
	}
	hybridStatic := Routing{Mode: "hybrid", FakeIPBackend: "static", DNSBackend: "static", ManualBackend: "static"}
	if hybridStatic.UsesAPI() || hybridStatic.UsesBGP() {
		t.Fatal("hybrid without selected backends must report neither")
	}
	if e := hybrid.Validate(); e != nil {
		t.Fatal(e)
	}
	if e := (Routing{Mode: "api", FakeIPBackend: "static"}).Effective().Validate(); e != nil {
		t.Fatal(e)
	}
	if (Routing{Mode: "api", FakeIPBackend: "static"}).Effective().FakeIPBackend != "api" {
		t.Fatal("api mode must pin the backend selection")
	}
	if e := (Routing{Mode: "hybrid", FakeIPBackend: "bgp", DNSBackend: "bgp", ManualBackend: "custom"}).Validate(); e == nil {
		t.Fatal("invalid manual backend must be rejected")
	}
}

func TestDocumentValidateRequiresRoutingAPIWhenUsed(t *testing.T) {
	d := Default()
	d.Settings.Routing = Routing{Mode: "api"}
	if e := d.Validate(); e == nil {
		t.Fatal("api mode without router API settings must be rejected")
	}
	d.Settings.RoutingAPI = RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1", Username: "root", Password: "secret"}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	// IPv6 fake prefixes still need an explicit next hop in api mode.
	d.Settings.Surge.FakePrefixes = []string{"fd00::/64"}
	if e := d.Validate(); e == nil {
		t.Fatal("ipv6 fake prefix without next hop must fail in api mode")
	}
}

func TestRedactedStripsRoutingAPISecrets(t *testing.T) {
	d := Default()
	d.Settings.RoutingAPI = RoutingAPI{Preset: "openwrt", BaseURL: "http://192.168.1.1", Username: "root", Password: "secret", HeaderValue: "key"}
	redacted := d.Redacted()
	if redacted.Settings.RoutingAPI.Password != "" || redacted.Settings.RoutingAPI.HeaderValue != "" {
		t.Fatal("secrets must be stripped")
	}
	if !redacted.Settings.RoutingAPI.PasswordConfigured || !redacted.Settings.RoutingAPI.HeaderConfigured {
		t.Fatal("configured flags must survive redaction")
	}
	if redacted.Settings.RoutingAPI.Username != "root" {
		t.Fatal("username is not a secret")
	}
}
