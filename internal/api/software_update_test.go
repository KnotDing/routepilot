package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/softwareupdate"
	"testing"
)

func TestSoftwareUpdateAPIGuards(t *testing.T) {
	s := &routeService{doc: config.Default()}
	a := &API{Service: s}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest(method, "/api/v1/updates/"+path, strings.NewReader(body)))
		return w
	}
	if w := call("GET", "status", ""); w.Code != 501 {
		t.Fatal(w.Code, w.Body.String())
	}
	// A temporary database deliberately prevents the root service update path.
	a.SoftwareUpdates = softwareupdate.New(context.Background(), t.TempDir()+"/test.sqlite", nil)
	w := call("GET", "status", "")
	var status softwareupdate.Status
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || w.Code != 200 || status.Supported || status.Current == "" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	if w := call("POST", "install", `{"version":"v1.0.0"}`); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	s.doc.Settings.Security.Authentication = true
	for _, body := range []string{`{`, `{"version":4}`, `{"version":"v1.0.0","url":"https://example.com"}`, `{"version":"v1.0.0","command":"anything"}`, `{} {}`, strings.Repeat(" ", 512*1024) + `{}`} {
		if w := call("POST", "install", body); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call("POST", "install", `{"version":"v1.0.0"}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, endpoint := range []struct{ method, path string }{{"GET", "install"}, {"POST", "status"}, {"GET", "unknown"}} {
		if w := call(endpoint.method, endpoint.path, ""); w.Code != 404 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if s.applies != 0 {
		t.Fatal("Update endpoint unexpectedly applied configuration")
	}
}
