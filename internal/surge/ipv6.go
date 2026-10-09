package surge

import (
	"errors"
	"strings"
	"time"
)

// ParseIPv6 reads the effective General setting, independently of ipv6-vif.
// Surge's documented default for an omitted ipv6 setting is false.
func ParseIPv6(profile string) (bool, error) {
	if len(profile) > outputLimit {
		return false, errors.New("Surge profile exceeds read limit")
	}
	section, value := "", "false"
	for _, line := range strings.Split(profile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.TrimSpace(strings.SplitN(line[1:], "]", 2)[0]))
			continue
		}
		if section != "general" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		key, setting, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "ipv6") {
			continue
		}
		for _, comment := range []string{"#", "//", ";"} {
			setting = strings.SplitN(setting, comment, 2)[0]
		}
		value = strings.ToLower(strings.TrimSpace(setting))
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.New("Surge IPv6 setting could not be read")
	}
}

// Query workers use the observed profile, never invoke the CLI per query.
// Unavailable or stale observations do not activate conditional AAAA handling.
func (a *Adapter) IPv6Disabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.snapshot.Digest != "" && time.Since(a.observedAt) <= a.MaxSnapshotAge &&
		a.ipv6State == "disabled" && time.Since(a.ipv6ObservedAt) <= 5*time.Second+a.MaxSnapshotAge
}
