package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"surge-controller/internal/config"
	"testing"
)

func TestEncryptedPersistenceAndTransaction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "controller.sqlite")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	d := config.Default()
	d.Settings.Surge.APIToken = "test-only-credential-should-not-appear"
	d.Routes = []config.Route{{ID: "manual", Prefix: "10.20.0.0/16", NextHop: "192.168.20.1", Enabled: true, Binding: "none"}}
	if e = s.Save(ctx, d); e != nil {
		t.Fatal(e)
	}
	var raw []byte
	if e = s.DB.QueryRow("SELECT document FROM settings").Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), d.Settings.Surge.APIToken) {
		t.Fatal("plaintext token in database")
	}
	bad := d
	bad.Settings.BGP.RouteLimit = 1
	if e = s.Save(ctx, bad); e == nil {
		t.Fatal("unsafe config persisted")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	loaded, e := s.Load(ctx)
	if e != nil || loaded.Settings.Surge.APIToken != d.Settings.Surge.APIToken || len(loaded.Routes) != 1 || loaded.Routes[0].Binding != "none" {
		t.Fatalf("persistence failed: %v", e)
	}
	if loaded.Redacted().Settings.Surge.APIToken != "" {
		t.Fatal("secret export")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("database permissions")
	}
}
func TestEventRetentionAndNoQueryTable(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "private", "c.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := 0; i < 105; i++ {
		if _, e = s.Event(context.Background(), "INFO", "test", "operational event", 100); e != nil {
			t.Fatal(e)
		}
	}
	ev, e := s.Events(context.Background(), 0)
	if e != nil || len(ev) != 100 {
		t.Fatal(len(ev), e)
	}
	var tables string
	_ = s.DB.QueryRow("SELECT group_concat(name) FROM sqlite_master WHERE type='table'").Scan(&tables)
	if strings.Contains(tables, "queries") {
		t.Fatal("persistent query history unexpectedly enabled")
	}
}

func TestVersionOneMigrationRetainsConfigAndLocalHosts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "c.sqlite")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	d := config.Default()
	d.Settings.General.Theme = "dark"
	if e = s.Save(ctx, d); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("DROP TABLE local_dns_hosts"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("DELETE FROM schema_migrations WHERE version=2"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	loaded, e := s.Load(ctx)
	if e != nil || loaded.Settings.General.Theme != d.Settings.General.Theme || len(loaded.Hosts) != 0 {
		t.Fatal(loaded, e)
	}
	loaded.Hosts = []config.Host{{ID: "h", Domain: "nas.test", Match: "exact", Addresses: []string{"192.0.2.53"}, TTL: 60, Source: "manual", Enabled: true}}
	if e = s.Save(ctx, loaded); e != nil {
		t.Fatal(e)
	}
	loaded, e = s.Load(ctx)
	if e != nil || len(loaded.Hosts) != 1 || loaded.Hosts[0].Addresses[0] != "192.0.2.53" {
		t.Fatal(loaded, e)
	}
}
