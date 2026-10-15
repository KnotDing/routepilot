package remote

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/security"
	"sync/atomic"
	"testing"
)

type browser struct {
	client     *http.Client
	base, csrf string
}

func TestRelayAllowsOnlyPostForDurableVersionSkip(t *testing.T) {
	if !allowed("updates/skip", "POST") || allowed("updates/skip", "GET") || allowed("updates/skip", "DELETE") {
		t.Fatal("version skip relay policy is incorrect")
	}
}

func TestRelayBindsReadsAndMutationsToSelectedOrigin(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-RoutePilot-Expected-Backend") != "" {
			t.Error("local binding header forwarded upstream")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	configured := upstream.URL
	m := New(func() []config.Backend { return []config.Backend{{ID: "remote", URL: configured}} })
	defer m.transport.CloseIdleConnections()
	for _, tc := range []struct{ method, path string }{{"GET", "status"}, {"PUT", "settings"}, {"POST", "dns/cache/flush"}, {"GET", "operations/status?id=job"}, {"POST", "login"}} {
		for _, changed := range []bool{false, true} {
			configured = upstream.URL
			if changed {
				configured = "http://127.0.0.1:1"
			}
			r := httptest.NewRequest(tc.method, prefix+"remote/"+tc.path, strings.NewReader(`{}`))
			r.AddCookie(&http.Cookie{Name: "controller_session", Value: "isolated-browser"})
			r.Header.Set("X-RoutePilot-Expected-Backend", upstream.URL)
			w := httptest.NewRecorder()
			before := calls.Load()
			m.ServeHTTP(w, r)
			if changed {
				if w.Code != http.StatusConflict || calls.Load() != before {
					t.Fatalf("%s %s reached changed origin: %d", tc.method, tc.path, w.Code)
				}
			} else if w.Code != http.StatusOK || calls.Load() <= before {
				t.Fatalf("selected origin rejected: %s %s: %d", tc.method, tc.path, w.Code)
			}
		}
	}
}

func openBrowser(t *testing.T, base string) *browser {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	b := &browser{client: &http.Client{Jar: jar}, base: base}
	r, err := b.client.Get(base + "/api/v1/session")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var session struct {
		CSRF string `json:"csrf"`
	}
	if json.NewDecoder(r.Body).Decode(&session) != nil {
		t.Fatal("no session")
	}
	b.csrf = session.CSRF
	return b
}
func (b *browser) call(t *testing.T, path, method, body, csrf string) (int, []byte, http.Header) {
	t.Helper()
	r, _ := http.NewRequest(method, b.base+prefix+path, strings.NewReader(body))
	r.Header.Set("X-CSRF-Token", csrf)
	r.Header.Set("Origin", b.base)
	r.Header.Set("Authorization", "local-credential-must-not-leak")
	response, err := b.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data, response.Header
}

func TestRelayIsolatesLocalAndRemoteAuthenticationAndTargets(t *testing.T) {
	settings := config.Default().Settings.Security
	settings.Authentication = true
	hash, err := security.Hash("fixture-password-123")
	if err != nil {
		t.Fatal(err)
	}
	settings.PasswordHash = hash
	var mutations atomic.Int64
	upstream := func(name string) *httptest.Server {
		manager := security.New(settings)
		return httptest.NewServer(manager.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "" {
				t.Error("local credentials leaked")
			}
			if r.Method == "PUT" {
				mutations.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"name": name})
		})))
	}
	one, two := upstream("one"), upstream("two")
	defer one.Close()
	defer two.Close()
	m := New(func() []config.Backend {
		return []config.Backend{{ID: "one", Name: "one", URL: one.URL}, {ID: "two", Name: "two", URL: two.URL}}
	})
	defer m.transport.CloseIdleConnections()
	local := httptest.NewServer(security.New(config.Default().Settings.Security).Wrap(m))
	defer local.Close()
	b := openBrowser(t, local.URL)
	if status, _, _ := b.call(t, "one/status", "GET", "", ""); status != 401 {
		t.Fatal("remote authenticated without login", status)
	}
	status, data, headers := b.call(t, "one/session", "GET", "", "")
	if status != 200 || len(headers.Values("Set-Cookie")) != 0 || bytes.Contains(data, []byte(b.csrf)) {
		t.Fatal("remote session leaked or mixed with local session")
	}
	if status, _, _ := b.call(t, "one/login", "POST", `{"username":"admin","password":"fixture-password-123"}`, "wrong"); status != 403 {
		t.Fatal("local CSRF bypass")
	}
	if status, data, _ = b.call(t, "one/login", "POST", `{"username":"admin","password":"fixture-password-123"}`, b.csrf); status != 200 {
		t.Fatal(status, string(data))
	}
	if status, data, _ = b.call(t, "one/settings", "PUT", `{}`, b.csrf); status != 200 || !bytes.Contains(data, []byte("one")) {
		t.Fatal("remote mutation failed", status, string(data))
	}
	if mutations.Load() != 1 {
		t.Fatal("remote mutation not confirmed")
	}
	if status, _, _ := b.call(t, "two/status", "GET", "", ""); status != 401 {
		t.Fatal("remote targets shared login")
	}
	other := openBrowser(t, local.URL)
	if status, _, _ := other.call(t, "one/status", "GET", "", ""); status != 401 {
		t.Fatal("browsers shared remote login")
	}
	if status, _, _ := b.call(t, "one/logout", "POST", `{}`, b.csrf); status != 200 {
		t.Fatal("remote logout failed")
	}
	if status, _, _ := b.call(t, "one/status", "GET", "", ""); status != 401 {
		t.Fatal("remote logout retained credentials")
	}
	response, err := b.client.Get(local.URL + "/api/v1/session")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.NewDecoder(response.Body).Decode(&session)
	if session.CSRF != b.csrf {
		t.Fatal("remote logout changed local session")
	}
}

func TestRelayRejectsRedirectsUnconfiguredOriginsAndRecursivePaths(t *testing.T) {
	var destinationCalls atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer upstream.Close()
	m := New(func() []config.Backend { return []config.Backend{{ID: "remote", URL: upstream.URL}} })
	defer m.transport.CloseIdleConnections()
	local := httptest.NewServer(security.New(config.Default().Settings.Security).Wrap(m))
	defer local.Close()
	b := openBrowser(t, local.URL)
	for _, path := range []string{"remote/management/relay/remote/status", "remote/events/stream", "unknown/status", "remote/bgp/peers/../settings"} {
		if status, _, _ := b.call(t, path, "PUT", `{}`, b.csrf); status == 200 {
			t.Fatal("unsupported path accepted", path)
		}
	}
	if status, _, _ := b.call(t, "remote/status", "GET", "", ""); status != 502 {
		t.Fatal("redirect not rejected", status)
	}
	if destinationCalls.Load() != 0 {
		t.Fatal("followed remote redirect")
	}
}

func TestRelayDoesNotBypassTLSVerificationOrLocalSession(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS endpoint reached") }))
	defer upstream.Close()
	m := New(func() []config.Backend { return []config.Backend{{ID: "remote", URL: upstream.URL}} })
	defer m.Close()
	local := httptest.NewServer(security.New(config.Default().Settings.Security).Wrap(m))
	defer local.Close()
	response, err := http.Get(local.URL + prefix + "remote/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("missing local session accepted")
	}
	b := openBrowser(t, local.URL)
	if status, _, _ := b.call(t, "remote/status", "GET", "", ""); status != 502 {
		t.Fatal("untrusted TLS accepted", status)
	}
}

func TestRouteImportRelayEndpoints(t *testing.T) {
	for _, path := range []string{"bgp/routes/import-url", "bgp/routes/import"} {
		if !allowed(path, "POST") || allowed(path, "GET") {
			t.Fatal("route import relay method", path)
		}
	}

	if allowed("bgp/routes/import-url/anything", "POST") {
		t.Fatal("nested import route allowed")
	}
}

func TestRelayForwardsClientStatisticsToOlderBackend(t *testing.T) {
	var requestedPath atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath.Store(r.URL.Path)
		http.Error(w, `{"error":"unknown API endpoint"}`, http.StatusNotFound)
	}))
	defer upstream.Close()
	m := New(func() []config.Backend { return []config.Backend{{ID: "remote", URL: upstream.URL}} })
	defer m.Close()
	local := httptest.NewServer(security.New(config.Default().Settings.Security).Wrap(m))
	defer local.Close()
	b := openBrowser(t, local.URL)

	status, _, _ := b.call(t, "remote/dns/clients", "GET", "", "")
	if status != http.StatusNotFound || requestedPath.Load() != "/api/v1/dns/clients" {
		t.Fatalf("old backend response was not forwarded: status=%d path=%v", status, requestedPath.Load())
	}
}

func TestMonitoringRelayIsReadOnly(t *testing.T) {
	for _, path := range []string{"dns/monitoring", "dns/monitoring/history"} {
		if !allowed(path, "GET") {
			t.Fatal("monitoring read blocked", path)
		}
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			if allowed(path, method) {
				t.Fatal("monitoring mutation allowed", path, method)
			}
		}
	}
	if allowed("dns/monitoring/history/anything", "GET") {
		t.Fatal("unexpected monitoring subresource allowed")
	}
}

func TestClientSettingsRelayOnlyAllowsResourceOperations(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		if !allowed("dns/client-settings", method) {
			t.Fatal(method)
		}
	}
	for _, method := range []string{"PUT", "DELETE"} {
		if !allowed("dns/client-settings/client-id", method) {
			t.Fatal(method)
		}
	}
	for _, path := range []string{"dns/client-settings/", "dns/client-settings/client/nested", "dns/client-settings/../settings"} {
		if allowed(path, "PUT") {
			t.Fatal("unsafe client endpoint allowed", path)
		}
	}
	if allowed("dns/client-settings/client-id", "POST") || allowed("dns/client-settings/client-id", "GET") {
		t.Fatal("unsupported client operation allowed")
	}
}

func TestRelayAllowsOnlyPostForForcedAppRestart(t *testing.T) {
	if !allowed("updates/restart-app", "POST") || allowed("updates/restart-app", "GET") || allowed("updates/restart-app", "DELETE") {
		t.Fatal("App restart relay policy is incorrect")
	}
}
