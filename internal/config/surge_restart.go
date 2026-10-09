package config

import (
	"errors"
	"net/url"
	"strings"
)

// Separate tolerance from display health thresholds. Zero values migrate older
// databases to conservative defaults without enabling automatic restarts.
type SurgeRestart struct {
	ProbeTimeoutMS      int      `json:"probe_timeout_ms"`
	Enabled             bool     `json:"enabled"`
	API                 bool     `json:"api"`
	DNS                 bool     `json:"dns"`
	Groups              []string `json:"groups"`
	Nodes               []string `json:"nodes"`
	ProbeURL            string   `json:"probe_url"`
	Failures            int      `json:"failures"`
	DurationSeconds     int      `json:"duration_seconds"`
	StartupGraceSeconds int      `json:"startup_grace_seconds"`
	CooldownSeconds     int      `json:"cooldown_seconds"`
	MaxPerHour          int      `json:"max_per_hour"`
}

func (s SurgeRestart) Effective() SurgeRestart {
	if s.ProbeURL == "" {
		s.ProbeURL = "https://www.apple.com/library/test/success.html"
	}
	if s.ProbeTimeoutMS == 0 {
		s.ProbeTimeoutMS = 3000
	}
	if s.Failures == 0 {
		s.Failures = 5
	}
	if s.DurationSeconds == 0 {
		s.DurationSeconds = 15
	}
	if s.StartupGraceSeconds == 0 {
		s.StartupGraceSeconds = 60
	}
	if s.CooldownSeconds == 0 {
		s.CooldownSeconds = 300
	}
	if s.MaxPerHour == 0 {
		s.MaxPerHour = 3
	}
	return s
}
func (s SurgeRestart) Validate(apiURL string) error {
	s = s.Effective()
	if s.ProbeTimeoutMS < 500 || s.ProbeTimeoutMS > 15000 || s.Failures < 2 || s.Failures > 100 || s.DurationSeconds < 5 || s.DurationSeconds > 3600 || s.StartupGraceSeconds < 10 || s.StartupGraceSeconds > 3600 || s.CooldownSeconds < 30 || s.CooldownSeconds > 86400 || s.MaxPerHour < 1 || s.MaxPerHour > 10 {
		return errors.New("invalid Surge restart tolerance")
	}
	if s.Enabled && ((!s.API && !s.DNS && len(s.Groups)+len(s.Nodes) == 0) || s.API && apiURL == "") {
		return errors.New("Surge restart requires selected checks and an API URL for API checks")
	}
	if len(s.Groups)+len(s.Nodes) > 16 {
		return errors.New("at most 16 Surge restart targets are allowed")
	}
	for _, targets := range [][]string{s.Groups, s.Nodes} {
		seen := map[string]bool{}
		for _, name := range targets {
			if name == "" || strings.TrimSpace(name) != name || len(name) > 256 || strings.ContainsAny(name, "\x00\r\n") || seen[name] {
				return errors.New("invalid or duplicate Surge restart target")
			}
			seen[name] = true
		}
	}
	u, e := url.Parse(s.ProbeURL)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("Surge restart probe URL must use HTTP(S) without credentials")
	}
	return nil
}
