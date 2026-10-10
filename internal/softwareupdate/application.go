package softwareupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Both installed components must reach the selected release. Repair an
// incomplete installation using the same complete application package.
func applicationUpdateAvailable(target, backend, app string) bool {
	if _, err := versionParts(target); err != nil {
		return false
	}
	if _, err := versionParts(backend); err != nil {
		return false
	}
	if newer(backend, target) || newer(app, target) {
		return false
	}
	return newer(target, backend) || app == "" || newer(target, app)
}

type applicationActivation struct {
	target, candidate, previous string
	activateService             func() error
	retained                    func()
}

// Swap the bundle first, so the new backend sees matching App metadata at
// startup. A failed service activation restores the old bundle as well.
func activateApplication(a applicationActivation) (result error) {
	info, err := os.Lstat(a.target)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if exists && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("Installed App path is unsafe")
	}
	if exists {
		if err := os.Rename(a.target, a.previous); err != nil {
			return err
		}
	}
	swapped := false
	defer func() {
		if result == nil {
			return
		}
		if swapped {
			if err := os.Rename(a.target, a.candidate); err != nil {
				a.retained()
				result = errors.New("App update failed; repair the application using the retained backup in " + a.previous)
				return
			}
		}
		if exists {
			if err := os.Rename(a.previous, a.target); err != nil {
				a.retained()
				result = errors.New("App update failed; previous App retained in " + a.previous)
			}
		}
	}()
	if err := os.Rename(a.candidate, a.target); err != nil {
		return err
	}
	swapped = true
	return a.activateService()
}

type applicationInstaller struct {
	uid          int
	remoteWorker bool
	expectedPID  int
	stageParent  string
	archive      []byte
	phaseChanged func(string)
}

func (a *applicationInstaller) phase(value string) { a.phaseChanged(value) }

func (a *applicationInstaller) install(ctx context.Context, r release) (result error) {
	unlock, err := lockInstall()
	if err != nil {
		return err
	}
	defer unlock()

	name := "RoutePilot-macos-arm64.zip"
	urls, err := releaseAssets(r, name)
	if err != nil {
		return err
	}
	b, err := download(ctx, urls["SHA256SUMS"], 1<<20)
	if err != nil {
		return err
	}
	expected, err := packageChecksum(b, name)
	if err != nil {
		return err
	}
	b = a.archive
	if b == nil && a.uid > 0 {
		b = cachedAppArchive(a.uid, expected)
	}
	if b == nil {
		b, err = download(ctx, urls[name], 256<<20)
		if err != nil {
			return err
		}
	}
	if verifyChecksum(b, expected) != nil {
		return errors.New("App package SHA-256 verification failed")
	}
	a.phase("validating")
	parent := a.stageParent
	if parent == "" {
		parent = agentDir
	}
	stage, err := os.MkdirTemp(parent, "app-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	archive := stage + "/app.zip"
	if err = os.WriteFile(archive, b, 0600); err != nil {
		return err
	}
	if err = command(ctx, "/usr/bin/ditto", "-x", "-k", archive, stage); err != nil {
		return errors.New("Cannot unpack the App update")
	}
	source := stage + "/RoutePilot.app"
	info, err := os.Lstat(source)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Invalid App update bundle")
	}
	if err = command(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", source); err != nil {
		return errors.New("App update code signature is invalid")
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err = exec.CommandContext(c, "/usr/bin/plutil", "-convert", "json", "-o", "-", source+"/Contents/Info.plist").Output()
	if err != nil {
		return err
	}
	var metadata map[string]any
	if json.Unmarshal(b, &metadata) != nil || metadata["CFBundleIdentifier"] != "com.routepilot.app" || metadata["CFBundleShortVersionString"] != strings.TrimPrefix(r.Tag, "v") {
		return errors.New("App update identity does not match the release")
	}
	// ZIP metadata may retain the build user's ownership. The assistant owns
	// its private staging bundle before any executable validation or launch.
	if err = command(ctx, "/usr/sbin/chown", "-R", "root:wheel", source); err != nil {
		return err
	}
	nextBackend := source + "/Contents/Resources/Service/bin/surge-controller"
	if err = validateBinary(ctx, nextBackend, r.Tag); err != nil {
		return err
	}
	if err = applicationsOwned(); err != nil {
		return err
	}
	info, err = os.Lstat(appTarget)
	if err != nil && !(a.remoteWorker && errors.Is(err, os.ErrNotExist)) || err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("Installed App path is unsafe")
	}
	if newer(appVersion(ctx), r.Tag) {
		return errors.New("The installed App is newer than this release")
	}
	swap, err := os.MkdirTemp("/Applications", ".RoutePilot-update-")
	if err != nil {
		return err
	}
	keepSwap := false
	defer func() {
		if !keepSwap {
			_ = os.RemoveAll(swap)
		}
	}()
	candidate := swap + "/RoutePilot.app"
	if err = command(ctx, "/usr/bin/ditto", source, candidate); err != nil {
		return err
	}
	if err = command(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", candidate); err != nil {
		return errors.New("Copied App code signature is invalid")
	}
	activateService := func() error {
		if _, err := os.Lstat(servicePlist); errors.Is(err, os.ErrNotExist) && !a.remoteWorker {
			return nil
		} else if err != nil {
			return err
		}
		for _, file := range []string{servicePlist, serviceBin} {
			if err := privateOwned(file, false); err != nil {
				return err
			}
		}
		for _, directory := range []string{serviceDir, "/private/var/db/surge-controller"} {
			if err := privateOwned(directory, true); err != nil {
				return err
			}
		}
		pid := servicePID()
		if pid <= 1 {
			return errors.New("Start the installed network service before updating the App")
		}
		if a.remoteWorker && pid != a.expectedPID {
			return errors.New("Backend process changed before the update; check and retry")
		}
		if worker := workerPID(); worker > 0 && !(a.remoteWorker && worker == os.Getpid()) {
			return errors.New("A backend update is already in progress")
		}
		current := installedBackendVersion(ctx)
		if current == "" {
			return errors.New("Cannot identify the installed backend version")
		}
		if newer("v"+current, r.Tag) {
			return errors.New("The installed backend is newer than this App release")
		}
		backendStage, err := os.MkdirTemp(serviceDir, ".app-update-")
		if err != nil {
			return err
		}
		previous := backendStage + "/previous"
		defer func() {
			// Keep evidence if recovery itself failed.
			if _, err := os.Stat(previous); errors.Is(err, os.ErrNotExist) {
				_ = os.RemoveAll(backendStage)
			}
		}()
		next := backendStage + "/next"
		if err := copyExecutable(appTarget+"/Contents/Resources/Service/bin/surge-controller", next); err != nil {
			return err
		}
		a.phase("restarting")
		return activateBackend(ctx, r.Tag, pid, backendActivation{
			executable: serviceBin, next: next, previous: previous, directory: serviceDir,
			pid: servicePID, wait: waitProcess, ready: waitReady,
			stop: func(ctx context.Context) error {
				return command(ctx, "/bin/launchctl", "bootout", "system", servicePlist)
			},
			start: func(ctx context.Context) error {
				return command(ctx, "/bin/launchctl", "bootstrap", "system", servicePlist)
			},
		})
	}
	if err = activateApplication(applicationActivation{target: appTarget, candidate: candidate, previous: swap + "/previous.app", activateService: activateService, retained: func() { keepSwap = true }}); err != nil {
		return err
	}
	// Update the root-owned helper executable for its next launch. The currently
	// running daemon remains alive long enough for the App to receive success.
	if privateOwned(agentDir, true) == nil {
		if err = copyExecutable(appTarget+"/Contents/Resources/Service/bin/surge-controller", agentDir+"/agent.next"); err == nil {
			_ = os.Rename(agentDir+"/agent.next", agentDir+"/agent")
		}
	}
	return nil
}

// The one-shot remote worker downloads the same complete App package as the
// local assistant. It never creates a user session or launches a GUI as root.
func installRemoteApplication(ctx context.Context, r release, previousPID int) error {
	if err := applicationsOwned(); err != nil {
		return err
	}
	if servicePID() != previousPID {
		return errors.New("Backend process changed before the update; check and retry")
	}
	archiveFile := stagingDir + "/application.zip"
	if err := privateOwned(archiveFile, false); err != nil {
		return err
	}
	info, err := os.Stat(archiveFile)
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > 256<<20 {
		return errors.New("Release asset exceeds the size limit")
	}
	archive, err := os.ReadFile(archiveFile)
	if err != nil {
		return err
	}
	a := &applicationInstaller{remoteWorker: true, expectedPID: previousPID, stageParent: stagingDir, archive: archive, phaseChanged: func(phase string) {
		_ = writeJSON(stateFile, Status{Phase: phase, Target: r.Tag, UpdatedAt: time.Now().UTC()})
	}}
	return a.install(ctx, r)
}
