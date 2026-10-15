package softwareupdate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"surge-controller/internal/buildinfo"
)

const appRestartFile = "/var/db/surge-controller/software-update-app.json"

type appRestartStatus struct {
	Target string `json:"target"`
	Phase  string `json:"phase"`
	Error  string `json:"error,omitempty"`
}

type appRestartOperations struct {
	read  func() appRestartStatus
	write func(appRestartStatus) error
	run   func(context.Context, time.Time, bool, func(string, string))
}

func (m *Manager) appRestartForceBusy() bool {
	m.appRestartMu.Lock()
	defer m.appRestartMu.Unlock()
	return m.appRestartForced
}

// ForceRestartApp only completes the GUI handoff for the installed update. The
// caller supplies the confirmed version, never a process, user, path or command.
func (m *Manager) ForceRestartApp(version string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.snapshot()
	if !m.supported {
		return s, errors.New(m.reason)
	}
	if m.running || m.appRestartForceBusy() || s.Phase != "succeeded" || version != s.Target || version != "v"+s.Current || s.AppCurrent != s.Current || s.UpdatedAt.IsZero() || (s.AppRestart != "awaiting_confirmation" && s.AppRestart != "deferred" && s.AppRestart != "failed") {
		return s, errors.New("Refresh update status before forcing the remote App to restart")
	}
	if err := m.beginAppRestart(s.Target, s.UpdatedAt, true); err != nil {
		return s, err
	}
	return m.snapshot(), nil
}

// Supersede the waiting helper, then join it before starting a forced handoff.
// Generation checks prevent late output from an old dialog overwriting status.
func (m *Manager) beginAppRestart(target string, before time.Time, force bool) error {
	m.appRestartMu.Lock()
	defer m.appRestartMu.Unlock()
	phase := "checking"
	if force {
		phase = "restarting"
	}
	if err := m.appOps.write(appRestartStatus{Target: target, Phase: phase}); err != nil {
		return err
	}
	previous := m.appRestartDone
	if m.appRestartCancel != nil {
		m.appRestartCancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	if force {
		cancel()
		ctx, cancel = context.WithTimeout(m.ctx, 2*time.Minute)
	}
	done := make(chan struct{})
	m.appRestartCancel, m.appRestartDone = cancel, done
	m.appRestartID++
	id := m.appRestartID
	m.appRestartForced = force
	go func() {
		defer cancel()
		defer close(done)
		defer func() {
			m.appRestartMu.Lock()
			defer m.appRestartMu.Unlock()
			if m.appRestartID == id {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					_ = m.appOps.write(appRestartStatus{Target: target, Phase: "failed", Error: "Remote App restart timed out; reopen RoutePilot on the remote Mac"})
				}
				m.appRestartForced = false
			}
		}()
		if previous != nil {
			select {
			case <-previous:
			case <-ctx.Done():
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		m.appOps.run(ctx, before, force, func(phase, message string) {
			m.appRestartMu.Lock()
			defer m.appRestartMu.Unlock()
			if m.appRestartID == id && ctx.Err() == nil {
				_ = m.appOps.write(appRestartStatus{Target: target, Phase: phase, Error: message})
			}
		})
	}()
	return nil
}

func readAppRestart() appRestartStatus {
	var s appRestartStatus
	if privateOwned(appRestartFile, false) == nil {
		b, _ := os.ReadFile(appRestartFile)
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func installedAppProcess(path string) bool {
	return path == appTarget+"/Contents/MacOS/RoutePilot" ||
		strings.HasPrefix(path, "/Applications/.RoutePilot-update-") && strings.HasSuffix(path, "/previous.app/Contents/MacOS/RoutePilot")
}

// Inspect real process launch times instead of treating the installed bundle's
// version as proof that a running App has restarted. Helpers aren't normal Apps.
func appsReopened(output string, updatedAt time.Time) (bool, bool) {
	if updatedAt.IsZero() {
		return false, false
	}
	found := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || !installedAppProcess(fields[5]) {
			continue
		}
		helper := false
		for _, argument := range fields[6:] {
			if argument == "--finish-remote-update" {
				helper = true
			}
		}
		if helper {
			continue
		}
		launched, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.Join(fields[:5], " "), time.Local)
		if err != nil || launched.Before(updatedAt) {
			return false, false
		}
		found = true
	}
	return true, found
}

// Reconcile asynchronously so status requests remain quick. A manual quit or
// relaunch clears an obsolete error without installing or restarting anything.
func (m *Manager) reconcileAppRestart(update Status, restart appRestartStatus) {
	if update.Phase != "succeeded" || update.Target != "v"+buildinfo.Current().Version || update.AppCurrent != buildinfo.Current().Version || restart.Phase != "failed" && restart.Phase != "deferred" {
		return
	}
	m.appReconcileMu.Lock()
	if time.Since(m.appReconcileAt) < 3*time.Second {
		m.appReconcileMu.Unlock()
		return
	}
	m.appReconcileAt = time.Now()
	m.appReconcileMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/ps", "-axo", "lstart=,command=")
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		output, err := cmd.Output()
		if err != nil {
			return
		}
		clear, running := appsReopened(string(output), update.UpdatedAt)
		m.appRestartMu.Lock()
		defer m.appRestartMu.Unlock()
		if !clear || m.appOps.read() != restart || m.ops.read().Target != update.Target {
			return
		}
		phase := "not_running"
		if running {
			phase = "restarted"
		}
		_ = m.appOps.write(appRestartStatus{Target: restart.Target, Phase: phase})
	}()
}

// The newly installed backend handles the GUI handoff after the update worker
// confirms service readiness and commits the installation.
func (m *Manager) resumeRemoteApp(initial Status) {
	if initial.Phase != "restarting" || initial.Target != "v"+buildinfo.Current().Version || time.Since(initial.UpdatedAt) > 10*time.Minute {
		return
	}
	_ = writeJSON(appRestartFile, appRestartStatus{Target: initial.Target, Phase: "checking"})
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
		defer cancel()
		for {
			s := readState()
			if s.Target != initial.Target || s.Phase == "failed" {
				return
			}
			if s.Phase == "succeeded" {
				if appVersion(ctx) == buildinfo.Current().Version {
					if err := m.beginAppRestart(initial.Target, initial.UpdatedAt, false); err != nil {
						_ = m.appOps.write(appRestartStatus{Target: initial.Target, Phase: "failed", Error: err.Error()})
					}
				} else {
					_ = writeJSON(appRestartFile, appRestartStatus{Target: initial.Target, Phase: "failed", Error: "Cannot validate the updated App; reopen RoutePilot on the remote Mac"})
				}
				return
			}
			if pause(ctx, time.Second) != nil {
				_ = writeJSON(appRestartFile, appRestartStatus{Target: initial.Target, Phase: "failed", Error: "Remote App restart did not complete; reopen RoutePilot on the remote Mac"})
				return
			}
		}
	}()
}

func appSessionUIDs(output string) []int {
	seen := map[int]bool{}
	var uids []int
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		if !installedAppProcess(fields[2]) {
			continue
		}
		uid, err := strconv.Atoi(fields[1])
		if err == nil && uid > 0 && uid <= 2147483647 && !seen[uid] {
			seen[uid] = true
			uids = append(uids, uid)
		}
	}
	return uids
}

func (m *Manager) restartRemoteApp(ctx context.Context, before time.Time, force bool, report func(string, string)) {
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	output, err := exec.CommandContext(inspectCtx, "/bin/ps", "-axo", "pid=,uid=,comm=").Output()
	cancel()
	if err != nil {
		report("failed", "Cannot inspect the remote App session; reopen RoutePilot on the remote Mac")
		return
	}
	uids := appSessionUIDs(string(output))
	if len(uids) == 0 {
		report("not_running", "")
		return
	}
	// Fixed installed bundle only. No client-supplied path, UID or command.
	if applicationsOwned() != nil || privateOwned(appTarget, true) != nil || command(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", appTarget) != nil {
		report("failed", "Cannot validate the updated App; reopen RoutePilot on the remote Mac")
		return
	}
	// Each helper lives in its original GUI session and drops root credentials.
	// It notifies first, asks if the management window is in use, and quits only
	// the exact installed RoutePilot instances after approval.
	var mu sync.Mutex
	var wg sync.WaitGroup
	sessions := make(map[int]appRestartStatus, len(uids))
	for _, uid := range uids {
		sessions[uid] = appRestartStatus{Phase: "checking"}
	}
	for _, uid := range uids {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			update := func(phase, message string) {
				mu.Lock()
				defer mu.Unlock()
				sessions[uid] = appRestartStatus{Phase: phase, Error: message}
				// Keep another user's pending decision visible after this session
				// finishes; one successful relaunch cannot hide a deferred one.
				for _, priority := range []string{"failed", "awaiting_confirmation", "deferred", "checking", "restarting", "restarted", "not_running"} {
					for _, session := range sessions {
						if session.Phase == priority {
							report(priority, session.Error)
							return
						}
					}
				}
			}
			if err := m.restartAppSession(ctx, uid, before, force, update); err != nil {
				update("failed", err.Error())
			}
		}(uid)
	}
	wg.Wait()
}

func cancelAppRestartCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}

func (m *Manager) restartAppSession(ctx context.Context, uid int, before time.Time, force bool, report func(string, string)) error {
	user := strconv.Itoa(uid)
	if command(ctx, "/bin/launchctl", "print", "gui/"+user) != nil {
		return errors.New("Remote App user session is unavailable; reopen RoutePilot on the remote Mac")
	}
	// asuser adopts the bootstrap namespace, not Unix credentials; sudo drops
	// those before the AppKit helper is started. Neither operation prompts.
	args := []string{"asuser", user, "/usr/bin/sudo", "-n", "-H", "-u", "#" + user, appTarget + "/Contents/MacOS/RoutePilot", "--finish-remote-update", "--restart-before", strconv.FormatFloat(float64(before.UnixNano())/1e9, 'f', 9, 64)}
	if force {
		args = append(args, "--force-app-restart")
	}
	cmd := exec.CommandContext(ctx, "/bin/launchctl", args...)
	// Cancel the entire helper process group, including sudo's child, so an
	// unattended modal cannot later approve a second relaunch.
	cancelAppRestartCommand(cmd)
	// EOF also stops the unprivileged helper if sudo gives it a separate group,
	// or the backend exits while a remote confirmation dialog is open.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			_ = stdin.Close()
		case <-finished:
		}
	}()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	if err = cmd.Start(); err != nil {
		return errors.New("Cannot start the remote App restart helper; reopen RoutePilot on the remote Mac")
	}
	scanner := bufio.NewScanner(stdout)
	terminal := false
	for scanner.Scan() {
		var s appRestartStatus
		if json.Unmarshal(scanner.Bytes(), &s) != nil {
			continue
		}
		switch s.Phase {
		case "awaiting_confirmation", "restarting", "restarted", "deferred", "not_running", "failed":
			report(s.Phase, s.Error)
			terminal = s.Phase == "restarted" || s.Phase == "deferred" || s.Phase == "not_running" || s.Phase == "failed"
		}
	}
	err = cmd.Wait()
	if err != nil || scanner.Err() != nil || !terminal {
		return errors.New("Remote App restart did not complete; reopen RoutePilot on the remote Mac")
	}
	return nil
}
