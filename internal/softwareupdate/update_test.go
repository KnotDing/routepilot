package softwareupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"surge-controller/internal/buildinfo"
)

const origin = giteaOrigin

func TestStableVersionsAndUpdateOrdering(t *testing.T) {
	for _, value := range []string{"0.5.2", "v1.0.0", "v12.30.40", "v1.0.1-beta.1"} {
		if _, err := versionParts(value); err != nil {
			t.Errorf("%s: %v", value, err)
		}
	}
	for _, value := range []string{"", "v", "1", "1.2", "1.2.3.4", "01.2.3", "+1.2.3", "1.-2.3", "1.2.3-beta", "1.2.3-beta.0", "1.2.3-beta.01", "1.2.3-rc.1", "1.2.3+build", "release-1.2.3", "vv1.2.3", "1.2.3\n", "999999999999999999999.0.0"} {
		if _, err := versionParts(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"v1.0.0", "0.5.2", true}, {"v0.10.0", "0.9.99", true}, {"v0.5.10", "0.5.2", true}, {"v0.5.2", "0.5.2", false}, {"v0.5.1", "0.5.2", false}, {"v1.0.0-beta.1", "0.5.2", true}, {"v1.0.0-beta.2", "1.0.0-beta.1", true}, {"v1.0.0", "1.0.0-beta.2", true}, {"v1.0.0-beta.1", "1.0.0", false}, {"v1.0.0", "invalid", false}} {
		if newer(tc.a, tc.b) != tc.want {
			t.Errorf("newer(%s,%s)", tc.a, tc.b)
		}
	}
	for _, value := range []string{"501", "2147483647"} {
		if _, err := ParseAgentUID(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"0", "-1", "+501", "0501", "501\n", "2147483648", "root", ""} {
		if _, err := ParseAgentUID(value); err == nil {
			t.Errorf("accepted UID %q", value)
		}
	}
}

func TestNewestBetaRelease(t *testing.T) {
	releases := []release{
		{Tag: "v1.0.1-beta.1", Prerelease: true},
		{Tag: "v1.0.1-beta.3", Prerelease: true},
		{Tag: "v1.0.1-beta.4", Prerelease: true, Draft: true},
		{Tag: "v1.0.1-rc.1", Prerelease: true},
		{Tag: "v1.1.0"},
	}
	latest, ok := newestBetaRelease(releases)
	if !ok || latest.Tag != "v1.0.1-beta.3" {
		t.Fatalf("newest beta = %q, %v", latest.Tag, ok)
	}
	if _, ok := newestBetaRelease([]release{{Tag: "v1.0.1-beta.1"}}); ok {
		t.Fatal("accepted a stable release incorrectly tagged as beta")
	}
}

func fixtureRelease(assets ...string) release {
	r := release{Tag: "v1.0.0"}
	for _, name := range assets {
		r.Assets = append(r.Assets, struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		}{name, origin + "/download/" + name})
	}
	return r
}
func TestReleaseAssetsAndChecksums(t *testing.T) {
	const name = "package.zip"
	r := fixtureRelease(name, "SHA256SUMS", "unrelated.dmg")
	urls, err := releaseAssets(r, name)
	if err != nil || len(urls) != 2 {
		t.Fatal(urls, err)
	}
	for _, bad := range []release{fixtureRelease(name), fixtureRelease("SHA256SUMS"), fixtureRelease(name, "SHA256SUMS", name)} {
		if _, err := releaseAssets(bad, name); err == nil {
			t.Fatal("accepted missing/duplicate asset")
		}
	}
	// An empty URL on the first occurrence must not hide a duplicate.
	r.Assets = append(r.Assets, r.Assets[0])
	r.Assets[0].URL = ""
	if _, err := releaseAssets(r, name); err == nil {
		t.Fatal("accepted duplicate empty URL")
	}
	data := []byte("verified archive")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	for _, line := range []string{digest + "  " + name, strings.ToUpper(digest) + " *" + name, "\n" + digest + " other.zip\n" + digest + "  " + name + "\n"} {
		got, err := packageChecksum([]byte(line), name)
		if err != nil || got != digest {
			t.Fatal(got, err)
		}
		if err := verifyChecksum(data, got); err != nil {
			t.Fatal(err)
		}
	}
	for _, line := range []string{"", digest + " other.zip", "invalid  " + name, strings.Repeat("a", 63) + " " + name, digest + " " + name + "\n" + digest + " " + name} {
		if _, err := packageChecksum([]byte(line), name); err == nil {
			t.Fatalf("accepted invalid checksum %q", line)
		}
	}
	if err := verifyChecksum([]byte("tampered"), digest); err == nil {
		t.Fatal("accepted tampered archive")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func httpResponse(code int, body string, headers http.Header) *http.Response {
	return &http.Response{StatusCode: code, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
}
func TestReleaseDownloadTrustLimitsAndCancellation(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return httpResponse(200, "abcd", nil), nil })
	for _, url := range []string{"http://gitea.knotding.com/a", "https://evil.test/a", "https://gitea.knotding.com:443/a", "https://user@gitea.knotding.com/a", origin + "/a#fragment", "https:gitea.knotding.com/a", "://bad"} {
		if _, err := downloadWithTransport(context.Background(), url, 10, transport); err == nil {
			t.Errorf("accepted %s", url)
		}
	}
	if calls != 0 {
		t.Fatal("untrusted URL reached transport")
	}
	data, err := downloadWithTransport(context.Background(), origin+"/a", 4, transport)
	if err != nil || string(data) != "abcd" {
		t.Fatal(string(data), err)
	}
	if _, err := downloadWithTransport(context.Background(), origin+"/a", 3, transport); err == nil {
		t.Fatal("accepted oversized download")
	}
	for _, code := range []int{204, 400, 403, 404, 500} {
		_, err := downloadWithTransport(context.Background(), origin+"/a", 10, roundTripFunc(func(*http.Request) (*http.Response, error) { return httpResponse(code, "secret key", nil), nil }))
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid HTTP response", err)
		}
	}
	for _, location := range []string{"http://gitea.knotding.com/a", "https://evil.test/a", "https://user@gitea.knotding.com/a", origin + "/a#fragment", origin + "/loop"} {
		count := 0
		_, err := downloadWithTransport(context.Background(), origin+"/a", 10, roundTripFunc(func(*http.Request) (*http.Response, error) {
			count++
			return httpResponse(302, "", http.Header{"Location": {location}}), nil
		}))
		if err == nil || count > 6 {
			t.Fatalf("redirect %s: calls=%d err=%v", location, count, err)
		}
	}
	count := 0
	_, err = downloadWithTransport(context.Background(), origin+"/a", 10, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		count++
		if r.URL.Path == "/a" {
			return httpResponse(302, "", http.Header{"Location": {"/b"}}), nil
		}
		return httpResponse(200, "ok", nil), nil
	}))
	if err != nil || count != 2 {
		t.Fatal(count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = downloadWithTransport(ctx, origin+"/a", 10, roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() }))
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, err = downloadWithTransport(context.Background(), origin+"/a", 10, roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret connection detail") }))
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

func TestAtomicStateAndExecutableCopy(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "state.json")
	if err := writeJSON(file, Status{Phase: "downloading", Target: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(file, Status{Phase: "failed", Error: "failure"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	var status Status
	if json.Unmarshal(data, &status) != nil || status.Phase != "failed" {
		t.Fatal(string(data))
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	names, _ := os.ReadDir(dir)
	if len(names) != 1 {
		t.Fatal("temporary state file leaked", names)
	}
	if err := writeJSON(file, make(chan int)); err == nil {
		t.Fatal("accepted non-JSON value")
	}
	if err := writeJSON(filepath.Join(dir, "missing", "state"), status); err == nil {
		t.Fatal("ignored missing parent")
	}
	next := filepath.Join(dir, "next")
	if err := copyExecutable(file, next); err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(next)
	if !bytes.Equal(data, copied) {
		t.Fatal("copy changed bytes")
	}
	info, _ = os.Stat(next)
	if info.Mode().Perm() != 0700 {
		t.Fatal(info.Mode())
	}
	if err := copyExecutable(filepath.Join(dir, "missing"), next); err == nil {
		t.Fatal("accepted missing executable")
	}
}

func managerTarget() string {
	parts, _ := versionParts(buildinfo.Current().Version)
	return fmt.Sprintf("v%d.%d.%d", parts[0]+1, parts[1], parts[2])
}

type managerFixture struct {
	mu      sync.Mutex
	state   Status
	manager *Manager
}

func isolatedManager(t *testing.T) *managerFixture {
	t.Helper()
	f := &managerFixture{state: Status{Phase: "idle"}}
	m := New(context.Background(), filepath.Join(t.TempDir(), "database"), func() error { return nil })
	m.SetSource("gitea")
	m.supported = true
	m.reason = ""
	m.latest = fixtureRelease("package", "SHA256SUMS")
	m.latest.Tag = managerTarget()
	m.checkedAt = time.Now()
	m.ops.read = func() Status { f.mu.Lock(); defer f.mu.Unlock(); return f.state }
	m.ops.write = func(s Status) error { f.mu.Lock(); defer f.mu.Unlock(); f.state = s; return nil }
	m.ops.pid = func() int { return 0 }
	m.ops.appVersion = func() string { return buildinfo.Current().Version }
	m.ops.prepare = func(context.Context, release, func() error, func(DownloadProgress) error) error {
		return errors.New("fixture preparation failed")
	}
	f.manager = m
	return f
}
func TestManagerCheckValidationCacheAndConcurrency(t *testing.T) {
	f := isolatedManager(t)
	m := f.manager
	m.checkedAt = time.Time{}
	var calls atomic.Int32
	m.ops.fetch = func(ctx context.Context, address string, limit int64) ([]byte, error) {
		calls.Add(1)
		if address != origin+"/api/v1/repos/knot/routepilot/releases/latest" || limit != 2<<20 {
			t.Error("wrong release request")
		}
		return []byte(fmt.Sprintf(`{"tag_name":%q,"body":"## Notes"}`, managerTarget())), nil
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := m.Check(context.Background(), false)
			if err != nil || !s.Available || s.ReleaseNotes != "## Notes" {
				t.Error(s, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("check cache failed", calls.Load())
	}
	for _, body := range []string{`{`, `{"tag_name":"v1.0.0","draft":true}`, `{"tag_name":"v1.0.0","prerelease":true}`, `{"tag_name":"v1.0.0-beta"}`} {
		m.checkedAt = time.Time{}
		m.ops.fetch = func(context.Context, string, int64) ([]byte, error) { return []byte(body), nil }
		if _, err := m.Check(context.Background(), false); err == nil {
			t.Fatal("accepted invalid release", body)
		}
	}
	m.checkedAt = time.Time{}
	m.ops.fetch = func(context.Context, string, int64) ([]byte, error) { return nil, errors.New("offline") }
	s, err := m.Check(context.Background(), false)
	if err == nil || s.Latest != managerTarget() {
		t.Fatal("failed check lost last release", s, err)
	}
}

func TestManagerCheckBetaChannel(t *testing.T) {
	stable := managerTarget()
	beta := "v" + fmt.Sprintf("%d.%d.%d-beta.2", mustVersion(stable)[0], mustVersion(stable)[1], mustVersion(stable)[2]+1)
	sources := func(stableBody, listBody string) func(context.Context, string, int64) ([]byte, error) {
		return func(_ context.Context, address string, _ int64) ([]byte, error) {
			switch address {
			case origin + "/api/v1/repos/knot/routepilot/releases/latest":
				return []byte(stableBody), nil
			case origin + "/api/v1/repos/knot/routepilot/releases?limit=50":
				return []byte(listBody), nil
			default:
				return nil, errors.New("unexpected release request")
			}
		}
	}
	list := func(tags ...string) string {
		entries := make([]string, len(tags))
		for i, tag := range tags {
			entries[i] = fmt.Sprintf(`{"tag_name":%q,"prerelease":true}`, tag)
		}
		return "[" + strings.Join(entries, ",") + "]"
	}
	f := isolatedManager(t)
	m := f.manager
	m.checkedAt = time.Time{}
	m.ops.fetch = sources(fmt.Sprintf(`{"tag_name":%q}`, stable), list(beta))
	if s, err := m.Check(context.Background(), true); err != nil || s.Latest != beta {
		t.Fatal("beta channel missed a newer numbered beta", s, err)
	}
	m.checkedAt = time.Time{}
	m.ops.fetch = sources(fmt.Sprintf(`{"tag_name":%q}`, stable), list())
	if s, err := m.Check(context.Background(), true); err != nil || s.Latest != stable {
		t.Fatal("beta channel without beta releases must fall back to stable", s, err)
	}
	m.checkedAt = time.Time{}
	m.ops.fetch = sources(fmt.Sprintf(`{"tag_name":%q}`, stable), list("v0.0.1-beta.1"))
	if s, err := m.Check(context.Background(), true); err != nil || s.Latest != stable {
		t.Fatal("older beta must not replace the stable release", s, err)
	}
	sameBase := "v" + strings.TrimPrefix(stable, "v") + "-beta.1"
	m.checkedAt = time.Time{}
	m.ops.fetch = sources(fmt.Sprintf(`{"tag_name":%q}`, stable), list(sameBase))
	if s, err := m.Check(context.Background(), true); err != nil || s.Latest != stable {
		t.Fatal("equal base versions must prefer the stable release", s, err)
	}
	m.checkedAt = time.Time{}
	m.ops.fetch = sources(fmt.Sprintf(`{"tag_name":%q}`, stable), list("v1.0.0-beta"))
	if s, err := m.Check(context.Background(), true); err != nil || s.Latest != stable {
		t.Fatal("unnumbered beta tag in the release list must be ignored", s, err)
	}
}

func TestCheckCacheSeparatesChannelsAndInstallRejectsChangedChannel(t *testing.T) {
	m := isolatedManager(t).manager
	m.checkedAt = time.Time{}
	stable := managerTarget()
	parts := mustVersion(stable)
	beta := fmt.Sprintf("v%d.%d.%d-beta.2", parts[0], parts[1], parts[2]+1)
	calls := 0
	m.ops.fetch = func(_ context.Context, address string, _ int64) ([]byte, error) {
		calls++
		if strings.Contains(address, "?limit=") {
			return []byte(fmt.Sprintf(`[{"tag_name":%q,"prerelease":true}]`, beta)), nil
		}
		return []byte(fmt.Sprintf(`{"tag_name":%q}`, stable)), nil
	}
	if status, err := m.Check(context.Background(), true); err != nil || status.Latest != beta {
		t.Fatal(status, err)
	}
	if _, err := m.StartForChannel(beta, false); err == nil {
		t.Fatal("accepted the cached beta after disabling beta checks")
	}
	if status, err := m.Check(context.Background(), false); err != nil || status.Latest != stable || calls != 3 {
		t.Fatal("stable channel reused a beta result", status, err, calls)
	}
	if _, err := m.Check(context.Background(), false); err != nil || calls != 3 {
		t.Fatal("same channel no longer reuses its short cache", err, calls)
	}
	if status, err := m.Check(context.Background(), true); err != nil || status.Latest != beta || calls != 5 {
		t.Fatal("beta channel reused a stable result", status, err, calls)
	}
}

func TestFailedCheckIsVisibleInStatusAndClearsOnSuccess(t *testing.T) {
	m := isolatedManager(t).manager
	m.checkedAt = time.Time{}
	m.ops.fetch = func(context.Context, string, int64) ([]byte, error) { return nil, errors.New("offline") }
	if status, err := m.Check(context.Background(), false); err == nil || status.CheckError != "offline" || m.Status().CheckError != "offline" {
		t.Fatal(status, err)
	}
	m.ops.fetch = func(context.Context, string, int64) ([]byte, error) {
		return []byte(fmt.Sprintf(`{"tag_name":%q}`, managerTarget())), nil
	}
	if status, err := m.Check(context.Background(), false); err != nil || status.CheckError != "" {
		t.Fatal(status, err)
	}
}

func TestInstallableRelease(t *testing.T) {
	for _, valid := range []release{
		{Tag: "v1.0.1"},
		{Tag: "v1.0.1-beta.3", Prerelease: true},
	} {
		if !installableRelease(valid, valid.Tag) {
			t.Fatal("rejected installable release", valid)
		}
	}
	for _, invalid := range []struct {
		r      release
		target string
	}{
		{release{Tag: "v1.0.1", Draft: true}, "v1.0.1"},
		{release{Tag: "v1.0.2"}, "v1.0.1"},
		{release{Tag: "v1.0.1-beta.3"}, "v1.0.1-beta.3"},
		{release{Tag: "v1.0.1", Prerelease: true}, "v1.0.1"},
		{release{Tag: "v1.0.1-beta"}, "v1.0.1-beta"},
	} {
		if installableRelease(invalid.r, invalid.target) {
			t.Fatal("accepted uninstallable release metadata", invalid)
		}
	}
}

func mustVersion(tag string) []int {
	parts, err := versionParts(tag)
	if err != nil {
		panic(err)
	}
	return parts
}
func TestManagerStartReturnsBeforePreparationAndRecordsFailure(t *testing.T) {
	f := isolatedManager(t)
	m := f.manager
	entered, finish := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseWorker := func() { once.Do(func() { close(finish) }) }
	defer releaseWorker()
	m.ops.prepare = func(ctx context.Context, r release, before func() error, progress func(DownloadProgress) error) error {
		close(entered)
		<-finish
		return errors.New("download verification failed")
	}
	started := time.Now()
	s, err := m.Start(managerTarget())
	if err != nil || s.Phase != "downloading" || time.Since(started) > time.Second {
		t.Fatal(s, err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("preparation never started")
	}
	if _, err := m.Start(managerTarget()); err == nil {
		t.Fatal("accepted concurrent update")
	}
	// Release the worker without racing deferred cleanup.
	releaseWorker()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s = m.Status()
		if s.Phase == "failed" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if s.Phase != "failed" || s.Error != "download verification failed" {
		t.Fatal(s)
	}
}
func TestManagerRejectsUnsupportedStaleOrBusyUpdates(t *testing.T) {
	for _, tc := range []string{"unsupported", "stale", "wrong target", "worker running", "recent state", "write failed"} {
		t.Run(tc, func(t *testing.T) {
			f := isolatedManager(t)
			m := f.manager
			called := false
			m.ops.prepare = func(context.Context, release, func() error, func(DownloadProgress) error) error {
				called = true
				return nil
			}
			version := managerTarget()
			switch tc {
			case "unsupported":
				m.supported = false
				m.reason = "not installed"
			case "stale":
				m.checkedAt = time.Now().Add(-11 * time.Minute)
			case "wrong target":
				version = "v1.0.1"
			case "worker running":
				m.ops.pid = func() int { return 123 }
			case "recent state":
				f.state = Status{Phase: "restarting", UpdatedAt: time.Now()}
			case "write failed":
				m.ops.write = func(Status) error { return errors.New("disk full") }
			}
			if _, err := m.Start(version); err == nil || called {
				t.Fatal("invalid start was accepted", err)
			}
		})
	}
	f := isolatedManager(t)
	f.state = Status{Phase: "restarting", UpdatedAt: time.Now().Add(-11 * time.Minute)}
	if s := f.manager.Status(); s.Phase != "failed" || !strings.Contains(s.Error, "interrupted") {
		t.Fatal(s)
	}
}

func FuzzVersionParser(f *testing.F) {
	for _, s := range []string{"v1.0.0", "01.0.0", "1.2.3-beta", "", "0.0.0-beta.1", "v1.0.1-beta.7", "1.0.1-beta.01"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		parts, err := versionParts(s)
		if err == nil {
			canonical := fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2])
			// Numbered Beta tags are supported too; reconstruct their canonical
			// suffix independently rather than treating every accepted tag as stable.
			if _, suffix, beta := strings.Cut(strings.TrimPrefix(s, "v"), "-beta."); beta {
				number, suffixErr := strconv.Atoi(suffix)
				if suffixErr != nil || number <= 0 {
					t.Fatalf("invalid beta suffix accepted %q", s)
				}
				canonical += fmt.Sprintf("-beta.%d", number)
			}
			if strings.TrimPrefix(s, "v") != canonical {
				t.Fatal("noncanonical version accepted", s)
			}
		}
	})
}
