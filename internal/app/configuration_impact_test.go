package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/miekg/dns"
	"net"
	"reflect"
	"surge-controller/internal/bgp"
	"surge-controller/internal/config"
	"surge-controller/internal/state"
	"testing"
	"time"
)

func TestConfigurationModuleDependencies(t *testing.T) {
	old := config.Default().Clone()
	old.Settings.BGP.Enabled = true
	old.Settings.BGP.RouterID = "127.0.0.1"
	old.Settings.DNS.Source4 = "127.0.0.1"
	old.Settings.Surge.NextHop4 = "127.0.0.1"
	old.Peers = []config.Peer{{ID: "peer", Address: "127.0.0.1", ASN: 64513, Enabled: true}}
	for _, tc := range []struct {
		name             string
		edit             func(*config.Document)
		dns, bgp, routes bool
	}{
		{"upstream timeout", func(d *config.Document) { d.Settings.DNS.TimeoutMS++ }, true, false, false},
		{"cache capacity", func(d *config.Document) { d.Settings.Cache.Capacity++ }, true, false, false},
		{"health interval", func(d *config.Document) { d.Settings.Health.IntervalMS++ }, true, false, false},
		{"query history", func(d *config.Document) { d.Settings.Logging.QueryHistory = !d.Settings.Logging.QueryHistory }, false, false, false},
		{"BGP ASN", func(d *config.Document) { d.Settings.BGP.LocalASN++ }, false, true, true},
		{"BGP disabled", func(d *config.Document) { d.Settings.BGP.Enabled = false }, false, true, true},
		{"neighbor", func(d *config.Document) { d.Peers[0].Description = "updated" }, false, false, true},
		{"bound source", func(d *config.Document) { d.Settings.DNS.Source4 = "127.0.0.2" }, true, true, false},
		{"unused source family", func(d *config.Document) { d.Settings.DNS.Source6 = "::1" }, true, false, false},
		{"next hop", func(d *config.Document) { d.Settings.Surge.NextHop4 = "127.0.0.2" }, false, false, true},
		{"fake prefixes", func(d *config.Document) { d.Settings.Surge.FakePrefixes = []string{"198.19.0.0/16"} }, true, false, true},
		{"manual route", func(d *config.Document) {
			d.Routes = []config.Route{{ID: "r", Prefix: "10.20.0.0/16", NextHop: "127.0.0.1", Enabled: true}}
		}, false, false, true},
		{"log level", func(d *config.Document) { d.Settings.Logging.Level = "debug" }, false, false, false},
		{"notification", func(d *config.Document) { d.Settings.Notifications.Enabled = true }, false, false, false},
		{"management security", func(d *config.Document) { d.Settings.Security.SessionMinutes++ }, false, false, false},
		{"update channel", func(d *config.Document) { d.Settings.Updates.CheckBeta = true }, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := old.Clone()
			tc.edit(&next)
			impact := configurationChanges(old, next)
			// Any BGP replacement necessarily refreshes its route document too.
			if impact.DNS != tc.dns || impact.BGP != tc.bgp || impact.Routes != (tc.routes || tc.bgp) {
				t.Fatalf("wrong module impact: %+v", impact)
			}
		})
	}
	disabled := old.Clone()
	disabled.Settings.Routing = config.Routing{Mode: "static"}
	next := disabled.Clone()
	next.Settings.BGP.LocalASN++
	if impact := configurationChanges(disabled, next); impact.DNS || impact.BGP {
		t.Fatalf("unused BGP settings restarted a module: %+v", impact)
	}
}

func configurationTestPort(t *testing.T) int32 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return int32(listener.Addr().(*net.TCPAddr).Port)
}

func TestDNSUpdatesKeepEstablishedBGPSessionAndIndependentRoutes(t *testing.T) {
	a := fixtureApp(t)
	d := a.Configuration()
	portA, portB := configurationTestPort(t), configurationTestPort(t)
	d.Settings.BGP.Enabled = true
	d.Settings.BGP.RouterID = "127.0.0.1"
	d.Settings.BGP.ListenPort = portA
	d.Settings.DNS.Source4 = "127.0.0.1"
	d.Settings.Surge.NextHop4 = "127.0.0.1"
	d.Settings.Health.GraceSeconds = 0
	d.Peers = []config.Peer{{ID: "remote", Address: "127.0.0.1", ASN: 64513, Port: uint32(portB), Enabled: true}}
	d.Routes = []config.Route{{ID: "independent", Prefix: "10.20.0.0/16", NextHop: "127.0.0.1", Enabled: true, Binding: "none"}}
	remoteConfig := config.Default()
	remoteConfig.Settings.BGP.LocalASN = 64513
	remoteConfig.Settings.BGP.RouterID = "127.0.0.2"
	remoteConfig.Settings.BGP.ListenPort = portB
	remoteConfig.Settings.DNS.Source4 = "127.0.0.1"
	remoteConfig.Peers = []config.Peer{{ID: "app", Address: "127.0.0.1", ASN: d.Settings.BGP.LocalASN, Port: uint32(portA), Enabled: true}}
	remote, err := bgp.New(context.Background(), remoteConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	if err := a.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	session := func() string {
		peers, err := remote.Peers(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(peers) != 1 || peers[0].State != "ESTABLISHED" {
			return ""
		}
		var detail struct {
			Timers struct {
				State struct {
					Uptime string `json:"uptime"`
				} `json:"state"`
			} `json:"timers"`
		}
		if err := json.Unmarshal(peers[0].Details, &detail); err != nil {
			t.Fatal(err)
		}
		return detail.Timers.State.Uptime
	}
	wait := func(fn func() bool) {
		t.Helper()
		for end := time.Now().Add(8 * time.Second); time.Now().Before(end); {
			if fn() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("isolated BGP did not converge")
	}
	wait(func() bool { return session() != "" })
	established := session()
	manager := a.runtime.routes
	// Privacy and client settings are hot updates, including the PTR task.
	hotRuntime := a.runtime
	for _, edit := range []func(*config.Document){
		func(d *config.Document) { d.Settings.Logging.QueryHistory = false },
		func(d *config.Document) { d.Settings.Logging.LogDomains = false },
		func(d *config.Document) { d.Settings.Logging.HistoryCapacity = 100 },
		func(d *config.Document) { d.Settings.Logging.ProbeHostnames = true },
		func(d *config.Document) { d.Settings.Logging.HostnameDeviceOnly = true },
		func(d *config.Document) { d.Settings.Logging.HostnameDeviceOnly = false },
		func(d *config.Document) { d.Settings.Logging.ProbeHostnames = false },
		func(d *config.Document) {
			d.Settings.DNS.Clients = []config.Client{{ID: "remark", IP: "192.0.2.10", Name: "Office"}}
		},
		func(d *config.Document) { d.Settings.DNS.Clients[0].Decision = "REJECT" },
	} {
		next := a.Configuration()
		edit(&next)
		if err := a.Apply(context.Background(), next); err != nil {
			t.Fatal(err)
		}
		if a.runtime != hotRuntime || a.runtime.routes != manager || session() != established {
			t.Fatal("privacy/client edit restarted DNS or BGP")
		}
	}
	for _, edit := range []func(*config.Document){
		func(d *config.Document) { d.Settings.DNS.TimeoutMS += 100 },
		func(d *config.Document) { d.Settings.Cache.Capacity++ },
		func(d *config.Document) {
			d.Settings.DNS.TimeoutMS += 100
			d.Routes = append(d.Routes, config.Route{ID: "second", Prefix: "10.21.0.0/16", NextHop: "127.0.0.1", Enabled: true, Binding: "none"})
		},
	} {
		old := a.runtime
		next := a.Configuration()
		edit(&next)
		if err := a.Apply(context.Background(), next); err != nil {
			t.Fatal(err)
		}
		if a.runtime == old || a.runtime.routes != manager {
			t.Fatal("DNS replacement recreated the BGP manager")
		}
		if got := session(); got != established {
			t.Fatalf("DNS update interrupted established BGP: before %q, after %q", established, got)
		}
		wait(func() bool {
			_, err := manager.Refresh(context.Background())
			return err == nil && len(manager.ActiveStatus().Routes) == len(next.Routes)
		})
	}
	// GoBGP route-selection options are process-global. Stop the isolated
	// remote speaker before starting another local speaker with new globals.
	if err := remote.Close(); err != nil {
		t.Fatal(err)
	}
	// A BGP-global change takes the inverse path: the DNS core is retained.
	rt := a.runtime
	next := a.Configuration()
	next.Settings.BGP.LocalASN++
	if err := a.Apply(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if a.runtime != rt || a.runtime.routes == manager {
		t.Fatal("BGP-global update replaced the DNS core or did not replace BGP")
	}
	if !reflect.DeepEqual(a.activeConfiguration().Settings.BGP, next.Settings.BGP) {
		t.Fatal("BGP configuration was not applied")
	}
}

func TestRoutingUpdateClosesIssuanceAndDrainsResponses(t *testing.T) {
	machine := state.New(config.Health{StabilizeSeconds: 1}, "auto")
	input := state.Input{Controller: true, DNS: true, Surge: true, FakeIP: true, Routes: true}
	machine.Step(time.Now().Add(-2*time.Second), input)
	machine.Step(time.Now(), input)
	lease, ok := machine.AcquireFakeIP()
	if !ok {
		t.Fatal("fixture gate did not open")
	}
	defer lease.Release()
	reply := new(dns.Msg)
	reply.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}}}
	lease.(interface{ Observe(*dns.Msg) }).Observe(reply)
	machine.BeginResponse()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- (&Runtime{machine: machine}).suspendRouting(ctx, true) }()
	<-ctx.Done()
	select {
	case <-result:
		machine.EndResponse()
		t.Fatal("routing update passed an outstanding response")
	default:
	}
	machine.EndResponse()
	if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("routing update did not wait for issued TTL", err)
	}
	if lease.Valid() || machine.Snapshot().AllowFakeIP {
		t.Fatal("routing update left Fake-IP issuance open")
	}
}
