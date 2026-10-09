package softwareupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"surge-controller/internal/buildinfo"
)

type workerJob struct {
	Target      string `json:"target"`
	SHA256      string `json:"sha256"`
	PreviousPID int    `json:"previous_pid"`
}
type ready struct {
	Version string    `json:"version"`
	PID     int       `json:"pid"`
	At      time.Time `json:"at"`
}

// MarkReady is written only by the installed service after its management
// listener is created. The helper uses it to identify the new process.
func (m *Manager) MarkReady() {
	if m.standardService && servicePID() == os.Getpid() && privateOwned("/private/var/db/surge-controller", true) == nil {
		_ = writeJSON(readyFile, ready{buildinfo.Current().Version, os.Getpid(), time.Now().UTC()})
	}
}

// RunWorker is a root-only, fixed-path, one-shot launchd job. It is deliberately
// independent of the service process it stops. No CLI arguments are accepted.
func RunWorker() (result error) {
	if os.Geteuid() != 0 {
		return errors.New("Backend update helper requires root")
	}
	for _, dir := range []string{serviceDir, stagingDir, "/private/var/db/surge-controller", "/Library/LaunchDaemons"} {
		if err := privateOwned(dir, true); err != nil {
			return err
		}
	}
	if workerPID() != os.Getpid() {
		return errors.New("Backend update helper must be launched by its dedicated system job")
	}
	s := Status{Phase: "restarting", Target: readState().Target, UpdatedAt: time.Now().UTC()}
	defer func() {
		if result != nil {
			s.Phase = "failed"
			s.Error = result.Error()
		} else {
			s.Phase = "succeeded"
			s.Error = ""
		}
		s.UpdatedAt = time.Now().UTC()
		_ = writeJSON(stateFile, s)
		_ = os.Remove(workerPlist)
		_ = os.Remove("/var/db/surge-controller/software-update-sessions.json")
	}()
	unlock, err := lockInstall()
	if err != nil {
		return err
	}
	defer unlock()

	for _, file := range []string{filepath.Join(stagingDir, "job.json"), filepath.Join(stagingDir, "next"), serviceBin, servicePlist} {
		if err := privateOwned(file, false); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(filepath.Join(stagingDir, "job.json"))
	if err != nil {
		return err
	}
	var job workerJob
	if json.Unmarshal(b, &job) != nil || job.PreviousPID <= 1 || !newer(job.Target, buildinfo.Current().Version) {
		return errors.New("Invalid backend update job")
	}
	s.Target = job.Target
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	next := filepath.Join(stagingDir, "next")
	b, err = os.ReadFile(next)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != job.SHA256 {
		return errors.New("Staged backend checksum changed")
	}
	if err = validateBinary(ctx, next, job.Target); err != nil {
		return err
	}
	// Give the accepted API response time to reach the client before stopping DNS.
	time.Sleep(2 * time.Second)
	if servicePID() != job.PreviousPID {
		return errors.New("Backend process changed before the update; check and retry")
	}
	if err = command(ctx, "/bin/launchctl", "bootout", "system", servicePlist); err != nil {
		return errors.New("Could not stop the existing backend service")
	}
	stopped, swapped := true, false
	previous := filepath.Join(stagingDir, "previous")
	defer func() {
		if result == nil || !stopped {
			return
		}
		if swapped {
			failedPID := servicePID()
			_ = command(context.Background(), "/bin/launchctl", "bootout", "system", servicePlist)
			if err := waitProcess(failedPID, 35*time.Second); err != nil {
				result = errors.New("New backend did not stop during rollback; repair the service on the remote host")
				return
			}
			if err := os.Rename(previous, serviceBin); err != nil {
				result = errors.New("Update failed and the old executable could not be restored; repair the service on the remote host")
				return
			}
		}
		if err := command(context.Background(), "/bin/launchctl", "bootstrap", "system", servicePlist); err != nil {
			result = errors.New("Update failed and the previous service could not be restarted; repair the service on the remote host")
		}
	}()
	if err := waitProcess(job.PreviousPID, 35*time.Second); err != nil {
		return errors.New("Previous backend did not stop; replacement cancelled")
	}
	_ = os.Remove(previous)
	if err = copyExecutable(serviceBin, previous); err != nil {
		return err
	}
	if err = os.Chmod(next, 0755); err != nil {
		return err
	}
	if err = os.Rename(next, serviceBin); err != nil {
		return err
	}
	swapped = true
	if dir, err := os.Open(serviceDir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	startedAt := time.Now()
	if err = command(ctx, "/bin/launchctl", "bootstrap", "system", servicePlist); err != nil {
		return errors.New("New backend could not start; restoring the previous executable")
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(readyFile)
		var r ready
		if json.Unmarshal(b, &r) == nil && r.Version == job.Target[1:] && r.PID > 1 && r.PID == servicePID() && r.At.After(startedAt) {
			time.Sleep(3 * time.Second)
			if servicePID() == r.PID {
				stopped = false
				_ = os.Remove(previous)
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("New backend did not become ready; restoring the previous executable")
}

func waitProcess(pid int, timeout time.Duration) error {
	if pid <= 1 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			return errors.New("process has not stopped")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}
