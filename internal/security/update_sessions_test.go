package security

import (
	"encoding/json"
	"strings"
	"surge-controller/internal/config"
	"testing"
	"time"
)

func TestUpgradeSessionSnapshotPreservesExpiryAndAuthentication(t *testing.T) {
	now := time.Now()
	settings := config.Default().Settings.Security
	settings.Authentication = true
	m := New(settings)
	valid, expired, anonymous := Token(), Token(), Token()
	deadline := now.Add(10 * time.Minute)
	m.sessions[valid] = session{CSRF: Token(), Authenticated: true, Expires: deadline}
	m.sessions[expired] = session{CSRF: Token(), Authenticated: true, Expires: now.Add(-time.Second)}
	m.sessions[anonymous] = session{CSRF: Token(), Expires: deadline}
	handoff := m.snapshotUpdateSessions(now)
	if len(handoff.Sessions) != 1 || handoff.Sessions[valid].Expires != deadline {
		t.Fatal("snapshot altered authentication or expiry")
	}
	data, err := json.Marshal(handoff)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeUpdateSessions(data)
	if err != nil {
		t.Fatal(err)
	}
	restored := New(settings)
	if err := restored.restoreUpdateSessions(decoded, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(restored.sessions) != 1 || !restored.sessions[valid].Expires.Equal(deadline) || restored.sessions[valid].CSRF != m.sessions[valid].CSRF {
		t.Fatal("handoff failed or extended session")
	}
	restored.Update(settings)
	if len(restored.sessions) != 0 {
		t.Fatal("configuration update resurrected sessions")
	}
}
func TestUpgradeSessionHandoffRejectsStaleIncompatibleOrMalformedState(t *testing.T) {
	now := time.Now()
	settings := config.Default().Settings.Security
	settings.Authentication = true
	m := New(settings)
	token := Token()
	m.sessions[token] = session{CSRF: Token(), Authenticated: true, Expires: now.Add(time.Hour)}
	for _, failure := range []string{"old", "future", "wrong settings", "authentication disabled", "expired", "unauthenticated", "invalid token", "invalid CSRF", "too many"} {
		t.Run(failure, func(t *testing.T) {
			h := m.snapshotUpdateSessions(now)
			next := New(settings)
			value := h.Sessions[token]
			switch failure {
			case "old":
				h.Created = now.Add(-6 * time.Minute)
			case "future":
				h.Created = now.Add(time.Second)
			case "wrong settings":
				h.Settings = "different hash"
			case "authentication disabled":
				next.settings.Authentication = false
			case "expired":
				value.Expires = now
				h.Sessions[token] = value
			case "unauthenticated":
				value.Authenticated = false
				h.Sessions[token] = value
			case "invalid token":
				delete(h.Sessions, token)
				h.Sessions[strings.Repeat("!", 43)] = value
			case "invalid CSRF":
				value.CSRF = strings.Repeat("!", 43)
				h.Sessions[token] = value
			case "too many":
				for range 1025 {
					h.Sessions[Token()] = value
				}
			}
			err := next.restoreUpdateSessions(h, now)
			if len(next.sessions) != 0 || (failure == "too many" && err == nil) {
				t.Fatal("unsafe snapshot restored", failure, err)
			}
		})
	}
	for _, raw := range []string{"{", "{} {}", "null garbage", `{"unknown":true}`, strings.Repeat(" ", (1<<20)+1)} {
		if _, err := decodeUpdateSessions([]byte(raw)); err == nil {
			t.Fatal("malformed handoff accepted")
		}
	}
}
