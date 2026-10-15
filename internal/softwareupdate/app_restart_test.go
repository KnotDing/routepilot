package softwareupdate

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"

	"surge-controller/internal/buildinfo"
)

func restartFixture(t *testing.T) (*Manager, *appRestartStatus) {
	t.Helper()
	f := isolatedManager(t)
	m := f.manager
	f.state = Status{Phase: "succeeded", Target: "v" + buildinfo.Current().Version, UpdatedAt: time.Now().Add(-time.Hour)}
	restart := &appRestartStatus{Target: f.state.Target, Phase: "awaiting_confirmation"}
	var mu sync.Mutex
	m.appOps.read = func() appRestartStatus { mu.Lock(); defer mu.Unlock(); return *restart }
	m.appOps.write = func(s appRestartStatus) error { mu.Lock(); defer mu.Unlock(); *restart = s; return nil }
	m.appOps.run = func(context.Context, time.Time, bool, func(string, string)) { t.Error("unexpected real restart") }
	// Reconciliation is tested separately; no OS inspection is needed here.
	m.appReconcileAt = time.Now()
	return m, restart
}

func TestForceAppRestartJoinsWaitingHelperAndRejectsDuplicate(t *testing.T) {
	m, _ := restartFixture(t)
	waiting, stopped, forced, finish := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	m.appOps.run = func(ctx context.Context, before time.Time, force bool, report func(string, string)) {
		if before.IsZero() {
			t.Error("missing launch cutoff")
		}
		if !force {
			report("awaiting_confirmation", "")
			close(waiting)
			<-ctx.Done()
			report("deferred", "late output")
			close(stopped)
			return
		}
		select {
		case <-stopped:
		default:
			t.Error("started before old helper exited")
		}
		close(forced)
		<-finish
		report("restarted", "")
	}
	target := m.ops.read().Target
	if err := m.beginAppRestart(target, m.ops.read().UpdatedAt, false); err != nil {
		t.Fatal(err)
	}
	<-waiting
	s, err := m.ForceRestartApp(target)
	if err != nil || s.AppRestart != "restarting" {
		t.Fatal(s, err)
	}
	<-forced
	if _, err = m.ForceRestartApp(target); err == nil {
		t.Fatal("duplicate force accepted")
	}
	if _, err = m.Start(m.latest.Tag); err == nil {
		t.Fatal("installation raced with App handoff")
	}
	if got := m.Status(); got.AppRestart != "restarting" || got.AppRestartError != "" {
		t.Fatal("old helper overwrote status", got)
	}
	close(finish)
	m.appRestartMu.Lock()
	done := m.appRestartDone
	m.appRestartMu.Unlock()
	<-done
	if got := m.Status(); got.AppRestart != "restarted" || m.appRestartForceBusy() {
		t.Fatal(got)
	}
}

func TestForceAppRestartValidatesInstalledUpdate(t *testing.T) {
	for _, change := range []string{"unsupported", "wrong version", "old backend", "old App", "no completion", "missing time", "not pending", "already running"} {
		t.Run(change, func(t *testing.T) {
			m, restart := restartFixture(t)
			version := m.ops.read().Target
			switch change {
			case "unsupported":
				m.supported = false
				m.reason = "unsupported"
			case "wrong version":
				version = "v0.0.1"
			case "old backend":
				s := m.ops.read()
				s.Target = "v0.0.1"
				_ = m.ops.write(s)
				restart.Target = s.Target
			case "old App":
				m.ops.appVersion = func() string { return "0.0.1" }
			case "no completion":
				s := m.ops.read()
				s.Phase = "restarting"
				_ = m.ops.write(s)
			case "missing time":
				s := m.ops.read()
				s.UpdatedAt = time.Time{}
				_ = m.ops.write(s)
			case "not pending":
				restart.Phase = "restarted"
			case "already running":
				m.running = true
			}
			if _, err := m.ForceRestartApp(version); err == nil {
				t.Fatal("invalid restart accepted")
			}
		})
	}
}

func TestForceAppRestartDoesNotCancelWaitingHelperOnStorageFailure(t *testing.T) {
	m, _ := restartFixture(t)
	canceled := false
	m.appRestartCancel = func() { canceled = true }
	m.appOps.write = func(appRestartStatus) error { return errors.New("disk full") }
	if _, err := m.ForceRestartApp(m.ops.read().Target); err == nil || err.Error() != "disk full" || canceled {
		t.Fatal(err, canceled)
	}
}

func TestCancelAppRestartCommandStopsDescendantHoldingOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "cat & echo ready >&2; wait")
	cancelAppRestartCommand(cmd)
	cmd.Stdout = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := bufio.NewScanner(stderr)
	if !ready.Scan() || ready.Text() != "ready" {
		t.Fatal("helper did not create its child")
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled helper succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper descendants survived cancellation")
	}
}
