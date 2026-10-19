package app

import (
	"context"
	"net/netip"
	"reflect"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"surge-controller/internal/routing"
	"time"
)

// Dependencies follow the resources actually constructed by buildRuntime.
// DNS sources also bind BGP peer sockets; route declarations alone do not
// require replacing a speaker or its unrelated neighbor sessions. The router
// API backend swaps in place, so its configuration never rebuilds BGP.
type configurationImpact struct {
	DNS, BGP, Peers, Routes, Gate, API bool
}

func configurationChanges(old, next config.Document) configurationImpact {
	oldBGP := old.Settings.Routing.UsesBGP() && old.Settings.BGP.Enabled
	nextBGP := next.Settings.Routing.UsesBGP() && next.Settings.BGP.Enabled
	bgp := oldBGP != nextBGP || oldBGP && nextBGP && !reflect.DeepEqual(old.Settings.BGP, next.Settings.BGP)
	if oldBGP && nextBGP {
		for _, peers := range [][]config.Peer{old.Peers, next.Peers} {
			for _, peer := range peers {
				if !peer.Enabled {
					continue
				}
				ip, err := netip.ParseAddr(peer.Address)
				if err == nil && (ip.Unmap().Is4() && old.Settings.DNS.Source4 != next.Settings.DNS.Source4 || ip.Is6() && !ip.Is4In6() && old.Settings.DNS.Source6 != next.Settings.DNS.Source6) {
					bgp = true
				}
			}
		}
	}
	oldAPI := old.Settings.Routing.UsesAPI()
	nextAPI := next.Settings.Routing.UsesAPI()
	api := oldAPI != nextAPI || oldAPI && nextAPI && !reflect.DeepEqual(old.Settings.RoutingAPI, next.Settings.RoutingAPI)
	peers := !reflect.DeepEqual(old.Peers, next.Peers)
	routes := bgp || peers || api || !reflect.DeepEqual(old.Settings.Routing, next.Settings.Routing) ||
		!reflect.DeepEqual(old.Settings.BGP, next.Settings.BGP) ||
		old.Settings.DNS.InterceptionEnabled != next.Settings.DNS.InterceptionEnabled ||
		!reflect.DeepEqual(routing.Definitions(old), routing.Definitions(next))
	gate := !reflect.DeepEqual(old.Settings.Routing, next.Settings.Routing) ||
		old.Settings.Surge.NextHop4 != next.Settings.Surge.NextHop4 || old.Settings.Surge.NextHop6 != next.Settings.Surge.NextHop6
	return configurationImpact{DNS: !reflect.DeepEqual(dnsRuntimeDocument(old), dnsRuntimeDocument(next)), BGP: bgp, Peers: peers, Routes: routes, Gate: gate, API: api}
}

// Called while routingMu excludes monitor reconciliation. Keep the old paths
// until issued answers expire, even when only route declarations are replaced.
func (rt *Runtime) suspendRouting(ctx context.Context, waitTTL bool) error {
	expiry := rt.machine.SuspendRouting()
	rt.machine.DrainResponses()
	if waitTTL && time.Now().Before(expiry) {
		timer := time.NewTimer(time.Until(expiry))
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func dnsRuntimeDocument(d config.Document) config.Document {
	d.Settings.DNS.AAAAFallback = ""
	d.Settings.DNS.Clients = nil
	d.Settings.DHCP = config.DHCP{}
	d.Settings.RemoteBackends = nil
	d.Settings.General = config.General{}
	d.Settings.Security = config.Security{}
	d.Settings.Notifications = config.Notifications{}
	d.Settings.Updates = config.Updates{}
	d.Settings.Logging = config.Logging{}
	d.Settings.Routing = config.Routing{}
	d.Settings.RoutingAPI = config.RoutingAPI{}
	d.Settings.Surge.Restart = config.SurgeRestart{}
	d.Settings.Surge.NextHop4, d.Settings.Surge.NextHop6 = "", ""
	d.Settings.BGP = config.BGP{}
	d.Peers, d.Routes, d.RouteFeeds, d.Hosts, d.Overrides = nil, nil, nil, nil, nil
	return d
}

func clientPolicies(clients []config.Client) map[netip.Addr]model.Decision {
	policies := make(map[netip.Addr]model.Decision)
	for _, client := range clients {
		if client.Decision != "" {
			policies[netip.MustParseAddr(client.IP).Unmap()] = client.Decision
		}
	}
	return policies
}
