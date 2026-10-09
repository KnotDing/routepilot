package softwareupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"surge-controller/internal/buildinfo"
)

func TestCompleteApplicationUpdateOrdering(t *testing.T) {
	for _, tc := range []struct {
		backend, app string
		want         bool
	}{
		{"0.5.4", "0.5.4", true}, {"0.5.5", "0.5.4", true},
		{"0.5.4", "0.5.5", true}, {"0.5.5", "", true},
		{"0.5.5", "0.5.5", false}, {"0.5.6", "0.5.4", false},
		{"0.5.4", "0.5.6", false}, {"invalid", "0.5.4", false},
	} {
		if got := applicationUpdateAvailable("v0.5.5", tc.backend, tc.app); got != tc.want {
			t.Errorf("backend=%s app=%s: got %v", tc.backend, tc.app, got)
		}
	}
	if applicationUpdateAvailable("v0.5.6-beta", "0.5.4", "0.5.4") {
		t.Fatal("prerelease accepted")
	}
}

func TestApplicationAndServiceUpdateTransaction(t *testing.T) {
	for _, scenario := range []string{"success", "missing App", "missing candidate", "stop failure", "readiness failure", "cancelled", "rollback failure", "unsafe target"} {
		t.Run(scenario, func(t *testing.T) {
			service := newActivationFixture(t)
			dir := t.TempDir()
			a := applicationActivation{target: filepath.Join(dir, "RoutePilot.app"), candidate: filepath.Join(dir, "candidate.app"), previous: filepath.Join(dir, "previous.app")}
			for path, body := range map[string]string{a.target: "old App", a.candidate: "new App"} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "identity"), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			retained := false
			a.retained = func() { retained = true }
			ctx := context.Background()
			switch scenario {
			case "missing App":
				_ = os.RemoveAll(a.target)
			case "missing candidate":
				_ = os.RemoveAll(a.candidate)
			case "stop failure":
				service.a.stop = func(context.Context) error { return errors.New("stop refused") }
			case "readiness failure":
				service.a.ready = func(context.Context, string, time.Time) error { return errors.New("not ready") }
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "unsafe target":
				_ = os.RemoveAll(a.target)
				if err := os.Symlink(a.candidate, a.target); err != nil {
					t.Fatal(err)
				}
			}
			a.activateService = func() error {
				if readFixture(t, filepath.Join(a.target, "identity")) != "new App" {
					t.Fatal("service started before App swap")
				}
				if scenario == "rollback failure" {
					_ = os.RemoveAll(a.target)
					_ = os.Mkdir(a.target, 0700)
					_ = os.Mkdir(a.candidate, 0700)
					_ = os.WriteFile(filepath.Join(a.candidate, "blocked"), []byte("retained"), 0600)
					return errors.New("failed")
				}
				return activateBackend(ctx, "v1.0.0", 101, service.a)
			}
			err := activateApplication(a)
			if scenario == "success" || scenario == "missing App" {
				if err != nil || readFixture(t, filepath.Join(a.target, "identity")) != "new App" || readFixture(t, service.a.executable) != "new executable" {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("failure accepted")
				}
				if scenario == "rollback failure" {
					if !retained || readFixture(t, filepath.Join(a.previous, "identity")) != "old App" {
						t.Fatal("repair backup lost")
					}
				} else if scenario != "unsafe target" && readFixture(t, filepath.Join(a.target, "identity")) != "old App" {
					t.Fatal("old App not restored")
				}
				if readFixture(t, service.a.executable) != "old executable" {
					t.Fatal("old service not restored")
				}
			}
			if readFixture(t, service.db) != "persistent database" || readFixture(t, service.key) != "persistent key" {
				t.Fatal("configuration touched")
			}
		})
	}
}

func TestManagerOffersCompleteAppRepairAtCurrentBackendVersion(t *testing.T) {
	f := isolatedManager(t)
	m := f.manager
	m.latest.Tag = "v" + buildinfo.Current().Version
	app := "0.0.1"
	m.ops.appVersion = func() string { return app }
	if s := m.Status(); !s.Available || !s.CompleteApplication || s.AppCurrent != "0.0.1" {
		t.Fatal(s)
	}
	m.ops.prepare = func(context.Context, release, func() error) error { return nil }
	if _, err := m.Start(m.latest.Tag); err != nil {
		t.Fatal("App-only repair rejected", err)
	}
}

func TestAppVersionPlistParsing(t *testing.T) {
	for _, tc := range []struct{ xml, want string }{
		{`<plist><dict><key>CFBundleIdentifier</key><string>com.routepilot.app</string><key>CFBundleShortVersionString</key><string>0.5.5</string></dict></plist>`, "0.5.5"},
		{`<plist><dict><key>CFBundleShortVersionString</key><string> 0.5.4 </string></dict></plist>`, "0.5.4"},
		{`<plist><dict><key>CFBundleShortVersionString</key><integer>5</integer></dict></plist>`, ""},
		{`<plist><dict><key>CFBundleShortVersionString</key>`, ""},
	} {
		if got := xmlAppVersion([]byte(tc.xml)); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}
func TestUpdateStatusReadsAppVersionAfterRollback(t *testing.T) {
	f := isolatedManager(t)
	m := f.manager
	m.latest.Tag = "v" + buildinfo.Current().Version
	app := buildinfo.Current().Version
	m.ops.appVersion = func() string { return app }
	if m.Status().Available {
		t.Fatal("matching App offered update")
	}
	app = "0.0.1"
	if !m.Status().Available {
		t.Fatal("App rollback hidden by stale version cache")
	}
}

func TestManualAppReopenClearsOnlyOldRestartFailures(t *testing.T) {
	at := time.Date(2026, 10, 9, 21, 10, 57, 0, time.Local)
	old := "Fri Oct 9 20:51:41 2026 /Applications/RoutePilot.app/Contents/MacOS/RoutePilot\n"
	fresh := "Fri Oct 9 21:11:56 2026 /Applications/RoutePilot.app/Contents/MacOS/RoutePilot\n"
	helper := "Fri Oct 9 21:10:20 2026 /Applications/RoutePilot.app/Contents/MacOS/RoutePilot --finish-remote-update\n"
	for _, tc := range []struct {
		output         string
		clear, running bool
	}{{old, false, false}, {fresh, true, true}, {fresh + old, false, false}, {helper, true, false}, {fresh + helper, true, true}, {"", true, false}, {"invalid invalid invalid invalid invalid /Applications/RoutePilot.app/Contents/MacOS/RoutePilot", false, false}} {
		clear, running := appsReopened(tc.output, at)
		if clear != tc.clear || running != tc.running {
			t.Fatal(tc, clear, running)
		}
	}
}
