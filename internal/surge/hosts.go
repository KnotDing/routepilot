package surge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"path"
	"strings"
	"surge-controller/internal/config"
)

type HostSkip struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}
type HostImport struct {
	Entries []config.Host `json:"entries"`
	Skipped []HostSkip    `json:"skipped"`
	Total   int           `json:"total"`
}

func ReadHosts(ctx context.Context, runner Runner) (HostImport, error) {
	profile, err := readEffectiveProfile(ctx, runner)
	if err != nil {
		return HostImport{}, err
	}
	return ParseHosts(profile)
}

func readEffectiveProfile(ctx context.Context, runner Runner) (string, error) {
	raw, e := runner.Run(ctx, "dump", "profile", "effective")
	if e != nil {
		return "", e
	}
	var response struct {
		Profile string `json:"profile"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Profile == "" {
		return "", errors.New("effective Surge profile is unavailable")
	}
	if len(response.Profile) > outputLimit {
		return "", errors.New("Surge profile exceeds read limit")
	}
	return response.Profile, nil
}

// Parse only the effective [Host] section. Never expose or persist other
// sections, raw lines, DNS script code or credential-bearing resource URLs.
func ParseHosts(profile string) (HostImport, error) {
	result := HostImport{Entries: []config.Host{}, Skipped: []HostSkip{}}
	if len(profile) > 4<<20 {
		return result, errors.New("Surge profile exceeds read limit")
	}
	section := ""
	blocked := []string{}
	seen := map[string]bool{}
	for index, line := range strings.Split(profile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.TrimSpace(strings.SplitN(line[1:], "]", 2)[0]))
			continue
		}
		if section != "host" || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		result.Total++
		if result.Total > 1000 {
			return result, errors.New("Surge Host section exceeds 1000 entries")
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		for _, comment := range []string{" #", " //", " ;"} {
			value = strings.SplitN(value, comment, 2)[0]
		}
		match := "exact"
		if strings.ContainsAny(key, "*?") {
			match = "wildcard"
		}
		pattern, e := config.HostPattern(key, match)
		reason := ""
		addresses := []string{}
		if !ok || e != nil || strings.Contains(key, ":") {
			reason = "unsupported domain or remote rule/domain set"
		} else {
			for _, v := range strings.Split(value, ",") {
				a, e := netip.ParseAddr(strings.TrimSpace(v))
				if e != nil || a.IsMulticast() || a.Zone() != "" {
					reason = "non-static IP mapping (DNS server, alias or script)"
					break
				}
				addresses = append(addresses, a.Unmap().String())
			}
			if len(addresses) > 16 {
				reason = "too many mapped addresses"
			}
			for _, earlier := range blocked {
				affected, _ := path.Match(earlier, pattern)
				if earlier == "*" || affected || strings.ContainsAny(pattern, "*?") {
					reason = "shadowed by an unsupported earlier mapping"
					break
				}
			}
			if seen[pattern] {
				reason = "duplicate mapping; first entry takes precedence"
			}
		}
		if reason != "" {
			result.Skipped = append(result.Skipped, HostSkip{index + 1, reason})
			if e == nil {
				blocked = append(blocked, pattern)
			} else {
				blocked = append(blocked, "*")
			}
			continue
		}
		seen[pattern] = true
		hash := sha256.Sum256([]byte(pattern))
		result.Entries = append(result.Entries, config.Host{ID: "surge-" + hex.EncodeToString(hash[:16]), Domain: strings.TrimSuffix(strings.ToLower(key), "."), Match: match, Addresses: addresses, TTL: 60, Enabled: true, Source: "surge", Order: index + 1})
	}
	return result, nil
}
