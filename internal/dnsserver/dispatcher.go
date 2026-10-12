package dnsserver

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
)

// SafetyGate belongs to the health/BGP state manager. Fake-IP issuance requires
// a current route lease; successful local DNS resolution alone is insufficient.
type SafetySnapshot struct {
	AllowFakeIP    bool
	DirectFallback bool
	Generation     uint64
}
type SafetyGate interface{ Snapshot() SafetySnapshot }
type IssueLease interface {
	Valid() bool
	Release()
}
type FakeIssuer interface{ AcquireFakeIP() (IssueLease, bool) }
type CounterSnapshot struct{ Queries, Direct, Surge, Reject, Unknown, Intercepted, Addressed, Errors uint64 }
type Counters struct{ Queries, Direct, Surge, Reject, Unknown, Intercepted, Addressed, Errors atomic.Uint64 }

func (c *Counters) Snapshot() CounterSnapshot {
	return CounterSnapshot{c.Queries.Load(), c.Direct.Load(), c.Surge.Load(), c.Reject.Load(), c.Unknown.Load(), c.Intercepted.Load(), c.Addressed.Load(), c.Errors.Load()}
}

// Restore is only called before the counters are exposed to query workers.
func (c *Counters) Restore(s CounterSnapshot) {
	c.Queries.Store(s.Queries)
	c.Direct.Store(s.Direct)
	c.Surge.Store(s.Surge)
	c.Reject.Store(s.Reject)
	c.Unknown.Store(s.Unknown)
	c.Intercepted.Store(s.Intercepted)
	c.Addressed.Store(s.Addressed)
	c.Errors.Store(s.Errors)
}

type ClientStats struct {
	ClientIP string    `json:"client_ip"`
	Queries  uint64    `json:"queries"`
	Direct   uint64    `json:"direct"`
	Surge    uint64    `json:"surge"`
	Reject   uint64    `json:"reject"`
	Unknown  uint64    `json:"unknown"`
	Errors   uint64    `json:"errors"`
	LastSeen time.Time `json:"last_seen"`
}

type ClientStatsTotals struct {
	Clients uint64 `json:"clients"`
	Queries uint64 `json:"queries"`
	Direct  uint64 `json:"direct"`
	Surge   uint64 `json:"surge"`
	Reject  uint64 `json:"reject"`
	Unknown uint64 `json:"unknown"`
	Errors  uint64 `json:"errors"`
}

type ClientStatsSnapshot struct {
	Clients        []ClientStats     `json:"clients"`
	Stats          ClientStatsTotals `json:"stats"`
	Capacity       int               `json:"capacity"`
	EvictedClients uint64            `json:"evicted_clients"`
}

type ClientStatistics struct {
	mu       sync.Mutex
	capacity int
	clients  map[string]ClientStats
	stats    ClientStatsTotals
	evicted  uint64
}

func NewClientStatistics(capacity int) *ClientStatistics {
	if capacity < 1 {
		panic("client statistics capacity must be positive")
	}
	return &ClientStatistics{capacity: capacity, clients: make(map[string]ClientStats)}
}

func (s *ClientStatistics) Record(ip netip.Addr, decision model.Decision, failed bool, at time.Time) {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	key := ip.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	client, ok := s.clients[key]
	if !ok {
		if len(s.clients) >= s.capacity {
			var oldest string
			for candidate, entry := range s.clients {
				if oldest == "" || entry.LastSeen.Before(s.clients[oldest].LastSeen) ||
					entry.LastSeen.Equal(s.clients[oldest].LastSeen) && candidate < oldest {
					oldest = candidate
				}
			}
			delete(s.clients, oldest)
			s.evicted++
		}
		client.ClientIP = key
	}
	client.Queries++
	if at.After(client.LastSeen) {
		client.LastSeen = at
	}
	switch decision {
	case model.Direct:
		client.Direct++
	case model.Surge:
		client.Surge++
	case model.Reject:
		client.Reject++
	case model.Unknown:
		client.Unknown++
	}
	if failed {
		client.Errors++
	}
	s.clients[key] = client
	s.stats.Queries++
	switch decision {
	case model.Direct:
		s.stats.Direct++
	case model.Surge:
		s.stats.Surge++
	case model.Reject:
		s.stats.Reject++
	case model.Unknown:
		s.stats.Unknown++
	}
	if failed {
		s.stats.Errors++
	}
}

func (s *ClientStatistics) Snapshot() ClientStatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	clients := make([]ClientStats, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].Queries != clients[j].Queries {
			return clients[i].Queries > clients[j].Queries
		}
		if !clients[i].LastSeen.Equal(clients[j].LastSeen) {
			return clients[i].LastSeen.After(clients[j].LastSeen)
		}
		return clients[i].ClientIP < clients[j].ClientIP
	})
	stats := s.stats
	stats.Clients = uint64(len(s.clients))
	return ClientStatsSnapshot{Clients: clients, Stats: stats, Capacity: s.capacity, EvictedClients: s.evicted}
}

type QueryRecord struct {
	Context          model.DNSQueryContext `json:"context"`
	Result           model.DecisionResult  `json:"result"`
	Latency          time.Duration         `json:"latency"`
	Answers          []string              `json:"answers"`
	Rcode            int                   `json:"rcode"`
	ClassificationMS float64               `json:"classification_ms"`
	ResolverMS       float64               `json:"resolver_ms"`
	Resolver         string                `json:"resolver"`
	ResponseCacheHit bool                  `json:"response_cache_hit"`
	Response         DNSResponse           `json:"response"`
	ResolutionError  string                `json:"resolution_error,omitempty"`
}
type DNSResponse struct {
	ID            uint16   `json:"id"`
	Status        string   `json:"status"`
	Answers       []string `json:"answers"`
	Authority     []string `json:"authority"`
	Additional    []string `json:"additional"`
	Truncated     bool     `json:"truncated"`
	Authoritative bool     `json:"authoritative"`
}

func summarizeResponse(m *dns.Msg) DNSResponse {
	r := DNSResponse{ID: m.Id, Status: dns.RcodeToString[m.Rcode], Truncated: m.Truncated, Authoritative: m.Authoritative, Answers: []string{}, Authority: []string{}, Additional: []string{}}
	for _, section := range []struct {
		records []dns.RR
		output  *[]string
	}{{m.Answer, &r.Answers}, {m.Ns, &r.Authority}, {m.Extra, &r.Additional}} {
		for _, rr := range section.records {
			*section.output = append(*section.output, rr.String())
		}
	}
	return r
}

type resolutionKey struct{}
type resolution struct {
	model.ResolutionTrace
	Destination string
	Duration    time.Duration
	Error       string
}
type History struct {
	mu          sync.Mutex
	records     []QueryRecord
	next, count int
	Enabled     bool
}

func NewHistory(size int, enabled bool) *History {
	if size < 1 {
		panic("history capacity must be positive")
	}
	return &History{records: make([]QueryRecord, size), Enabled: enabled}
}
func (h *History) add(r QueryRecord) {
	if !h.Enabled {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records[h.next] = r
	h.next = (h.next + 1) % len(h.records)
	if h.count < len(h.records) {
		h.count++
	}
}
func (h *History) Records() []QueryRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]QueryRecord, 0, h.count)
	for i := 0; i < h.count; i++ {
		ix := (h.next - 1 - i + len(h.records)) % len(h.records)
		out = append(out, h.records[ix])
	}
	return out
}

type LocalResolver interface {
	ResolveLocal(context.Context, model.DNSQueryContext, *dns.Msg) (*dns.Msg, bool)
}

type Dispatcher struct {
	AAAAFallback     string
	aaaaOverride     atomic.Pointer[string]
	IPv6Status       interface{ IPv6Disabled() bool }
	Local            LocalResolver
	Classifier       model.Classifier
	Real             model.Resolver
	Surge            model.SurgeAdapter
	Gate             SafetyGate
	FakeIssuer       FakeIssuer
	FakePrefixes     []netip.Prefix
	ForbiddenClients []netip.Addr
	ClientPolicies   map[netip.Addr]model.Decision
	Counters         Counters
	SharedCounters   *Counters
	History          *History
	ClientStatistics *ClientStatistics
	Monitoring       *Monitoring
	FakeTTLMax       *uint32
	RealIPDomains    interface{ MatchAlwaysRealIP(string) (string, bool) }
	PolicyDNS        []PolicyDNSBinding
}

// SetAAAAFallback changes only query policy; listeners and route leases remain live.
func (d *Dispatcher) SetAAAAFallback(mode string) {
	d.aaaaOverride.Store(&mode)
}

// Statistics allows successive runtimes to use the same cumulative counters.
// Standalone dispatchers keep their own counters without extra initialization.
func (d *Dispatcher) Statistics() *Counters {
	if d.SharedCounters != nil {
		return d.SharedCounters
	}
	return &d.Counters
}

func failure(q *dns.Msg, code int) *dns.Msg {
	r := new(dns.Msg)
	r.SetRcode(q, code)
	r.RecursionAvailable = true
	return r
}
func (d *Dispatcher) Dispatch(ctx context.Context, q model.DNSQueryContext, m *dns.Msg) (reply *dns.Msg) {
	counters := d.Statistics()
	started := time.Now()
	trace := new(resolution)
	ctx = context.WithValue(ctx, resolutionKey{}, trace)
	classificationDuration := time.Duration(0)
	counters.Queries.Add(1)
	if q.Intercepted {
		counters.Intercepted.Add(1)
	} else {
		counters.Addressed.Add(1)
	}
	result := model.DecisionResult{Decision: model.Unknown, Confidence: model.Uncertain, Source: "safety", CreatedAt: started, ClientIP: q.ClientIP}
	if d.Monitoring != nil {
		d.Monitoring.BeginSource(q.ClientIP)
	}
	defer func() {
		if reply == nil {
			reply = failure(m, dns.RcodeServerFailure)
		}
		if reply.Rcode == dns.RcodeServerFailure {
			counters.Errors.Add(1)
		}
		if d.Monitoring != nil {
			destination := trace.Destination
			if destination == "" {
				destination = "unavailable"
			}
			if trace.CacheHit {
				destination = "cache"
				if trace.OptimisticCacheHit {
					destination = "optimistic_cache"
				}
			}
			rcode := dns.RcodeToString[reply.Rcode]
			if rcode == "" {
				rcode = "RCODE" + strconv.Itoa(reply.Rcode)
			}
			d.Monitoring.FinishSource(q.ClientIP, q.Intercepted, result, destination, rcode, time.Since(started))
		}
		if d.ClientStatistics != nil {
			d.ClientStatistics.Record(q.ClientIP, result.Decision, reply.Rcode == dns.RcodeServerFailure, q.ReceivedAt)
		}
		if d.History != nil {
			answers := make([]string, 0, len(reply.Answer))
			for _, rr := range reply.Answer {
				answers = append(answers, rr.String())
			}
			d.History.add(QueryRecord{Context: q, Result: result, Latency: time.Since(started), Answers: answers, Rcode: reply.Rcode, ClassificationMS: float64(classificationDuration) / float64(time.Millisecond), ResolverMS: float64(trace.Duration) / float64(time.Millisecond), Resolver: trace.Endpoint, ResponseCacheHit: trace.CacheHit, Response: summarizeResponse(reply), ResolutionError: trace.Error})
		}
	}()
	if m.Response || m.Opcode != dns.OpcodeQuery || len(m.Question) != 1 || m.Question[0].Qclass != dns.ClassINET {
		return failure(m, dns.RcodeFormatError)
	}
	name, err := model.NormalizeDomain(m.Question[0].Name)
	if err != nil {
		return failure(m, dns.RcodeFormatError)
	}
	q.Question = m.Question[0]
	q.Question.Name = name
	result.Domain = name
	if q.Intercepted {
		if !q.OriginalDestinationIP.IsValid() || q.OriginalDestinationPort != 53 {
			return failure(m, dns.RcodeServerFailure)
		}
		for _, local := range d.ForbiddenClients {
			if q.ClientIP.Unmap() == local.Unmap() {
				result.Reason = "self-originated intercepted query refused to prevent forwarding loop"
				return failure(m, dns.RcodeRefused)
			}
		}
	}
	gate := d.Gate.Snapshot()
	// ANY can return address records and bypass A/AAAA steering. A forwarding
	// policy server may decline it (RFC 8482); do not send it to real upstreams.
	if q.Question.Qtype == dns.TypeANY {
		trace.Destination = "blocked"
		result.Decision = model.Reject
		result.Reason = "ANY is refused to avoid address-policy bypass"
		counters.Reject.Add(1)
		return failure(m, dns.RcodeRefused)
	}
	if decision, ok := d.ClientPolicies[q.ClientIP.Unmap()]; ok {
		return d.resolveClientPolicy(ctx, q, m, &result, decision, gate)
	}
	aaaaFallback := d.AAAAFallback
	if mode := d.aaaaOverride.Load(); mode != nil {
		aaaaFallback = *mode
	}
	if q.Question.Qtype == dns.TypeAAAA && (aaaaFallback == "block" || aaaaFallback == "upstream") && d.IPv6Status != nil && d.IPv6Status.IPv6Disabled() {
		result.Confidence = model.Certain
		result.Source = "surge-ipv6-fallback"
		if aaaaFallback == "block" {
			trace.Destination = "blocked"
			result.Decision = model.Reject
			result.Reason = "Surge IPv6 is disabled; AAAA query blocked with an empty successful response"
			trace.Endpoint = "AAAA fallback: blocked"
			counters.Reject.Add(1)
			r := failure(m, dns.RcodeSuccess)
			if opt := m.IsEdns0(); opt != nil {
				r.SetEdns0(min(opt.UDPSize(), 1232), false)
			}
			return r
		}
		result.Decision = model.Direct
		result.Reason = "Surge IPv6 is disabled; AAAA query sent to configured real DNS upstreams"
		counters.Direct.Add(1)
		return d.realUpstream(ctx, q, m, true)
	}
	if d.Local != nil {
		if r, ok := d.Local.ResolveLocal(ctx, q, m); ok {
			trace.Destination = "local"
			result.Decision = model.Direct
			result.Confidence = model.Certain
			result.Source = "local-dns"
			result.Reason = "explicit local DNS mapping"
			trace.Endpoint = "Local DNS"
			counters.Direct.Add(1)
			return r
		}
	}
	if d.RealIPDomains != nil {
		if pattern, matched := d.RealIPDomains.MatchAlwaysRealIP(name); matched {
			result.Decision = model.Direct
			result.Confidence = model.Certain
			result.Source = "surge-always-real-ip"
			result.MatchedRule = "always-real-ip = " + pattern
			result.Reason = "Matched Surge always-real-ip; querying configured real DNS upstreams without Surge evaluation"
			counters.Direct.Add(1)
			return d.realUpstream(ctx, q, m, true)
		}
	}
	if !model.Eligible(q.Question.Qtype) {
		result.Decision = model.Direct
		result.Source = "dns-type"
		return d.real(ctx, q, m)
	}
	if gate.DirectFallback {
		result.Decision = model.Direct
		result.Source = "state-fallback"
		result.Reason = "系统处于 DIRECT_FALLBACK，未执行 Surge 规则判断；使用真实 DNS 回退"
		counters.Direct.Add(1)
		return d.real(ctx, q, m)
	}
	classificationStarted := time.Now()
	result, err = d.Classifier.Classify(ctx, q)
	classificationDuration = time.Since(classificationStarted)
	if err != nil {
		result.Decision = model.Unknown
		result.Confidence = model.Uncertain
	}
	if result.Decision == model.Direct && (result.Confidence == model.Certain || result.Confidence == model.Configured) {
		counters.Direct.Add(1)
		return d.real(ctx, q, m)
	}
	if result.Decision == model.Reject && result.Confidence == model.Certain {
		trace.Destination = "blocked"
		counters.Reject.Add(1)
		return failure(m, dns.RcodeRefused)
	}
	if err == nil && result.Decision == model.Surge {
		if binding, matched := d.policyDNS(result); matched != nil {
			result.PolicyDNS = matched
			counters.Surge.Add(1)
			return d.resolvePolicyDNS(ctx, q, m, binding, matched, gate)
		}
	}
	if result.Decision == model.Unknown {
		counters.Unknown.Add(1)
	}
	counters.Surge.Add(1)
	return d.resolveSurge(ctx, q, m, gate)
}

// All Surge paths, including a configured policy backup, share issuance leases
// and route/health checks. A real DNS answer never enters this Fake-IP path.
func (d *Dispatcher) resolveSurge(ctx context.Context, q model.DNSQueryContext, m *dns.Msg, gate SafetySnapshot) *dns.Msg {
	trace, _ := ctx.Value(resolutionKey{}).(*resolution)
	if q.Question.Qtype == dns.TypeHTTPS || q.Question.Qtype == dns.TypeSVCB {
		trace.Destination = "blocked"
		trace.Endpoint = "策略空响应：使用 A/AAAA 解析，避免地址提示绕过 Surge"
		// RFC 9460 section 3: an empty successful response signals fallback to
		// A/AAAA. No address hints, alias target, AD bit, or forged DNSSEC proofs.
		r := failure(m, dns.RcodeSuccess)
		if opt := m.IsEdns0(); opt != nil {
			r.SetEdns0(min(opt.UDPSize(), 1232), false)
		}
		return r
	}
	if !gate.AllowFakeIP || d.FakeIssuer == nil || d.Surge == nil {
		trace.Error = "Fake-IP 发行未就绪，未返回地址"
		return failure(m, dns.RcodeServerFailure)
	}
	lease, ok := d.FakeIssuer.AcquireFakeIP()
	if !ok {
		return failure(m, dns.RcodeServerFailure)
	}
	defer lease.Release()
	fakeStarted := time.Now()
	trace.Destination = "surge"
	r, err := d.Surge.ResolveFakeIP(ctx, m)
	trace.Duration = time.Since(fakeStarted)
	trace.Endpoint = "Surge Fake-IP DNS"
	if err != nil || r == nil || ctx.Err() != nil {
		trace.Error = "Surge DNS 请求失败或超时"
		return failure(m, dns.RcodeServerFailure)
	}
	// Readiness can disappear while Surge DNS is in flight. Never expose that
	// result; graceful withdrawal must also wait for issuance leases.
	if !lease.Valid() {
		trace.Error = "处理期间 Fake-IP 发行资格失效"
		return failure(m, dns.RcodeServerFailure)
	}
	current := d.Gate.Snapshot()
	if !current.AllowFakeIP || current.Generation != gate.Generation {
		trace.Error = "处理期间路由或健康状态发生变化"
		return failure(m, dns.RcodeServerFailure)
	}
	if !fakeAddressesValid(r, d.FakePrefixes) {
		trace.Error = "Surge DNS 返回了 Fake-IP 网段之外的地址，未作为 Fake-IP 发布"
		return failure(m, dns.RcodeServerFailure)
	}
	if d.FakeTTLMax != nil {
		r = r.Copy()
		for _, rr := range r.Answer {
			if rr.Header().Ttl > *d.FakeTTLMax {
				rr.Header().Ttl = *d.FakeTTLMax
			}
		}
	}
	r.AuthenticatedData = false
	if observer, ok := lease.(interface{ Observe(*dns.Msg) }); ok {
		observer.Observe(r)
	}
	return r
}
func (d *Dispatcher) real(ctx context.Context, q model.DNSQueryContext, m *dns.Msg) *dns.Msg {
	return d.realUpstream(ctx, q, m, false)
}

func (d *Dispatcher) realUpstream(ctx context.Context, q model.DNSQueryContext, m *dns.Msg, configured bool) *dns.Msg {
	return d.resolveReal(ctx, q, m, d.Real, configured)
}

func (d *Dispatcher) resolveReal(ctx context.Context, q model.DNSQueryContext, m *dns.Msg, upstream model.Resolver, configured bool) *dns.Msg {
	trace, _ := ctx.Value(resolutionKey{}).(*resolution)
	started := time.Now()
	var record *model.ResolutionTrace
	if trace != nil {
		record = &trace.ResolutionTrace
		defer func() { trace.Duration = time.Since(started) }()
	}
	if upstream == nil {
		return failure(m, dns.RcodeServerFailure)
	}
	if trace != nil {
		trace.Destination = "real"
	}
	r, err := upstream.Resolve(ctx, model.ResolveRequest{Query: q, Message: m, Trace: record, UseConfiguredUpstreams: configured})
	if err != nil || r == nil {
		if trace != nil {
			trace.Error = "真实 DNS 请求失败或超时，请检查上游连通性及源地址绕行策略"
		}
		return failure(m, dns.RcodeServerFailure)
	}
	// A broken egress bypass can send the "real" upstream request through
	// Surge and return a Fake-IP. Never let that answer evade the route gate.
	if ContainsFakeAddress(r, d.FakePrefixes) {
		if trace != nil {
			trace.Error = "真实 DNS 上游返回 Fake-IP，请检查源地址绕行策略及 Surge DNS 劫持"
		}
		return failure(m, dns.RcodeServerFailure)
	}
	return r
}

// ContainsFakeAddress also checks SVCB/HTTPS hints and additional records.
func ContainsFakeAddress(r *dns.Msg, prefixes []netip.Prefix) bool {
	for _, section := range [][]dns.RR{r.Answer, r.Ns, r.Extra} {
		for _, rr := range section {
			var addresses []net.IP
			var hints []dns.SVCBKeyValue
			switch v := rr.(type) {
			case *dns.A:
				addresses = append(addresses, v.A)
			case *dns.AAAA:
				addresses = append(addresses, v.AAAA)
			case *dns.SVCB:
				hints = v.Value
			case *dns.HTTPS:
				hints = v.Value
			}
			for _, hint := range hints {
				switch v := hint.(type) {
				case *dns.SVCBIPv4Hint:
					addresses = append(addresses, v.Hint...)
				case *dns.SVCBIPv6Hint:
					addresses = append(addresses, v.Hint...)
				}
			}
			for _, address := range addresses {
				ip, _ := netip.AddrFromSlice(address)
				for _, prefix := range prefixes {
					if prefix.Contains(ip.Unmap()) {
						return true
					}
				}
			}
		}
	}
	return false
}
func fakeAddressesValid(m *dns.Msg, prefixes []netip.Prefix) bool {
	for _, section := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range section {
			var a netip.Addr
			switch v := rr.(type) {
			case *dns.A:
				a, _ = netip.AddrFromSlice(v.A)
			case *dns.AAAA:
				a, _ = netip.AddrFromSlice(v.AAAA)
			default:
				continue
			}
			found := false
			for _, p := range prefixes {
				if p.Contains(a.Unmap()) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

var ErrOriginalDestination = errors.New("cannot recover original DNS destination")
