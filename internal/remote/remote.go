// Package remote relays controller API calls using separate remote cookie jars.
// Only configured origins are reachable; local browser credentials never leave
// the hosting controller. Connection state is scoped to a local browser session.
package remote

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"surge-controller/internal/config"
	"sync"
	"time"
)

const prefix = "/api/v1/management/relay/"

type connection struct {
	mu     sync.Mutex
	client *http.Client
	csrf   string
	used   time.Time // protected by Manager.mu
}
type Manager struct {
	mu          sync.Mutex
	connections map[string]*connection
	profiles    func() []config.Backend
	transport   *http.Transport
}

func New(profiles func() []config.Backend) *Manager {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 60 * time.Second
	return &Manager{profiles: profiles, transport: transport, connections: map[string]*connection{}}
}
func failure(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Explicit resources also prevent relay recursion and access to unrelated HTTP
// endpoints. SSE remains local; remote dashboards poll their selected backend.
func allowed(path, method string) bool {
	read := map[string]bool{"session": true, "settings": true, "configuration/export": true, "mode": true, "status": true, "health": true, "dns/queries": true, "dns/cache": true, "surge/status": true, "surge/hosts": true, "bgp/status": true, "routing/status": true, "events": true, "updates/status": true, "updates/check": true}
	post := map[string]bool{"login": true, "logout": true, "configuration/import": true, "bgp/export-frr": true, "bgp/routes/import-url": true, "bgp/routes/import": true, "dns/hosts/import-surge": true, "routing/verify": true, "test/domain": true, "dns/cache/flush": true, "resources/bulk-delete": true, "updates/install": true}
	if method == "GET" && read[path] || method == "POST" && post[path] || method == "PUT" && (path == "settings" || path == "mode") {
		return true
	}
	for _, resource := range []string{"bgp/peers", "bgp/routes", "dns/interception", "dns/hosts", "overrides"} {
		if path == resource {
			return method == "GET" || method == "POST"
		}
		if strings.HasPrefix(path, resource+"/") {
			id := strings.TrimPrefix(path, resource+"/")
			return len(id) > 0 && len(id) <= 64 && !strings.ContainsAny(id, "/\\.%?# ") && (method == "PUT" || method == "DELETE")
		}
	}
	return false
}

func (m *Manager) get(key string) *connection {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, c := range m.connections {
		if now.Sub(c.used) > 30*time.Minute {
			delete(m.connections, k)
		}
	}
	if c := m.connections[key]; c != nil {
		c.used = now
		return c
	}
	if len(m.connections) >= 128 {
		return nil
	}
	jar, _ := cookiejar.New(nil)
	c := &connection{used: now, client: &http.Client{Jar: jar, Transport: m.transport, Timeout: 65 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	m.connections[key] = c
	return c
}
func send(r *http.Request, c *connection, target *url.URL, method string, body []byte, csrf string) (*http.Response, error) {
	out, err := http.NewRequestWithContext(r.Context(), method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	out.Header.Set("Accept", "application/json")
	if method != "GET" {
		out.Header.Set("Content-Type", "application/json")
		out.Header.Set("X-CSRF-Token", csrf)
	}
	// Cookies come exclusively from the remote jar. Browser Authorization,
	// Origin, Host and CSRF headers are intentionally never forwarded.
	return c.client.Do(out)
}
func remember(c *connection, body []byte) {
	var session struct {
		CSRF string `json:"csrf"`
	}
	if json.Unmarshal(body, &session) == nil && session.CSRF != "" {
		c.csrf = session.CSRF
	}
}
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-RoutePilot-Backend", "remote")
	cookie, err := r.Cookie("controller_session")
	if err != nil {
		failure(w, 401, "local management session required")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, prefix), "/", 2)
	if len(parts) != 2 || !allowed(parts[1], r.Method) {
		failure(w, 404, "unsupported remote API endpoint")
		return
	}
	var backend *config.Backend
	for _, b := range m.profiles() {
		if b.ID == parts[0] {
			value := b
			backend = &value
			break
		}
	}
	if backend == nil {
		failure(w, 404, "remote backend not found")
		return
	}
	target, err := config.BackendURL(backend.URL)
	if err != nil {
		failure(w, 422, "invalid remote backend origin")
		return
	}
	// Bind credential submissions to the address the client actually selected.
	// A concurrent profile edit must not forward saved passwords to a new host.
	if r.Method == "POST" && parts[1] == "login" {
		if expected := r.Header.Get("X-RoutePilot-Expected-Backend"); expected != "" {
			selected, e := config.BackendURL(expected)
			origin := func(u *url.URL) string {
				port := u.Port()
				if port == "" {
					if u.Scheme == "https" {
						port = "443"
					} else {
						port = "80"
					}
				}
				return u.Scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
			}
			if e != nil || origin(selected) != origin(target) {
				failure(w, 409, "remote backend address changed; reconnect before signing in")
				return
			}
		}
	}

	c := m.get(cookie.Value + "|" + backend.ID + "|" + target.String())
	if c == nil {
		failure(w, 503, "remote connection capacity exhausted")
		return
	}
	if len(r.URL.RawQuery) > 8192 {
		failure(w, 400, "remote query too long")
		return
	}
	target.Path = "/api/v1/" + parts[1]
	target.RawQuery = r.URL.RawQuery
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512*1024))
	if err != nil {
		failure(w, 413, "remote request exceeds 512 KiB")
		return
	}
	// Serialize session establishment and mutations, while allowing concurrent
	// read-only status requests to share the thread-safe cookie jar.
	locked := r.Method != "GET" || parts[1] == "session"
	if locked {
		c.mu.Lock()
		defer c.mu.Unlock()
	}
	if r.Method != "GET" && c.csrf == "" {
		sessionURL := *target
		sessionURL.Path = "/api/v1/session"
		sessionURL.RawQuery = ""
		response, err := send(r, c, &sessionURL, "GET", nil, "")
		if err != nil {
			failure(w, 502, "无法连接远程后端，请检查地址和网络；使用 HTTPS 时请检查 TLS 证书")
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 8193))
		response.Body.Close()
		if readErr != nil || len(data) > 8192 || response.StatusCode != 200 {
			failure(w, 502, "远程后端会话建立失败")
			return
		}
		remember(c, data)
	}
	csrf := ""
	if locked {
		csrf = c.csrf
	}
	response, err := send(r, c, target, r.Method, body, csrf)
	if err != nil {
		failure(w, 502, "无法连接远程后端，请检查地址和网络；使用 HTTPS 时请检查 TLS 证书")
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		failure(w, 502, "远程后端重定向已拒绝，请填写最终管理地址")
		return
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		failure(w, 502, "远程后端响应过大或读取失败")
		return
	}
	if locked && (parts[1] == "session" || parts[1] == "login") && response.StatusCode == 200 {
		remember(c, data)
	}
	if locked && parts[1] == "logout" && response.StatusCode == 200 {
		c.csrf = ""
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(data)
}

func (m *Manager) Close() { m.transport.CloseIdleConnections() }
