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
	"time"

	"surge-controller/internal/buildinfo"
)

const appRestartFile = "/var/db/surge-controller/software-update-app.json"

type appRestartStatus struct {
	Target string `json:"target"`
	Phase  string `json:"phase"`
	Error  string `json:"error,omitempty"`
}

func readAppRestart() appRestartStatus {
	var s appRestartStatus
	if privateOwned(appRestartFile, false) == nil {
		b, _ := os.ReadFile(appRestartFile)
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// The newly installed backend handles the GUI handoff. This also works when an
// older update worker installed this release. Rollbacks never restart the App.
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
					m.restartRemoteApp(initial.Target, initial.UpdatedAt)
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
		path := fields[2]
		backup := strings.HasPrefix(path, "/Applications/.RoutePilot-update-") && strings.HasSuffix(path, "/previous.app/Contents/MacOS/RoutePilot")
		if path != appTarget+"/Contents/MacOS/RoutePilot" && !backup {
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

func (m *Manager) restartRemoteApp(target string, before time.Time) {
	report := func(phase, message string) { _ = writeJSON(appRestartFile, appRestartStatus{target, phase, message}) }
	report("checking", "")
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	output, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,uid=,comm=").Output()
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
	if applicationsOwned() != nil || privateOwned(appTarget, true) != nil || command(m.ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", appTarget) != nil {
		report("failed", "Cannot validate the updated App; reopen RoutePilot on the remote Mac")
		return
	}
	// Each helper lives in its original GUI session and drops root credentials.
	// It notifies first, asks if the management window is in use, and quits only
	// the exact installed RoutePilot instances after approval.
	var mu sync.Mutex
	sessions := make(map[int]appRestartStatus, len(uids))
	for _, uid := range uids {
		sessions[uid] = appRestartStatus{Phase: "checking"}
	}
	for _, uid := range uids {
		go func(uid int) {
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
			if err := m.restartAppSession(uid, before, update); err != nil {
				update("failed", err.Error())
			}
		}(uid)
	}
}

func (m *Manager) restartAppSession(uid int, before time.Time, report func(string, string)) error {
	user := strconv.Itoa(uid)
	if command(m.ctx, "/bin/launchctl", "print", "gui/"+user) != nil {
		return errors.New("Remote App user session is unavailable; reopen RoutePilot on the remote Mac")
	}
	// asuser adopts the bootstrap namespace, not Unix credentials; sudo drops
	// those before the AppKit helper is started. Neither operation prompts.
	cmd := exec.CommandContext(m.ctx, "/bin/launchctl", "asuser", user, "/usr/bin/sudo", "-n", "-H", "-u", "#"+user, appTarget+"/Contents/MacOS/RoutePilot", "--finish-remote-update", "--restart-before", strconv.FormatFloat(float64(before.UnixNano())/1e9, 'f', 9, 64))
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
