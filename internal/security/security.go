package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"surge-controller/internal/config"
	"sync"
	"time"
)

type session struct {
	CSRF          string
	Expires       time.Time
	Authenticated bool
}
type attempt struct {
	Start time.Time
	Count int
}
type Manager struct {
	mu         sync.Mutex
	settings   config.Security
	sessions   map[string]session
	attempts   map[string]attempt
	loginSlots chan struct{}
}

func New(c config.Security) *Manager {
	return &Manager{settings: c, sessions: map[string]session{}, attempts: map[string]attempt{}, loginSlots: make(chan struct{}, 4)}
}
func (m *Manager) Update(c config.Security) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = c
	m.sessions = map[string]session{}
}
func Hash(password string) (string, error) {
	b, e := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(b), e
}
func Token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func (m *Manager) session(r *http.Request) (session, bool) {
	c, e := r.Cookie("controller_session")
	if e != nil {
		return session{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[c.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(m.sessions, c.Value)
		return session{}, false
	}
	return s, true
}
func (m *Manager) create(w http.ResponseWriter, r *http.Request, authenticated bool) (session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, s := range m.sessions {
		if now.After(s.Expires) {
			delete(m.sessions, k)
		}
	}
	if len(m.sessions) >= 1024 {
		return session{}, false
	}
	id := Token()
	s := session{Token(), now.Add(time.Duration(m.settings.SessionMinutes) * time.Minute), authenticated}
	m.sessions[id] = s
	http.SetCookie(w, &http.Cookie{Name: "controller_session", Value: id, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, Expires: s.Expires})
	return s, true
}
func (m *Manager) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		// A concrete listener must not be exposed through an arbitrary DNS name:
		// rejecting such Host headers prevents browser DNS-rebinding attacks.
		hostName := r.Host
		if h, _, e := net.SplitHostPort(r.Host); e == nil {
			hostName = h
		}
		hostName = strings.Trim(hostName, "[]")
		if _, e := netip.ParseAddr(hostName); e != nil && hostName != "localhost" {
			http.Error(w, "invalid management host", 403)
			return
		}
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			http.Error(w, "forbidden", 403)
			return
		}
		ip, e := netip.ParseAddr(host)
		m.mu.Lock()
		c := m.settings
		m.mu.Unlock()
		allowed := false
		for _, n := range c.AllowedNetworks {
			p, pe := netip.ParsePrefix(n)
			if pe == nil && p.Contains(ip.Unmap()) {
				allowed = true
			}
		}
		if e != nil || !allowed {
			http.Error(w, "forbidden", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if origin != scheme+"://"+r.Host {
				http.Error(w, "invalid origin", 403)
				return
			}
		}
		if r.URL.Path == "/api/v1/session" && r.Method == "GET" {
			s, ok := m.session(r)
			if !ok {
				s, ok = m.create(w, r, false)
			}
			if !ok {
				http.Error(w, "session capacity exhausted", 503)
				return
			}
			write(w, 200, map[string]any{"authenticated": !c.Authentication || s.Authenticated, "authentication_required": c.Authentication, "csrf": s.CSRF})
			return
		}
		if r.URL.Path == "/api/v1/login" && r.Method == "POST" {
			m.login(w, r, c, host)
			return
		}
		s, ok := m.session(r)
		if r.URL.Path == "/api/v1/logout" && r.Method == "POST" {
			if !validCSRF(r, s, ok) {
				http.Error(w, "invalid CSRF token", 403)
				return
			}
			if cookie, e := r.Cookie("controller_session"); e == nil {
				m.mu.Lock()
				delete(m.sessions, cookie.Value)
				m.mu.Unlock()
			}
			http.SetCookie(w, &http.Cookie{Name: "controller_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
			write(w, 200, map[string]bool{"ok": true})
			return
		}
		api := len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" || r.URL.Path == "/metrics"
		if strings.HasPrefix(r.URL.Path, "/api/v1/management/") && !ok {
			http.Error(w, "management session required", 401)
			return
		}
		if api && c.Authentication && (!ok || !s.Authenticated) {
			http.Error(w, "authentication required", 401)
			return
		}
		if api && r.Method != "GET" && r.Method != "HEAD" && !validCSRF(r, s, ok) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func validCSRF(r *http.Request, s session, ok bool) bool {
	return ok && len(r.Header.Get("X-CSRF-Token")) == len(s.CSRF) && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) == 1
}
func (m *Manager) login(w http.ResponseWriter, r *http.Request, c config.Security, ip string) {
	s, ok := m.session(r)
	if !validCSRF(r, s, ok) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	if !c.Authentication {
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	m.mu.Lock()
	now := time.Now()
	for k, a := range m.attempts {
		if now.Sub(a.Start) > time.Minute {
			delete(m.attempts, k)
		}
	}
	a := m.attempts[ip]
	if a.Start.IsZero() {
		a.Start = now
	}
	a.Count++
	capacity := len(m.attempts) < 256 || !m.attempts[ip].Start.IsZero()
	if capacity {
		m.attempts[ip] = a
	}
	m.mu.Unlock()
	if !capacity || a.Count > 5 {
		http.Error(w, "login rate limit", 429)
		return
	}
	select {
	case m.loginSlots <- struct{}{}:
		defer func() { <-m.loginSlots }()
	default:
		http.Error(w, "login capacity exhausted", 429)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil {
		http.Error(w, "invalid login", 400)
		return
	}
	err := bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(body.Password))
	if err != nil || body.Username != c.Username {
		http.Error(w, "invalid credentials", 401)
		return
	}
	if cookie, e := r.Cookie("controller_session"); e == nil {
		m.mu.Lock()
		delete(m.sessions, cookie.Value)
		m.mu.Unlock()
	}
	fresh, ok := m.create(w, r, true)
	if !ok {
		http.Error(w, "session capacity exhausted", 503)
		return
	}
	write(w, 200, map[string]any{"authenticated": true, "csrf": fresh.CSRF})
}
