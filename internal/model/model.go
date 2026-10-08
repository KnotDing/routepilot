package model

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type DNSProtocol string

const (
	UDP DNSProtocol = "udp"
	TCP DNSProtocol = "tcp"
)

type DNSQueryContext struct {
	ClientIP                netip.Addr   `json:"client_ip"`
	ClientPort              uint16       `json:"client_port"`
	OriginalDestinationIP   netip.Addr   `json:"original_destination_ip"`
	OriginalDestinationPort uint16       `json:"original_destination_port"`
	Transport               DNSProtocol  `json:"transport"`
	Question                dns.Question `json:"question"`
	Intercepted             bool         `json:"intercepted"`
	ReceivedAt              time.Time    `json:"received_at"`
}

type Decision string

const (
	Direct  Decision = "DIRECT"
	Surge   Decision = "SURGE"
	Reject  Decision = "REJECT"
	Unknown Decision = "UNKNOWN"
)

type Confidence string

const (
	Certain    Confidence = "certain"
	Uncertain  Confidence = "uncertain"
	Configured Confidence = "configured"
)

type DecisionResult struct {
	Persistent  bool            `json:"persistent,omitempty"`
	QueryType   uint16          `json:"query_type,omitempty"`
	Decision    Decision        `json:"decision"`
	Confidence  Confidence      `json:"confidence"`
	Domain      string          `json:"domain"`
	ClientIP    netip.Addr      `json:"client_ip"`
	MatchedRule string          `json:"matched_rule,omitempty"`
	Policy      string          `json:"policy,omitempty"`
	FinalPolicy string          `json:"final_policy,omitempty"`
	Source      string          `json:"source"`
	Reason      string          `json:"reason,omitempty"`
	RuleNotes   []string        `json:"rule_notes,omitempty"`
	RuleDNS     *RuleDNSResult  `json:"rule_dns,omitempty"`
	PolicyDNS   *PolicyDNSMatch `json:"policy_dns,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
}

// PolicyDNSMatch records an explicit DNS override without changing Surge's decision.
type PolicyDNSMatch struct {
	Field        string             `json:"field"`
	Policy       string             `json:"policy"`
	Fallback     string             `json:"fallback,omitempty"`
	FallbackUsed bool               `json:"fallback_used,omitempty"`
	Attempts     []PolicyDNSAttempt `json:"attempts,omitempty"`
}

type PolicyDNSAttempt struct {
	Source    string  `json:"source"`
	Resolver  string  `json:"resolver"`
	Rcode     *int    `json:"rcode,omitempty"`
	Error     string  `json:"error,omitempty"`
	LatencyMS float64 `json:"latency_ms"`
	CacheHit  bool    `json:"cache_hit,omitempty"`
}

type RuleMatchRequest struct {
	Domain   string
	ClientIP netip.Addr
}
type RuleMatchResult struct {
	MatchedRule     string         `json:"rule"`
	Policy          string         `json:"rule-policy"`
	FinalPolicy     string         `json:"final"`
	FinalType       string         `json:"final-type"`
	Steps           []PolicyStep   `json:"steps"`
	Notes           []string       `json:"notes"`
	UnderlyingChain []string       `json:"underlying-chain"`
	ContextSafe     bool           `json:"context_safe"`
	SafetyReason    string         `json:"safety_reason"`
	RuleDNS         *RuleDNSResult `json:"rule_dns,omitempty"`
}

// RuleDNSResult describes resolution used by Surge's rule engine, separately
// from the DNS answer that the Controller ultimately sends to the client.
type RuleDNSResult struct {
	State           string `json:"state"`
	LookupPerformed bool   `json:"lookup_performed"`
	ReEvaluated     bool   `json:"re_evaluated"`
	Reason          string `json:"reason,omitempty"`
}
type PolicyStep struct {
	Group    string `json:"group"`
	Type     string `json:"type"`
	Selected string `json:"selected"`
	Reason   string `json:"reason"`
}
type HealthResult struct {
	Healthy   bool          `json:"healthy"`
	CheckedAt time.Time     `json:"checked_at"`
	Latency   time.Duration `json:"latency"`
	Error     string        `json:"error,omitempty"`
}
type ProfileInfo struct {
	Digest      string
	ContextSafe bool
	Reason      string
}
type PolicyState struct{ Digest string }

type SurgeAdapter interface {
	Health(context.Context) HealthResult
	MatchRule(context.Context, RuleMatchRequest) (RuleMatchResult, error)
	ExplainRule(context.Context, RuleMatchRequest) (RuleMatchResult, error)
	ResolveFakeIP(context.Context, *dns.Msg) (*dns.Msg, error)
	CurrentProfile(context.Context) (ProfileInfo, error)
	PolicyState(context.Context) (PolicyState, error)
}
type Classifier interface {
	Classify(context.Context, DNSQueryContext) (DecisionResult, error)
}
type ResolveRequest struct {
	Query                  DNSQueryContext
	Message                *dns.Msg
	Trace                  *ResolutionTrace
	UseConfiguredUpstreams bool
	RetryDNSFailures       bool
}
type ResolutionTrace struct {
	Endpoint string
	CacheHit bool
}
type Resolver interface {
	Resolve(context.Context, ResolveRequest) (*dns.Msg, error)
}

func NormalizeDomain(s string) (string, error) {
	s = strings.ToLower(dns.Fqdn(s))
	if len(s) > 254 {
		return "", errors.New("DNS name is too long")
	}
	if _, ok := dns.IsDomainName(s); !ok {
		return "", errors.New("invalid DNS name")
	}
	// Accept DNS labels used by SRV etc., but prevent CLI options/control characters.
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
			return "", errors.New("DNS name requires ASCII wire labels")
		}
	}
	if strings.HasPrefix(s, "-") {
		return "", errors.New("invalid host name")
	}
	return s, nil
}

func Eligible(t uint16) bool {
	return t == dns.TypeA || t == dns.TypeAAAA || t == dns.TypeHTTPS || t == dns.TypeSVCB
}
