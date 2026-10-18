package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"surge-controller/internal/config"
	"surge-controller/internal/dhcpserver"
)

func TestDHCPConfigurationIsIsolatedAndBackedUp(t *testing.T) {
	doc := config.Default()
	service := &routeService{doc: doc}
	handler := (&API{Service: service}).Handler()
	dhcp := config.DHCP{Options: config.DHCPOptions{LeaseSeconds: 3600, Domain: "example.lan"}}
	body, _ := json.Marshal(dhcp)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("PUT", "/api/v1/dhcp/configuration", bytes.NewReader(body)))
	if response.Code != 200 {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	if service.doc.Settings.DHCP.Options.Domain != "example.lan" || service.doc.Settings.DNS.Listeners[0] != doc.Settings.DNS.Listeners[0] {
		t.Fatal("DHCP did not save independently")
	}
	response = httptest.NewRecorder()
	body, _ = json.Marshal(doc.Settings)
	handler.ServeHTTP(response, httptest.NewRequest("PUT", "/api/v1/settings", bytes.NewReader(body)))
	if response.Code != 200 || service.doc.Settings.DHCP.Options.Domain != "example.lan" {
		t.Fatalf("older settings save erased DHCP: %d %s", response.Code, response.Body.String())
	}
	backup, err := makeConfigurationBackup(service.doc, []string{"dhcp"})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := mergeBackup(config.Default(), backup.Configuration, []string{"dhcp"})
	if err != nil || merged.Settings.DHCP.Options.Domain != "example.lan" || merged.Settings.General.WebListen != doc.Settings.General.WebListen {
		t.Fatalf("DHCP restore failed: %v", err)
	}
}

type dhcpCheckService struct {
	*routeService
	checked string
}

func (s *dhcpCheckService) CheckDHCP(_ context.Context, d config.DHCP) (dhcpserver.CheckResult, error) {
	s.checked = d.Interface
	return dhcpserver.CheckResult{Interface: d.Interface, AddressMode: "dhcp", Servers: []dhcpserver.DiscoveredServer{{Address: "192.0.2.1"}}, Warnings: []string{"existing server"}}, nil
}

func TestDHCPPreflightReturnsWarningsWithoutSavingConfiguration(t *testing.T) {
	s := &dhcpCheckService{routeService: &routeService{doc: config.Default()}}
	handler := (&API{Service: s}).Handler()
	body, _ := json.Marshal(config.DHCP{Interface: "en0"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/dhcp/check", bytes.NewReader(body)))
	var result dhcpserver.CheckResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || s.checked != "en0" || result.AddressMode != "dhcp" || len(result.Servers) != 1 || len(result.Warnings) != 1 {
		t.Fatalf("missing preflight result: %d %+v", w.Code, result)
	}
	if s.doc.Settings.DHCP.Enabled || s.doc.Settings.DHCP.Interface != "" {
		t.Fatal("preflight modified the saved DHCP configuration")
	}
}
