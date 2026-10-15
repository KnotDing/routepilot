package app

import (
	"context"
	"github.com/miekg/dns"
	"net/netip"
	"path/filepath"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"testing"
)

func TestClientPoliciesPersistAndHotApplyWithTheSelectedBackend(t *testing.T) {
	a := fixtureApp(t)
	path := filepath.Join(a.dir, "c.sqlite")
	initial := a.runtime
	d := a.Configuration()
	d.Settings.DNS.Clients = []config.Client{{ID: "client", IP: "2001:db8::10", Name: "Server", Decision: model.Reject}}
	if err := a.SaveConfiguration(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	waitConfiguration(t, a, "succeeded")
	if a.runtime != initial {
		t.Fatal("client policy change restarted DNS")
	}
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	if reply := a.runtime.dispatcher.Dispatch(context.Background(), model.DNSQueryContext{ClientIP: netip.MustParseAddr("2001:db8::10")}, q); reply.Rcode != dns.RcodeRefused {
		t.Fatal(reply)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got := restored.Configuration().Settings.DNS.Clients; len(got) != 1 || got[0].Name != "Server" || got[0].Decision != model.Reject {
		t.Fatal(got)
	}
	if restored.runtime.dispatcher.ClientPolicies[netip.MustParseAddr("2001:db8::10")] != model.Reject {
		t.Fatal("policy not restored into runtime")
	}
}
