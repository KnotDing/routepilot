// Controller includes the management service and a separate PF/DNS diagnostic
// harness used for reproducible packet-level acceptance.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/app"
	"surge-controller/internal/buildinfo"
	"surge-controller/internal/classifier"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/internal/softwareupdate"
	"surge-controller/internal/surge"
)

type values []string

func (v *values) String() string     { return strings.Join(*v, ",") }
func (v *values) Set(s string) error { *v = append(*v, s); return nil }

// The standalone diagnostic harness has no BGP session and keeps Fake-IP shut.
// The normal "serve" command uses the integrated BGP/state machine.
type closedGate struct{ forceDirect bool }

func (g closedGate) Snapshot() dnsserver.SafetySnapshot {
	return dnsserver.SafetySnapshot{DirectFallback: g.forceDirect, Generation: 1}
}

type journal struct {
	mu    sync.Mutex
	file  *os.File
	runID string
}

func (j *journal) record(kind string, payload any) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_ = json.NewEncoder(j.file).Encode(map[string]any{"run_id": j.runID, "kind": kind, "at": time.Now().UTC(), "payload": payload})
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "update-worker" {
		if err := softwareupdate.RunWorker(); err != nil {
			slog.Error("backend update failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "cleanup-pf" {
		fs := flag.NewFlagSet("cleanup-pf", flag.ExitOnError)
		dir := fs.String("state-directory", "/var/db/surge-controller", "private service state directory")
		_ = fs.Parse(os.Args[2:])
		if err := dnsserver.CleanupPF(*dir); err != nil {
			slog.Error("PF cleanup refused", "error", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 1 || os.Args[1] == "serve" {
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		path := fs.String("database", "data/controller.sqlite", "private SQLite path")
		listen := fs.String("web-listen", "", "optional management address override, persisted to this database")
		if len(os.Args) > 1 {
			_ = fs.Parse(os.Args[2:])
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := app.Run(ctx, *path, *listen); err != nil {
			slog.Error("controller stopped", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	var binds, transparent, upstreams, prefixes, interfaces values
	flag.Var(&binds, "listen", "concrete direct DNS IP:port; repeat for IPv4/IPv6")
	flag.Var(&transparent, "transparent-listen", "concrete PF redirect IP:port; repeat for IPv4/IPv6")
	flag.Var(&upstreams, "upstream", "UDP/TCP/DoT/DoH endpoint URL; repeat for pool")
	flag.Var(&prefixes, "intercept-prefix", "resolver prefix; repeat; used by PF rule generation")
	flag.Var(&interfaces, "ingress-interface", "PF incoming interface; repeat, including bridge if applicable")
	cliPath := flag.String("surge-cli", "/Applications/Surge.app/Contents/Applications/surge-cli", "absolute local Surge CLI path")
	fakeEndpoint := flag.String("surge-dns", "198.18.0.2:53", "Surge-owned Fake-IP DNS endpoint")
	fakePrefix := flag.String("fake-prefix", "198.18.0.0/15", "configured Surge Fake-IP network")
	mode := flag.String("mode", "auto", "core-stage mode: auto or force-direct (explicit diagnostic override)")
	iface := flag.String("egress-interface", "", "physical interface for real upstream DNS, e.g. en0")
	source4 := flag.String("egress-source4", "", "pinned IPv4 source; must have a gateway source-policy exception")
	source6 := flag.String("egress-source6", "", "pinned IPv6 source; needs independent IPv6 gateway exception")
	gateway4 := flag.String("gateway4", "", "gateway IPv4 used in generated PF route-to")
	gateway6 := flag.String("gateway6", "", "gateway IPv6 used in generated PF route-to")
	first := flag.Uint("source-port-first", 20000, "first reserved upstream source port (below 49152)")
	last := flag.Uint("source-port-last", 20999, "last reserved upstream source port")
	pfOutput := flag.String("write-pf", "", "write reviewed PF anchor to this file and exit; never load PF automatically")
	bypassVerified := flag.Bool("gateway-bypass-verified", false, "operator confirms gateway source policy has been configured; not an automatic proof")
	systemEgress := flag.Bool("development-system-egress", false, "development only: permit system routing when transparent ingress is disabled")
	journalPath := flag.String("verification-journal", "", "explicit opt-in private JSONL query journal for Phase 2 acceptance; create new file")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *mode != "auto" && *mode != "force-direct" {
		return errors.New("core stage supports auto and explicit force-direct; FORCE_SURGE requires validated BGP readiness")
	}
	parseAddr := func(value string) (netip.Addr, error) {
		if value == "" {
			return netip.Addr{}, nil
		}
		return netip.ParseAddr(value)
	}
	s4, err := parseAddr(*source4)
	if err != nil {
		return err
	}
	s6, err := parseAddr(*source6)
	if err != nil {
		return err
	}
	if *first > 65535 || *last > 65535 {
		return errors.New("source port out of range")
	}
	var intercepts []netip.Prefix
	for _, p := range prefixes {
		parsed, err := netip.ParsePrefix(p)
		if err != nil {
			return err
		}
		if parsed != parsed.Masked() || parsed.Bits() == 0 || parsed.Addr().IsLoopback() || parsed.Addr().IsMulticast() {
			return errors.New("invalid interception prefix")
		}
		intercepts = append(intercepts, parsed)
	}
	var transparentAddresses []netip.AddrPort
	for _, b := range transparent {
		a, err := netip.ParseAddrPort(b)
		if err != nil {
			return err
		}
		transparentAddresses = append(transparentAddresses, a)
	}
	fp, err := netip.ParsePrefix(*fakePrefix)
	if err != nil || fp != fp.Masked() {
		return errors.New("invalid Fake-IP prefix")
	}
	fakeAddr, err := netip.ParseAddrPort(*fakeEndpoint)
	if err != nil || fakeAddr.Port() == 0 {
		return errors.New("invalid Surge DNS endpoint")
	}
	for _, p := range intercepts {
		if p.Overlaps(fp) || p.Contains(fakeAddr.Addr()) || p.Contains(s4) || p.Contains(s6) {
			return errors.New("interception prefixes must exclude Fake-IP, local egress sources and the Surge DNS endpoint")
		}
	}
	if *pfOutput != "" {
		g4, err := parseAddr(*gateway4)
		if err != nil {
			return err
		}
		g6, err := parseAddr(*gateway6)
		if err != nil {
			return err
		}
		cfg := dnsserver.PFConfig{IngressInterfaces: interfaces, Prefixes: intercepts, EgressInterface: *iface, Source4: s4, Source6: s6, Gateway4: g4, Gateway6: g6, SourcePortFirst: uint16(*first), SourcePortLast: uint16(*last)}
		for _, a := range transparentAddresses {
			if a.Addr().Is4() {
				cfg.Listener4 = a
			} else {
				cfg.Listener6 = a
			}
		}
		rules, err := cfg.Rules()
		if err != nil {
			return err
		}
		return os.WriteFile(*pfOutput, []byte(rules), 0600)
	}
	if len(transparent) > 0 && (*systemEgress || !*bypassVerified || len(intercepts) == 0) {
		return errors.New("transparent ingress requires explicit pinned egress, interception prefixes and a configured gateway bypass")
	}
	if len(transparent) > 0 {
		for _, a := range transparentAddresses {
			if a.Port() == 53 {
				return errors.New("PF redirect must use a separate port from original DNS")
			}
			for _, p := range intercepts {
				if p.Contains(a.Addr()) {
					return errors.New("interception prefixes cannot contain a local redirect listener")
				}
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var dial resolver.EgressDialer
	if *systemEgress {
		dial = &net.Dialer{Timeout: 2 * time.Second}
		slog.Warn("development system routing enabled")
	} else {
		dial, err = resolver.NewPinnedDialer(*iface, s4, s6, 2*time.Second, uint16(*first), uint16(*last))
		if err != nil {
			return err
		}
	}
	wire, err := resolver.NewWireTransport(dial, 2*time.Second)
	if err != nil {
		return err
	}
	if len(upstreams) == 0 {
		upstreams = append(upstreams, "udp://1.1.1.1:53")
	}
	endpoints := make([]resolver.ResolverEndpoint, 0, len(upstreams))
	for _, u := range upstreams {
		e, err := parseEndpoint(u)
		if err != nil {
			return err
		}
		endpoints = append(endpoints, e)
	}
	real, err := resolver.NewReal(wire, endpoints, true, 10000)
	if err != nil {
		return err
	}
	cli, err := surge.NewCLI(*cliPath, 500*time.Millisecond)
	if err != nil {
		return err
	}
	// Surge DNS is an explicitly configured internal endpoint. Interception
	// prefixes cannot cover it; never substitute the real external egress path.
	fakeWire, err := resolver.NewWireTransport(&net.Dialer{Timeout: time.Second}, time.Second)
	if err != nil {
		return err
	}
	adapter, err := surge.New(cli, fakeWire, resolver.ResolverEndpoint{Protocol: "udp", Address: *fakeEndpoint, Enabled: true})
	if err != nil {
		return err
	}
	engine, err := classifier.New(ctx, adapter, classifier.DefaultConfig())
	if err != nil {
		return err
	}
	defer engine.Close()
	dispatcher := &dnsserver.Dispatcher{Classifier: engine, Real: real, Surge: adapter, Gate: closedGate{*mode == "force-direct"}, FakePrefixes: []netip.Prefix{fp}, History: dnsserver.NewHistory(1000, false)}
	for _, a := range []netip.Addr{s4, s6} {
		if a.IsValid() {
			dispatcher.ForbiddenClients = append(dispatcher.ForbiddenClients, a)
		}
	}
	var j *journal
	if *journalPath != "" {
		file, err := os.OpenFile(*journalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		j = &journal{file: file, runID: strconv.FormatInt(time.Now().UnixNano(), 36)}
		j.record("start", map[string]any{"interface": *iface, "source4": s4, "source6": s6, "mode": *mode})
	}
	var pf *dnsserver.PF
	var ingresses []dnsserver.Ingress
	if len(binds) == 0 {
		binds = append(binds, "127.0.0.1:1053")
	}
	for _, b := range binds {
		ap, err := netip.ParseAddrPort(b)
		if err != nil {
			return err
		}
		ingresses = append(ingresses, dnsserver.Ingress{Bind: ap})
	}
	if len(transparent) > 0 {
		pf, err = dnsserver.OpenPF()
		if err != nil {
			return err
		}
		defer pf.Close()
		for _, a := range transparentAddresses {
			ingresses = append(ingresses, dnsserver.Ingress{Bind: a, Intercepted: true, Lookup: pf})
		}
	}
	dispatch := func(ctx context.Context, q model.DNSQueryContext, m *dns.Msg) *dns.Msg {
		reply := dispatcher.Dispatch(ctx, q, m)
		j.record("query", map[string]any{"id": m.Id, "query": q, "rcode": reply.Rcode, "answer_count": len(reply.Answer)})
		return reply
	}
	server, err := dnsserver.Listen(ctx, ingresses, dnsserver.DefaultServerConfig(), dispatch)
	if err != nil {
		return err
	}
	defer server.Close()
	// Poll the active Surge inventories. The direct cache is invalidated on any
	// profile/rules/groups/mode/restart change or loss of the audit.
	var monitors sync.WaitGroup
	monitors.Add(1)
	go func() {
		defer monitors.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		previous := ""
		for {
			auditCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			snap, err := adapter.Refresh(auditCtx)
			cancel()
			if err != nil || snap.Digest != previous {
				engine.Cache.Flush()
				real.Cache.Flush()
				previous = snap.Digest
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	slog.Info("core DNS service ready", "listeners", server.Addresses(), "mode", *mode, "transparent", len(transparent) > 0, "fake_ip_issuance", false)
	if *mode == "auto" {
		slog.Warn("Phase 2 acceptance gate: AUTO fails closed for Surge decisions until BGP/state integration is available")
	}
	select {
	case <-ctx.Done():
	case err = <-server.Errors():
		stop()
	}
	server.Close()
	monitors.Wait()
	return err
}
func parseEndpoint(s string) (resolver.ResolverEndpoint, error) {
	u, err := url.Parse(s)
	if err != nil {
		return resolver.ResolverEndpoint{}, err
	}
	e := resolver.ResolverEndpoint{Name: u.Host, Enabled: true, Protocol: u.Scheme, Address: u.Host, TLSName: u.Query().Get("tls_name"), URL: u.Query().Get("url")}
	return e, e.Validate()
}
