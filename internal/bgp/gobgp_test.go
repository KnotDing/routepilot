package bgp

import (
	"context"
	api "github.com/osrg/gobgp/v3/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"net"
	"surge-controller/internal/config"
	"testing"
	"time"
)

func freePort(t *testing.T) int32 {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return int32(port)
}
func until(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("local GoBGP exchange did not converge")
}
func TestRealGoBGPAnnounceWithdrawAndRejectImportedTransit(t *testing.T) {
	ctx := context.Background()
	a, b := config.Default(), config.Default()
	portA, portB := freePort(t), freePort(t)
	a.Settings.BGP.ListenPort = portA
	b.Settings.BGP.ListenPort = portB
	a.Settings.BGP.RouterID = "127.0.0.1"
	b.Settings.BGP.RouterID = "127.0.0.2"
	b.Settings.BGP.LocalASN = 64513
	a.Settings.DNS.Source4 = "127.0.0.1"
	b.Settings.DNS.Source4 = "127.0.0.1"
	a.Peers = []config.Peer{{ID: "b", Address: "127.0.0.1", ASN: 64513, Port: uint32(portB), Enabled: true}}
	b.Peers = []config.Peer{{ID: "a", Address: "127.0.0.1", ASN: 64512, Port: uint32(portA), Enabled: true}}
	ga, e := New(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	defer ga.Close()
	gb, e := New(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	defer gb.Close()
	until(t, func() bool {
		peers, e := ga.Peers(ctx)
		return e == nil && len(peers) == 1 && peers[0].State == "ESTABLISHED"
	})
	route := Advertisement{Prefix: "10.20.0.0/16", NextHop: "192.168.20.5", Kind: "manual", Binding: "none", MED: 42, LocalPreference: 100, Communities: []string{"64512:7"}}
	id, e := ga.Add(ctx, route)
	if e != nil {
		t.Fatal(e)
	}
	received := func() (bool, string) {
		found := false
		next := ""
		_ = gb.server.ListPath(ctx, &api.ListPathRequest{TableType: api.TableType_ADJ_IN, Name: "127.0.0.1", Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}}, func(d *api.Destination) {
			if d.Prefix == route.Prefix {
				for _, p := range d.Paths {
					found = true
					for _, attr := range p.Pattrs {
						v := new(api.NextHopAttribute)
						if anypb.UnmarshalTo(attr, v, proto.UnmarshalOptions{}) == nil {
							next = v.NextHop
						}
					}
				}
			}
		})
		return found, next
	}
	until(t, func() bool { found, _ := received(); return found })
	if _, next := received(); next != route.NextHop {
		t.Fatalf("advertised next hop changed: %s", next)
	}
	imported := 0
	_ = gb.server.ListPath(ctx, &api.ListPathRequest{TableType: api.TableType_GLOBAL, Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST}}, func(d *api.Destination) { imported += len(d.Paths) })
	if imported != 0 {
		t.Fatal("received routes imported into controller global RIB")
	}
	if e = ga.Delete(ctx, id); e != nil {
		t.Fatal(e)
	}
	until(t, func() bool { found, _ := received(); return !found })
}

func TestRealGoBGPSessionLossRemovesManagedIPv4AndIPv6Routes(t *testing.T) {
	ctx := context.Background()
	a, b := config.Default(), config.Default()
	portA, portB := freePort(t), freePort(t)
	a.Settings.BGP.ListenPort, b.Settings.BGP.ListenPort = portA, portB
	a.Settings.BGP.RouterID, b.Settings.BGP.RouterID = "127.0.0.1", "127.0.0.2"
	b.Settings.BGP.LocalASN = 64513
	a.Settings.DNS.Source4, b.Settings.DNS.Source4 = "127.0.0.1", "127.0.0.1"
	a.Peers = []config.Peer{{ID: "b", Address: "127.0.0.1", ASN: 64513, Port: uint32(portB), Enabled: true}}
	b.Peers = []config.Peer{{ID: "a", Address: "127.0.0.1", ASN: 64512, Port: uint32(portA), Enabled: true}}
	ga, e := New(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	defer ga.Close()
	gb, e := New(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	defer gb.Close()
	until(t, func() bool {
		peers, e := ga.Peers(ctx)
		return e == nil && len(peers) == 1 && peers[0].State == "ESTABLISHED"
	})
	for _, route := range []Advertisement{
		{Prefix: "198.18.0.0/15", NextHop: "192.168.20.5", Kind: "fake-ip", Binding: "surge"},
		{Prefix: "8.8.8.8/32", NextHop: "192.168.20.2", Kind: "dns-interception", Binding: "controller"},
		{Prefix: "fd00:198:18::/48", NextHop: "fd00:20::5", Kind: "fake-ip", Binding: "surge"},
	} {
		if _, e = ga.Add(ctx, route); e != nil {
			t.Fatal(e)
		}
	}
	counts := func() int {
		count := 0
		for _, afi := range []api.Family_Afi{api.Family_AFI_IP, api.Family_AFI_IP6} {
			e := gb.server.ListPath(ctx, &api.ListPathRequest{TableType: api.TableType_ADJ_IN, Name: "127.0.0.1", Family: &api.Family{Afi: afi, Safi: api.Family_SAFI_UNICAST}}, func(d *api.Destination) { count += len(d.Paths) })
			if e != nil {
				return -1
			}
		}
		return count
	}
	until(t, func() bool { return counts() == 3 })
	if e = ga.Close(); e != nil {
		t.Fatal(e)
	}
	until(t, func() bool { return counts() == 0 })
	peers, e := gb.Peers(ctx)
	if e != nil || len(peers) != 1 || peers[0].State == "ESTABLISHED" {
		t.Fatal("session loss retained peer routes", peers, e)
	}
}
