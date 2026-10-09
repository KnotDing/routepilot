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
type OptimisticOptions struct {
	Context context.Context
	Enabled bool
	Timeout time.Duration
	Workers int
}
type Real struct {
	optimistic       bool
	refreshTimeout   time.Duration
	refreshLimit     int
	ctx              context.Context
	cancel           context.CancelFunc
	refreshMu        sync.Mutex
	refreshing       map[string]bool
	closed           bool
	refreshWorkers   sync.WaitGroup
	transport        UpstreamTransport
	endpoints        []ResolverEndpoint
	PreserveOriginal bool
	Cache            *cache.Cache[string, response]
	statsMu          sync.Mutex
	stats            []EndpointStats
	next             atomic.Uint64
	now              func() time.Time
}

func NewReal(t UpstreamTransport, endpoints []ResolverEndpoint, preserve bool, capacity int, options ...OptimisticOptions) (*Real, error) {
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
	option := OptimisticOptions{Context: context.Background(), Timeout: 2 * time.Second, Workers: 8}
	if len(options) > 0 {
		option = options[0]
	}
	if option.Context == nil {
		option.Context = context.Background()
	}
	if option.Timeout <= 0 {
		option.Timeout = 2 * time.Second
	}
	if option.Workers < 1 {
		option.Workers = 8
	}
	r.ctx, r.cancel = context.WithCancel(option.Context)
	r.optimistic, r.refreshTimeout, r.refreshLimit = option.Enabled, option.Timeout, min(option.Workers, 256)
	r.refreshing = make(map[string]bool)

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
	gen := r.Cache.Generation()
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
		v, ok, stale, generation := r.Cache.GetState(key)
		gen = generation
		if ok && (!stale || r.optimistic) {
			if stale {
				r.refresh(req, endpoints, original, key, gen)
			}
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
	return r.exchange(ctx, req, endpoints, original, key, cacheable, gen)
}

func (r *Real) exchange(ctx context.Context, req model.ResolveRequest, endpoints []ResolverEndpoint, original bool, key string, cacheable bool, gen uint64) (*dns.Msg, error) {
	q := req.Message
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
			} else {
				r.Cache.Delete(key, gen)
			}
		}
		if req.Trace != nil {
			req.Trace.Endpoint = ep.Protocol + "://" + ep.Address
		}
		return reply, nil
	}
	return nil, fmt.Errorf("real resolver failed: %w", last)
}

// Configuration changes can retain valid responses for a background refresh.
// Response TTLs and query/resolver partitions remain unchanged.
func (r *Real) ConfigurationChanged(reason string, optimistic bool) {
	if optimistic {
		r.Cache.MarkStale(func(response) bool { return true })
	} else {
		r.Cache.FlushReason(reason)
	}
}
func (r *Real) Close() {
	r.refreshMu.Lock()
	r.closed = true
	r.cancel()
	r.refreshMu.Unlock()
	r.refreshWorkers.Wait()
}
func (r *Real) refresh(req model.ResolveRequest, endpoints []ResolverEndpoint, original bool, key string, gen uint64) {
	r.refreshMu.Lock()
	if r.closed || r.refreshing[key] || len(r.refreshing) >= r.refreshLimit {
		r.refreshMu.Unlock()
		return
	}
	r.refreshing[key] = true
	r.refreshWorkers.Add(1)
	r.refreshMu.Unlock()
	req.Message = req.Message.Copy()
	req.Trace = nil
	req.RetryDNSFailures = true
	endpoints = append([]ResolverEndpoint(nil), endpoints...)
	go func() {
		defer r.refreshWorkers.Done()
		defer func() { r.refreshMu.Lock(); delete(r.refreshing, key); r.refreshMu.Unlock() }()
		ctx, cancel := context.WithTimeout(r.ctx, r.refreshTimeout)
		_, err := r.exchange(ctx, req, endpoints, original, key, true, gen)
		cancel()
		if err == nil {
			return
		}
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-r.ctx.Done():
		case <-timer.C:
		}
	}()
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
