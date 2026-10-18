package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surge-controller/internal/dhcpserver"
)

func TestDHCPLeasesSurviveStoreReopenWithoutExtendingExpiry(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "controller.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	want := dhcpserver.State{Subnet: "192.0.2.0/24", Leases: []dhcpserver.Lease{{Address: "192.0.2.100", MAC: "02:00:00:00:00:01", ClientID: "id:01ff80", ExpiresAt: expires, State: "bound"}}}
	if err = s.SaveDHCP(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.LoadDHCP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Subnet != want.Subnet || len(got.Leases) != 1 || !got.Leases[0].ExpiresAt.Equal(expires) || got.Leases[0].ClientID != "id:01ff80" {
		t.Fatalf("%+v", got)
	}
}
