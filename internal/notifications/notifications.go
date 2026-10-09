package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"surge-controller/internal/config"
	"surge-controller/internal/state"
)

type Payload struct {
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Backend    string    `json:"backend"`
	State      string    `json:"state"`
	Components []string  `json:"components"`
	At         time.Time `json:"at"`
}
type job struct {
	url     string
	payload Payload
}
type componentState struct {
	down      bool
	lastAlert time.Time
}
type Manager struct {
	mu        sync.Mutex
	states    map[string]componentState
	queue     chan job
	event     func(string, string, string)
	activeURL string
}

func New(event func(string, string, string)) *Manager {
	return &Manager{states: map[string]componentState{}, queue: make(chan job, 32), event: event}
}

func selected(name string, s config.Notifications) bool {
	switch {
	case strings.HasPrefix(name, "Surge"):
		return s.Surge
	case strings.HasPrefix(name, "BGP") || strings.Contains(name, "routing"):
		return s.BGP
	case strings.Contains(name, "DNS"):
		return s.DNS
	default:
		return s.Controller
	}
}

// Observe consumes already-debounced health states. Delivery never blocks probes.
func (m *Manager) Observe(name string, s config.Notifications, values []state.Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeURL != s.WebhookURL {
		m.states = map[string]componentState{}
	}
	m.activeURL = s.WebhookURL
	if !s.Enabled || s.WebhookURL == "" {
		m.activeURL = ""
		m.states = map[string]componentState{}
		return
	}
	now := time.Now()
	seen := map[string]bool{}
	var down, recovered []string
	for _, c := range values {
		if !selected(c.Name, s) || c.State == "DISABLED" {
			continue
		}
		seen[c.Name] = true
		previous := m.states[c.Name]
		switch c.State {
		case "DOWN":
			if previous.lastAlert.IsZero() || (!previous.down || s.CooldownSeconds > 0) && now.Sub(previous.lastAlert) >= time.Duration(s.CooldownSeconds)*time.Second {
				down = append(down, c.Name)
			}
		case "UP":
			if previous.down && s.Recovery && !previous.lastAlert.IsZero() {
				recovered = append(recovered, c.Name)
			}
		}
	}
	for key := range m.states {
		if !seen[key] {
			delete(m.states, key)
		}
	}
	for _, batch := range []struct {
		state string
		names []string
	}{{"DOWN", down}, {"UP", recovered}} {
		if len(batch.names) == 0 {
			continue
		}
		sort.Strings(batch.names)
		title := "服务异常"
		if batch.state == "UP" {
			title = "服务已恢复"
		}
		payload := Payload{Title: "RoutePilot · " + name + " · " + title, Body: strings.Join(batch.names, "、"), Backend: name, State: batch.state, Components: batch.names, At: now}
		select {
		case m.queue <- job{s.WebhookURL, payload}:
			for _, key := range batch.names {
				m.states[key] = componentState{down: batch.state == "DOWN", lastAlert: now}
			}
		default: // Retry on the next observation when the bounded queue is full.
		}
	}
	// Clear resolved alarms even when recovery notifications are disabled.
	for _, c := range values {
		if c.State == "UP" && (!s.Recovery || !m.states[c.Name].down) {
			previous := m.states[c.Name]
			previous.down = false
			m.states[c.Name] = previous
		}
	}
}

func (m *Manager) Run(ctx context.Context) {
	client := &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-m.queue:
			data, err := json.Marshal(item.payload)
			if err != nil {
				continue
			}
			for attempt := 0; attempt < 3; attempt++ {
				m.mu.Lock()
				active := m.activeURL == item.url
				m.mu.Unlock()
				if !active {
					break
				}
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, item.url, bytes.NewReader(data))
				if err != nil {
					break
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := client.Do(request)
				status := 0
				if response != nil {
					status = response.StatusCode
					_, _ = io.CopyN(io.Discard, response.Body, 4096)
					response.Body.Close()
				}
				if err == nil && status >= 200 && status < 300 {
					m.event("INFO", "notification_sent", "Service notification delivered by webhook")
					break
				}
				if ctx.Err() != nil {
					return
				}
				if attempt == 2 || status >= 400 && status < 500 && status != 429 {
					// URLs and response bodies can contain push keys; never log them.
					m.event("WARNING", "notification_failed", fmt.Sprintf("Service notification webhook failed (HTTP %d)", status))
					break
				}
				timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
	}
}
