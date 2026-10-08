package surge

import (
	"context"
	"errors"
	"net/netip"
	"path"
	"strings"
	"time"

	"surge-controller/internal/config"
	"surge-controller/internal/model"
)

// ParseAlwaysRealIP extracts only the effective General host list. Preserve
// exclusions and order: Surge stops on the first matching host-list entry.
func ParseAlwaysRealIP(profile string) ([]string, error) {
	if len(profile) > outputLimit {
		return nil, errors.New("Surge profile exceeds read limit")
	}
	entries := []string{}
	section := ""
	for _, line := range strings.Split(profile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.TrimSpace(strings.SplitN(line[1:], "]", 2)[0]))
			continue
		}
		if section != "general" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "always-real-ip") {
			continue
		}
		for _, comment := range []string{" #", " //", " ;"} {
			value = strings.SplitN(value, comment, 2)[0]
		}
		entries = []string{}
		for _, item := range strings.Split(value, ",") {
			item = strings.ToLower(strings.TrimSpace(item))
			if item == "" {
				continue
			}
			pattern := strings.TrimPrefix(item, "-")
			// DNS has no future connection port. Only a DNS port or all-port
			// suffix is meaningful here; reject other forms instead of widening
			// a potentially excluded entry to a host-only bypass.
			if strings.Contains(pattern, ":") {
				host, port, found := strings.Cut(pattern, ":")
				if !found || port != "0" && port != "53" {
					return nil, errors.New("unsupported port in always-real-ip host list")
				}
				pattern = host
			}
			switch pattern {
			case "<ip-address>", "<ipv4-address>", "<ipv6-address>", "<simple-hostname>":
			default:
				if _, err := config.HostPattern(pattern, "wildcard"); err != nil {
					return nil, errors.New("unsupported always-real-ip host list entry")
				}
				pattern = strings.TrimSuffix(pattern, ".")
			}
			if strings.HasPrefix(item, "-") {
				pattern = "-" + pattern
			}
			entries = append(entries, pattern)
			if len(entries) > 4096 {
				return nil, errors.New("always-real-ip host list exceeds 4096 entries")
			}
		}
	}
	return entries, nil
}

// Refresh owns refreshMu. Read failures retain the last confirmed list, while
// a successful empty list removes old entries. No profile read occurs per DNS
// query, and no other profile sections are exposed or retained.
func (a *Adapter) refreshAlwaysRealIP(ctx context.Context) {
	if time.Since(a.realIPCheckedAt) < 5*time.Second {
		return
	}
	a.realIPCheckedAt = time.Now()
	profile, err := readEffectiveProfile(ctx, a.Runner)
	var entries []string
	if err == nil {
		entries, err = ParseAlwaysRealIP(profile)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.realIPReadError = "Surge always-real-ip configuration could not be read; retaining the last confirmed list"
		return
	}
	a.realIPEntries = entries
	a.realIPObservedAt = time.Now()
	a.realIPReadError = ""
}

func (a *Adapter) MatchAlwaysRealIP(domain string) (string, bool) {
	name, err := model.NormalizeDomain(domain)
	if err != nil {
		return "", false
	}
	name = strings.TrimSuffix(name, ".")
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, entry := range a.realIPEntries {
		pattern := strings.TrimPrefix(entry, "-")
		matched := false
		switch pattern {
		case "<simple-hostname>":
			matched = !strings.Contains(name, ".")
		case "<ip-address>", "<ipv4-address>", "<ipv6-address>":
			ip, e := netip.ParseAddr(name)
			matched = e == nil && (pattern == "<ip-address>" || pattern == "<ipv4-address>" && ip.Is4() || pattern == "<ipv6-address>" && ip.Is6())
		default:
			matched, _ = path.Match(pattern, name)
		}
		if matched {
			return entry, !strings.HasPrefix(entry, "-")
		}
	}
	return "", false
}
