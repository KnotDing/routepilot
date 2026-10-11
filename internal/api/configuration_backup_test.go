package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"surge-controller/internal/config"
	"testing"
)

func TestConfigurationBackupExportsAndRestoresOnlySelectedSections(t *testing.T) {
	source := config.Default()
	source.Settings.General.Theme = "dark"
	source.Settings.DNS.Listeners = []string{"192.0.2.53:1053"}
	source.Settings.Surge.APIToken = "source-secret"
	backup, err := makeConfigurationBackup(source, []string{"general"})
	if err != nil {
		t.Fatal(err)
	}
	if backup.Configuration.Settings.General.Theme != "dark" || len(backup.Configuration.Settings.DNS.Listeners) != 0 {
		t.Fatal("backup did not contain exactly the selected general settings")
	}
	if backup.Configuration.Settings.Surge.APIToken != "" {
		t.Fatal("backup exposed a hidden credential")
	}

	current := config.Default()
	current.Settings.DNS.Listeners = []string{"127.0.0.1:1053"}
	current.Settings.Surge.APIToken = "existing-secret"
	service := &routeService{doc: current}
	handler := (&API{Service: service}).Handler()
	requestBody, err := json.Marshal(configurationRestore{Backup: backup, Sections: []string{"general"}})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/v1/configuration/import", bytes.NewReader(requestBody)))
	if response.Code != 200 {
		t.Fatalf("partial restore failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if service.doc.Settings.General.Theme != "dark" {
		t.Fatal("selected section was not restored")
	}
	if len(service.doc.Settings.DNS.Listeners) != 1 || service.doc.Settings.DNS.Listeners[0] != "127.0.0.1:1053" {
		t.Fatal("unselected section was overwritten")
	}
	if service.doc.Settings.Surge.APIToken != "existing-secret" {
		t.Fatal("restore did not preserve the existing credential")
	}
}

func TestConfigurationBackupRejectsEmptyUnknownAndDuplicateSections(t *testing.T) {
	for _, sections := range [][]string{nil, {"unknown"}, {"general", "general"}} {
		if _, err := makeConfigurationBackup(config.Default(), sections); err == nil {
			t.Fatalf("invalid section selection accepted: %#v", sections)
		}
	}
}
