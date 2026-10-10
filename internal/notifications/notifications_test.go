package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"surge-controller/internal/config"
	"surge-controller/internal/state"
)

func notificationSettings() config.Notifications {
	return config.Notifications{Enabled: true, WebhookURL: "http://127.0.0.1/webhook", Surge: true, BGP: true, DNS: true, Controller: true, Recovery: true, CooldownSeconds: 300}
}
func component(name, value string) state.Component { return state.Component{Name: name, State: value} }
func queued(t *testing.T, m *Manager) job {
	t.Helper()
	select {
	case item := <-m.queue:
		return item
	default:
		t.Fatal("notification was not queued")
		return job{}
	}
}
func assertNoQueue(t *testing.T, m *Manager) {
	t.Helper()
	if len(m.queue) != 0 {
		t.Fatal("unexpected queued notification", len(m.queue))
	}
}
func TestSelectionDebounceRecoveryAndCooldown(t *testing.T) {
	m := New(func(string, string, string) {})
	s := notificationSettings()
	s.BGP = false
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN"), component("BGP engine", "DOWN"), component("DNS", "DISABLED")})
	item := queued(t, m)
	if item.payload.Title != "RoutePilot · 服务异常" || strings.Contains(item.payload.Title, "Surge Controller") ||
		item.payload.State != "DOWN" || len(item.payload.Components) != 1 || item.payload.Components[0] != "Surge CLI" ||
		item.payload.At.IsZero() {
		t.Fatal(item)
	}
	encoded, err := json.Marshal(item.payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"backend"`) {
		t.Fatalf("webhook payload contains a backend name field: %s", encoded)
	}
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	assertNoQueue(t, m)
	m.states["Surge CLI"] = componentState{down: true, lastAlert: time.Now().Add(-301 * time.Second)}
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	queued(t, m)
	m.Observe(s, []state.Component{component("Surge CLI", "UP")})
	if queued(t, m).payload.State != "UP" {
		t.Fatal("recovery not sent")
	}
	m.Observe(s, []state.Component{component("Surge CLI", "UP")})
	assertNoQueue(t, m)
	s.CooldownSeconds = 0
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	queued(t, m)
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	assertNoQueue(t, m)
	s.Recovery = false
	m.Observe(s, []state.Component{component("Surge CLI", "UP")})
	assertNoQueue(t, m)
	if m.states["Surge CLI"].down {
		t.Fatal("disabled recovery left alarm stuck")
	}
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	queued(t, m)
}
func TestExplicitSelectionDisabledStateAndQueueSaturation(t *testing.T) {
	m := New(func(string, string, string) {})
	s := notificationSettings()
	s.Errors = []string{}
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	assertNoQueue(t, m)
	s.Errors = nil
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	queued(t, m)
	s.Enabled = false
	m.Observe(s, nil)
	if len(m.states) != 0 || m.activeURL != "" {
		t.Fatal("disabled notifications remained active")
	}
	s.Enabled = true
	s.WebhookURL = ""
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	assertNoQueue(t, m)
	s = notificationSettings()
	for range cap(m.queue) {
		m.queue <- job{}
	}
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	if m.states["Surge CLI"].down {
		t.Fatal("full queue incorrectly acknowledged alarm")
	}
	for len(m.queue) > 0 {
		<-m.queue
	}
	m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
	queued(t, m)
	m.Observe(s, []state.Component{component("Surge CLI", "DISABLED")})
	assertNoQueue(t, m)
	if _, found := m.states["Surge CLI"]; found {
		t.Fatal("disabled component kept alarm")
	}
}
func TestWebhookDeliveryRetriesAndNeverLeaksSecrets(t *testing.T) {
	for _, scenario := range []string{"success", "retry", "client failure", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			var payload Payload
			events := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("wrong webhook request")
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				code := 204
				switch scenario {
				case "retry":
					if calls == 1 {
						code = 503
					}
				case "client failure":
					code = 401
				case "redirect":
					w.Header().Set("Location", "http://example.invalid/secret")
					code = 302
				}
				w.WriteHeader(code)
				_, _ = w.Write([]byte("secret response"))
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m := New(func(_, kind, message string) { events <- kind + ":" + message })
			s := notificationSettings()
			s.WebhookURL = server.URL + "/secret-key"
			m.Observe(s, []state.Component{component("Surge CLI", "DOWN"), component("BGP engine", "DOWN")})
			done := make(chan struct{})
			go func() { defer close(done); m.Run(ctx) }()
			var event string
			select {
			case event = <-events:
			case <-time.After(5 * time.Second):
				t.Fatal("delivery timed out")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("delivery worker did not stop")
			}
			mu.Lock()
			defer mu.Unlock()
			wantCalls := 1
			if scenario == "retry" {
				wantCalls = 2
			}
			if scenario == "redirect" {
				wantCalls = 3
			}
			if calls != wantCalls {
				t.Fatal(calls, wantCalls)
			}
			if payload.Title != "RoutePilot · 服务异常" || strings.Contains(payload.Title, "Surge Controller") {
				t.Fatal("webhook title contains backend name or wrong brand", payload.Title)
			}
			if len(payload.Components) != 2 || payload.Components[0] != "BGP engine" || payload.Body != "BGP engine、Surge CLI" {
				t.Fatal(payload)
			}
			if strings.Contains(event, "secret") || strings.Contains(event, server.URL) {
				t.Fatal("sensitive webhook details leaked", event)
			}
			want := "notification_sent"
			if scenario == "client failure" || scenario == "redirect" {
				want = "notification_failed"
			}
			if !strings.HasPrefix(event, want) {
				t.Fatal(event)
			}
		})
	}
}
func TestQueuedNotificationIsDiscardedAfterSettingsChange(t *testing.T) {
	for _, change := range []string{"disable", "URL changed", "selection changed"} {
		t.Run(change, func(t *testing.T) {
			events := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("obsolete notification delivered") }))
			defer server.Close()
			m := New(func(_, kind, _ string) { events <- kind })
			s := notificationSettings()
			s.WebhookURL = server.URL
			m.Observe(s, []state.Component{component("Surge CLI", "DOWN")})
			switch change {
			case "disable":
				s.Enabled = false
			case "URL changed":
				s.WebhookURL = server.URL + "/new"
			case "selection changed":
				s.Errors = []string{}
			}
			m.Observe(s, nil)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); m.Run(ctx) }()
			deadline := time.Now().Add(time.Second)
			for len(m.queue) > 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			cancel()
			<-done
			select {
			case event := <-events:
				t.Fatal(event)
			default:
			}
		})
	}
}
