package security

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"surge-controller/internal/config"
	"testing"
)

func TestRequestIdentityTrustsOnlyConfiguredProxyHeader(t *testing.T) {
	settings := config.Default().Settings.Security
	settings.TrustedProxies = []string{"10.0.0.0/8"}
	settings.ProxyClientIPHeader = "X-Forwarded-For"
	cases := []struct {
		name, remote, forwarded string
		wantIP                  string
		wantForwarded           bool
		wantErr                 bool
	}{
		{name: "direct spoof ignored", remote: "198.51.100.9:1234", forwarded: "192.0.2.7", wantIP: "198.51.100.9"},
		{name: "walk trusted chain from right", remote: "10.0.0.1:443", forwarded: "192.0.2.7, 10.0.0.2", wantIP: "192.0.2.7", wantForwarded: true},
		{name: "malformed client rejected", remote: "10.0.0.1:443", forwarded: "unknown", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://127.0.0.1:1055/api/v1/session", nil)
			r.RemoteAddr = tc.remote
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			identity, err := requestIdentity(r, settings)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if err != nil {
				return
			}
			if identity.ip.String() != tc.wantIP || identity.forwarded != tc.wantForwarded {
				t.Fatalf("identity=%+v want ip=%s forwarded=%t", identity, tc.wantIP, tc.wantForwarded)
			}
		})
	}
}

func TestTrustedProxyCannotUseUnselectedHeaderForAuthenticationBypass(t *testing.T) {
	settings := config.Default().Settings.Security
	settings.Authentication = true
	settings.AllowedNetworks = nil
	settings.TrustedProxies = []string{"10.0.0.0/8"}
	settings.AuthBypassNetworks = []string{"192.0.2.0/24"}
	settings.ProxyClientIPHeader = "Forwarded"
	handler := New(settings).Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "http://127.0.0.1:1055/api/v1/session", nil)
	r.RemoteAddr = "10.0.0.1:443"
	r.Header.Set("X-Forwarded-For", "192.0.2.7")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("proxy without a forwarded client identity borrowed its own bypass entry: status=%d", w.Code)
	}
}

func TestTrustedProxyForwardsWhitelistedClientAndAllowedOrigin(t *testing.T) {
	settings := config.Default().Settings.Security
	settings.Authentication = true
	settings.AllowedNetworks = nil
	settings.TrustedProxies = []string{"10.0.0.0/8"}
	settings.AuthBypassNetworks = []string{"192.0.2.0/24"}
	settings.ProxyClientIPHeader = "Forwarded"
	settings.ProxyOrigins = []string{"https://manage.example.test"}
	handler := New(settings).Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "http://127.0.0.1:1055/api/v1/session", nil)
	r.RemoteAddr = "10.0.0.1:443"
	r.Header.Set("Forwarded", `for=192.0.2.7;proto=https;host=manage.example.test`)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("trusted proxy request failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Authenticated bool   `json:"authenticated"`
		IPWhitelisted bool   `json:"ip_whitelisted"`
		ClientIP      string `json:"client_ip"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Authenticated || !response.IPWhitelisted || response.ClientIP != netip.MustParseAddr("192.0.2.7").String() {
		t.Fatalf("wrong proxied session identity: %+v", response)
	}
}
