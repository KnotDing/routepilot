package softwareupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type activationFixture struct {
	a             backendActivation
	stops, starts int
	pid           int
	db, key       string
}

func newActivationFixture(t *testing.T) *activationFixture {
	t.Helper()
	dir := t.TempDir()
	f := &activationFixture{pid: 101, db: filepath.Join(dir, "database"), key: filepath.Join(dir, "key")}
	f.a = backendActivation{executable: filepath.Join(dir, "installed"), next: filepath.Join(dir, "next"), previous: filepath.Join(dir, "previous"), directory: dir}
	for file, body := range map[string]string{f.a.executable: "old executable", f.a.next: "new executable", f.db: "persistent database", f.key: "persistent key"} {
		if err := os.WriteFile(file, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.a.pid = func() int { return f.pid }
	f.a.stop = func(context.Context) error { f.stops++; f.pid = 0; return nil }
	f.a.start = func(context.Context) error { f.starts++; f.pid = 202; return nil }
	f.a.wait = func(int, time.Duration) error { return nil }
	f.a.ready = func(ctx context.Context, target string, after time.Time) error {
		if target != "v1.0.0" || after.IsZero() {
			t.Error("incorrect ready identity")
		}
		return nil
	}
	return f
}
func readFixture(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestBackendActivationSuccessPreservesDataAndCleansBackup(t *testing.T) {
	f := newActivationFixture(t)
	if err := activateBackend(context.Background(), "v1.0.0", 101, f.a); err != nil {
		t.Fatal(err)
	}
	if f.stops != 1 || f.starts != 1 || readFixture(t, f.a.executable) != "new executable" {
		t.Fatal(f.stops, f.starts)
	}
	if _, err := os.Stat(f.a.previous); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rollback backup leaked", err)
	}
	if readFixture(t, f.db) != "persistent database" || readFixture(t, f.key) != "persistent key" {
		t.Fatal("data touched")
	}
	info, _ := os.Stat(f.a.executable)
	if info.Mode().Perm() != 0755 {
		t.Fatal(info.Mode())
	}
}
func TestBackendActivationFailuresRestoreOldExecutable(t *testing.T) {
	for _, failure := range []string{"stop failed", "PID changed", "old process alive", "next missing", "new bootstrap failed", "new readiness failed", "cancelled readiness"} {
		t.Run(failure, func(t *testing.T) {
			f := newActivationFixture(t)
			ctx := context.Background()
			switch failure {
			case "stop failed":
				f.a.stop = func(context.Context) error { return errors.New("launchd refused") }
			case "PID changed":
				f.pid = 999
			case "old process alive":
				f.a.wait = func(pid int, _ time.Duration) error {
					if pid == 101 {
						return errors.New("still alive")
					}
					return nil
				}
			case "next missing":
				_ = os.Remove(f.a.next)
			case "new bootstrap failed":
				f.a.start = func(context.Context) error {
					f.starts++
					if f.starts == 1 {
						return errors.New("bad executable")
					}
					return nil
				}
			case "new readiness failed":
				f.a.ready = func(context.Context, string, time.Time) error { return errors.New("not ready") }
			case "cancelled readiness":
				f.a.ready = func(context.Context, string, time.Time) error { return context.Canceled }
			}
			if err := activateBackend(ctx, "v1.0.0", 101, f.a); err == nil {
				t.Fatal("failure accepted")
			}
			if readFixture(t, f.a.executable) != "old executable" || readFixture(t, f.db) != "persistent database" || readFixture(t, f.key) != "persistent key" {
				t.Fatal("failed update lost original")
			}
			if (failure == "stop failed" || failure == "PID changed") && f.starts != 0 {
				t.Fatal("started a service after rejected stop")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := newActivationFixture(t)
	if err := activateBackend(ctx, "v1.0.0", 101, f.a); !errors.Is(err, context.Canceled) || f.stops != 0 {
		t.Fatal("cancelled before swap", err)
	}
}
func TestRollbackFailuresRetainRepairEvidence(t *testing.T) {
	for _, failure := range []string{"new process alive", "backup missing", "old bootstrap failed"} {
		t.Run(failure, func(t *testing.T) {
			f := newActivationFixture(t)
			f.a.ready = func(context.Context, string, time.Time) error {
				if failure == "backup missing" {
					_ = os.Remove(f.a.previous)
				}
				return errors.New("not ready")
			}
			if failure == "new process alive" {
				f.a.wait = func(pid int, _ time.Duration) error {
					if pid == 202 {
						return errors.New("alive")
					}
					return nil
				}
			}
			if failure == "old bootstrap failed" {
				f.a.start = func(context.Context) error {
					f.starts++
					f.pid = 202
					if f.starts == 2 {
						return errors.New("launchd failed")
					}
					return nil
				}
			}
			err := activateBackend(context.Background(), "v1.0.0", 101, f.a)
			if err == nil || !strings.Contains(err.Error(), "repair the service") {
				t.Fatal(err)
			}
			if failure == "new process alive" && readFixture(t, f.a.previous) != "old executable" {
				t.Fatal("rollback evidence deleted")
			}
			if failure == "old bootstrap failed" && readFixture(t, f.a.executable) != "old executable" {
				t.Fatal("old executable not restored")
			}
		})
	}
}
func TestReadinessRequiresVersionPIDFreshnessAndStability(t *testing.T) {
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, failure := range []string{"wrong version", "wrong pid", "invalid pid", "stale marker", "invalid json", "missing marker", "unstable pid", "success"} {
		t.Run(failure, func(t *testing.T) {
			now := after
			marker := ready{Version: "1.0.0", PID: 202, At: after.Add(time.Second)}
			pid := 202
			switch failure {
			case "wrong version":
				marker.Version = "0.5.2"
			case "wrong pid":
				marker.PID = 303
			case "invalid pid":
				marker.PID = 1
			case "stale marker":
				marker.At = after
			}
			data, _ := json.Marshal(marker)
			if failure == "invalid json" {
				data = []byte("{")
			}
			p := readinessProbe{now: func() time.Time { return now }, read: func() ([]byte, error) {
				if failure == "missing marker" {
					return nil, os.ErrNotExist
				}
				return data, nil
			}, pid: func() int { return pid }, pause: func(ctx context.Context, d time.Duration) error {
				now = now.Add(d)
				if failure == "unstable pid" && d == 3*time.Second {
					pid = 0
				}
				return ctx.Err()
			}}
			err := waitForReady(context.Background(), "v1.0.0", after, p)
			if (err == nil) != (failure == "success") {
				t.Fatal(failure, err)
			}
			if failure == "success" && now.Sub(after) != 3*time.Second {
				t.Fatal("stability interval not enforced")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := readinessProbe{now: time.Now, read: func() ([]byte, error) { t.Fatal("cancelled probe read marker"); return nil, nil }}
	if err := waitForReady(ctx, "v1.0.0", after, p); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := pause(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestUpdateCommandAndProcessWaitCancellation(t *testing.T) {
	for _, pid := range []int{-1, 0, 1} {
		if err := waitProcess(pid, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := waitProcess(os.Getpid(), 0); err == nil {
		t.Fatal("live process marked stopped")
	}
	if err := command(context.Background(), "/bin/sh", "-c", "exit 0"); err != nil {
		t.Fatal(err)
	}
	if err := command(context.Background(), "/bin/sh", "-c", "exit 7"); err == nil {
		t.Fatal("command failure ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := command(ctx, "/bin/sh", "-c", "exit 0"); err == nil {
		t.Fatal("cancel ignored")
	}
}
