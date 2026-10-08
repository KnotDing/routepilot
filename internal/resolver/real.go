package resolver

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/cache"
	"surge-controller/internal/model"
)

type response struct {
	Msg      *dns.Msg
	Stored   time.Time
	Endpoint string
}
type EndpointStats struct {
	Endpoint    ResolverEndpoint `json:"endpoint"`
	Healthy     bool             `json:"healthy"`
	Latency     time.Duration    `json:"latency"`
	Queries     uint64           `json:"queries"`
	Failures    uint64           `json:"failures"`
	LastSuccess time.Time        `json:"last_success"`
	LastFailure time.Time        `json:"last_failure"`
}
type Real struct {
	transport        UpstreamTransport
	endpoints        []ResolverEndpoint
	PreserveOriginal bool
	Cache            *cache.Cache[string, response]
	statsMu          sync.Mutex
	stats            []EndpointStats
	next             atomic.Uint64
	now              func() time.Time
}

func NewReal(t UpstreamTransport, endpoints []ResolverEndpoint, preserve bool, capacity int) (*Real, error) {
	if t == nil || capacity < 1 {
		return nil, errors.New("invalid real resolver configuration")
	}
	r := &Real{transport: t, PreserveOriginal: preserve, Cache: cache.New[string, response](capacity), now: time.Now}
	for _, e := range endpoints {
		if err := e.Validate(); err != nil {
			return nil, err
		}
		if e.Enabled {
			r.endpoints = append(r.endpoints, e)
			r.stats = append(r.stats, EndpointStats{Endpoint: e})
		}
	}
	if len(r.endpoints) == 0 {
		return nil, errors.New("at least one enabled resolver is required")
	}
	return r, nil
}
func (r *Real) Resolve(ctx context.Context, req model.ResolveRequest) (*dns.Msg, error) {
	if req.Message == nil || len(req.Message.Question) != 1 {
		return nil, errors.New("exactly one DNS question required")
	}
	q := req.Message
	endpoints := r.endpoints
	original := req.Query.Intercepted && r.PreserveOriginal && !req.UseConfiguredUpstreams
	if original {
		addr := req.Query.OriginalDestinationIP
		if !addr.IsValid() || req.Query.OriginalDestinationPort != 53 {
			return nil, errors.New("missing original destination")
		}
		endpoints = []ResolverEndpoint{{Name: "original", Enabled: true, Protocol: string(req.Query.Transport), Address: netip.AddrPortFrom(addr, 53).String()}}
	}
	// Cache partition includes resolver identity, DO/CD, and full query options
	// (including ECS) to avoid mixing split-horizon or DNSSEC responses.
	key := ""
	cacheable := !q.CheckingDisabled
	if opt := q.IsEdns0(); opt != nil && len(opt.Option) > 0 {
		cacheable = false
	}
	if cacheable {
		kq := q.Copy()
		kq.Id = 0
		kq.Question[0].Name = dns.CanonicalName(kq.Question[0].Name)
		raw, err := kq.Pack()
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, ep := range endpoints {
			fmt.Fprintf(&b, "%s:%s:%s|", ep.Protocol, ep.Address, ep.URL)
		}
		b.Write(raw)
		key = b.String()
		if v, ok := r.Cache.Get(key); ok {
			if req.Trace != nil {
				req.Trace.Endpoint = v.Endpoint
				req.Trace.CacheHit = true
			}
			m := v.Msg.Copy()
			m.Id = q.Id
			m.Question = append([]dns.Question(nil), q.Question...)
			age := uint32(r.now().Sub(v.Stored) / time.Second)
			for _, section := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
				for _, rr := range section {
					if rr.Header().Rrtype == dns.TypeOPT {
						continue
					}
					if rr.Header().Ttl > age {
						rr.Header().Ttl -= age
					} else {
						rr.Header().Ttl = 0
					}
				}
			}
			return m, nil
		}
	}
	gen := r.Cache.Generation()
	start := int(r.next.Add(1) % uint64(len(endpoints)))
	var last error
	for i := 0; i < len(endpoints); i++ {
		ix := (start + i) % len(endpoints)
		ep := endpoints[ix]
		if req.Trace != nil {
			req.Trace.Endpoint = ep.Protocol + "://" + ep.Address
		}
		began := r.now()
		reply, err := r.transport.Exchange(ctx, ep, q)
		if err == nil {
			err = ValidateResponse(q, reply)
		}
		if err == nil && req.RetryDNSFailures {
			err = ResolutionFailure(reply)
		}
		if !original {
			r.statsMu.Lock()
			s := &r.stats[ix]
			s.Queries++
			s.Latency = r.now().Sub(began)
			s.Healthy = err == nil
			if err != nil {
				s.Failures++
				s.LastFailure = r.now()
			} else {
				s.LastSuccess = r.now()
			}
			r.statsMu.Unlock()
		}
		if err != nil {
			last = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if cacheable {
			if ttl := ResponseTTL(reply); ttl > 0 {
				r.Cache.Put(key, response{Msg: reply.Copy(), Stored: r.now(), Endpoint: ep.Protocol + "://" + ep.Address}, gen, ttl)
			}
		}
		if req.Trace != nil {
			req.Trace.Endpoint = ep.Protocol + "://" + ep.Address
		}
		return reply, nil
	}
	return nil, fmt.Errorf("real resolver failed: %w", last)
}

// NXDOMAIN and successful empty answers are completed lookups, not failures.
// Policy pools retry transient DNS errors before selecting their backup.
func ResolutionFailure(reply *dns.Msg) error {
	if reply == nil {
		return errors.New("empty DNS response")
	}
	if reply.Truncated {
		return errors.New("truncated DNS response")
	}
	if reply.Rcode != dns.RcodeSuccess && reply.Rcode != dns.RcodeNameError {
		return fmt.Errorf("upstream DNS returned %s", dns.RcodeToString[reply.Rcode])
	}
	return nil
}
func ResponseTTL(m *dns.Msg) time.Duration {
	if m.Truncated || m.Rcode != dns.RcodeSuccess && m.Rcode != dns.RcodeNameError {
		return 0
	}
	ttl := uint32(86400)
	found := false
	for _, section := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range section {
			if rr.Header().Rrtype == dns.TypeOPT {
				continue
			}
			v := rr.Header().Ttl
			if soa, ok := rr.(*dns.SOA); ok && (len(m.Answer) == 0 || m.Rcode == dns.RcodeNameError) {
				if soa.Minttl < v {
					v = soa.Minttl
				}
			}
			if v < ttl {
				ttl = v
			}
			found = true
		}
	}
	if !found {
		return 0
	}
	if len(m.Answer) == 0 || m.Rcode == dns.RcodeNameError {
		hasSOA := false
		for _, rr := range m.Ns {
			if _, ok := rr.(*dns.SOA); ok {
				hasSOA = true
			}
		}
		if !hasSOA {
			return 0
		}
	}
	return time.Duration(ttl) * time.Second
}
func (r *Real) Stats() []EndpointStats {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	return append([]EndpointStats(nil), r.stats...)
}
