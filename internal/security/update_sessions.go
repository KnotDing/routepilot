package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const updateSessionFile = "/var/db/surge-controller/software-update-sessions.json"

type updateSessions struct {
	Created  time.Time          `json:"created"`
	Settings string             `json:"settings"`
	Sessions map[string]session `json:"sessions"`
}

func (m *Manager) settingsDigest() string {
	b, _ := json.Marshal(m.settings)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
func updateSessionDirectory() error {
	info, err := os.Lstat(filepath.Dir(updateSessionFile))
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if os.Geteuid() != 0 || !ok || stat.Uid != 0 || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Update session handoff requires the private root-owned service directory")
	}
	return nil
}

// SaveUpdateSessions hands off existing authenticated sessions for this one
// restart. It stores no passwords and never extends a session's expiry.
func (m *Manager) SaveUpdateSessions() error {
	if err := updateSessionDirectory(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.settings.Authentication {
		return errors.New("Management authentication is required for backend updates")
	}
	m.updateHandoff = true
	now := time.Now().UTC()
	handoff := updateSessions{Created: now, Settings: m.settingsDigest(), Sessions: map[string]session{}}
	for token, s := range m.sessions {
		if s.Authenticated && s.Expires.After(now) {
			handoff.Sessions[token] = s
		}
	}
	data, err := json.Marshal(handoff)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(updateSessionFile), ".update-sessions-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err == nil {
		err = closed
	}
	if err == nil {
		err = os.Rename(file.Name(), updateSessionFile)
	}
	return err
}
func (m *Manager) RestoreUpdateSessions() error {
	if err := updateSessionDirectory(); err != nil {
		return err
	}
	m.mu.Lock()
	m.updateHandoff = true
	m.mu.Unlock()
	info, err := os.Lstat(updateSessionFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return errors.New("Invalid update session handoff file")
	}
	file, err := os.Open(updateSessionFile)
	if err != nil {
		return err
	}
	defer file.Close()
	// Consume even an expired or incompatible snapshot, preventing later reuse.
	if err = os.Remove(updateSessionFile); err != nil {
		return err
	}
	var handoff updateSessions
	if json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&handoff) != nil {
		return errors.New("Invalid update session handoff")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if !m.settings.Authentication || handoff.Settings != m.settingsDigest() || handoff.Created.After(now) || now.Sub(handoff.Created) > 5*time.Minute {
		return nil
	}
	if len(handoff.Sessions) > 1024 {
		return errors.New("Invalid update session count")
	}
	for token, s := range handoff.Sessions {
		if len(token) == 43 && len(s.CSRF) == 43 && s.Authenticated && s.Expires.After(now) {
			m.sessions[token] = s
		}
	}
	return nil
}

// Caller holds m.mu. Logout or changed credentials must never be resurrected
// by a snapshot captured just before the service restart.
func (m *Manager) discardUpdateSessions() {
	if m.updateHandoff {
		_ = os.Remove(updateSessionFile)
	}
}
