package softwareupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const agentDir = "/usr/local/libexec/routepilot-updater"
const agentSocket = "/var/run/com.routepilot.updater.sock"
const agentPlist = "/Library/LaunchDaemons/com.routepilot.update-agent.plist"
const appTarget = "/Applications/RoutePilot.app"

type agentConfig struct {
	UID int `json:"uid"`
}
type agentRequest struct {
	Action  string `json:"action"`
	Version string `json:"version,omitempty"`
}
type agentStatus struct {
	Status
	Ready bool `json:"ready"`
}
type agent struct {
	mu     sync.Mutex
	state  Status
	latest release
	busy   bool
	uid    int
}

// InstallAgent requires a one-time, explicit administrator authorization. The
// daemon's socket grants the selected local user only fixed official updates.
func InstallAgent(uid int) error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 || uid <= 0 || uid > 2147483647 {
		return errors.New("Invalid update assistant installation")
	}
	if err := applicationsOwned(); err != nil {
		return errors.New("The Applications directory must be root-owned and not writable by other users")
	}
	info, err := os.Lstat(appTarget)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Install RoutePilot.app in /Applications before enabling the update assistant")
	}
	if err = command(context.Background(), "/usr/bin/codesign", "--verify", "--deep", "--strict", appTarget); err != nil {
		return errors.New("Installed App code signature is invalid")
	}
	source, err := os.Executable()
	if err != nil {
		return err
	}
	installedSource := appTarget + "/Contents/Resources/Service/bin/surge-controller"
	a, err := os.Stat(source)
	if err != nil {
		return err
	}
	b, err := os.Stat(installedSource)
	if err != nil || !os.SameFile(a, b) {
		return errors.New("Enable the update assistant from the installed RoutePilot App")
	}
	for _, parent := range []string{"/usr/local", "/usr/local/libexec", "/Library/LaunchDaemons"} {
		if err = privateOwned(parent, true); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(agentDir, 0700); err != nil {
		return err
	}
	if err = privateOwned(agentDir, true); err != nil {
		return err
	}
	var previous Status
	if b, err := os.ReadFile(agentDir + "/status.json"); err == nil {
		_ = json.Unmarshal(b, &previous)
	}
	if active(previous.Phase) && time.Since(previous.UpdatedAt) < 10*time.Minute {
		return errors.New("An App update is already in progress")
	}
	_ = command(context.Background(), "/bin/launchctl", "bootout", "system/com.routepilot.update-agent")
	if err = copyExecutable(source, agentDir+"/agent.next"); err != nil {
		return err
	}
	if err = os.Rename(agentDir+"/agent.next", agentDir+"/agent"); err != nil {
		return err
	}
	if err = writeJSON(agentDir+"/config.json", agentConfig{uid}); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>com.routepilot.update-agent</string><key>ProgramArguments</key><array><string>` + agentDir + `/agent</string><string>update-agent</string></array><key>UserName</key><string>root</string><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>10</integer><key>ProcessType</key><string>Background</string><key>Umask</key><integer>63</integer></dict></plist>`
	if err = os.WriteFile(agentPlist, []byte(plist), 0600); err != nil {
		return err
	}
	return command(context.Background(), "/bin/launchctl", "bootstrap", "system", agentPlist)
}
func ServeAgent(ctx context.Context) error {
	if os.Geteuid() != 0 || runtime.GOOS != "darwin" {
		return errors.New("Update assistant requires the macOS system service")
	}
	for _, p := range []string{agentDir + "/config.json", agentDir + "/agent"} {
		if err := privateOwned(p, false); err != nil {
			return err
		}
	}
	for _, parent := range []string{"/usr/local", "/usr/local/libexec", "/Library/LaunchDaemons"} {
		if err := privateOwned(parent, true); err != nil {
			return err
		}
	}
	if err := privateOwned(agentDir, true); err != nil {
		return err
	}
	if jobPID("com.routepilot.update-agent") != os.Getpid() {
		return errors.New("Update assistant must be launched by its system job")
	}
	b, err := os.ReadFile(agentDir + "/config.json")
	if err != nil {
		return err
	}
	var cfg agentConfig
	if json.Unmarshal(b, &cfg) != nil || cfg.UID <= 0 {
		return errors.New("Invalid update assistant owner")
	}
	if info, err := os.Lstat(agentSocket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("Unsafe update assistant socket")
		}
		if err = os.Remove(agentSocket); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: agentSocket, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(agentSocket)
	if err = os.Chmod(agentSocket, 0600); err != nil {
		return err
	}
	if err = os.Chown(agentSocket, cfg.UID, -1); err != nil {
		return err
	}
	agent := &agent{uid: cfg.UID, state: Status{Phase: "idle"}}
	if b, e := os.ReadFile(agentDir + "/status.json"); e == nil {
		_ = json.Unmarshal(b, &agent.state)
	}
	if active(agent.state.Phase) {
		agent.state.Phase = "failed"
		agent.state.Error = "App update was interrupted; check the installed App and retry"
		agent.persist()
	}
	go func() { <-ctx.Done(); _ = listener.Close() }()
	slots := make(chan struct{}, 8)
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			go func() { defer func() { <-slots }(); defer conn.Close(); agent.serve(ctx, conn) }()
		default:
			conn.Close()
		}
	}
}
func appVersion(ctx context.Context) string {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(c, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleShortVersionString", appTarget+"/Contents/Info.plist").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func (a *agent) persist() {
	a.state.UpdatedAt = time.Now().UTC()
	_ = writeJSON(agentDir+"/status.json", a.state)
}
func (a *agent) view(ctx context.Context) agentStatus {
	s := a.state
	s.Supported = true
	s.Current = appVersion(ctx)
	s.Latest = a.latest.Tag
	s.Available = newer(s.Latest, s.Current)
	return agentStatus{Status: s, Ready: true}
}
func (a *agent) serve(ctx context.Context, c *net.UnixConn) {
	_ = c.SetDeadline(time.Now().Add(25 * time.Second))
	reader := bufio.NewReader(io.LimitReader(c, 4097))
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) > 4096 {
		return
	}
	var req agentRequest
	decoder := json.NewDecoder(strings.NewReader(string(line)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var failure error
	switch req.Action {
	case "status":
	case "check":
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		b, e := download(checkCtx, origin+"/api/v1/repos/knot/routepilot/releases/latest", 2<<20)
		failure = e
		if failure == nil {
			var r release
			if json.Unmarshal(b, &r) != nil || r.Draft || r.Prerelease {
				failure = errors.New("Latest release metadata is invalid")
			} else if _, e = versionParts(r.Tag); e != nil {
				failure = e
			} else {
				a.latest = r
				a.state.CheckedAt = time.Now().UTC()
			}
		}
	case "install":
		if a.busy {
			failure = errors.New("An App update is already in progress")
		} else if req.Version != a.latest.Tag || !newer(req.Version, appVersion(ctx)) || time.Since(a.state.CheckedAt) > 10*time.Minute {
			failure = errors.New("Check the latest release again before updating")
		} else {
			a.busy = true
			a.state.Phase = "downloading"
			a.state.Target = req.Version
			a.state.Error = ""
			a.persist()
			r := a.latest
			go func() {
				updateCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
				defer cancel()
				err := a.install(updateCtx, r)
				a.mu.Lock()
				defer a.mu.Unlock()
				a.busy = false
				if err != nil {
					a.state.Phase = "failed"
					a.state.Error = err.Error()
				} else {
					a.state.Phase = "succeeded"
					a.state.Error = ""
				}
				a.persist()
			}()
		}
	case "refresh-agent":
		if a.busy || a.state.Phase != "succeeded" {
			failure = errors.New("Finish the App update before refreshing its assistant")
		} else {
			go func() {
				time.Sleep(2 * time.Second)
				_ = command(context.Background(), "/bin/launchctl", "kickstart", "-k", "system/com.routepilot.update-agent")
			}()
		}
	default:
		failure = errors.New("Unsupported update assistant action")
	}
	if failure != nil {
		_ = json.NewEncoder(c).Encode(map[string]string{"error": failure.Error()})
	} else {
		_ = json.NewEncoder(c).Encode(a.view(ctx))
	}
}
func (a *agent) phase(value string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Phase = value
	a.persist()
}
func (a *agent) install(ctx context.Context, r release) (result error) {
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
	b = cachedAppArchive(a.uid, expected)
	if b == nil {
		b, err = download(ctx, urls[name], 256<<20)
		if err != nil {
			return err
		}
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != expected {
		return errors.New("App package SHA-256 verification failed")
	}
	a.phase("validating")
	stage, err := os.MkdirTemp(agentDir, "app-update-")
	if err != nil {
		return err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = os.RemoveAll(stage)
		}
	}()
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
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Installed App path is unsafe")
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
	// Keep an independent service executable for rollback. Its database is never copied.
	serviceChanged := false
	previous := stage + "/previous-backend"
	defer func() {
		if result == nil || !serviceChanged {
			return
		}
		pid := servicePID()
		_ = command(context.Background(), "/bin/launchctl", "bootout", "system", servicePlist)
		if waitProcess(pid, 35*time.Second) != nil {
			keepStage = true
			result = errors.New("App update failed; previous backend retained in " + stage)
			return
		}
		if err := os.Rename(previous, serviceBin); err != nil {
			keepStage = true
			result = errors.New("App update failed; previous backend retained in " + stage)
			return
		}
		if command(context.Background(), "/bin/launchctl", "bootstrap", "system", servicePlist) != nil {
			result = errors.New("App update failed; could not restart the previous backend")
		}
	}()
	if _, err = os.Lstat(servicePlist); err == nil {
		if err = privateOwned(servicePlist, false); err != nil {
			return err
		}
		if err = privateOwned(serviceBin, false); err != nil {
			return err
		}
		pid := servicePID()
		if pid <= 1 {
			return errors.New("Start the installed network service before updating the App")
		}
		if workerPID() > 0 {
			return errors.New("A backend update is already in progress")
		}
		for _, parent := range []string{serviceDir, "/private/var/db/surge-controller"} {
			if err = privateOwned(parent, true); err != nil {
				return err
			}
		}
		if err = copyExecutable(serviceBin, previous); err != nil {
			return err
		}
		if err = os.Chmod(previous, 0755); err != nil {
			return err
		}
		versionOutput, versionErr := exec.CommandContext(ctx, serviceBin, "version").Output()
		var current struct {
			Version string `json:"version"`
		}
		if versionErr != nil || json.Unmarshal(versionOutput, &current) != nil || current.Version == "" {
			return errors.New("Cannot identify the installed backend version")
		}
		if newer("v"+current.Version, r.Tag) {
			return errors.New("The installed backend is newer than this App release")
		}
		a.phase("restarting")
		started := time.Now()
		serviceChanged = true
		if err = command(ctx, "/bin/sh", source+"/Contents/Resources/Service/scripts/update_macos.sh"); err != nil {
			return errors.New("Network service update failed; App was not replaced")
		}
		if err = waitReady(ctx, r.Tag, started); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	backup := swap + "/previous.app"
	if err = os.Rename(appTarget, backup); err != nil {
		return err
	}
	if err = os.Rename(candidate, appTarget); err != nil {
		if restore := os.Rename(backup, appTarget); restore != nil {
			keepSwap = true
			return errors.New("App replacement failed; previous App retained in " + backup)
		}
		return err
	}
	// Update the root-owned helper executable for its next launch. The currently
	// running daemon remains alive long enough for the App to receive success.
	if err = copyExecutable(nextBackend, agentDir+"/agent.next"); err == nil {
		_ = os.Rename(agentDir+"/agent.next", agentDir+"/agent")
	}
	return nil
}
func waitReady(ctx context.Context, tag string, after time.Time) error {
	return waitForReady(ctx, tag, after, readinessProbe{
		read: func() ([]byte, error) { return os.ReadFile(readyFile) }, pid: servicePID, now: time.Now, pause: pause,
	})
}

type readinessProbe struct {
	read  func() ([]byte, error)
	pid   func() int
	now   func() time.Time
	pause func(context.Context, time.Duration) error
}

func waitForReady(ctx context.Context, tag string, after time.Time, p readinessProbe) error {
	deadline := p.now().Add(60 * time.Second)
	for p.now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, _ := p.read()
		var r ready
		if json.Unmarshal(b, &r) == nil && r.Version == strings.TrimPrefix(tag, "v") && r.PID > 1 && r.PID == p.pid() && r.At.After(after) {
			if err := p.pause(ctx, 3*time.Second); err != nil {
				return err
			}
			if p.pid() == r.PID {
				return nil
			}
		}
		if err := p.pause(ctx, time.Second); err != nil {
			return err
		}
	}
	return errors.New("Updated network service did not become ready; restoring the previous executable")
}

// ParseAgentUID keeps installer arguments decimal and excludes the root account.
func ParseAgentUID(raw string) (int, error) {
	uid, err := strconv.Atoi(raw)
	if err != nil || uid <= 0 || uid > 2147483647 || strconv.Itoa(uid) != raw {
		return 0, errors.New("Invalid update assistant owner")
	}
	return uid, nil
}

// /Applications commonly allows the macOS admin group to install applications.
// Only root ownership and that specific trusted group are accepted here.
func applicationsOwned() error {
	info, err := os.Lstat("/Applications")
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0002 != 0 || info.Mode().Perm()&0020 != 0 && stat.Gid != 80 {
		return errors.New("Unsafe Applications directory ownership or permissions")
	}
	return nil
}

// App and remote-backend workers must not replace the service concurrently.
func lockInstall() (func(), error) {
	fd, err := syscall.Open("/var/run/com.routepilot.update.lock", syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "update-lock")
	if err = privateOwned("/var/run/com.routepilot.update.lock", false); err != nil {
		file.Close()
		return nil, err
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("Another software update is already in progress")
	}
	return func() { _ = file.Close() }, nil
}

func UninstallAgent() error {
	if runtime.GOOS != "darwin" || os.Geteuid() != 0 {
		return errors.New("Administrator authorization is required")
	}
	if err := privateOwned(agentDir, true); err != nil {
		return err
	}
	unlock, err := lockInstall()
	if err != nil {
		return err
	}
	defer unlock()
	_ = command(context.Background(), "/bin/launchctl", "bootout", "system/com.routepilot.update-agent")
	if jobPID("com.routepilot.update-agent") > 0 {
		return errors.New("Update assistant is still running")
	}
	if err = os.Remove(agentPlist); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info, err := os.Lstat(agentSocket); err == nil && info.Mode()&os.ModeSocket != 0 {
		_ = os.Remove(agentSocket)
	}
	return os.RemoveAll(agentDir)
}

// A user cache is only a source of bytes, never executable input until the
// checksum from the official HTTPS release has matched. Use a bounded regular
// file owned by the authorized user; never read root-private files via symlinks.
func cachedAppArchive(uid int, expected string) []byte {
	account, err := user.LookupId(strconv.Itoa(uid))
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		return nil
	}
	path := filepath.Join(account.HomeDir, "Library/Caches/RoutePilot/Updates", expected+".zip")
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid {
		return nil
	}
	bytes, err := io.ReadAll(io.LimitReader(file, (256<<20)+1))
	if err != nil || len(bytes) > 256<<20 {
		return nil
	}
	sum := sha256.Sum256(bytes)
	if hex.EncodeToString(sum[:]) != expected {
		return nil
	}
	return bytes
}
