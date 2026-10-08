package localdns

import (
	"context"
	"net"
	"path"
	"sort"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"sync"

	"github.com/miekg/dns"
)

type Resolver struct {
	mu      sync.RWMutex
	entries []config.Host
}

func New(entries []config.Host) *Resolver { r := new(Resolver); r.Update(entries); return r }
func (r *Resolver) Update(entries []config.Host) {
	copied := config.Document{Hosts: entries}.Clone().Hosts
	sort.SliceStable(copied, func(i, j int) bool {
		a, b := copied[i], copied[j]
		if a.Source != b.Source {
			return a.Source == "manual"
		}
		if a.Source == "surge" {
			return a.Order < b.Order
		}
		if a.Match != b.Match {
			return a.Match == "exact"
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	r.mu.Lock()
	r.entries = copied
	r.mu.Unlock()
}
func (r *Resolver) ResolveLocal(_ context.Context, q model.DNSQueryContext, m *dns.Msg) (*dns.Msg, bool) {
	if !model.Eligible(q.Question.Qtype) {
		return nil, false
	}
	name, e := model.NormalizeDomain(q.Question.Name)
	if e != nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, h := range r.entries {
		if !h.Enabled {
			continue
		}
		pattern, e := config.HostPattern(h.Domain, h.Match)
		if e != nil {
			continue
		}
		matches, _ := path.Match(pattern, name)
		if !matches {
			continue
		}
		reply := new(dns.Msg)
		reply.SetReply(m)
		reply.RecursionAvailable = true
		reply.Authoritative = true
		reply.AuthenticatedData = false
		for _, address := range h.Addresses {
			ip := net.ParseIP(address)
			header := dns.RR_Header{Name: m.Question[0].Name, Class: dns.ClassINET, Ttl: h.TTL, Rrtype: q.Question.Qtype}
			if q.Question.Qtype == dns.TypeA && ip.To4() != nil {
				reply.Answer = append(reply.Answer, &dns.A{Hdr: header, A: ip.To4()})
			}
			if q.Question.Qtype == dns.TypeAAAA && ip.To4() == nil {
				reply.Answer = append(reply.Answer, &dns.AAAA{Hdr: header, AAAA: ip.To16()})
			}
		}
		if opt := m.IsEdns0(); opt != nil {
			reply.SetEdns0(min(opt.UDPSize(), 1232), false)
		}
		return reply, true
	}
	return nil, false
}
