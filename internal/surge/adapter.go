package surge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
)

type Snapshot struct {
	IPv6State              string    `json:"ipv6_state"`
	IPv6ObservedAt         time.Time `json:"ipv6_observed_at"`
	IPv6ReadError          string    `json:"ipv6_read_error,omitempty"`
	Digest                 string    `json:"digest"`
	SelectionDigest        string    `json:"selection_digest"`
	Safe                   bool      `json:"context_safe"`
	Reason                 string    `json:"reason"`
	Profile                string    `json:"profile"`
	Mode                   string    `json:"mode"`
	AlwaysRealIPCount      int       `json:"always_real_ip_count"`
	AlwaysRealIP           []string  `json:"always_real_ip"`
	AlwaysRealIPObservedAt time.Time `json:"always_real_ip_observed_at"`
	AlwaysRealIPReadError  string    `json:"always_real_ip_read_error,omitempty"`
}
type Adapter struct {
	Runner            Runner
	FakeTransport     resolver.UpstreamTransport
	FakeEndpoint      resolver.ResolverEndpoint
	mu                sync.RWMutex
	refreshMu         sync.Mutex
	snapshot          Snapshot
	observedAt        time.Time
	MaxSnapshotAge    time.Duration
	resourceDigest    string
	resourceCheckedAt time.Time
	realIPEntries     []string
	realIPCheckedAt   time.Time
	realIPObservedAt  time.Time
	ipv6State         string
	ipv6ObservedAt    time.Time
	ipv6ReadError     string
	profileIdentity   string
	realIPReadError   string
}
type status struct {
	Profile      string          `json:"profile"`
	Mode         string          `json:"mode"`
	StartTime    float64         `json:"start-time"`
	Features     map[string]bool `json:"features"`
	ProfilePath  string          `json:"profile-path"`
	GlobalPolicy string          `json:"global-policy"`
}
type inventory struct {
	Rules []string `json:"rules"`
}

func New(r Runner, t resolver.UpstreamTransport, ep resolver.ResolverEndpoint) (*Adapter, error) {
	if r == nil || t == nil {
		return nil, errors.New("Surge runner and Fake-IP transport required")
	}
	if err := ep.Validate(); err != nil {
		return nil, err
	}
	if ep.Protocol != "udp" && ep.Protocol != "tcp" {
		return nil, errors.New("Surge Fake-IP endpoint requires classic DNS")
	}
	return &Adapter{Runner: r, FakeTransport: t, FakeEndpoint: ep, MaxSnapshotAge: 3 * time.Second}, nil
}

// ContextGuard is an allowlist of rule *input dependencies*, not a matching
// engine. Surge alone evaluates rules. Unexpanded rule sets, logical rules,
// IP rules and all future connection metadata make DIRECT unsafe.
func ContextGuard(rules []string) (bool, string) {
	if len(rules) == 0 {
		return false, "active rule inventory is empty"
	}
	for _, rule := range rules {
		fields := strings.Split(rule, ",")
		kind := strings.ToUpper(strings.TrimSpace(fields[0]))
		switch kind {
		case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-WILDCARD", "SRC-IP":
			if len(fields) != 3 {
				return false, "unsupported rule options"
			}
		case "FINAL":
			if len(fields) != 2 {
				return false, "unsupported FINAL options"
			}
		default:
			return false, "active rules require unavailable or unaudited connection context: " + kind
		}
	}
	return true, "active rule inputs are fully available at DNS time"
}
func (a *Adapter) Refresh(ctx context.Context) (Snapshot, error) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	raw, err := a.Runner.Run(ctx, "status")
	if err != nil {
		a.invalidate()
		return Snapshot{}, err
	}
	var st status
	if err = json.Unmarshal(raw, &st); err != nil {
		a.invalidate()
		return Snapshot{}, err
	}
	if st.Profile == "" || st.Mode == "" {
		a.invalidate()
		return Snapshot{}, errors.New("incomplete Surge status")
	}
	rulesRaw, err := a.Runner.Run(ctx, "dump", "rule")
	if err != nil {
		a.invalidate()
		return Snapshot{}, err
	}
	tempRaw, err := a.Runner.Run(ctx, "dump", "temp-rule")
	if err != nil {
		a.invalidate()
		return Snapshot{}, err
	}
	groups, err := a.Runner.Run(ctx, "policy-group", "list")
	if err != nil {
		a.invalidate()
		return Snapshot{}, err
	}
	var rules, temp inventory
	if json.Unmarshal(rulesRaw, &rules) != nil || json.Unmarshal(tempRaw, &temp) != nil || rules.Rules == nil || temp.Rules == nil {
		a.invalidate()
		return Snapshot{}, errors.New("unrecognized Surge rule inventory schema")
	}
	// Encode canonical objects so key order and status uptime do not invalidate
	// every cache entry. Start time detects Surge restarts.
	var groupObject struct {
		Groups []map[string]json.RawMessage `json:"groups"`
	}
	if err = json.Unmarshal(groups, &groupObject); err != nil || groupObject.Groups == nil {
		a.invalidate()
		return Snapshot{}, errors.New("unrecognized Surge policy groups schema")
	}
	selections := make(map[string]json.RawMessage)
	for _, group := range groupObject.Groups {
		var name string
		_ = json.Unmarshal(group["name"], &name)
		selections[name] = group["selected"]
		delete(group, "selected")
	}
	sort.Slice(groupObject.Groups, func(i, j int) bool {
		return string(groupObject.Groups[i]["name"]) < string(groupObject.Groups[j]["name"])
	})
	selectionBytes, _ := json.Marshal(selections)
	selectionSum := sha256.Sum256(selectionBytes)
	// Update timestamps describe effective resource revisions. Ignore transient
	// updating flags; a running download is not a policy change.
	if time.Since(a.resourceCheckedAt) >= 5*time.Second {
		a.resourceCheckedAt = time.Now()
		if raw, e := a.Runner.Run(ctx, "external-resource", "list"); e == nil {
			var resources struct {
				Defines []struct {
					Key       string  `json:"key"`
					Type      string  `json:"type"`
					Ready     bool    `json:"ready"`
					UpdatedAt float64 `json:"updatedAt"`
				} `json:"defines"`
			}
			if json.Unmarshal(raw, &resources) == nil && resources.Defines != nil {
				sort.Slice(resources.Defines, func(i, j int) bool { return resources.Defines[i].Key < resources.Defines[j].Key })
				canonical, _ := json.Marshal(resources)
				sum := sha256.Sum256(canonical)
				a.resourceDigest = hex.EncodeToString(sum[:])
			}
		}
	}
	identity := st.Profile + "\x00" + st.ProfilePath
	if a.profileIdentity != identity {
		a.realIPCheckedAt = time.Time{}
		a.profileIdentity = identity
	}
	a.refreshAlwaysRealIP(ctx)
	a.mu.RLock()
	realIPEntries := append([]string{}, a.realIPEntries...)
	ipv6State, ipv6Observed, ipv6Error := a.ipv6State, a.ipv6ObservedAt, a.ipv6ReadError
	realIPObserved, realIPError := a.realIPObservedAt, a.realIPReadError
	a.mu.RUnlock()
	canonical, err := json.Marshal(struct {
		Status           status
		Rules, Temporary inventory
		Groups           []map[string]json.RawMessage
		Resources        string
		AlwaysRealIP     []string
		IPv6             string
	}{st, rules, temp, groupObject.Groups, a.resourceDigest, realIPEntries, ipv6State})
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(canonical)
	snap := Snapshot{Digest: hex.EncodeToString(sum[:]), SelectionDigest: hex.EncodeToString(selectionSum[:]), Profile: st.Profile, Mode: st.Mode}
	snap.IPv6State, snap.IPv6ObservedAt, snap.IPv6ReadError = ipv6State, ipv6Observed, ipv6Error
	snap.AlwaysRealIPCount = len(realIPEntries)
	snap.AlwaysRealIP = realIPEntries
	snap.AlwaysRealIPObservedAt = realIPObserved
	snap.AlwaysRealIPReadError = realIPError
	all := append(append([]string(nil), temp.Rules...), rules.Rules...)
	snap.Safe, snap.Reason = ContextGuard(all)
	if st.Mode != "rule" {
		snap.Safe = false
		snap.Reason = "Surge is not in rule mode"
	}
	for _, feature := range []string{"mitm", "rewrite", "scripting"} {
		if st.Features[feature] {
			snap.Safe = false
			snap.Reason = "active Surge " + feature + " may require traffic processing"
		}
	}
	a.mu.Lock()
	a.snapshot = snap
	a.observedAt = time.Now()
	a.mu.Unlock()
	return snap, nil
}
func (a *Adapter) invalidate() {
	a.mu.Lock()
	a.snapshot = Snapshot{IPv6State: "unknown", IPv6ReadError: "Surge IPv6 setting is unavailable"}
	a.ipv6State = "unknown"
	a.ipv6ReadError = "Surge IPv6 setting is unavailable"
	a.realIPCheckedAt = time.Time{}
	a.observedAt = time.Time{}
	a.realIPReadError = "Surge is unavailable; retaining the last confirmed always-real-ip list"
	a.mu.Unlock()
}

func (a *Adapter) Snapshot() (Snapshot, time.Time) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	snapshot := a.snapshot
	snapshot.AlwaysRealIP = append([]string{}, a.realIPEntries...)
	snapshot.AlwaysRealIPCount = len(a.realIPEntries)
	snapshot.AlwaysRealIPObservedAt = a.realIPObservedAt
	snapshot.AlwaysRealIPReadError = a.realIPReadError
	return snapshot, a.observedAt
}
func (a *Adapter) MatchRule(ctx context.Context, r model.RuleMatchRequest) (model.RuleMatchResult, error) {
	return a.ExplainRule(ctx, r)
}
func (a *Adapter) ExplainRule(ctx context.Context, r model.RuleMatchRequest) (model.RuleMatchResult, error) {
	domain, err := model.NormalizeDomain(r.Domain)
	if err != nil {
		return model.RuleMatchResult{}, err
	}
	if domain == "." {
		return model.RuleMatchResult{}, errors.New("root domain is not a connection hostname")
	}
	if !r.ClientIP.IsValid() {
		return model.RuleMatchResult{}, errors.New("client address required for Surge evaluation")
	}
	a.mu.RLock()
	snap, observed := a.snapshot, a.observedAt
	a.mu.RUnlock()
	args := []string{"rule", "explain", strings.TrimSuffix(domain, "."), "dest-port=0", "protocol=", "url=", "user-agent=", "process-path=", "sni=", "http-host=", "source-address=" + r.ClientIP.String(), "source-port=0", "listen-port=0", "is-local=off"}
	m, err := a.explainWithDNS(ctx, strings.TrimSuffix(domain, "."), args)
	if err != nil {
		return m, err
	}
	m.ContextSafe = snap.Safe && snap.Digest != "" && time.Since(observed) <= a.MaxSnapshotAge
	m.SafetyReason = snap.Reason
	if m.FinalPolicy == "DIRECT" && !strings.EqualFold(m.FinalType, "direct") {
		m.ContextSafe = false
		m.SafetyReason = "final policy type is not a confirmed built-in DIRECT"
	}
	a.mu.RLock()
	unchanged := a.snapshot.Digest == snap.Digest && a.snapshot.SelectionDigest == snap.SelectionDigest
	a.mu.RUnlock()
	if !unchanged {
		m.ContextSafe = false
		m.SafetyReason = "Surge changed during evaluation"
	}
	if time.Since(observed) > a.MaxSnapshotAge {
		m.SafetyReason = "Surge input audit is stale"
	}
	// An automatic/random group may choose a different policy for the real
	// connection. Only stable manual selections can establish certain DIRECT.
	for _, step := range m.Steps {
		if step.Type != "select" && step.Type != "Select 策略组" && step.Type != "手动选择" {
			m.ContextSafe = false
			m.SafetyReason = "policy group selection is dynamic or unsupported"
		}
	}
	if len(m.Notes) > 0 {
		for _, note := range m.Notes {
			// A failed first resolver does not make the final successful DNS
			// lookup a connection-context limitation.
			if m.RuleDNS != nil && m.RuleDNS.State == "completed" && inspectRuleDNS([]string{note}).failure != "" {
				continue
			}
			lower := strings.ToLower(note)
			if strings.Contains(lower, "missing") || strings.Contains(lower, "unsupported") || strings.Contains(lower, "failed") {
				m.ContextSafe = false
				m.SafetyReason = "Surge evaluation reported a connection context limitation: " + note
			}
		}
	}
	return m, nil
}
func (a *Adapter) CurrentProfile(ctx context.Context) (model.ProfileInfo, error) {
	s, err := a.Refresh(ctx)
	return model.ProfileInfo{Digest: s.Digest, ContextSafe: s.Safe, Reason: s.Reason}, err
}
func (a *Adapter) PolicyState(context.Context) (model.PolicyState, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.snapshot.Digest == "" {
		return model.PolicyState{}, errors.New("Surge state unavailable")
	}
	return model.PolicyState{Digest: a.snapshot.Digest}, nil
}
func (a *Adapter) ResolveFakeIP(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	return a.FakeTransport.Exchange(ctx, a.FakeEndpoint, q)
}
func (a *Adapter) Health(ctx context.Context) model.HealthResult {
	start := time.Now()
	_, err := a.Runner.Run(ctx, "status")
	h := model.HealthResult{Healthy: err == nil, CheckedAt: time.Now(), Latency: time.Since(start)}
	if err != nil {
		h.Error = "Surge CLI unavailable"
	}
	return h
}
