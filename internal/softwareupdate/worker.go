package softwareupdate

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"surge-controller/internal/buildinfo"
)

type workerJob struct {
	Target      string `json:"target"`
	PreviousPID int    `json:"previous_pid"`
	Source      string `json:"source,omitempty"`
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
		update := readState()
		_ = writeJSON(readyFile, ready{buildinfo.Current().Version, os.Getpid(), time.Now().UTC()})
		m.appRestartOnce.Do(func() { m.resumeRemoteApp(update) })
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
	s := Status{Phase: "downloading", Target: readState().Target, UpdatedAt: time.Now().UTC()}
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
		_ = os.Remove(filepath.Join(stagingDir, "application.zip"))
		_ = os.Remove(workerPlist)
		_ = os.Remove("/var/db/surge-controller/software-update-sessions.json")
	}()

	for _, file := range []string{filepath.Join(stagingDir, "job.json"), serviceBin, servicePlist} {
		if err := privateOwned(file, false); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(filepath.Join(stagingDir, "job.json"))
	if err != nil {
		return err
	}
	var job workerJob
	if json.Unmarshal(b, &job) != nil || job.PreviousPID <= 1 {
		return errors.New("Invalid application update job")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	if !applicationUpdateAvailable(job.Target, buildinfo.Current().Version, appVersion(ctx)) {
		return errors.New("Invalid application update job")
	}
	s.Target = job.Target
	// The job never supplies asset URLs. Refetch the fixed official tag metadata.
	job.Source, _ = updateSource(job.Source)
	b, err = download(ctx, releaseEndpoint(job.Source, "/releases/tags/"+url.PathEscape(job.Target)), 2<<20)
	if err != nil {
		return err
	}
	var r release
	if json.Unmarshal(b, &r) != nil || !installableRelease(r, job.Target) {
		return errors.New("Release metadata does not match the application update")
	}
	if err = pause(ctx, 2*time.Second); err != nil {
		return err
	}
	return installRemoteApplication(ctx, r, job.PreviousPID)
}

// The fixed-path worker validates ownership, signatures and the job before this
// activation step. Tests supply only temporary files and simulated launchd/PIDs.
type backendActivation struct {
	executable, next, previous, directory string
	pid                                   func() int
	wait                                  func(int, time.Duration) error
	ready                                 func(context.Context, string, time.Time) error
	stop, start                           func(context.Context) error
}

func activateBackend(ctx context.Context, target string, previousPID int, a backendActivation) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.pid() != previousPID {
		return errors.New("Backend process changed before the update; check and retry")
	}
	if err := a.stop(ctx); err != nil {
		return errors.New("Could not stop the existing backend service")
	}
	swapped := false
	defer func() {
		if result == nil {
			return
		}
		if swapped {
			failedPID := a.pid()
			_ = a.stop(context.Background())
			if err := a.wait(failedPID, 35*time.Second); err != nil {
				result = errors.New("New backend did not stop during rollback; repair the service on the remote host")
				return
			}
			if err := os.Rename(a.previous, a.executable); err != nil {
				result = errors.New("Update failed and the old executable could not be restored; repair the service on the remote host")
				return
			}
		}
		if err := a.start(context.Background()); err != nil {
			result = errors.New("Update failed and the previous service could not be restarted; repair the service on the remote host")
		}
	}()
	if err := a.wait(previousPID, 35*time.Second); err != nil {
		return errors.New("Previous backend did not stop; replacement cancelled")
	}
	if err := copyExecutable(a.executable, a.previous); err != nil {
		return err
	}
	if err := os.Chmod(a.next, 0755); err != nil {
		return err
	}
	if err := os.Rename(a.next, a.executable); err != nil {
		return err
	}
	swapped = true
	if dir, err := os.Open(a.directory); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	startedAt := time.Now()
	if err := a.start(ctx); err != nil {
		return errors.New("New backend could not start; restoring the previous executable")
	}
	if err := a.ready(ctx, target, startedAt); err != nil {
		return err
	}
	_ = os.Remove(a.previous)
	return nil
}

func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
