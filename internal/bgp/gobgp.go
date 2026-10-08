package bgp

import (
	"context"
	"errors"
	"fmt"
	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
	"google.golang.org/protobuf/types/known/anypb"
	"net/netip"
	"surge-controller/internal/config"
	"sync"
	"time"
)

type PeerStatus struct {
	Address       string `json:"address"`
	ASN           uint32 `json:"asn"`
	State         string `json:"state"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	LastError     string `json:"last_error"`
	Prefixes      int    `json:"prefixes"`
}
type Advertisement struct {
	Prefix          string   `json:"prefix"`
	NextHop         string   `json:"next_hop"`
	Kind            string   `json:"kind"`
	Binding         string   `json:"health_binding"`
	MED             uint32   `json:"med"`
	LocalPreference uint32   `json:"local_preference"`
	Communities     []string `json:"communities"`
	Announced       bool     `json:"announced"`
	Installed       bool     `json:"installed"`
}
type Speaker interface {
	Add(context.Context, Advertisement) ([]byte, error)
	Delete(context.Context, []byte) error
	Peers(context.Context) ([]PeerStatus, error)
	Exported(context.Context, string) (map[string]bool, error)
	Close() error
}
type GoBGP struct {
	server    *server.BgpServer
	closeOnce sync.Once
	closeErr  error
}

func New(ctx context.Context, c config.Document) (*GoBGP, error) {
	s := server.NewBgpServer()
	go s.Serve()
	g := &GoBGP{server: s}
	port := c.Settings.BGP.ListenPort
	if port == 0 {
		port = -1
	}
	if e := s.StartBgp(ctx, &api.StartBgpRequest{Global: &api.Global{Asn: c.Settings.BGP.LocalASN, RouterId: c.Settings.BGP.RouterID, ListenPort: port}}); e != nil {
		s.Stop()
		return nil, e
	}
	// Normal peers share the global RIB: per-peer import policies alone apply
	// only to route-server clients. Admit only locally originated paths globally.
	policy := &api.Policy{Name: "controller-local-only", Statements: []*api.Statement{{Name: "local-only", Conditions: &api.Conditions{RouteType: api.Conditions_ROUTE_TYPE_LOCAL}, Actions: &api.Actions{RouteAction: api.RouteAction_ACCEPT}}}}
	if e := s.AddPolicy(ctx, &api.AddPolicyRequest{Policy: policy}); e != nil {
		g.Close()
		return nil, e
	}
	for _, direction := range []api.PolicyDirection{api.PolicyDirection_IMPORT, api.PolicyDirection_EXPORT} {
		if e := s.SetPolicyAssignment(ctx, &api.SetPolicyAssignmentRequest{Assignment: &api.PolicyAssignment{Name: "global", Direction: direction, Policies: []*api.Policy{policy}, DefaultAction: api.RouteAction_REJECT}}); e != nil {
			g.Close()
			return nil, e
		}
	}
	for _, p := range c.Peers {
		if !p.Enabled {
			continue
		}
		hold, keep := p.HoldSeconds, p.KeepaliveSeconds
		if hold == 0 {
			hold, keep = c.Settings.BGP.HoldSeconds, c.Settings.BGP.KeepaliveSeconds
		}
		families := []*api.AfiSafi{}
		for _, afi := range []api.Family_Afi{api.Family_AFI_IP, api.Family_AFI_IP6} {
			family := &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST}
			families = append(families, &api.AfiSafi{Config: &api.AfiSafiConfig{Family: family, Enabled: true}, PrefixLimits: &api.PrefixLimit{Family: family, MaxPrefixes: uint32(c.Settings.BGP.RouteLimit), ShutdownThresholdPct: 100}})
		}
		local := c.Settings.DNS.Source4
		if netip.MustParseAddr(p.Address).Is6() {
			local = c.Settings.DNS.Source6
		}
		remotePort := p.Port
		if remotePort == 0 {
			remotePort = 179
		}
		peer := &api.Peer{Conf: &api.PeerConf{NeighborAddress: p.Address, PeerAsn: p.ASN, LocalAsn: c.Settings.BGP.LocalASN, Description: p.Description}, Timers: &api.Timers{Config: &api.TimersConfig{HoldTime: hold, KeepaliveInterval: keep, ConnectRetry: 3}}, Transport: &api.Transport{LocalAddress: local, RemotePort: remotePort}, AfiSafis: families,
			// Reject received routes: this application originates a bounded route set
			// and must never become a transit route reflector or re-export peer routes.
			ApplyPolicy: &api.ApplyPolicy{ImportPolicy: &api.PolicyAssignment{DefaultAction: api.RouteAction_REJECT}, ExportPolicy: &api.PolicyAssignment{DefaultAction: api.RouteAction_ACCEPT}}}
		if e := s.AddPeer(ctx, &api.AddPeerRequest{Peer: peer}); e != nil {
			g.Close()
			return nil, e
		}
	}
	return g, nil
}
func family(p netip.Prefix) *api.Family {
	afi := api.Family_AFI_IP
	if p.Addr().Is6() {
		afi = api.Family_AFI_IP6
	}
	return &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST}
}
func path(r Advertisement) (*api.Path, error) {
	p, e := config.Prefix(r.Prefix)
	if e != nil {
		return nil, e
	}
	nh, e := netip.ParseAddr(r.NextHop)
	if e != nil || nh.IsUnspecified() || nh.Is4() != p.Addr().Is4() {
		return nil, errors.New("route requires a concrete next hop of the same address family")
	}
	nlri, _ := anypb.New(&api.IPAddressPrefix{Prefix: p.Addr().String(), PrefixLen: uint32(p.Bits())})
	origin, _ := anypb.New(&api.OriginAttribute{Origin: 0})
	attrs := []*anypb.Any{origin}
	if p.Addr().Is4() {
		next, _ := anypb.New(&api.NextHopAttribute{NextHop: nh.String()})
		attrs = append(attrs, next)
	} else {
		next, _ := anypb.New(&api.MpReachNLRIAttribute{Family: family(p), NextHops: []string{nh.String()}, Nlris: []*anypb.Any{nlri}})
		attrs = append(attrs, next)
	}
	med, _ := anypb.New(&api.MultiExitDiscAttribute{Med: r.MED})
	pref, _ := anypb.New(&api.LocalPrefAttribute{LocalPref: r.LocalPreference})
	attrs = append(attrs, med, pref)
	if len(r.Communities) > 0 {
		values := []uint32{}
		for _, s := range r.Communities {
			v, e := config.Community(s)
			if e != nil {
				return nil, e
			}
			values = append(values, v)
		}
		communities, _ := anypb.New(&api.CommunitiesAttribute{Communities: values})
		attrs = append(attrs, communities)
	}
	return &api.Path{Family: family(p), Nlri: nlri, Pattrs: attrs}, nil
}
func (g *GoBGP) Add(ctx context.Context, r Advertisement) ([]byte, error) {
	p, e := path(r)
	if e != nil {
		return nil, e
	}
	res, e := g.server.AddPath(ctx, &api.AddPathRequest{TableType: api.TableType_GLOBAL, Path: p})
	if e != nil {
		return nil, e
	}
	return res.Uuid, nil
}
func (g *GoBGP) Delete(ctx context.Context, id []byte) error {
	return g.server.DeletePath(ctx, &api.DeletePathRequest{TableType: api.TableType_GLOBAL, Uuid: id})
}
func (g *GoBGP) Peers(ctx context.Context) ([]PeerStatus, error) {
	out := []PeerStatus{}
	e := g.server.ListPeer(ctx, &api.ListPeerRequest{}, func(p *api.Peer) {
		up := int64(0)
		if p.State.SessionState == api.PeerState_ESTABLISHED && p.Timers.State.Uptime != nil {
			up = int64(time.Since(p.Timers.State.Uptime.AsTime()).Seconds())
		}
		out = append(out, PeerStatus{Address: p.Conf.NeighborAddress, ASN: p.Conf.PeerAsn, State: p.State.SessionState.String(), UptimeSeconds: up})
	})
	return out, e
}
func (g *GoBGP) Exported(ctx context.Context, address string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, afi := range []api.Family_Afi{api.Family_AFI_IP, api.Family_AFI_IP6} {
		if e := g.server.ListPath(ctx, &api.ListPathRequest{TableType: api.TableType_ADJ_OUT, Name: address, Family: &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST}}, func(d *api.Destination) {
			if len(d.Paths) > 0 {
				out[d.Prefix] = true
			}
		}); e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (g *GoBGP) Close() error {
	g.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		g.closeErr = g.server.StopBgp(ctx, &api.StopBgpRequest{})
		g.server.Stop()
	})
	return g.closeErr
}

func Desired(c config.Document, controller, dns, surge bool) ([]Advertisement, error) {
	out := []Advertisement{}
	s := c.Settings
	eligible := func(binding string) bool {
		switch binding {
		case "none":
			return true
		case "controller":
			return controller
		case "dns":
			return dns
		case "surge":
			return surge
		}
		return false
	}
	if surge {
		for _, p := range s.Surge.FakePrefixes {
			prefix, e := config.Prefix(p)
			if e != nil {
				return nil, e
			}
			next := s.Surge.NextHop4
			if prefix.Addr().Is6() {
				next = s.Surge.NextHop6
			}
			out = append(out, Advertisement{Prefix: p, NextHop: next, Kind: "fake-ip", Binding: "surge", LocalPreference: 100})
		}
	}
	if controller && s.DNS.InterceptionEnabled {
		for _, r := range c.Interception {
			if !r.Enabled {
				continue
			}
			prefix, e := config.Prefix(r.Prefix)
			if e != nil {
				return nil, e
			}
			next := s.DNS.Source4
			if prefix.Addr().Is6() {
				next = s.DNS.Source6
			}
			out = append(out, Advertisement{Prefix: r.Prefix, NextHop: next, Kind: "dns-interception", Binding: "controller", LocalPreference: 100})
		}
	}
	for _, r := range c.Routes {
		if r.Enabled && eligible(r.Binding) {
			out = append(out, Advertisement{Prefix: r.Prefix, NextHop: r.NextHop, Kind: "manual", Binding: r.Binding, MED: r.MED, LocalPreference: r.LocalPreference, Communities: r.Communities})
		}
	}
	if len(out) > s.BGP.RouteLimit {
		return nil, fmt.Errorf("hard BGP route limit %d exceeded", s.BGP.RouteLimit)
	}
	for _, r := range out {
		if _, e := path(r); e != nil {
			return nil, e
		}
	}
	return out, nil
}
