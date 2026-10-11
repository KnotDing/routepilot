// Package controlclient speaks the authenticated RoutePilot management API.
// It does not start services or read the Controller database.
package controlclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"surge-controller/internal/config"
	"time"
)

const responseLimit = 16 << 20

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"error"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

func (e *Error) Error() string { return e.Message }

type Options struct {
	URL, SessionDir, CAFile string
	Timeout, WaitTimeout    time.Duration
}
type Client struct {
	base              *url.URL
	http              *http.Client
	csrf, sessionFile string
	waitTimeout       time.Duration
}
type session struct {
	Origin  string         `json:"origin"`
	Cookies []*http.Cookie `json:"cookies"`
}

func New(o Options) (*Client, error) {
	u, err := url.Parse(o.URL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return nil, errors.New("--url must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if _, err := config.ManagementOrigin(o.URL); err != nil {
		return nil, err
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid management port")
		}
	}
	u.Path = ""
	if o.Timeout <= 0 || o.WaitTimeout <= 0 {
		return nil, errors.New("timeouts must be positive")
	}
	if o.SessionDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		o.SessionDir = filepath.Join(dir, "RoutePilot", "CLI")
	}
	jar, _ := cookiejar.New(nil)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Direct addresses honor the backend network allow-list and never route
	// credentials through an environment-selected HTTP proxy.
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, errors.New("cannot read CA certificate file")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("CA file contains no valid certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	digest := sha256.Sum256([]byte(u.String()))
	c := &Client{base: u, csrf: "", sessionFile: filepath.Join(o.SessionDir, fmt.Sprintf("session-%x.json", digest[:16])), waitTimeout: o.WaitTimeout,
		http: &http.Client{Jar: jar, Transport: transport, Timeout: o.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func private(path string, directory bool) error {
	s, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if s.Mode()&os.ModeSymlink != 0 || s.IsDir() != directory || !directory && !s.Mode().IsRegular() || s.Mode().Perm()&0077 != 0 {
		return errors.New("CLI session paths must be private regular files (0600) and directories (0700), without symlinks")
	}
	return nil
}
func (c *Client) load() error {
	dir := filepath.Dir(c.sessionFile)
	if err := private(dir, true); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := private(c.sessionFile, false); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	f, err := os.Open(c.sessionFile)
	if err != nil {
		return err
	}
	defer f.Close()
	var stored session
	if json.NewDecoder(io.LimitReader(f, 32<<10)).Decode(&stored) != nil || stored.Origin != c.base.String() {
		return errors.New("invalid CLI session file")
	}
	var cookies []*http.Cookie
	for _, cookie := range stored.Cookies {
		if cookie != nil && cookie.Name == "controller_session" && !cookie.Expires.IsZero() && time.Now().Before(cookie.Expires) {
			cookie.Domain, cookie.Path = "", "/"
			cookies = append(cookies, cookie)
		}
	}
	c.http.Jar.SetCookies(c.base, cookies)
	return nil
}
func (c *Client) persist(cookies []*http.Cookie) error {
	// Only cookies from this exact origin can be persisted. No password or
	// native App credentials are ever stored here.
	var saved []*http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "controller_session" && cookie.MaxAge >= 0 && time.Now().Before(cookie.Expires) {
			saved = append(saved, cookie)
		}
	}
	if len(saved) == 0 {
		return c.Forget()
	}
	dir := filepath.Dir(c.sessionFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := private(dir, true); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		err = json.NewEncoder(f).Encode(session{c.base.String(), saved})
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), c.sessionFile)
}
func (c *Client) Forget() error {
	err := os.Remove(c.sessionFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Path accepts a relative API path with a query, never a host or traversal.
func Path(path string) error {
	u, err := url.Parse(path)
	if err != nil || u == nil || u.IsAbs() || u.Host != "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "\\") {
		return errors.New("use a relative API path, for example dns/cache?limit=100")
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("invalid API path segment")
		}
	}
	return nil
}
func (c *Client) Do(ctx context.Context, method, path string, body json.RawMessage, async bool) (json.RawMessage, error) {
	if err := Path(path); err != nil {
		return nil, err
	}
	endpoint := c.base.String() + "/api/v1/" + path
	if path == "metrics" {
		endpoint = c.base.String() + "/metrics"
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", c.base.String())
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != "GET" && method != "HEAD" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if async {
		req.Header.Set("Prefer", "respond-async")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, &Error{Code: "connection", Message: "management request failed: " + err.Error()}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil {
		return nil, &Error{Code: "connection", Message: "cannot read management response"}
	}
	if len(data) > responseLimit {
		return nil, errors.New("management response exceeds 16 MiB")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var api struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &api)
		message := api.Error
		if message == "" {
			message = strings.TrimSpace(string(data))
			if len(message) > 1024 {
				message = http.StatusText(res.StatusCode)
			}
		}
		code := "http"
		if res.StatusCode == 401 {
			code = "authentication"
		}
		if res.StatusCode == 403 {
			code = "forbidden"
		}
		if res.StatusCode == 409 {
			code = "conflict"
		}
		return nil, &Error{code, message, res.StatusCode}
	}
	if path != "metrics" && !json.Valid(data) {
		return nil, errors.New("backend returned invalid JSON")
	}
	if cookies := res.Cookies(); len(cookies) > 0 {
		if err := c.persist(cookies); err != nil {
			return nil, fmt.Errorf("request succeeded, but session could not be saved: %w", err)
		}
	}
	return data, nil
}
func (c *Client) Session(ctx context.Context) (json.RawMessage, bool, error) {
	data, err := c.Do(ctx, "GET", "session", nil, false)
	if err != nil {
		return nil, false, err
	}
	var s struct {
		CSRF          string `json:"csrf"`
		Authenticated bool   `json:"authenticated"`
		Required      bool   `json:"authentication_required"`
		IPWhitelisted bool   `json:"ip_whitelisted"`
		ClientIP      string `json:"client_ip"`
	}
	if err := json.Unmarshal(data, &s); err != nil || s.CSRF == "" {
		return nil, false, errors.New("invalid management session")
	}
	c.csrf = s.CSRF
	// Never write CSRF tokens or cookies to terminal/agent output.
	value, _ := json.Marshal(map[string]any{"origin": c.base.String(), "authenticated": s.Authenticated, "authentication_required": s.Required, "ip_whitelisted": s.IPWhitelisted, "client_ip": s.ClientIP})
	return value, s.Authenticated, nil
}
func (c *Client) Login(ctx context.Context, username, password string) (json.RawMessage, error) {
	if _, _, err := c.Session(ctx); err != nil {
		return nil, err
	}
	data, _ := json.Marshal(map[string]string{"username": username, "password": password})
	if _, err := c.Do(ctx, "POST", "login", data, false); err != nil {
		return nil, err
	}
	value, authenticated, err := c.Session(ctx)
	if err == nil && !authenticated {
		return nil, errors.New("login did not establish an authenticated session")
	}
	return value, err
}
func (c *Client) Authenticate(ctx context.Context) error {
	_, ok, err := c.Session(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return &Error{Code: "authentication", Message: "login required; use login --username NAME --password-stdin"}
	}
	return nil
}
func (c *Client) Request(ctx context.Context, method, path string, body json.RawMessage, wait bool) (json.RawMessage, error) {
	deferred := map[string]bool{"bgp/routes/import-url": true, "dns/hosts/import-surge": true, "routing/verify": true, "test/domain": true, "dns/cache/flush": true, "updates/check": true}
	value, err := c.Do(ctx, method, path, body, deferred[path])
	if err != nil || !wait {
		return value, err
	}
	var envelope struct {
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(value, &envelope); err != nil || envelope.Operation.ID == "" {
		return value, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, c.waitTimeout)
	defer cancel()
	for {
		value, err = c.Do(waitCtx, "GET", "operations/status?id="+url.QueryEscape(envelope.Operation.ID), nil, false)
		if err != nil {
			if waitCtx.Err() != nil {
				return nil, &Error{Code: "operation_pending", Message: "background operation may still be running; use operation status " + envelope.Operation.ID + " before resubmitting"}
			}
			return nil, err
		}
		var job struct {
			ID, State, Error string
			Result           json.RawMessage
		}
		if json.Unmarshal(value, &job) != nil || job.ID != envelope.Operation.ID {
			return nil, errors.New("background operation status does not match submitted job")
		}
		switch job.State {
		case "succeeded":
			if len(job.Result) == 0 {
				return json.RawMessage("null"), nil
			}
			return job.Result, nil
		case "failed":
			return nil, &Error{Code: "operation_failed", Message: job.Error}
		case "running":
		default:
			return nil, errors.New("unknown background operation state")
		}
		if err := pause(waitCtx); err != nil {
			return nil, &Error{Code: "operation_pending", Message: "background operation may still be running; use operation status " + envelope.Operation.ID + " before resubmitting"}
		}
	}
}
func pause(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (c *Client) WaitConfiguration(ctx context.Context) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.waitTimeout)
	defer cancel()
	for {
		value, err := c.Do(ctx, "GET", "configuration/status", nil, false)
		if err != nil {
			if ctx.Err() != nil {
				return nil, &Error{Code: "apply_pending", Message: "configuration may still be applying; check config status before saving again"}
			}
			return nil, err
		}
		var state struct {
			State           string `json:"state"`
			Error           string `json:"error"`
			Revision        uint64 `json:"revision"`
			AppliedRevision uint64 `json:"applied_revision"`
		}
		if json.Unmarshal(value, &state) != nil {
			return nil, errors.New("invalid configuration state")
		}
		switch state.State {
		case "succeeded", "idle":
			if state.AppliedRevision < state.Revision {
				return nil, errors.New("configuration revision is not fully applied")
			}
			return value, nil
		case "":
			if state.Revision == 0 {
				return value, nil
			}
			return nil, errors.New("configuration state is missing")
		case "failed":
			return nil, &Error{Code: "apply_failed", Message: state.Error}
		case "pending", "applying":
		default:
			return nil, errors.New("unknown configuration state")
		}
		if pause(ctx) != nil {
			return nil, &Error{Code: "apply_pending", Message: "configuration saved but still applying; check config status before saving again"}
		}
	}
}
