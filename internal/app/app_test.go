package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/miekg/dns"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/model"
	"surge-controller/internal/security"
	"surge-controller/tests/mocks"
	"testing"
	"time"
)

func fixtureApp(t *testing.T) *App {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	cli := filepath.Join(dir, "surge-cli-mock")
	script := `#!/bin/sh
case "$2 $3" in
 "status ") printf '%s' '{"profile":"test","mode":"rule","start-time":1,"features":{}}' ;;
 "dump profile") printf '%s' '{"profile":"[Host]\n*.example.test = 192.0.2.1\nhome.example.test = server:system\n"}' ;;
 "dump rule") printf '%s' '{"rules":["FINAL,DIRECT"]}' ;;
 "dump temp-rule") printf '%s' '{"rules":[]}' ;;
 "policy-group list") printf '%s' '{"groups":[]}' ;;
 "rule explain") printf '%s' '{"rule":"FINAL,DIRECT","rule-policy":"DIRECT","final":"DIRECT","final-type":"Direct"}' ;;
 *) exit 1 ;;
esac
`
	if e := os.WriteFile(cli, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	udp, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	fake := &dns.Server{PacketConn: udp, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) { _ = w.WriteMsg(mocks.AddressReply(q, "198.18.1.1")) })}
	go fake.ActivateAndServe()
	t.Cleanup(func() { _ = fake.Shutdown() })
	path := filepath.Join(dir, "c.sqlite")
	store, e := database.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	d := config.Default()
	d.Settings.Cache.TimeoutMS = 5000
	d.Settings.Surge.CLIPath = cli
	d.Settings.Surge.DNS = udp.LocalAddr().String()
	d.Settings.Health.IntervalMS = 100
	d.Settings.Health.Successes = 1
	if e = store.Save(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	store.Close()
	a, e := Open(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}
func apiCall(h http.Handler, method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:1055/api/v1/"+path, bytes.NewReader(raw))
	r.RemoteAddr = "127.0.0.1:1234"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestFRRExportDoesNotChangeModeOrExternalRouteDeclarations(t *testing.T) {
	a := fixtureApp(t)
	d := a.Configuration()
	d.Settings.DNS.Source4 = "192.168.20.2"
	d.Settings.Routing = config.Routing{Mode: "static"}
	if e := a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	h := a.Handler()
	session := apiCall(h, "GET", "session", nil, nil, "")
	var credentials struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(session.Body.Bytes(), &credentials)
	before := a.Configuration()
	w := apiCall(h, "POST", "bgp/export-frr", map[string]any{"gateway_asn": 65001, "gateway_router_id": "192.168.20.1", "controller_address": "192.168.20.2"}, session.Result().Cookies()[0], credentials.CSRF)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var exported struct {
		Content string `json:"content"`
	}
	json.Unmarshal(w.Body.Bytes(), &exported)
	if !strings.Contains(exported.Content, "router bgp 65001") || strings.Contains(exported.Content, "permit 198.18.0.0/15") || a.Configuration().Settings.Routing != before.Settings.Routing || a.runtime.routes.Status().Enabled {
		t.Fatal("export changed routing or included static routes")
	}
}
func TestManagementCRUDPrivacyAtomicLimitAndRealRuleTest(t *testing.T) {
	a := fixtureApp(t)
	h := a.Handler()
	w := apiCall(h, "GET", "session", nil, nil, "")
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &session)
	cookie := w.Result().Cookies()[0]
	w = apiCall(h, "POST", "bgp/routes", map[string]any{"prefix": "10.20.0.0/16", "next_hop": "192.168.20.1", "enabled": true}, cookie, session.CSRF)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	d := a.Configuration()
	if len(d.Routes) != 1 || d.Routes[0].Binding != "none" {
		t.Fatal("manual route default binding")
	}
	invalid := d.Routes[0]
	invalid.NextHop = "invalid"
	w = apiCall(h, "PUT", "bgp/routes/"+invalid.ID, invalid, cookie, session.CSRF)
	if w.Code != 422 || a.Configuration().Routes[0].NextHop != d.Routes[0].NextHop {
		t.Fatal("rejected resource update mutated live configuration", w.Code, w.Body.String())
	}
	settings := d.Redacted().Settings
	settings.BGP.RouteLimit = 1
	w = apiCall(h, "PUT", "settings", settings, cookie, session.CSRF)
	if w.Code != 422 || a.Configuration().Settings.BGP.RouteLimit != 128 {
		t.Fatal("hard limit partially committed")
	}
	ev, _ := a.store.Events(context.Background(), 0)
	critical := false
	for _, event := range ev {
		critical = critical || event.Severity == "CRITICAL"
	}
	if !critical {
		t.Fatal("missing critical hard-limit event")
	}
	w = apiCall(h, "POST", "test/domain", map[string]any{"domain": "example.com", "client_ip": "192.0.2.1", "port": 443, "protocol": "tcp"}, cookie, session.CSRF)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = apiCall(h, "GET", "configuration/export", nil, cookie, "")
	if bytes.Contains(w.Body.Bytes(), []byte("password_hash")) || bytes.Contains(w.Body.Bytes(), []byte("api_token")) {
		t.Fatal("secret export")
	}
	w = apiCall(h, "DELETE", "bgp/routes/"+d.Routes[0].ID, nil, cookie, session.CSRF)
	if w.Code != 200 || len(a.Configuration().Routes) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if a.runtime.routes.Status().Advertised != 0 {
		t.Fatal("disabled BGP advertised a route")
	}
}

func TestHealthProbeAcceptsConfiguredDomainWithoutFinalDot(t *testing.T) {
	a := fixtureApp(t)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, component := range a.runtime.health.Values() {
			if component.Name == "Surge Fake-IP DNS" && component.State == "UP" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("local Fake-IP DNS probe failed for a valid undotted configured domain", a.runtime.health.Values())
}

func TestLocalDNSCRUDImportHotReloadAndNoBGPRoutes(t *testing.T) {
	a := fixtureApp(t)
	h := a.Handler()
	w := apiCall(h, "GET", "session", nil, nil, "")
	var session struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &session)
	cookie := w.Result().Cookies()[0]
	oldRuntime := a.runtime
	host := config.Host{Domain: "nas.example.test", Match: "exact", Addresses: []string{"192.0.2.53"}, TTL: 120, Source: "manual", Enabled: true}
	w = apiCall(h, "POST", "dns/hosts", host, cookie, session.CSRF)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	id := a.Configuration().Hosts[0].ID
	w = apiCall(h, "POST", "dns/hosts/import-surge", nil, cookie, session.CSRF)
	if w.Code != 200 || len(a.Configuration().Hosts) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = apiCall(h, "POST", "dns/hosts/import-surge", nil, cookie, session.CSRF)
	if w.Code != 200 || len(a.Configuration().Hosts) != 2 {
		t.Fatal("import was not idempotent", w.Body.String())
	}
	m := new(dns.Msg)
	m.SetQuestion("nas.example.test.", dns.TypeA)
	reply := a.runtime.dispatcher.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.100")}, m)
	if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 1 || reply.Answer[0].(*dns.A).A.String() != "192.0.2.53" || a.runtime != oldRuntime || a.runtime.routes.Status().Advertised != 0 {
		t.Fatal("local mapping changed BGP/runtime or lost precedence", reply)
	}
	host.Addresses = []string{"198.18.1.1"}
	w = apiCall(h, "PUT", "dns/hosts/"+id, host, cookie, session.CSRF)
	if w.Code != 422 || a.Configuration().Hosts[0].Addresses[0] != "192.0.2.53" {
		t.Fatal("invalid host edit leaked", w.Code, w.Body.String())
	}
	w = apiCall(h, "DELETE", "dns/hosts/"+id, nil, cookie, session.CSRF)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	reply = a.runtime.dispatcher.Dispatch(context.Background(), model.DNSQueryContext{}, m)
	if reply.Answer[0].(*dns.A).A.String() != "192.0.2.1" {
		t.Fatal("deleted manual host shadowed imported mapping")
	}
}

func TestServeStartsAfterPreappliedAddressAndReleasesPort(t *testing.T) {
	a := fixtureApp(t)
	reserved, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := reserved.Addr().String()
	reserved.Close()
	d := a.Configuration()
	d.Settings.General.WebListen = address
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx) }()
	client := &http.Client{Timeout: time.Second}
	ready := false
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); {
		select {
		case e := <-done:
			t.Fatal("management startup exited", e)
		default:
		}
		response, e := client.Get("http://" + address + "/api/v1/session")
		if e == nil {
			response.Body.Close()
			ready = response.StatusCode == 200
			if ready {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("management listener did not start")
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("management shutdown timed out")
	}
	listener, e := net.Listen("tcp", address)
	if e != nil {
		t.Fatal("listener port leaked", e)
	}
	listener.Close()
}

func TestRoutingSettingsClearStaleConfirmationWithoutRestartingDNSCore(t *testing.T) {
	a := fixtureApp(t)
	rt := a.runtime
	d := a.Configuration()
	d.Settings.Routing = config.Routing{Mode: "static", StaticFakeIPConfigured: true}
	d.Settings.Surge.NextHop4 = "192.0.2.1"
	if e := a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if a.Configuration().Settings.Routing.StaticFakeIPConfigured {
		t.Fatal("mode change retained a stale declaration")
	}
	rt = a.runtime
	d = a.Configuration()
	d.Settings.Routing.StaticFakeIPConfigured = true
	if e := a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if a.runtime != rt || !a.Configuration().Settings.Routing.StaticFakeIPConfigured || a.runtime.routes.Status().Enabled {
		t.Fatal("confirmation restarted DNS or depended on BGP")
	}
	d = a.Configuration()
	d.Settings.BGP.Enabled = true
	d.Settings.BGP.LocalASN = 0
	d.Settings.BGP.RouterID = "ignored in static mode"
	if e := a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if a.runtime != rt || a.runtime.routes.Status().Enabled {
		t.Fatal("static mode started a BGP engine")
	}
}

func TestCacheAPIValidatesPaginationAndReportsLifecycle(t *testing.T) {
	a := fixtureApp(t)
	for i := 0; i < 120; i++ {
		a.runtime.engine.Cache.Put(classifier.Key{Domain: fmt.Sprintf("item-%d.test.", i)}, model.DecisionResult{Domain: fmt.Sprintf("item-%d.test.", i), Decision: model.Surge}, a.runtime.engine.Cache.Generation(), time.Minute)
	}
	h := a.Handler()
	w := apiCall(h, "GET", "session", nil, nil, "")
	var session struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &session)
	cookie := w.Result().Cookies()[0]
	w = apiCall(h, "GET", "dns/cache?offset=100&limit=50", nil, cookie, "")
	var page struct {
		Entries  []model.DecisionResult `json:"entries"`
		Total    int                    `json:"total"`
		Capacity int                    `json:"capacity"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Entries) != 20 || page.Total != 120 || page.Capacity != 10000 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"dns/cache?limit=1001", "dns/cache?offset=-1", "dns/cache?decision=invalid"} {
		if w = apiCall(h, "GET", path, nil, cookie, ""); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
}

func TestRemoteProfileEditsDoNotRestartDNSAndRelayChangesOnlyRemote(t *testing.T) {
	local, remote := fixtureApp(t), fixtureApp(t)
	remoteServer := httptest.NewServer(remote.Handler())
	defer remoteServer.Close()
	h := local.Handler()
	session := apiCall(h, "GET", "session", nil, nil, "")
	cookie := session.Result().Cookies()[0]
	var credentials struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(session.Body.Bytes(), &credentials)
	runtime := local.runtime
	response := apiCall(h, "POST", "management/backends", config.Backend{Name: "Remote fixture", URL: remoteServer.URL}, cookie, credentials.CSRF)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	if local.runtime != runtime {
		t.Fatal("remote profile edit restarted DNS")
	}
	id := local.Configuration().Settings.RemoteBackends[0].ID
	response = apiCall(h, "POST", "management/relay/"+id+"/dns/hosts", config.Host{Domain: "remote-only.test", Match: "exact", Addresses: []string{"192.0.2.53"}, TTL: 60, Enabled: true}, cookie, credentials.CSRF)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	if len(local.Configuration().Hosts) != 0 || len(remote.Configuration().Hosts) != 1 {
		t.Fatal("relay mutation applied to wrong controller")
	}
	response = apiCall(h, "PUT", "management/relay/"+id+"/mode", map[string]string{"mode": "force-direct"}, cookie, credentials.CSRF)
	if response.Code != 200 || remote.Configuration().Settings.General.Mode != "force-direct" || local.Configuration().Settings.General.Mode != "auto" {
		t.Fatal("remote mode leaked into local service", response.Code)
	}
	persisted, err := local.store.Load(context.Background())
	if err != nil || len(persisted.Settings.RemoteBackends) != 1 || persisted.Settings.RemoteBackends[0].URL != remoteServer.URL {
		t.Fatal("remote profile not persisted", err)
	}
	response = apiCall(h, "DELETE", "management/backends/"+id, nil, cookie, credentials.CSRF)
	if response.Code != 200 || local.runtime != runtime {
		t.Fatal("remote profile removal restarted DNS")
	}
	response = apiCall(h, "GET", "management/relay/"+id+"/status", nil, cookie, "")
	if response.Code != 404 {
		t.Fatal("deleted target remained reachable")
	}
}

func TestAuthenticatedSettingsAndBackupSavePreserveHiddenSecrets(t *testing.T) {
	a := fixtureApp(t)
	d := a.Configuration()
	hash, err := security.Hash("fixture-password-123")
	if err != nil {
		t.Fatal(err)
	}
	d.Settings.Security.Authentication = true
	d.Settings.Security.PasswordHash = hash
	d.Settings.Surge.APIToken = "fixture-token"
	if err = a.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	session := apiCall(h, "GET", "session", nil, nil, "")
	cookie := session.Result().Cookies()[0]
	var credentials struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(session.Body.Bytes(), &credentials)
	login := apiCall(h, "POST", "login", map[string]string{"username": "admin", "password": "fixture-password-123"}, cookie, credentials.CSRF)
	if login.Code != 200 {
		t.Fatal(login.Code)
	}
	cookie = login.Result().Cookies()[0]
	_ = json.Unmarshal(login.Body.Bytes(), &credentials)
	settings := apiCall(h, "GET", "settings", nil, cookie, "")
	var input config.Settings
	_ = json.Unmarshal(settings.Body.Bytes(), &input)
	if input.Security.PasswordHash != "" || input.Surge.APIToken != "" {
		t.Fatal("management settings exposed secrets")
	}
	input.Cache.InvalidateOnReadFailure = true
	response := apiCall(h, "PUT", "settings", input, cookie, credentials.CSRF)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if a.Configuration().Settings.Security.PasswordHash != hash || a.Configuration().Settings.Surge.APIToken != "fixture-token" {
		t.Fatal("save lost hidden credentials")
	}
	doc := a.Configuration().Redacted()
	response = apiCall(h, "POST", "configuration/import", doc, cookie, credentials.CSRF)
	if response.Code != 200 {
		t.Fatal("redacted backup could not be restored", response.Code, response.Body.String())
	}
	input.Security.PasswordHash = "untrusted"
	response = apiCall(h, "PUT", "settings", input, cookie, credentials.CSRF)
	if response.Code != 400 {
		t.Fatal("API accepted caller-supplied password hash")
	}
}
