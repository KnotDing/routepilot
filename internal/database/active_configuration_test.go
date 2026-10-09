package database

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"surge-controller/internal/config"
)

func TestActiveConfigurationCheckpointIsEncryptedAndIndependent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "private", "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	d := config.Default()
	d.Settings.Surge.APIToken = "checkpoint-private-token"
	if err = s.SaveActiveConfiguration(ctx, d); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = s.DB.QueryRow("SELECT document FROM statistics WHERE id='active_configuration'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, d.Settings.Surge.APIToken) || strings.Contains(raw, "web_listen") {
		t.Fatal("checkpoint is not encrypted")
	}
	desired := d.Clone()
	desired.Settings.General.Name = "unapplied desired"
	if err = s.Save(ctx, desired); err != nil {
		t.Fatal(err)
	}
	active, err := s.LoadActiveConfiguration(ctx)
	if err != nil || active.Settings.General.Name != d.Settings.General.Name || active.Settings.Surge.APIToken != d.Settings.Surge.APIToken {
		t.Fatal("checkpoint changed with desired configuration", err)
	}
}
