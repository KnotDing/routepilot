package softwareupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"surge-controller/internal/buildinfo"
)

// ReplaceService backs the App's administrator-authorized repair. The command
// must execute the backend bundled in a RoutePilot.app; that bundle is the only
// accepted source for overwriting the installed network service.
func ReplaceService() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, e := filepath.EvalSymlinks(executable); e == nil {
		executable = resolved
	}
	app, err := bundledApp(executable)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return replaceService(ctx, app)
}

// RepairService is the update assistant's fixed-source repair used when the
// installed backend version no longer matches the installed App.
func RepairService(ctx context.Context) error {
	if err := applicationsOwned(); err != nil {
		return err
	}
	return replaceService(ctx, appTarget)
}

// replaceService swaps the installed service executable for the backend bundled
// in the given App and restarts the standard launchd service with rollback.
func replaceService(ctx context.Context, appBundle string) (result error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 {
		return errors.New("Service replacement requires the macOS system service")
	}
	unlock, err := lockInstall()
	if err != nil {
		return err
	}
	defer unlock()
	if workerPID() > 0 {
		return errors.New("A backend update is already in progress")
	}
	if s := readState(); active(s.Phase) && time.Since(s.UpdatedAt) < 10*time.Minute {
		return errors.New("A backend update is already in progress")
	}
	if err := privateOwned(servicePlist, false); err != nil {
		return err
	}
	if err := privateOwned(serviceBin, false); err != nil {
		return err
	}
	for _, directory := range []string{serviceDir, "/private/var/db/surge-controller", "/Library/LaunchDaemons"} {
		if err := privateOwned(directory, true); err != nil {
			return err
		}
	}
	pid := servicePID()
	if pid <= 1 {
		return errors.New("The installed network service is not running")
	}
	info, err := os.Lstat(appBundle)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Installed App path is unsafe")
	}
	if err := command(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", appBundle); err != nil {
		return errors.New("App code signature is invalid")
	}
	version, build, err := appIdentity(ctx, appBundle)
	if err != nil {
		return err
	}
	next := appBundle + "/Contents/Resources/Service/bin/surge-controller"
	if file, e := os.Lstat(next); e != nil || !file.Mode().IsRegular() || file.Mode()&os.ModeSymlink != 0 {
		return errors.New("The App bundle has no usable bundled backend")
	}
	bundled, err := bundledBackendInfo(ctx, next)
	if err != nil {
		return err
	}
	if err := bundledBackendMatches(bundled, version, build); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(serviceDir, ".app-update-")
	if err != nil {
		return err
	}
	previous := stage + "/previous"
	defer func() {
		// Keep evidence if recovery itself failed.
		if _, err := os.Stat(previous); errors.Is(err, os.ErrNotExist) {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := copyExecutable(next, stage+"/next"); err != nil {
		return err
	}
	if err := activateBackend(ctx, "v"+version, pid, backendActivation{
		executable: serviceBin, next: stage + "/next", previous: previous, directory: serviceDir,
		pid: servicePID, wait: waitProcess, ready: waitReady,
		stop: func(ctx context.Context) error {
			return command(ctx, "/bin/launchctl", "bootout", "system", servicePlist)
		},
		start: func(ctx context.Context) error {
			return command(ctx, "/bin/launchctl", "bootstrap", "system", servicePlist)
		},
	}); err != nil {
		return err
	}
	// Keep the update assistant's copy current for its next launch.
	if privateOwned(agentDir, true) == nil {
		if err := copyExecutable(next, agentDir+"/agent.next"); err == nil {
			_ = os.Rename(agentDir+"/agent.next", agentDir+"/agent")
		}
	}
	return nil
}

// The replace-service command must run the backend of a RoutePilot.app bundle,
// never an arbitrary executable that merely claims to be one.
func bundledApp(executable string) (string, error) {
	const failure = "Service replacement must run the backend bundled in a RoutePilot.app"
	clean := filepath.Clean(executable)
	const suffix = "/Contents/Resources/Service/bin/surge-controller"
	if !strings.HasSuffix(clean, suffix) {
		return "", errors.New(failure)
	}
	app := strings.TrimSuffix(clean, suffix)
	if filepath.Base(app) != "RoutePilot.app" {
		return "", errors.New(failure)
	}
	return app, nil
}

// The replacement must be the exact build declared by its App metadata.
func bundledBackendMatches(info buildinfo.Info, appVersion, appBuild string) error {
	if info.Name != buildinfo.Current().Name || info.OS != runtime.GOOS || info.Arch != runtime.GOARCH {
		return errors.New("Bundled backend platform does not match this system")
	}
	if info.Version != appVersion || info.Build != appBuild {
		return errors.New("Bundled backend version does not match its App metadata")
	}
	return nil
}

func appIdentity(ctx context.Context, appBundle string) (string, string, error) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(c, "/usr/bin/plutil", "-convert", "json", "-o", "-", appBundle+"/Contents/Info.plist").Output()
	if err != nil {
		return "", "", errors.New("Cannot read the bundled App metadata")
	}
	var metadata map[string]any
	if json.Unmarshal(b, &metadata) != nil || metadata["CFBundleIdentifier"] != "com.routepilot.app" {
		return "", "", errors.New("Bundled App identity is invalid")
	}
	version, _ := metadata["CFBundleShortVersionString"].(string)
	build, _ := metadata["CFBundleVersion"].(string)
	if version == "" || build == "" {
		return "", "", errors.New("Bundled App metadata is incomplete")
	}
	return version, build, nil
}

func bundledBackendInfo(ctx context.Context, file string) (buildinfo.Info, error) {
	var info buildinfo.Info
	if err := command(ctx, "/usr/bin/codesign", "--verify", "--strict", file); err != nil {
		return info, errors.New("Backend executable code signature is invalid")
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(c, file, "version").Output()
	if err != nil || json.Unmarshal(b, &info) != nil {
		return info, errors.New("Cannot read the bundled backend version")
	}
	return info, nil
}
