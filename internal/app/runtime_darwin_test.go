//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"reflect"
	"strings"
	"surge-controller/tests/mocks"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/config"
	"surge-controller/internal/resolver"
)

func TestFailedDNSActivationReportsStatusRestoresDNSAndReleasesPartialListener(t *testing.T) {
	a := fixtureApp(t)
	reserve, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	bind := reserve.Addr().String()
	reserve.Close()
	d := a.Configuration()
	d.Settings.DNS.Listeners = []string{bind}
	d.Settings.DNS.EgressInterface = "lo0"
	d.Settings.DNS.Source4 = "127.0.0.1"
	d.Settings.Health.GraceSeconds = 0
	d.Hosts = []config.Host{{ID: "recovery", Domain: "recovery.test", Match: "exact", Addresses: []string{"203.0.113.7"}, TTL: 60, Enabled: true, Source: "manual"}}
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	old := a.Configuration()
	occupied, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	d = old.Clone()
	d.Settings.DNS.Listeners = []string{occupied.Addr().String()}
	h := a.Handler()
	session := apiCall(h, "GET", "session", nil, nil, "")
	var credentials struct {
		CSRF string `json:"csrf"`
	}
	if e = json.Unmarshal(session.Body.Bytes(), &credentials); e != nil {
		t.Fatal(e)
	}
	cookie := session.Result().Cookies()[0]
	response := apiCall(h, "PUT", "settings", d.Redacted().Settings, cookie, credentials.CSRF)
	if response.Code != 200 {
		t.Fatal("configuration save failed", response.Code, response.Body.String())
	}
	status := waitConfiguration(t, a, "failed")
	if !strings.Contains(status.Error, "已恢复原配置") {
		t.Fatal("activation failure did not explain recovery", status.Error)
	}
	persisted, e := a.store.Load(context.Background())
	if e != nil || !reflect.DeepEqual(a.activeConfiguration(), old) || !reflect.DeepEqual(persisted.Settings.DNS.Listeners, d.Settings.DNS.Listeners) {
		t.Fatal("failed activation lost saved configuration or working runtime", e)
	}
	query := new(dns.Msg)
	query.SetQuestion("recovery.test.", dns.TypeA)
	client := &dns.Client{Net: "udp", Timeout: time.Second}
	check := func(address string) {
		t.Helper()
		reply, _, e := client.Exchange(query, address)
		if e != nil || len(reply.Answer) != 1 || reply.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
			t.Fatal("DNS did not recover after rejected configuration", e, reply)
		}
	}
	check(bind)
	// TCP bind failed after UDP was acquired. Retrying the exact port must work.
	occupied.Close()
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal("partial listener leaked or failed update could not be retried", e)
	}
	check(d.Settings.DNS.Listeners[0])
}

func TestUnprivilegedTransparentApplyKeepsRunningRuntime(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission rejection requires a non-root process")
	}
	a := fixtureApp(t)
	old := a.Configuration()
	rt := a.runtime
	d := old.Clone()
	d.Settings.Routing = config.Routing{Mode: "static", StaticDNSInterception: true}
	d.Settings.DNS.InterceptionEnabled = true
	d.Settings.DNS.Transparent = []string{"127.0.0.1:1054"}
	d.Settings.DNS.IngressInterfaces = []string{"lo0"}
	d.Settings.DNS.EgressInterface = "lo0"
	d.Settings.DNS.Source4 = "127.0.0.1"
	d.Settings.DNS.Gateway4 = "127.0.0.1"
	d.Settings.DNS.BypassVerified = true
	d.Interception = []config.Interception{{ID: "test", Name: "test", Prefix: "8.8.8.8/32", Enabled: true}}
	if e := a.Apply(context.Background(), d); e == nil || !strings.Contains(e.Error(), "安装网络后台服务") {
		t.Fatal("PF access rejection did not explain how to enable privileges", e)
	}
	if a.runtime != rt || !reflect.DeepEqual(a.activeConfiguration(), old) || !a.Configuration().Settings.DNS.InterceptionEnabled {
		t.Fatal("permission rejection stopped the working runtime")
	}
	select {
	case <-rt.done:
		t.Fatal("working runtime was stopped before checking PF access")
	default:
	}
}

// Exercise the actual application wiring, pinned sockets and native GoBGP with
// local protocol servers. A stream of unique answers cannot grow its route set.
func TestRuntimeDNSCannotCreateRoutesAndSurgeFailureFallsBack(t *testing.T) {
	a := fixtureApp(t)
	upstream, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	real := &dns.Server{PacketConn: upstream, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) { _ = w.WriteMsg(mocks.AddressReply(q, "203.0.113.7")) })}
	go real.ActivateAndServe()
	defer real.Shutdown()
	reserve, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	bind := reserve.Addr().String()
	reserve.Close()
	d := a.Configuration()
	d.Settings.DNS.Listeners = []string{bind}
	d.Settings.DNS.EgressInterface = "lo0"
	d.Settings.DNS.Source4 = "127.0.0.1"
	d.Settings.DNS.SourcePortFirst, d.Settings.DNS.SourcePortLast = 31000, 31031
	d.Settings.DNS.Upstreams = []resolver.ResolverEndpoint{{Name: "local real", Protocol: "udp", Address: upstream.LocalAddr().String(), Enabled: true}}
	d.Settings.BGP.Enabled = true
	d.Settings.BGP.RouterID = "127.0.0.1"
	d.Settings.BGP.ListenPort = 0
	d.Settings.Surge.NextHop4 = "127.0.0.1"
	d.Settings.Health.GraceSeconds = 0
	d.Routes = []config.Route{{ID: "independent", Prefix: "10.20.0.0/16", NextHop: "127.0.0.1", Enabled: true, Binding: "none"}}
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	wait := func(fn func() bool) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if fn() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("runtime did not converge")
	}
	// Force real DNS while no BGP peer is configured. Manual None remains active.
	wait(func() bool {
		up := 0
		for _, component := range a.runtime.health.Values() {
			if (component.Name == "Surge Fake-IP DNS" || component.Name == "Real DNS") && component.State == "UP" {
				up++
			}
		}
		return up == 2
	})
	d = a.Configuration()
	d.Settings.General.Mode = "force-direct"
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	wait(func() bool { return len(a.runtime.routes.ActiveStatus().Routes) == 1 })
	client := &dns.Client{Net: "udp", Timeout: time.Second}
	for i := 0; i < 100; i++ {
		q := new(dns.Msg)
		q.SetQuestion(dns.Fqdn("q"+string(rune('a'+i/26))+string(rune('a'+i%26))+".test"), dns.TypeA)
		r, _, e := client.Exchange(q, bind)
		if e != nil || r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
			t.Fatal("runtime DNS failed", e, r)
		}
	}
	time.Sleep(150 * time.Millisecond)
	if rows := a.runtime.routes.ActiveStatus().Routes; len(rows) != 1 || rows[0].Prefix != "10.20.0.0/16" {
		t.Fatal("DNS answers created BGP routes", rows)
	}
	// Remove only this test's CLI. AUTO must use independent real-DNS fallback;
	// disappearance of Surge cannot withdraw an unbound manual route.
	if e = os.Remove(d.Settings.Surge.CLIPath); e != nil {
		t.Fatal(e)
	}
	d = a.Configuration()
	d.Settings.General.Mode = "auto"
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	wait(func() bool { return a.runtime.machine.Snapshot().DirectFallback })
	q := new(dns.Msg)
	q.SetQuestion("after-failure.test.", dns.TypeA)
	r, _, e := client.Exchange(q, bind)
	if e != nil || r.Rcode != dns.RcodeSuccess || len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatal("Surge loss did not enter independent fallback", e, r)
	}
	if rows := a.runtime.routes.ActiveStatus().Routes; len(rows) != 1 || rows[0].Prefix != "10.20.0.0/16" {
		t.Fatal("Surge failure removed manual None route", rows)
	}
}

func TestStaticFakeAndRealDNSWorkWithoutBGPAndSurgeLossKeepsDeclaration(t *testing.T) {
	a := fixtureApp(t)
	upstream, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	real := &dns.Server{PacketConn: upstream, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) { w.WriteMsg(mocks.AddressReply(q, "203.0.113.7")) })}
	go real.ActivateAndServe()
	defer real.Shutdown()
	reserved, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	bind := reserved.Addr().String()
	reserved.Close()
	d := a.Configuration()
	d.Settings.Routing = config.Routing{Mode: "static"}
	d.Settings.DNS.Listeners = []string{bind}
	d.Settings.DNS.EgressInterface = "lo0"
	d.Settings.DNS.Source4 = "127.0.0.1"
	d.Settings.DNS.SourcePortFirst = 32000
	d.Settings.DNS.SourcePortLast = 32031
	d.Settings.DNS.Upstreams = []resolver.ResolverEndpoint{{Protocol: "udp", Address: upstream.LocalAddr().String(), Enabled: true}}
	d.Settings.Surge.NextHop4 = "127.0.0.1"
	d.Settings.Health.StabilizeSeconds = 1
	d.Settings.Health.GraceSeconds = 0
	d.Settings.Health.Failures = 1
	// Use the production success threshold: a disabled optional API must not
	// reset its counters and hold otherwise healthy CLI integration in fallback.
	d.Settings.Health.Successes = 3
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	d = a.Configuration()
	d.Settings.Routing.StaticFakeIPConfigured = true
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	rt := a.runtime
	server := rt.server
	client := &dns.Client{Net: "udp", Timeout: time.Second}
	q := new(dns.Msg)
	q.SetQuestion("without-bgp.test.", dns.TypeA)
	wait := func(fn func() bool) {
		t.Helper()
		for end := time.Now().Add(4 * time.Second); time.Now().Before(end); {
			if fn() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("runtime did not converge")
	}
	wait(func() bool { return rt.machine.Snapshot().AllowFakeIP })
	reply, _, e := client.Exchange(q, bind)
	if e != nil || len(reply.Answer) != 1 || reply.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatal("real DNS depended on BGP", reply, e)
	}
	d = a.Configuration()
	d.Settings.General.Mode = "force-surge"
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	wait(func() bool { return rt.machine.Snapshot().AllowFakeIP })
	reply, _, e = client.Exchange(q, bind)
	if e != nil || len(reply.Answer) != 1 || reply.Answer[0].(*dns.A).A.String() != "198.18.1.1" {
		t.Fatal("Fake-IP resolution depended on BGP", reply, e)
	}
	d = a.Configuration()
	d.Settings.BGP.Enabled = true
	d.Settings.BGP.RouterID = "127.0.0.1"
	if e = a.Apply(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if a.runtime != rt || rt.server != server || rt.routes.Status().Enabled {
		t.Fatal("ignored BGP setting restarted DNS or enabled a BGP speaker")
	}
	if e = os.Remove(d.Settings.Surge.CLIPath); e != nil {
		t.Fatal(e)
	}
	wait(func() bool {
		return rt.machine.Plan().State == "DIRECT_FALLBACK" && rt.machine.Snapshot().DirectFallback
	})
	reply, _, e = client.Exchange(q, bind)
	if e != nil || len(reply.Answer) != 1 || reply.Answer[0].(*dns.A).A.String() != "203.0.113.7" {
		t.Fatal("Surge loss did not fall back", reply, e)
	}
	row := rt.routes.Status().Routes[0]
	if row.Backend != "static" || row.Installed != nil || row.Verified || !row.OperatorConfirmed || rt.routes.Status().Advertised != 0 {
		t.Fatal("static route declaration changed on failure", row)
	}
}
