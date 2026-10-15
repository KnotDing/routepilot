// Package softwareupdate updates the complete macOS App and its network service.
// Clients select a release version, never an executable URL, path or command.
package softwareupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"surge-controller/internal/buildinfo"
)

const (
	giteaOrigin  = "https://gitea.knotding.com"
	githubAPI    = "https://api.github.com/repos/KnotDing/routepilot"
	serviceDir   = "/usr/local/libexec/surge-controller"
	serviceBin   = serviceDir + "/surge-controller"
	servicePlist = "/Library/LaunchDaemons/com.routepilot.surge-controller.plist"
	workerPlist  = "/Library/LaunchDaemons/com.routepilot.update-worker.plist"
	stagingDir   = serviceDir + "/.release-update"
	stateFile    = "/var/db/surge-controller/software-update.json"
	readyFile    = "/var/db/surge-controller/software-update-ready.json"
)

type Status struct {
	Supported                bool      `json:"supported"`
	Reason                   string    `json:"reason,omitempty"`
	Current                  string    `json:"current"`
	AppCurrent               string    `json:"app_current,omitempty"`
	Latest                   string    `json:"latest,omitempty"`
	ReleaseNotes             string    `json:"release_notes,omitempty"`
	Available                bool      `json:"available"`
	Phase                    string    `json:"phase"`
	Target                   string    `json:"target,omitempty"`
	Error                    string    `json:"error,omitempty"`
	CheckedAt                time.Time `json:"checked_at,omitempty"`
	UpdatedAt                time.Time `json:"updated_at,omitempty"`
	AppRestart               string    `json:"app_restart,omitempty"`
	AppRestartError          string    `json:"app_restart_error,omitempty"`
	AppRestartForceSupported bool      `json:"app_restart_force_supported,omitempty"`
	SkippedVersion           string    `json:"skipped_version,omitempty"`
	CheckError               string    `json:"check_error,omitempty"`
	AutomationError          string    `json:"automation_error,omitempty"`
	NextRetryAt              time.Time `json:"next_retry_at,omitempty"`
	DownloadBytes            int64     `json:"download_bytes,omitempty"`
	DownloadTotal            int64     `json:"download_total,omitempty"`
}
type release struct {
	Tag        string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
	Source string `json:"-"`
}
type Manager struct {
	mu               sync.Mutex
	checkMu          sync.Mutex
	ctx              context.Context
	standardService  bool
	supported        bool
	reason           string
	running          bool
	latest           release
	checkedAt        time.Time
	checkedBeta      bool
	checkedSource    string
	source           string
	checkError       string
	beforeRestart    func() error
	appRestartOnce   sync.Once
	appReconcileMu   sync.Mutex
	appReconcileAt   time.Time
	appRestartMu     sync.Mutex
	appRestartCancel context.CancelFunc
	appRestartDone   chan struct{}
	appRestartID     uint64
	appRestartForced bool
	appOps           appRestartOperations
	ops              updateOperations
}

type updateOperations struct {
	appVersion func() string
	fetch      func(context.Context, string, int64) ([]byte, error)
	read       func() Status
	write      func(Status) error
	pid        func() int
	prepare    func(context.Context, release, func() error, func(DownloadProgress) error) error
}

func New(ctx context.Context, database string, beforeRestart func() error) *Manager {
	m := &Manager{ctx: ctx, beforeRestart: beforeRestart, source: "github", ops: updateOperations{
		fetch: download, read: readState, write: func(s Status) error { return writeJSON(stateFile, s) },
		pid: workerPID, prepare: prepare, appVersion: func() string { return appVersion(ctx) },
	}}
	m.standardService = runtime.GOOS == "darwin" && os.Geteuid() == 0 && filepath.Clean(database) == "/var/db/surge-controller/controller.sqlite"
	m.appOps = appRestartOperations{read: readAppRestart, write: func(s appRestartStatus) error { return writeJSON(appRestartFile, s) }, run: m.restartRemoteApp}
	m.reason = supportReason(database)
	m.supported = m.reason == ""
	return m
}
func supportReason(database string) string {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return "Online backend updates currently require macOS Apple Silicon"
	}
	if os.Geteuid() != 0 || filepath.Clean(database) != "/var/db/surge-controller/controller.sqlite" {
		return "Install the standard root-owned launchd service to enable online updates"
	}
	for _, dir := range []string{"/usr/local", "/usr/local/libexec", serviceDir, "/private/var/db/surge-controller", "/Library/LaunchDaemons"} {
		if err := privateOwned(dir, true); err != nil {
			return "Online update directories must be owned by root and not writable by other users"
		}
	}
	for _, file := range []string{serviceBin, servicePlist} {
		if err := privateOwned(file, false); err != nil {
			return "The standard service executable and launchd configuration are required"
		}
	}
	current, err := os.Executable()
	if err != nil {
		return "Cannot identify the running backend executable"
	}
	running, err := os.Stat(current)
	if err != nil {
		return "Cannot identify the running backend executable"
	}
	installed, err := os.Stat(serviceBin)
	if err != nil || !os.SameFile(running, installed) || servicePID() != os.Getpid() {
		return "Online updates require the installed launchd service to be running"
	}
	return ""
}
func privateOwned(file string, directory bool) error {
	info, err := os.Lstat(file)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Mode()&os.ModeSymlink != 0 || directory != info.IsDir() || !directory && !info.Mode().IsRegular() {
		return errors.New("unsafe update file ownership or permissions")
	}
	return nil
}
func versionParts(value string) ([]int, error) {
	version := strings.TrimPrefix(value, "v")
	segments := strings.Split(version, "-")
	if len(segments) > 2 || len(segments) == 2 && !validBetaSuffix(segments[1]) {
		return nil, errors.New("Only stable or numbered beta semantic releases can be installed online")
	}
	parts := strings.Split(segments[0], ".")
	if len(parts) != 3 {
		return nil, errors.New("Only stable or numbered beta semantic releases can be installed online")
	}
	result := make([]int, 3)
	for i, part := range parts {
		n, e := strconv.Atoi(part)
		if e != nil || n < 0 || strconv.Itoa(n) != part {
			return nil, errors.New("Only stable or numbered beta semantic releases can be installed online")
		}
		result[i] = n
	}
	return result, nil
}

// ValidateVersion accepts only the release identities supported by the installer.
func ValidateVersion(value string) error {
	_, err := versionParts(value)
	return err
}

func validBetaSuffix(suffix string) bool {
	parts := strings.Split(suffix, ".")
	if len(parts) != 2 || parts[0] != "beta" {
		return false
	}
	n, err := strconv.Atoi(parts[1])
	return err == nil && n > 0 && strconv.Itoa(n) == parts[1]
}

func betaNumber(value string) (int, bool) {
	parts := strings.Split(strings.TrimPrefix(value, "v"), "-")
	if len(parts) != 2 || !validBetaSuffix(parts[1]) {
		return 0, false
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(parts[1], "beta."))
	return n, true
}

func newer(latest, current string) bool {
	a, e := versionParts(latest)
	b, f := versionParts(current)
	if e != nil || f != nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	latestBeta, latestIsBeta := betaNumber(latest)
	currentBeta, currentIsBeta := betaNumber(current)
	if latestIsBeta != currentIsBeta {
		return currentIsBeta
	}
	return latestIsBeta && latestBeta > currentBeta
}

// installableRelease keeps the official tag metadata as the only source: the
// tag must match the checked target, and numbered beta tags are only accepted
// from prerelease releases (stable tags only from stable releases).
func installableRelease(r release, target string) bool {
	if r.Tag != target || r.Draft {
		return false
	}
	if _, err := versionParts(r.Tag); err != nil {
		return false
	}
	_, beta := betaNumber(r.Tag)
	return beta == r.Prerelease
}
func readState() Status {
	var s Status
	if b, e := os.ReadFile(stateFile); e == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Phase == "" {
		s.Phase = "idle"
	}
	return s
}
func writeJSON(file string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".update-state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), file)
	}
	return err
}
func (m *Manager) snapshot() Status {
	s := Status{Phase: "idle"}
	if m.supported {
		s = m.ops.read()
		if active(s.Phase) && !m.running && time.Since(s.UpdatedAt) > 10*time.Minute {
			s.Phase = "failed"
			s.Error = "Backend update was interrupted; check the remote service before retrying"
		}
	}
	s.Supported, s.Reason, s.Current = m.supported, m.reason, buildinfo.Current().Version
	s.Latest, s.CheckedAt = m.latest.Tag, m.checkedAt
	s.ReleaseNotes = m.latest.Body
	s.CheckError = m.checkError
	if m.supported {
		s.AppRestartForceSupported = true
		s.AppCurrent = m.ops.appVersion()
		if restart := m.appOps.read(); restart.Target == s.Target {
			s.AppRestart, s.AppRestartError = restart.Phase, restart.Error
			m.reconcileAppRestart(s, restart)
		}
	}
	s.Available = newer(s.Latest, s.Current) || m.supported && applicationUpdateAvailable(s.Latest, s.Current, s.AppCurrent)
	return s
}
func (m *Manager) Status() Status { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot() }
func updateSource(source string) (string, string) {
	if source == "gitea" {
		return "gitea", giteaOrigin + "/api/v1/repos/knot/routepilot"
	}
	return "github", githubAPI
}

func releaseEndpoint(source, path string) string {
	_, api := updateSource(source)
	return api + path
}

func (m *Manager) SetSource(source string) {
	source, _ = updateSource(source)
	m.mu.Lock()
	m.source = source
	m.mu.Unlock()
}

func (m *Manager) Check(ctx context.Context, includeBeta bool) (result Status, checkErr error) {
	m.checkMu.Lock()
	defer m.checkMu.Unlock()
	defer func() {
		if checkErr != nil {
			m.mu.Lock()
			m.checkError = checkErr.Error()
			result = m.snapshot()
			m.mu.Unlock()
		}
	}()
	m.mu.Lock()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	source, api := updateSource(m.source)
	// Reuse a result briefly so dashboard mounts and multiple clients do not flood the release API.
	if m.checkedBeta == includeBeta && m.checkedSource == source && time.Since(m.checkedAt) < time.Minute {
		m.checkError = ""
		value := m.snapshot()
		m.mu.Unlock()
		return value, nil
	}
	m.mu.Unlock()
	var r release
	b, err := m.ops.fetch(ctx, api+"/releases/latest", 2<<20)
	if err != nil {
		return m.Status(), err
	}
	if err = json.Unmarshal(b, &r); err != nil || r.Draft || r.Prerelease {
		return m.Status(), errors.New("Latest release metadata is invalid")
	}
	_, isBeta := betaNumber(r.Tag)
	if _, err = versionParts(r.Tag); err != nil || isBeta || r.Prerelease {
		if err == nil {
			err = errors.New("The latest endpoint did not return a stable release")
		}
		return m.Status(), err
	}
	if includeBeta {
		// A numbered beta is only preferred when it is newer than the stable
		// release; equal base versions keep the stable release.
		listPath := "/releases?per_page=100"
		if source == "gitea" {
			listPath = "/releases?limit=50"
		}
		b, err = m.ops.fetch(ctx, api+listPath, 2<<20)
		if err != nil {
			return m.Status(), err
		}
		var releases []release
		if json.Unmarshal(b, &releases) != nil {
			return m.Status(), errors.New("Beta release metadata is invalid")
		}
		if beta, ok := newestBetaRelease(releases); ok && newer(beta.Tag, r.Tag) {
			r = beta
		}
	}
	r.Source = source
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest = r
	m.checkedAt = time.Now().UTC()
	m.checkedBeta = includeBeta
	m.checkedSource = source
	m.checkError = ""
	return m.snapshot(), nil
}
func active(phase string) bool {
	return phase == "downloading" || phase == "validating" || phase == "restarting"
}
func (m *Manager) Start(version string) (Status, error) {
	return m.startForSource(version, nil, "")
}

// StartForChannel prevents channel or source changes from installing an older
// result, even while that check is still recent.

func (m *Manager) StartForChannel(version string, includeBeta bool) (Status, error) {
	m.mu.Lock()
	source := m.source
	m.mu.Unlock()
	return m.startForSource(version, &includeBeta, source)
}

func (m *Manager) startForSource(version string, includeBeta *bool, source string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.supported {
		return m.snapshot(), errors.New(m.reason)
	}
	previous := m.ops.read()
	if m.running || m.appRestartForceBusy() || m.ops.pid() > 0 || active(previous.Phase) && time.Since(previous.UpdatedAt) < 10*time.Minute {
		return m.snapshot(), errors.New("A backend update is already in progress")
	}
	if includeBeta != nil && *includeBeta != m.checkedBeta || source != "" && source != m.checkedSource || version != m.latest.Tag || !applicationUpdateAvailable(version, buildinfo.Current().Version, m.ops.appVersion()) || time.Since(m.checkedAt) > 10*time.Minute {
		return m.snapshot(), errors.New("Check the latest release again before updating")
	}
	s := Status{Phase: "downloading", Target: version, UpdatedAt: time.Now().UTC()}
	if err := m.ops.write(s); err != nil {
		return m.snapshot(), err
	}
	m.running = true
	r := m.latest
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		defer cancel()
		err := m.ops.prepare(ctx, r, m.beforeRestart, func(p DownloadProgress) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			status := m.ops.read()
			status.DownloadBytes, status.DownloadTotal, status.UpdatedAt = p.Bytes, p.Total, time.Now().UTC()
			return m.ops.write(status)
		})
		m.mu.Lock()
		defer m.mu.Unlock()
		m.running = false
		if err != nil {
			s.Phase = "failed"
			s.Error = err.Error()
			s.UpdatedAt = time.Now().UTC()
			_ = m.ops.write(s)
		}
	}()
	return m.snapshot(), nil
}
func download(ctx context.Context, address string, limit int64) ([]byte, error) {
	return downloadWithTransport(ctx, address, limit, nil)
}

// A supplied transport lets offline checks exercise HTTP behavior without
// relaxing the fixed release origin or changing process-wide HTTP defaults.
func downloadWithTransport(ctx context.Context, address string, limit int64, transport http.RoundTripper) ([]byte, error) {
	return downloadWithTransportProgress(ctx, address, limit, transport, nil)
}

func downloadWithTransportProgress(ctx context.Context, address string, limit int64, transport http.RoundTripper, progress func(DownloadProgress) error) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Port() != "" || !trustedUpdateURL(u) || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("Release download origin is not trusted")
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" || req.URL.Port() != "" || !trustedUpdateURL(req.URL) || req.URL.User != nil || req.URL.Fragment != "" || req.URL.Opaque != "" {
			return errors.New("Untrusted release redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Cannot connect to the release server")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("Release request failed (HTTP %d)", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, errors.New("Release asset exceeds the size limit")
	}
	reader := &downloadProgressReader{reader: response.Body, total: max(0, response.ContentLength), report: progress}
	if err := reader.publish(); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.New("Release download failed")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("Release asset exceeds the size limit")
	}
	if response.ContentLength > 0 && int64(len(b)) != response.ContentLength {
		return nil, errors.New("Release download failed")
	}
	if err := reader.publish(); err != nil {
		return nil, err
	}
	return b, nil
}

func trustedUpdateURL(u *url.URL) bool {
	switch u.Hostname() {
	case "gitea.knotding.com":
		return true
	case "api.github.com":
		return strings.HasPrefix(u.Path, "/repos/KnotDing/routepilot/")
	case "github.com":
		return strings.HasPrefix(u.Path, "/KnotDing/routepilot/releases/download/")
	case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	default:
		return false
	}
}
func prepare(ctx context.Context, r release, beforeRestart func() error, progress func(DownloadProgress) error) error {
	name := "RoutePilot-macos-arm64.zip"
	urls, err := releaseAssets(r, name)
	if err != nil {
		return err
	}
	sums, err := download(ctx, urls["SHA256SUMS"], 1<<20)
	if err != nil {
		return err
	}
	expected, err := packageChecksum(sums, name)
	if err != nil {
		return err
	}
	archive, err := downloadWithTransportProgress(ctx, urls[name], 256<<20, nil, progress)
	if err != nil {
		return err
	}
	if err := verifyChecksum(archive, expected); err != nil {
		return err
	}
	// Preparation runs asynchronously. Hand off sessions only after downloading,
	// so a slow release server does not consume their short restart grace period.
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return err
	}
	if err := privateOwned(stagingDir, true); err != nil {
		return err
	}
	if err := os.Chmod(stagingDir, 0700); err != nil {
		return err
	}
	if workerPID() > 0 {
		return errors.New("A backend update is already in progress")
	}
	_ = command(ctx, "/bin/launchctl", "bootout", "system/com.routepilot.update-worker")
	archiveFile := filepath.Join(stagingDir, "application.zip")
	_ = os.Remove(archiveFile)
	file, err := os.OpenFile(archiveFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(archive)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := copyExecutable(serviceBin, filepath.Join(stagingDir, "worker")); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(stagingDir, "job.json"), workerJob{Target: r.Tag, PreviousPID: os.Getpid(), Source: r.Source}); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>com.routepilot.update-worker</string><key>ProgramArguments</key><array><string>` + stagingDir + `/worker</string><string>update-worker</string></array><key>UserName</key><string>root</string><key>RunAtLoad</key><true/><key>ProcessType</key><string>Background</string><key>StandardOutPath</key><string>/var/log/surge-controller/update.log</string><key>StandardErrorPath</key><string>/var/log/surge-controller/update.log</string><key>Umask</key><integer>63</integer></dict></plist>`
	if err := os.WriteFile(workerPlist, []byte(plist), 0600); err != nil {
		return err
	}
	if beforeRestart == nil {
		return errors.New("Update session handoff is unavailable")
	}
	if err := beforeRestart(); err != nil {
		return err
	}
	if err := command(ctx, "/bin/launchctl", "bootstrap", "system", workerPlist); err != nil {
		return errors.New("Could not start the backend update helper")
	}
	return nil
}

func releaseAssets(r release, asset string) (map[string]string, error) {
	urls := map[string]string{}
	seen := map[string]bool{}
	for _, a := range r.Assets {
		if a.Name == asset || a.Name == "SHA256SUMS" {
			if seen[a.Name] {
				return nil, errors.New("Duplicate release assets")
			}
			seen[a.Name] = true
			urls[a.Name] = a.URL
		}
	}
	if urls[asset] == "" || urls["SHA256SUMS"] == "" {
		return nil, errors.New("Release has no package for this platform")
	}
	return urls, nil
}

func packageChecksum(sums []byte, asset string) (string, error) {
	expected := ""
	found := false
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			if found {
				return "", errors.New("Duplicate package checksums")
			}
			found = true
			expected = strings.ToLower(fields[0])
		}
	}
	digest, err := hex.DecodeString(expected)
	if err != nil || len(digest) != sha256.Size {
		return "", errors.New("Release checksum is missing or invalid")
	}
	return expected, nil
}

func verifyChecksum(contents []byte, expected string) error {
	actual := sha256.Sum256(contents)
	if hex.EncodeToString(actual[:]) != expected {
		return errors.New("Package SHA-256 verification failed")
	}
	return nil
}

func copyExecutable(from, to string) error {
	_ = os.Remove(to)
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if err == nil {
		err = dst.Sync()
	}
	closeErr := dst.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func command(ctx context.Context, name string, args ...string) error {
	c, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	return exec.CommandContext(c, name, args...).Run()
}
func validateBinary(ctx context.Context, file, tag string) error {
	if err := privateOwned(file, false); err != nil {
		return err
	}
	if err := command(ctx, "/usr/bin/codesign", "--verify", "--strict", file); err != nil {
		return errors.New("Backend executable code signature is invalid")
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(c, file, "version").Output()
	if err != nil {
		return errors.New("Cannot read the update executable version")
	}
	var info buildinfo.Info
	if json.Unmarshal(b, &info) != nil || "v"+info.Version != tag || info.OS != runtime.GOOS || info.Arch != runtime.GOARCH || info.Name != buildinfo.Current().Name {
		return errors.New("Backend update version or platform does not match the release")
	}
	return nil
}
func jobPID(label string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/bin/launchctl", "print", "system/"+label).Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "pid" && fields[1] == "=" {
			pid, _ := strconv.Atoi(fields[2])
			return pid
		}
	}
	return 0
}
func servicePID() int { return jobPID("com.routepilot.surge-controller") }
func workerPID() int  { return jobPID("com.routepilot.update-worker") }
