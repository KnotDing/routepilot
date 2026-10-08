// Package mocks contains dependency fakes; production never imports it.
package mocks

import (
	"context"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/miekg/dns"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
)

type Surge struct {
	Match func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error)
	Fake  func(context.Context, *dns.Msg) (*dns.Msg, error)
	Calls atomic.Int64
}

func (s *Surge) ExplainRule(ctx context.Context, r model.RuleMatchRequest) (model.RuleMatchResult, error) {
	s.Calls.Add(1)
	return s.Match(ctx, r)
}
func (s *Surge) MatchRule(ctx context.Context, r model.RuleMatchRequest) (model.RuleMatchResult, error) {
	return s.ExplainRule(ctx, r)
}
func (s *Surge) ResolveFakeIP(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	return s.Fake(ctx, m)
}
func (*Surge) Health(context.Context) model.HealthResult { return model.HealthResult{Healthy: true} }
func (*Surge) CurrentProfile(context.Context) (model.ProfileInfo, error) {
	return model.ProfileInfo{Digest: "mock"}, nil
}
func (*Surge) PolicyState(context.Context) (model.PolicyState, error) {
	return model.PolicyState{Digest: "mock"}, nil
}

type Resolver struct {
	Func  func(context.Context, model.ResolveRequest) (*dns.Msg, error)
	Calls atomic.Int64
}

func (r *Resolver) Resolve(ctx context.Context, q model.ResolveRequest) (*dns.Msg, error) {
	r.Calls.Add(1)
	return r.Func(ctx, q)
}

type Transport struct {
	Func  func(context.Context, resolver.ResolverEndpoint, *dns.Msg) (*dns.Msg, error)
	Calls atomic.Int64
}

func (t *Transport) Exchange(ctx context.Context, e resolver.ResolverEndpoint, m *dns.Msg) (*dns.Msg, error) {
	t.Calls.Add(1)
	return t.Func(ctx, e, m)
}

type Lookup struct {
	Destination netip.AddrPort
	Err         error
	mu          sync.Mutex
	Clients     []netip.AddrPort
	Protocols   []model.DNSProtocol
}

func (l *Lookup) Lookup(_ context.Context, p model.DNSProtocol, c, _ netip.AddrPort) (netip.AddrPort, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Clients = append(l.Clients, c)
	l.Protocols = append(l.Protocols, p)
	return l.Destination, l.Err
}

type Gate struct {
	mu     sync.Mutex
	State  dnsserver.SafetySnapshot
	Leases int
}

func (g *Gate) Snapshot() dnsserver.SafetySnapshot { g.mu.Lock(); defer g.mu.Unlock(); return g.State }
func (g *Gate) Set(s dnsserver.SafetySnapshot)     { g.mu.Lock(); g.State = s; g.mu.Unlock() }

type lease struct {
	gate       *Gate
	generation uint64
}

func (l *lease) Valid() bool {
	s := l.gate.Snapshot()
	return s.AllowFakeIP && s.Generation == l.generation
}
func (l *lease) Release() { l.gate.mu.Lock(); l.gate.Leases--; l.gate.mu.Unlock() }
func (g *Gate) AcquireFakeIP() (dnsserver.IssueLease, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.State.AllowFakeIP {
		return nil, false
	}
	g.Leases++
	return &lease{g, g.State.Generation}, true
}

func AddressReply(q *dns.Msg, ip string) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(q)
	m.RecursionAvailable = true
	address := netip.MustParseAddr(ip)
	if q.Question[0].Qtype == dns.TypeA {
		m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: address.AsSlice()}}
	} else {
		m.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60}, AAAA: address.AsSlice()}}
	}
	return m
}
