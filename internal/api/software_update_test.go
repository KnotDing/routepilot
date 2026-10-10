package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/softwareupdate"
	"testing"
	"time"
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
func TestPlanUpdateAutomation(t *testing.T) {
	policy := config.Updates{AutomaticCheck: true, AutomaticInstall: true, CheckIntervalHours: 12, UpdateTime: "03:00"}
	day := time.Date(2026, 1, 10, 0, 0, 0, 0, time.Local)
	after := day.Add(3 * time.Hour)
	before := day.Add(2*time.Hour + 59*time.Minute)
	if plan := planUpdateAutomation(after, policy, time.Time{}, ""); !plan.check || !plan.install {
		t.Fatal("first tick after the scheduled time must check and install", plan)
	}
	if plan := planUpdateAutomation(after, policy, after.Add(-time.Hour), ""); plan.check || !plan.install {
		t.Fatal("recent manual check must only schedule the install", plan)
	}
	if plan := planUpdateAutomation(after, policy, time.Time{}, after.Format("2006-01-02")); !plan.check || plan.install {
		t.Fatal("the daily install must run at most once", plan)
	}
	if plan := planUpdateAutomation(before, policy, time.Time{}, ""); !plan.check || plan.install {
		t.Fatal("install must wait for the local update time", plan)
	}
	quiet := policy
	quiet.AutomaticInstall = false
	if plan := planUpdateAutomation(after, quiet, time.Time{}, ""); !plan.check || plan.install {
		t.Fatal("disabled automatic installation scheduled an install", plan)
	}
	off := policy
	off.AutomaticCheck = false
	if plan := planUpdateAutomation(after, off, time.Time{}, ""); plan.check || plan.install {
		t.Fatal("disabled automatic checks scheduled work", plan)
	}
	stale := policy
	stale.CheckIntervalHours = 12
	if plan := planUpdateAutomation(after, stale, after.Add(-time.Hour), ""); plan.check {
		t.Fatal("check ran before its interval elapsed", plan)
	}
	broken := policy
	broken.UpdateTime = "not-a-time"
	if plan := planUpdateAutomation(after, broken, time.Time{}, ""); !plan.check || plan.install {
		t.Fatal("invalid update time must not schedule an install", plan)
	}
}
