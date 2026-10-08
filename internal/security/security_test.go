package security

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"surge-controller/internal/config"
	"testing"
)

func call(h http.Handler, path, method string, body []byte, cookie *http.Cookie, csrf, origin, host string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:1055"+path, bytes.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12345"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if host != "" {
		r.Host = host
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestSessionAuthenticationCSRFAndRebinding(t *testing.T) {
	c := config.Default().Settings.Security
	hash, e := Hash("test-password-only-123")
	if e != nil {
		t.Fatal(e)
	}
	c.Authentication = true
	c.PasswordHash = hash
	m := New(c)
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	w := call(h, "/api/v1/session", "GET", nil, nil, "", "", "")
	var session struct {
		CSRF string `json:"csrf"`
	}
	if json.Unmarshal(w.Body.Bytes(), &session) != nil {
		t.Fatal("no session")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure cookie")
	}
	if w = call(h, "/api/v1/settings", "GET", nil, cookie, "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	body := []byte(`{"username":"admin","password":"test-password-only-123"}`)
	if w = call(h, "/api/v1/login", "POST", body, cookie, "", "", ""); w.Code != 403 {
		t.Fatal("login CSRF accepted")
	}
	w = call(h, "/api/v1/login", "POST", body, cookie, session.CSRF, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookie = w.Result().Cookies()[0]
	_ = json.Unmarshal(w.Body.Bytes(), &session)
	if w = call(h, "/api/v1/settings", "PUT", nil, cookie, session.CSRF, "http://evil.example", ""); w.Code != 403 {
		t.Fatal("cross-origin mutation")
	}
	if w = call(h, "/api/v1/settings", "PUT", nil, cookie, session.CSRF, "", ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w = call(h, "/api/v1/session", "GET", nil, nil, "", "", "evil.example"); w.Code != 403 {
		t.Fatal("DNS rebinding host accepted")
	}
	m.Update(c)
	if w = call(h, "/api/v1/settings", "GET", nil, cookie, "", "", ""); w.Code != 401 {
		t.Fatal("session survived authentication configuration invalidation")
	}
}
func TestAnonymousMutationsStillNeedCSRFAndNetwork(t *testing.T) {
	m := New(config.Default().Settings.Security)
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	if w := call(h, "/api/v1/settings", "PUT", nil, nil, "", "", ""); w.Code != 403 {
		t.Fatal("anonymous CSRF bypass")
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:1055/api/v1/settings", nil)
	r.RemoteAddr = "192.0.2.55:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("network allowlist bypass")
	}
}
