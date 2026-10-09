package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"time"
)

type General struct {
	Name      string `json:"name"`
	WebListen string `json:"web_listen"`
	Mode      string `json:"mode"`
	Theme     string `json:"theme"`
}
type DNS struct {
	Listeners           []string                    `json:"listeners"`
	Transparent         []string                    `json:"transparent_listeners"`
	InterceptionEnabled bool                        `json:"interception_enabled"`
	IngressInterfaces   []string                    `json:"ingress_interfaces"`
	EgressInterface     string                      `json:"egress_interface"`
	Source4             string                      `json:"source4"`
	Source6             string                      `json:"source6"`
	Gateway4            string                      `json:"gateway4"`
	Gateway6            string                      `json:"gateway6"`
	BypassVerified      bool                        `json:"bypass_verified"`
	PreserveOriginal    bool                        `json:"preserve_original"`
	Upstreams           []resolver.ResolverEndpoint `json:"upstreams"`
	PolicyUpstreams     []PolicyUpstream            `json:"policy_upstreams"`
	TimeoutMS           int                         `json:"timeout_ms"`
	SourcePortFirst     uint16                      `json:"source_port_first"`
	SourcePortLast      uint16                      `json:"source_port_last"`
}
type PolicyUpstream struct {
	Field             string                      `json:"field"`
	Policy            string                      `json:"policy"`
	Enabled           bool                        `json:"enabled"`
	Upstreams         []resolver.ResolverEndpoint `json:"upstreams"`
	Fallback          string                      `json:"fallback,omitempty"`
	FallbackUpstreams []resolver.ResolverEndpoint `json:"fallback_upstreams,omitempty"`
}
type Surge struct {
	TrustDirect     bool     `json:"trust_direct"`
	UserName        string   `json:"user_name"`
	CLIPath         string   `json:"cli_path"`
	APIURL          string   `json:"api_url"`
	APIToken        string   `json:"api_token,omitempty"`
	TokenConfigured bool     `json:"token_configured"`
	DNS             string   `json:"dns"`
	FakePrefixes    []string `json:"fake_prefixes"`
	NextHop4        string   `json:"next_hop4"`
	NextHop6        string   `json:"next_hop6"`
}
type BGP struct {
	Enabled          bool   `json:"enabled"`
	LocalASN         uint32 `json:"local_asn"`
	RouterID         string `json:"router_id"`
	ListenPort       int32  `json:"listen_port"`
	RouteLimit       int    `json:"route_limit"`
	HoldSeconds      uint64 `json:"hold_seconds"`
	KeepaliveSeconds uint64 `json:"keepalive_seconds"`
}
type Cache struct {
	ExpireDecisions                   bool `json:"expire_decisions"`
	InvalidateOnReadFailure           bool `json:"invalidate_on_read_failure"`
	InvalidateDirectOnSelectionChange bool `json:"invalidate_direct_on_selection_change"`
	InvalidateRejectOnSelectionChange bool `json:"invalidate_reject_on_selection_change"`
	Capacity                          int  `json:"capacity"`
	ResponseCapacity                  int  `json:"response_capacity"`
	Workers                           int  `json:"workers"`
	Queue                             int  `json:"queue"`
	TimeoutMS                         int  `json:"timeout_ms"`
	DirectTTLSeconds                  int  `json:"direct_ttl_seconds"`
	SurgeTTLSeconds                   int  `json:"surge_ttl_seconds"`
	UnknownTTLSeconds                 int  `json:"unknown_ttl_seconds"`
	ErrorTTLSeconds                   int  `json:"error_ttl_seconds"`
}
type Health struct {
	IntervalMS       int    `json:"interval_ms"`
	Failures         int    `json:"failures"`
	Successes        int    `json:"successes"`
	GraceSeconds     int    `json:"grace_seconds"`
	StabilizeSeconds int    `json:"stabilize_seconds"`
	ProbeDomain      string `json:"probe_domain"`
}
type Logging struct {
	Level           string `json:"level"`
	QueryHistory    bool   `json:"query_history"`
	HistoryCapacity int    `json:"history_capacity"`
	LogDomains      bool   `json:"log_domains"`
	EventRetention  int    `json:"event_retention"`
}
type Notifications struct {
	Enabled           bool     `json:"enabled"`
	Errors            []string `json:"errors"`
	Surge             bool     `json:"surge"`
	BGP               bool     `json:"bgp"`
	DNS               bool     `json:"dns"`
	Controller        bool     `json:"controller"`
	Recovery          bool     `json:"recovery"`
	CooldownSeconds   int      `json:"cooldown_seconds"`
	WebhookURL        string   `json:"webhook_url,omitempty"`
	WebhookConfigured bool     `json:"webhook_configured"`
	ClearWebhook      bool     `json:"clear_webhook,omitempty"`
}
type Security struct {
	Authentication     bool     `json:"authentication"`
	Username           string   `json:"username"`
	PasswordHash       string   `json:"password_hash,omitempty"`
	Password           string   `json:"password,omitempty"`
	PasswordConfigured bool     `json:"password_configured"`
	SessionMinutes     int      `json:"session_minutes"`
	AllowedNetworks    []string `json:"allowed_networks"`
	TLSCert            string   `json:"tls_cert"`
	TLSKey             string   `json:"tls_key"`
}
type Backend struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// BackendURL accepts explicit controller origins, without embedded credentials.
func BackendURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || len(raw) > 512 {
		return nil, errors.New("backend URL must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if addr, e := netip.ParseAddr(u.Hostname()); e == nil && (addr.IsUnspecified() || addr.IsMulticast()) {
		return nil, errors.New("backend IP must be unicast and concrete")
	}
	if _, e := netip.ParseAddr(u.Hostname()); e != nil && u.Hostname() != "localhost" {
		return nil, errors.New("backend host must be a concrete IP or localhost")
	}
	if port := u.Port(); port != "" {
		if n, e := strconv.Atoi(port); e != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid backend port")
		}
	}
	u.Path = ""
	return u, nil
}

type Settings struct {
	RemoteBackends []Backend     `json:"remote_backends"`
	Routing        Routing       `json:"routing"`
	General        General       `json:"general"`
	DNS            DNS           `json:"dns"`
	Surge          Surge         `json:"surge"`
	BGP            BGP           `json:"bgp"`
	Cache          Cache         `json:"cache"`
	Health         Health        `json:"health"`
	Logging        Logging       `json:"logging"`
	Notifications  Notifications `json:"notifications"`
	Security       Security      `json:"security"`
}
type Peer struct {
	ID               string `json:"id"`
	Address          string `json:"address"`
	ASN              uint32 `json:"asn"`
	HoldSeconds      uint64 `json:"hold_seconds"`
	KeepaliveSeconds uint64 `json:"keepalive_seconds"`
	Port             uint32 `json:"port"`
	Enabled          bool   `json:"enabled"`
	Description      string `json:"description"`
}
type Route struct {
	ID              string   `json:"id"`
	Prefix          string   `json:"prefix"`
	NextHop         string   `json:"next_hop"`
	MED             uint32   `json:"med"`
	LocalPreference uint32   `json:"local_preference"`
	Communities     []string `json:"communities"`
	Enabled         bool     `json:"enabled"`
	Description     string   `json:"description"`
	Binding         string   `json:"health_binding"`
	SourceID        string   `json:"source_id,omitempty"`
}
type RouteFeed struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	IntervalMinutes int       `json:"interval_minutes"`
	SyncIPv4        bool      `json:"sync_ipv4"`
	SyncIPv6        bool      `json:"sync_ipv6"`
	NextHop4        string    `json:"next_hop4"`
	NextHop6        string    `json:"next_hop6"`
	MED             uint32    `json:"med"`
	LocalPreference uint32    `json:"local_preference"`
	Communities     []string  `json:"communities"`
	Binding         string    `json:"health_binding"`
	Enabled         bool      `json:"enabled"`
	RouteEnabled    bool      `json:"route_enabled"`
	Description     string    `json:"description"`
	LastAttempt     time.Time `json:"last_attempt,omitempty"`
	LastUpdated     time.Time `json:"last_updated,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
}
type Interception struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Prefix  string `json:"prefix"`
	Enabled bool   `json:"enabled"`
}
type Override struct {
	ID          string         `json:"id"`
	Domain      string         `json:"domain"`
	Match       string         `json:"match"`
	Decision    model.Decision `json:"decision"`
	Enabled     bool           `json:"enabled"`
	Description string         `json:"description"`
}
type Document struct {
	Version      int            `json:"version"`
	Settings     Settings       `json:"settings"`
	Peers        []Peer         `json:"peers"`
	Routes       []Route        `json:"routes"`
	RouteFeeds   []RouteFeed    `json:"route_feeds"`
	Interception []Interception `json:"interception"`
	Overrides    []Override     `json:"overrides"`
	Hosts        []Host         `json:"hosts"`
}

type Host struct {
	ID        string   `json:"id"`
	Domain    string   `json:"domain"`
	Match     string   `json:"match"`
	Addresses []string `json:"addresses"`
	TTL       uint32   `json:"ttl"`
	Enabled   bool     `json:"enabled"`
	Source    string   `json:"source"`
	Order     int      `json:"order,omitempty"`
}

func HostPattern(domain, match string) (string, error) {
	if domain == "" || domain == "." {
		return "", errors.New("local DNS name cannot be empty or root")
	}
	if match != "exact" && match != "wildcard" {
		return "", errors.New("local DNS match must be exact or wildcard")
	}
	if match == "exact" && strings.ContainsAny(domain, "*?") {
		return "", errors.New("exact local DNS name cannot contain wildcards")
	}
	if _, e := model.NormalizeDomain(strings.NewReplacer("*", "a", "?", "a").Replace(domain)); e != nil {
		return "", e
	}
	p := strings.ToLower(strings.TrimSuffix(domain, ".")) + "."
	if _, e := path.Match(p, "example.com."); e != nil {
		return "", errors.New("invalid local DNS pattern")
	}
	return p, nil
}

// Clone prevents an unvalidated management edit from mutating live slices.
func (d Document) Clone() Document {
	c := d
	if d.Settings.Notifications.Errors != nil {
		c.Settings.Notifications.Errors = append([]string{}, d.Settings.Notifications.Errors...)
	}
	c.Settings.RemoteBackends = append([]Backend{}, d.Settings.RemoteBackends...)
	c.Settings.DNS.Listeners = append([]string{}, d.Settings.DNS.Listeners...)
	c.Settings.DNS.Transparent = append([]string{}, d.Settings.DNS.Transparent...)
	c.Settings.DNS.IngressInterfaces = append([]string{}, d.Settings.DNS.IngressInterfaces...)
	c.Settings.DNS.Upstreams = append([]resolver.ResolverEndpoint{}, d.Settings.DNS.Upstreams...)
	c.Settings.DNS.PolicyUpstreams = append([]PolicyUpstream{}, d.Settings.DNS.PolicyUpstreams...)
	for i := range c.Settings.DNS.PolicyUpstreams {
		c.Settings.DNS.PolicyUpstreams[i].Upstreams = append([]resolver.ResolverEndpoint{}, d.Settings.DNS.PolicyUpstreams[i].Upstreams...)
		c.Settings.DNS.PolicyUpstreams[i].FallbackUpstreams = append([]resolver.ResolverEndpoint{}, d.Settings.DNS.PolicyUpstreams[i].FallbackUpstreams...)
	}
	c.Settings.Surge.FakePrefixes = append([]string{}, d.Settings.Surge.FakePrefixes...)
	c.Settings.Security.AllowedNetworks = append([]string{}, d.Settings.Security.AllowedNetworks...)
	c.Peers = append([]Peer{}, d.Peers...)
	c.Routes = append([]Route{}, d.Routes...)
	for i := range c.Routes {
		c.Routes[i].Communities = append([]string{}, d.Routes[i].Communities...)
	}
	c.RouteFeeds = append([]RouteFeed{}, d.RouteFeeds...)
	for i := range c.RouteFeeds {
		c.RouteFeeds[i].Communities = append([]string{}, d.RouteFeeds[i].Communities...)
	}
	c.Interception = append([]Interception{}, d.Interception...)
	c.Overrides = append([]Override{}, d.Overrides...)
	c.Hosts = append([]Host{}, d.Hosts...)
	for i := range c.Hosts {
		c.Hosts[i].Addresses = append([]string{}, d.Hosts[i].Addresses...)
	}
	return c
}

func Default() Document {
	return Document{Version: 1, Settings: Settings{
		Routing: Routing{Mode: "bgp", FakeIPBackend: "bgp", DNSBackend: "bgp"},
		General: General{"Surge Controller", "127.0.0.1:1055", "auto", "system"},
		DNS:     DNS{Listeners: []string{"127.0.0.1:1053"}, PreserveOriginal: true, TimeoutMS: 2000, SourcePortFirst: 20000, SourcePortLast: 20999, Upstreams: []resolver.ResolverEndpoint{{Name: "Cloudflare", Protocol: "udp", Address: "1.1.1.1:53", Enabled: true}}},
		Surge:   Surge{CLIPath: "/Applications/Surge.app/Contents/Applications/surge-cli", DNS: "198.18.0.2:53", FakePrefixes: []string{"198.18.0.0/15"}},
		BGP:     BGP{LocalASN: 64512, ListenPort: 179, RouteLimit: 128, HoldSeconds: 30, KeepaliveSeconds: 10},
		Cache:   Cache{Capacity: 10000, ResponseCapacity: 10000, Workers: 8, Queue: 256, TimeoutMS: 500, DirectTTLSeconds: 300, SurgeTTLSeconds: 300, UnknownTTLSeconds: 10, ErrorTTLSeconds: 1, InvalidateDirectOnSelectionChange: true, InvalidateRejectOnSelectionChange: true},
		Health:  Health{1000, 3, 3, 60, 3, "example.com"}, Logging: Logging{"info", true, 1000, true, 2000},
		Notifications: Notifications{Surge: true, BGP: true, DNS: true, Controller: true, CooldownSeconds: 300},
		Security:      Security{Username: "admin", SessionMinutes: 60, AllowedNetworks: []string{"127.0.0.0/8", "::1/128"}},
	}, Peers: []Peer{}, Routes: []Route{}, RouteFeeds: []RouteFeed{}, Interception: []Interception{}, Overrides: []Override{}}
}

func Prefix(s string) (netip.Prefix, error) {
	p, e := netip.ParsePrefix(s)
	if e != nil || p != p.Masked() || p.Bits() == 0 || p.Addr().IsMulticast() || p.Addr().IsLoopback() {
		return p, errors.New("prefix must be a canonical non-default unicast network")
	}
	return p, nil
}
func addr(s string, family int) error {
	if s == "" {
		return nil
	}
	a, e := netip.ParseAddr(s)
	if e != nil || a.IsUnspecified() || a.IsMulticast() || a.Is4() != (family == 4) {
		return errors.New("invalid source/next-hop address")
	}
	return nil
}
func Community(s string) (uint32, error) {
	var a, b uint32
	var tail string
	n, e := fmt.Sscanf(s, "%d:%d%s", &a, &b, &tail)
	if e != nil && n != 2 || n != 2 || a > 65535 || b > 65535 {
		return 0, errors.New("community must be ASN:value with 16-bit components")
	}
	if s != fmt.Sprintf("%d:%d", a, b) {
		return 0, errors.New("non-canonical community")
	}
	return a<<16 | b, nil
}
func (d Document) Validate() error {
	s := d.Settings
	if !validNotificationErrors(s.Notifications.Errors) {
		return errors.New("notification errors must be unique supported event IDs")
	}
	if s.Notifications.CooldownSeconds < 0 || s.Notifications.CooldownSeconds > 86400 {
		return errors.New("notification cooldown must be between 0 and 86400 seconds")
	}
	if raw := s.Notifications.WebhookURL; raw != "" {
		u, err := url.Parse(raw)
		if err != nil || len(raw) > 4096 || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" || u.Opaque != "" {
			return errors.New("notification webhook must be an HTTP(S) URL without user info or fragment")
		}
	}
	if len(s.RemoteBackends) > 32 {
		return errors.New("maximum 32 remote backends")
	}
	backendIDs := map[string]bool{}
	for _, b := range s.RemoteBackends {
		if b.ID == "" || len(b.ID) > 64 || strings.IndexFunc(b.ID, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) >= 0 || backendIDs[b.ID] || len(strings.TrimSpace(b.Name)) == 0 || len(b.Name) > 100 {
			return errors.New("invalid or duplicate backend")
		}
		if _, e := BackendURL(b.URL); e != nil {
			return e
		}
		backendIDs[b.ID] = true
	}
	routing := s.Routing.Effective()
	if e := routing.Validate(); e != nil {
		return e
	}
	if s.DNS.InterceptionEnabled && routing.DNSBackend == "static" && !routing.StaticDNSInterception {
		return errors.New("persistent static DNS interception requires explicit opt-in")
	}
	if d.Version != 1 || len(s.General.Name) == 0 || len(s.General.Name) > 100 {
		return errors.New("invalid configuration version or controller name")
	}
	if s.General.Mode != "auto" && s.General.Mode != "force-direct" && s.General.Mode != "force-surge" {
		return errors.New("invalid mode")
	}
	web, e := netip.ParseAddrPort(s.General.WebListen)
	if e != nil || web.Port() == 0 || web.Addr().IsUnspecified() {
		return errors.New("management listen must be a concrete IP:port")
	}
	if !web.Addr().IsLoopback() && (!s.Security.Authentication || s.Security.PasswordHash == "") {
		return errors.New("remote management requires authentication")
	}
	if s.Security.Authentication && (s.Security.Username == "" || s.Security.PasswordHash == "") {
		return errors.New("authentication requires a username and password")
	}
	if s.Security.SessionMinutes < 1 || s.Security.SessionMinutes > 1440 || len(s.Security.AllowedNetworks) == 0 {
		return errors.New("invalid management session or allowed networks")
	}
	for _, p := range s.Security.AllowedNetworks {
		if _, e := netip.ParsePrefix(p); e != nil {
			return errors.New("invalid management allowed network")
		}
	}
	if !filepath.IsAbs(s.Surge.CLIPath) || s.DNS.TimeoutMS < 100 || s.DNS.TimeoutMS > 10000 {
		return errors.New("absolute CLI path and bounded DNS timeout required")
	}
	if s.Surge.APIURL != "" {
		u, e := url.Parse(s.Surge.APIURL)
		if e != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname()))) {
			return errors.New("Surge API URL requires HTTPS or local HTTP")
		}
	}
	for _, p := range []string{s.Security.TLSCert, s.Security.TLSKey} {
		if p != "" && !filepath.IsAbs(p) {
			return errors.New("TLS paths must be absolute")
		}
	}
	if (s.Security.TLSCert == "") != (s.Security.TLSKey == "") {
		return errors.New("TLS certificate and key must both be configured or both be empty")
	}
	if ap, e := netip.ParseAddrPort(s.Surge.DNS); e != nil || ap.Port() == 0 {
		return errors.New("invalid Surge DNS endpoint")
	}
	if len(s.DNS.Listeners) < 1 || len(s.DNS.Listeners) > 8 || len(s.DNS.Transparent) > 2 {
		return errors.New("invalid DNS listener count")
	}
	seenBinds := map[string]bool{}
	for _, b := range append(append([]string{}, s.DNS.Listeners...), s.DNS.Transparent...) {
		a, e := netip.ParseAddrPort(b)
		if e != nil || a.Port() == 0 || a.Addr().IsUnspecified() || a.Addr().IsMulticast() || seenBinds[b] {
			return errors.New("invalid or duplicate concrete DNS listener")
		}
		seenBinds[b] = true
	}
	if addr(s.DNS.Source4, 4) != nil || addr(s.DNS.Source6, 6) != nil || addr(s.DNS.Gateway4, 4) != nil || addr(s.DNS.Gateway6, 6) != nil || addr(s.Surge.NextHop4, 4) != nil || addr(s.Surge.NextHop6, 6) != nil {
		return errors.New("invalid egress or Surge next-hop address")
	}
	if s.DNS.SourcePortFirst < 1024 || s.DNS.SourcePortLast < s.DNS.SourcePortFirst || s.DNS.SourcePortLast >= 49152 || int(s.DNS.SourcePortLast)-int(s.DNS.SourcePortFirst) > 4095 {
		return errors.New("invalid reserved source port pool")
	}
	if len(s.DNS.Upstreams) < 1 || len(s.DNS.Upstreams) > 16 {
		return errors.New("invalid upstream pool size")
	}
	for _, u := range s.DNS.Upstreams {
		if e := u.Validate(); e != nil {
			return e
		}
	}
	if len(s.DNS.PolicyUpstreams) > 128 {
		return errors.New("at most 128 policy DNS mappings are allowed")
	}
	policyKeys := map[string]bool{}
	for _, rule := range s.DNS.PolicyUpstreams {
		key := rule.Field + "\x00" + rule.Policy
		if rule.Field != "policy" && rule.Field != "final_policy" || rule.Policy == "" || strings.TrimSpace(rule.Policy) != rule.Policy || len(rule.Policy) > 256 || strings.ContainsAny(rule.Policy, "\r\n\x00") || policyKeys[key] {
			return errors.New("policy DNS mappings require a unique field and exact policy name")
		}
		policyKeys[key] = true
		if len(rule.Upstreams) < 1 || len(rule.Upstreams) > 16 {
			return errors.New("policy DNS mapping requires 1 to 16 upstreams")
		}
		enabled := false
		for _, upstream := range rule.Upstreams {
			if err := upstream.Validate(); err != nil {
				return err
			}
			enabled = enabled || upstream.Enabled
		}
		if rule.Enabled && !enabled {
			return errors.New("enabled policy DNS mapping requires an enabled upstream")
		}
		switch rule.Fallback {
		case "", "none", "real", "surge", "upstreams":
		default:
			return errors.New("invalid policy DNS fallback")
		}
		if len(rule.FallbackUpstreams) > 16 || rule.Fallback == "upstreams" && len(rule.FallbackUpstreams) == 0 {
			return errors.New("policy DNS fallback requires 1 to 16 upstreams")
		}
		fallbackEnabled := false
		for _, upstream := range rule.FallbackUpstreams {
			if err := upstream.Validate(); err != nil {
				return err
			}
			fallbackEnabled = fallbackEnabled || upstream.Enabled
		}
		if rule.Enabled && rule.Fallback == "upstreams" && !fallbackEnabled {
			return errors.New("enabled policy DNS fallback requires an enabled upstream")
		}
	}
	if s.Cache.Capacity < 1 || s.Cache.Capacity > 1000000 || s.Cache.ResponseCapacity < 1 || s.Cache.ResponseCapacity > 1000000 || s.Cache.Workers < 1 || s.Cache.Workers > 256 || s.Cache.Queue < 1 || s.Cache.Queue > 65536 || s.Cache.TimeoutMS < 10 || s.Cache.TimeoutMS > 5000 {
		return errors.New("invalid cache limits")
	}
	for _, v := range []int{s.Cache.DirectTTLSeconds, s.Cache.SurgeTTLSeconds, s.Cache.UnknownTTLSeconds, s.Cache.ErrorTTLSeconds} {
		if v < 0 || v > 86400 {
			return errors.New("invalid decision TTL")
		}
	}
	if s.Health.IntervalMS < 100 || s.Health.IntervalMS > 60000 || s.Health.Failures < 1 || s.Health.Failures > 100 || s.Health.Successes < 1 || s.Health.Successes > 100 || s.Health.GraceSeconds < 0 || s.Health.GraceSeconds > 86400 || s.Health.StabilizeSeconds < 1 || s.Health.StabilizeSeconds > 3600 {
		return errors.New("invalid health settings")
	}
	if _, e := model.NormalizeDomain(s.Health.ProbeDomain); e != nil {
		return e
	}
	if s.Logging.HistoryCapacity < 1 || s.Logging.HistoryCapacity > 100000 || s.Logging.EventRetention < 100 || s.Logging.EventRetention > 100000 {
		return errors.New("invalid privacy retention")
	}
	if routing.UsesBGP() && (s.BGP.LocalASN == 0 || s.BGP.RouteLimit < 1 || s.BGP.RouteLimit > 10000 || s.BGP.ListenPort < 0 || s.BGP.ListenPort > 65535 || s.BGP.HoldSeconds < 3 || s.BGP.KeepaliveSeconds == 0 || s.BGP.KeepaliveSeconds*3 > s.BGP.HoldSeconds) {
		return errors.New("invalid BGP settings")
	}
	if routing.UsesBGP() && s.BGP.Enabled {
		a, e := netip.ParseAddr(s.BGP.RouterID)
		if e != nil || !a.Is4() || a.IsUnspecified() || a.IsMulticast() {
			return errors.New("BGP router ID must be IPv4 when BGP is enabled")
		}
	}
	ids := map[string]bool{}
	checkID := func(id string) bool {
		if id == "" || len(id) > 64 || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	networks := map[string]bool{}
	count := 0
	for _, p := range s.Surge.FakePrefixes {
		prefix, e := Prefix(p)
		if e != nil {
			return e
		}
		if (routing.FakeIPBackend == "bgp" && s.BGP.Enabled || routing.FakeIPBackend == "static" && routing.StaticFakeIPConfigured) && (prefix.Addr().Is4() && s.Surge.NextHop4 == "" || prefix.Addr().Is6() && s.Surge.NextHop6 == "") {
			return errors.New("configured Fake-IP routing requires a Surge next hop for every address family")
		}
		if networks[p] {
			return errors.New("duplicate fake prefix")
		}
		networks[p] = true
		if routing.FakeIPBackend == "bgp" {
			count++
		}
	}
	if len(s.Surge.FakePrefixes) == 0 {
		return errors.New("Fake-IP prefix required")
	}
	if len(s.Surge.FakePrefixes) > 16 || len(d.Peers) > 128 || len(d.Overrides) > 1000 || len(d.RouteFeeds) > 128 {
		return errors.New("configuration resource limits exceeded")
	}
	allUpstreams := append([]resolver.ResolverEndpoint{}, s.DNS.Upstreams...)
	for _, rule := range s.DNS.PolicyUpstreams {
		allUpstreams = append(allUpstreams, rule.Upstreams...)
		allUpstreams = append(allUpstreams, rule.FallbackUpstreams...)
	}
	for _, upstream := range allUpstreams {
		endpoint := netip.MustParseAddrPort(upstream.Address).Addr()
		for _, p := range s.Surge.FakePrefixes {
			if netip.MustParsePrefix(p).Contains(endpoint) {
				return errors.New("real upstream cannot use the Surge Fake-IP network")
			}
		}
	}
	if s.Logging.Level != "debug" && s.Logging.Level != "info" && s.Logging.Level != "warn" && s.Logging.Level != "error" {
		return errors.New("invalid logging level")
	}
	if s.General.Theme != "system" && s.General.Theme != "dark" && s.General.Theme != "light" {
		return errors.New("invalid theme")
	}
	for _, r := range d.Interception {
		p, e := Prefix(r.Prefix)
		if e != nil || !checkID(r.ID) || networks[r.Prefix] {
			return errors.New("invalid/duplicate interception route")
		}
		networks[r.Prefix] = true
		if r.Enabled && (s.DNS.InterceptionEnabled || routing.DNSBackend == "static" && routing.StaticDNSConfigured) {
			if p.Addr().Is4() && s.DNS.Source4 == "" || p.Addr().Is6() && s.DNS.Source6 == "" {
				return errors.New("DNS interception routing requires a Controller next hop for each address family")
			}
		}
		for _, f := range s.Surge.FakePrefixes {
			if p.Overlaps(netip.MustParsePrefix(f)) {
				return errors.New("interception overlaps Fake-IP network")
			}
		}
		protected := []string{s.DNS.Source4, s.DNS.Source6, netip.MustParseAddrPort(s.Surge.DNS).Addr().String(), s.Surge.NextHop4, s.Surge.NextHop6}
		for _, bind := range append(append([]string{}, s.DNS.Listeners...), s.DNS.Transparent...) {
			protected = append(protected, netip.MustParseAddrPort(bind).Addr().String())
		}
		for _, v := range protected {
			if a, e := netip.ParseAddr(v); e == nil && p.Contains(a) {
				return errors.New("interception covers an egress, listener or Surge address")
			}
		}
		if r.Enabled && routing.DNSBackend == "bgp" {
			count++
		}
	}
	feedIDs := map[string]bool{}
	for _, f := range d.RouteFeeds {
		u, urlErr := url.Parse(f.URL)
		if !checkID(f.ID) || urlErr != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(f.URL) > 4096 || f.IntervalMinutes < 15 || f.IntervalMinutes > 10080 || !f.SyncIPv4 && !f.SyncIPv6 || (f.Binding != "none" && f.Binding != "surge" && f.Binding != "controller" && f.Binding != "dns") {
			return errors.New("invalid route URL update source")
		}
		if err := addr(f.NextHop4, 4); err != nil {
			return err
		}
		if err := addr(f.NextHop6, 6); err != nil {
			return err
		}
		if f.SyncIPv4 && f.NextHop4 == "" || f.SyncIPv6 && f.NextHop6 == "" {
			return errors.New("automatic route URL updates require a next hop for each selected address family")
		}
		feedIDs[f.ID] = true
		for _, c := range f.Communities {
			if _, err := Community(c); err != nil {
				return err
			}
		}
	}
	for _, r := range d.Routes {
		p, e := Prefix(r.Prefix)
		next, ne := netip.ParseAddr(r.NextHop)
		if e != nil || ne != nil || next.IsUnspecified() || next.IsMulticast() || next.Is4() != p.Addr().Is4() || !checkID(r.ID) || networks[r.Prefix] || r.SourceID != "" && !feedIDs[r.SourceID] {
			return errors.New("invalid/duplicate manual route")
		}
		networks[r.Prefix] = true
		if r.Binding != "none" && r.Binding != "surge" && r.Binding != "controller" && r.Binding != "dns" {
			return errors.New("invalid health binding")
		}
		for _, c := range r.Communities {
			if _, e := Community(c); e != nil {
				return e
			}
		}
		if len(r.Communities) > 64 {
			return errors.New("too many communities")
		}
		if r.Enabled && routing.UsesBGP() {
			count++
		}
	}
	if routing.UsesBGP() && count > s.BGP.RouteLimit {
		return errors.New("hard BGP route limit exceeded")
	}
	peers := map[string]bool{}
	for _, p := range d.Peers {
		a, e := netip.ParseAddr(p.Address)
		if e != nil || a.IsUnspecified() || a.IsMulticast() || p.ASN == 0 || !checkID(p.ID) || peers[p.Address] || p.Port > 65535 {
			return errors.New("invalid/duplicate BGP peer")
		}
		peers[p.Address] = true
		if p.HoldSeconds != 0 && (p.HoldSeconds < 3 || p.KeepaliveSeconds == 0 || p.KeepaliveSeconds*3 > p.HoldSeconds) {
			return errors.New("invalid peer timers")
		}
	}
	for _, o := range d.Overrides {
		if !checkID(o.ID) || (o.Match != "exact" && o.Match != "suffix") || (o.Decision != model.Direct && o.Decision != model.Surge && o.Decision != model.Reject) {
			return errors.New("invalid override")
		}
		if _, e := model.NormalizeDomain(o.Domain); e != nil {
			return e
		}
	}
	if len(d.Hosts) > 1000 {
		return errors.New("local DNS entry limit exceeded")
	}
	hosts := map[string]bool{}
	for _, h := range d.Hosts {
		p, e := HostPattern(h.Domain, h.Match)
		if e != nil || !checkID(h.ID) || h.TTL < 1 || h.TTL > 86400 || len(h.Addresses) < 1 || len(h.Addresses) > 16 || (h.Source != "manual" && h.Source != "surge") || h.Order < 0 || h.Order > 100000 {
			return errors.New("invalid local DNS entry")
		}
		key := h.Source + ":" + p
		if hosts[key] {
			return errors.New("duplicate local DNS entry")
		}
		hosts[key] = true
		for _, v := range h.Addresses {
			a, e := netip.ParseAddr(v)
			if e != nil || a.IsMulticast() || a.Zone() != "" {
				return errors.New("local DNS requires literal IPv4/IPv6 addresses")
			}
			for _, prefix := range s.Surge.FakePrefixes {
				if netip.MustParsePrefix(prefix).Contains(a.Unmap()) {
					return errors.New("local DNS cannot independently issue Surge Fake-IP addresses")
				}
			}
		}
	}
	if s.DNS.InterceptionEnabled && (len(s.DNS.Transparent) == 0 || len(s.DNS.IngressInterfaces) == 0 || !s.DNS.BypassVerified || s.DNS.EgressInterface == "" || s.DNS.Source4 == "" && s.DNS.Source6 == "") {
		return errors.New("transparent ingress requires explicit listeners/interfaces/egress and verified gateway bypass")
	}
	return nil
}
func isLoopback(s string) bool {
	a, e := netip.ParseAddr(s)
	return s == "localhost" || e == nil && a.IsLoopback()
}
func (d Document) Redacted() Document {
	d.Settings.Notifications.WebhookConfigured = d.Settings.Notifications.WebhookURL != ""
	d.Settings.Notifications.WebhookURL = ""
	d.Settings.Notifications.ClearWebhook = false
	d.Settings.Surge.TokenConfigured = d.Settings.Surge.APIToken != ""
	d.Settings.Surge.APIToken = ""
	d.Settings.Security.PasswordConfigured = d.Settings.Security.PasswordHash != ""
	d.Settings.Security.PasswordHash = ""
	d.Settings.Security.Password = ""
	return d
}
