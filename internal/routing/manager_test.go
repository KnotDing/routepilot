package routing

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"surge-controller/internal/bgp"
	"surge-controller/internal/config"
	"testing"
)

type backend struct {
	rows   []bgp.Advertisement
	ready  bool
	err    error
	closed bool
	calls  int
}

func (b *backend) Reconcile(_ context.Context, r []bgp.Advertisement) error {
	b.calls++
	b.rows = append([]bgp.Advertisement{}, r...)
	return b.err
}
func (b *backend) Refresh(context.Context) (bool, error) { return b.ready, b.err }
func (b *backend) Close(context.Context) error           { b.closed = true; return nil }
func (b *backend) Status() bgp.Status {
	state := "DOWN"
	if b.ready {
		state = "ESTABLISHED"
	}
	rows := append([]bgp.Advertisement{}, b.rows...)
	for i := range rows {
		rows[i].Installed = true
		rows[i].Announced = b.ready
	}
	return bgp.Status{Enabled: true, Peers: []bgp.PeerStatus{{State: state}}, Routes: rows}
}
func staticDocument() config.Document {
	d := config.Default()
	d.Settings.Routing = config.Routing{Mode: "static", StaticFakeIPConfigured: true}
	d.Settings.Surge.NextHop4 = "192.168.20.5"
	return d
}
func TestStaticNeverStartsBGPOrClaimsInstallation(t *testing.T) {
	d := staticDocument()
	d.Settings.BGP.Enabled = true
	d.Settings.BGP.LocalASN = 0
	d.Settings.BGP.RouterID = "invalid"
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	notifications := 0
	m := New(context.Background(), d, func(string, string) { notifications++ })
	if m.bgp != nil {
		t.Fatal("static mode created BGP")
	}
	ready, e := m.Refresh(context.Background())
	if e != nil || !ready {
		t.Fatal(ready, e)
	}
	row := m.Status().Routes[0]
	if row.Backend != "static" || row.Managed || row.Installed != nil || row.Announced || row.Verified || !row.OperatorConfirmed {
		t.Fatal(row)
	}
	raw, _ := json.Marshal(row)
	if !strings.Contains(string(raw), `"installed":null`) {
		t.Fatal(string(raw))
	}
	if len(m.Desired(true, true, true)) != 0 {
		t.Fatal("static route emitted advertisement")
	}
	m.Reconcile(context.Background(), nil)
	m.Close(context.Background())
	m.Refresh(context.Background())
	if !m.Status().Routes[0].OperatorConfirmed || notifications != 0 {
		t.Fatal("static routes withdrawn or changed")
	}
	d.Settings.Routing.StaticFakeIPConfigured = false
	m = newManager(d, nil, "")
	if ready, _ := m.Refresh(context.Background()); ready {
		t.Fatal("unconfirmed static routes opened gate")
	}
}
func TestHybridRoutingClassesRemainIndependent(t *testing.T) {
	for _, fake := range []string{"static", "bgp"} {
		d := staticDocument()
		other := "bgp"
		if fake == "bgp" {
			other = "static"
		}
		d.Settings.Routing = config.Routing{Mode: "hybrid", FakeIPBackend: fake, DNSBackend: other, StaticFakeIPConfigured: true, StaticDNSInterception: true, StaticDNSConfigured: true}
		d.Settings.DNS.InterceptionEnabled = true
		d.Settings.DNS.Source4 = "192.168.20.2"
		d.Interception = []config.Interception{{ID: "dns", Prefix: "8.8.8.8/32", Enabled: true}}
		b := &backend{ready: true}
		m := newManager(d, b, "")
		desired := m.Desired(true, true, true)
		if len(desired) != 1 || desired[0].Kind == "fake-ip" && fake != "bgp" || desired[0].Kind == "dns-interception" && other != "bgp" {
			t.Fatal(desired)
		}
		m.Reconcile(context.Background(), desired)
		if ready, e := m.Refresh(context.Background()); e != nil || !ready {
			t.Fatal(ready, e)
		}
		b.ready = false
		b.err = errors.New("peer down")
		ready, _ := m.Refresh(context.Background())
		if ready != (fake == "static") {
			t.Fatal("unrelated BGP failure blocked static Fake-IP", fake, ready)
		}
		if fake == "static" {
			for _, row := range m.Status().Routes {
				if row.Kind == "fake-ip" && (!row.OperatorConfirmed || row.Installed != nil) {
					t.Fatal(row)
				}
			}
		}
		b.err = nil
		b.ready = true
		m.Reconcile(context.Background(), m.Desired(true, true, false))
		m.Refresh(context.Background())
		for _, row := range m.Status().Routes {
			if row.Backend == "static" && row.State != "operator-confirmed" {
				t.Fatal("Surge failure changed static declaration")
			}
		}
	}
}
func TestProbeCannotConfirmExternalInstallation(t *testing.T) {
	d := staticDocument()
	d.Settings.Routing.StaticFakeIPConfigured = false
	m := newManager(d, nil, "")
	calls := 0
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		calls++
		if network != "tcp" || address != "198.18.1.1:443" {
			t.Fatal(network, address)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded probe")
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	if _, e := m.Verify(context.Background(), "198.18.0.0/15", "192.0.2.1", 443); e == nil || calls != 0 {
		t.Fatal("out-of-prefix probe accepted")
	}
	v, e := m.Verify(context.Background(), "198.18.0.0/15", "198.18.1.1", 443)
	if e != nil || !v.Success || v.CheckedAt.IsZero() || calls != 1 {
		t.Fatal(v, e)
	}
	row := m.Status().Routes[0]
	if row.Verified || row.OperatorConfirmed || row.Installed != nil || m.Status().FakeIPReady || row.Connectivity == nil {
		t.Fatal("TCP probe falsely verified route installation", row)
	}
	m.dial = func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("unreachable") }
	v, e = m.Verify(context.Background(), "198.18.0.0/15", "198.18.1.1", 443)
	if e != nil || v.Success {
		t.Fatal(v, e)
	}
}
