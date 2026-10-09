package app

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSaveConfigurationDoesNotWaitForActivation(t *testing.T) {
	a := fixtureApp(t)
	old := a.activeConfiguration()
	a.transition.Lock()
	var once sync.Once
	release := func() { once.Do(a.transition.Unlock) }
	t.Cleanup(release)
	for _, name := range []string{"first saved", "latest saved"} {
		d := a.Configuration()
		d.Settings.General.Name = name
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := a.SaveConfiguration(ctx, d); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
	}
	saved, err := a.store.Load(context.Background())
	if err != nil || saved.Settings.General.Name != "latest saved" {
		t.Fatalf("desired configuration was not durably saved: %v", err)
	}
	if a.activeConfiguration().Settings.General.Name != old.Settings.General.Name {
		t.Fatal("blocked activation unexpectedly changed the runtime")
	}
	release()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		status := a.application
		a.mu.RUnlock()
		if status.AppliedRevision == 2 && status.State == "succeeded" {
			if a.activeConfiguration().Settings.General.Name != "latest saved" {
				t.Fatal("worker did not apply the latest saved configuration")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("latest saved configuration did not finish activation")
}

func TestFailedActivationKeepsDurableDesiredConfiguration(t *testing.T) {
	a := fixtureApp(t)
	old := a.activeConfiguration()
	d := a.Configuration()
	d.Settings.Security.TLSCert = "/nonexistent/routepilot-test.crt"
	d.Settings.Security.TLSKey = "/nonexistent/routepilot-test.key"
	if err := a.SaveConfiguration(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		status := a.application
		a.mu.RUnlock()
		if status.State == "failed" {
			saved, err := a.store.Load(context.Background())
			if err != nil || saved.Settings.Security.TLSCert != d.Settings.Security.TLSCert {
				t.Fatal("activation failure rolled back the saved configuration", err)
			}
			if a.activeConfiguration().Settings.Security.TLSCert != old.Settings.Security.TLSCert || status.Error == "" {
				t.Fatal("failed activation did not retain the working runtime and report its error")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("activation failure was not reported")
}

func TestStartupRestoresWorkingConfigurationAfterSavedTLSFailure(t *testing.T) {
	a := fixtureApp(t)
	old := a.activeConfiguration()
	d := a.Configuration()
	d.Settings.Security.TLSCert = "/nonexistent/routepilot-test.crt"
	d.Settings.Security.TLSKey = "/nonexistent/routepilot-test.key"
	if err := a.SaveConfiguration(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	waitConfiguration(t, a, "failed")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), filepath.Join(a.dir, "c.sqlite"))
	if err != nil {
		t.Fatal("saved activation failure prevented recovery on restart", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if reopened.activeConfiguration().Settings.Security.TLSCert != old.Settings.Security.TLSCert || reopened.Configuration().Settings.Security.TLSCert != d.Settings.Security.TLSCert {
		t.Fatal("restart did not preserve both working and desired configurations")
	}
	waitConfiguration(t, reopened, "failed")
}

func waitConfiguration(t *testing.T, a *App, state string) ApplicationStatus {
	t.Helper()
	a.mu.RLock()
	revision := a.application.Revision
	a.mu.RUnlock()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		status := a.application
		a.mu.RUnlock()
		if status.Revision >= revision && status.State == state {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("configuration did not reach %s", state)
	return ApplicationStatus{}
}
