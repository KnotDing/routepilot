package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"testing"
)

func TestClientSettingsCRUDNormalizeAndSurviveOldSettingsForms(t *testing.T) {
	s := &durableService{routeService: routeService{doc: config.Default()}}
	a := &API{Service: s}
	call := func(method, path string, value any) {
		t.Helper()
		raw, _ := json.Marshal(value)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest(method, "/api/v1/"+path, bytes.NewReader(raw)))
		if w.Code < 200 || w.Code >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
	}
	call("POST", "dns/client-settings", config.Client{IP: "::ffff:192.0.2.10", Name: " 办公电脑 ", Decision: model.Surge})
	row := s.doc.Settings.DNS.Clients[0]
	if row.ID == "" || row.IP != "192.0.2.10" || row.Name != "办公电脑" {
		t.Fatal(row)
	}
	call("GET", "dns/client-settings", nil)
	legacySettings := config.Default().Settings
	legacySettings.General.Theme = "dark"
	call("PUT", "settings", legacySettings)
	if len(s.doc.Settings.DNS.Clients) != 1 || s.doc.Settings.DNS.Clients[0].Decision != model.Surge {
		t.Fatal("old settings form erased client settings")
	}
	row.Decision = model.Reject
	call("PUT", "dns/client-settings/"+row.ID, row)
	if s.doc.Settings.DNS.Clients[0].Decision != model.Reject {
		t.Fatal("client update not saved")
	}
	call("DELETE", "dns/client-settings/"+row.ID, nil)
	if len(s.doc.Settings.DNS.Clients) != 0 {
		t.Fatal("client deletion not saved")
	}
}
