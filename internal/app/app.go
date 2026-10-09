package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"golang.org/x/net/netutil"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"surge-controller/internal/api"
	"surge-controller/internal/buildinfo"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/events"
	"surge-controller/internal/localdns"
	"surge-controller/internal/model"
	"surge-controller/internal/notifications"
	"surge-controller/internal/remote"
	"surge-controller/internal/resolver"
	"surge-controller/internal/routing"
	"surge-controller/internal/security"
	"surge-controller/internal/state"
	"surge-controller/internal/surge"
	"surge-controller/web"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type policy struct {
	mu            sync.RWMutex
	engine        *classifier.Engine
	mode          string
	overrides     []config.Override
	realIPDomains interface{ MatchAlwaysRealIP(string) (string, bool) }
}

func (p *policy) update(mode string, overrides []config.Override) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = mode
	p.overrides = append([]config.Override{}, overrides...)
	sort.SliceStable(p.overrides, func(i, j int) bool {
		a, b := p.overrides[i], p.overrides[j]
		if a.Match != b.Match {
			return a.Match == "exact"
		}
		return len(a.Domain) > len(b.Domain)
	})
	p.engine.Cache.FlushReason("全局模式或域名覆盖已更新")
}
func (p *policy) Classify(ctx context.Context, q model.DNSQueryContext) (model.DecisionResult, error) {
	name, e := model.NormalizeDomain(q.Question.Name)
	if e != nil {
		return model.DecisionResult{}, e
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := func(d model.Decision, source string) model.DecisionResult {
		return model.DecisionResult{Decision: d, Confidence: model.Certain, Domain: name, ClientIP: q.ClientIP, Source: source, CreatedAt: time.Now()}
	}
	if p.realIPDomains != nil {
		if pattern, ok := p.realIPDomains.MatchAlwaysRealIP(name); ok {
			d := result(model.Direct, "surge-always-real-ip")
			d.MatchedRule = "always-real-ip = " + pattern
			d.Reason = "Matched Surge always-real-ip; querying configured real DNS upstreams without Surge evaluation"
			return d, nil
		}
	}
	if p.mode == "force-surge" {
		return result(model.Surge, "manual-mode"), nil
	}
	for _, o := range p.overrides {
		if !o.Enabled {
			continue
		}
		d, _ := model.NormalizeDomain(o.Domain)
		match := name == d
		if o.Match == "suffix" {
			match = match || strings.HasSuffix(name, "."+d)
		}
		if match {
			return result(o.Decision, "override"), nil
		}
	}
	return p.engine.Classify(ctx, q)
}

type Runtime struct {
	decisionMu                                       sync.Mutex
	decisionRevision                                 decisionRevision
	decisionScope                                    string
	pendingDecisions                                 *decisionCacheSnapshot
	routingMu                                        sync.RWMutex
	routingWake                                      chan struct{}
	hosts                                            *localdns.Resolver
	closeOnce                                        sync.Once
	closeErr                                         error
	faultMu                                          sync.RWMutex
	cancel                                           context.CancelFunc
	done                                             chan struct{}
	dispatcher                                       *dnsserver.Dispatcher
	engine                                           *classifier.Engine
	adapter                                          *surge.Adapter
	real                                             *resolver.Real
	policyResolvers                                  []*resolver.Real
	wire                                             *resolver.WireTransport
	server                                           *dnsserver.Server
	pf                                               *dnsserver.PF
	pfLease                                          *dnsserver.PFLease
	machine                                          *state.Machine
	health                                           *state.Health
	routes                                           *routing.Manager
	policy                                           *policy
	fault                                            string
	settings                                         config.Document
	started                                          time.Time
	previousQueries                                  atomic.Uint64
	queryRate                                        atomic.Uint64
	realQueries, realErrors, fakeQueries, fakeErrors atomic.Uint64
	latency                                          [8]atomic.Uint64
	latencySum                                       atomic.Uint64
}
type App struct {
	notifications      *notifications.Manager
	notificationCancel context.CancelFunc
	notificationDone   chan struct{}
	cachePersistMu     sync.Mutex
	savedCacheDigest   [32]byte
	dnsCounters        dnsserver.Counters
	statsCancel        context.CancelFunc
	statsDone          chan struct{}
	ruleTests          chan struct{}
	lockFile           *os.File
	eventRetention     atomic.Int64
	logLevel           slog.LevelVar
	mu                 sync.RWMutex
	transition         sync.Mutex
	config             config.Document
	runtime            *Runtime
	store              *database.Store
	bus                *events.Bus
	security           *security.Manager
	remote             *remote.Manager
	control            *api.API
	ctx                context.Context
	dir                string
	webChanged         chan struct{}
	closing            atomic.Bool
}

const managementRecoveryAddress = "127.0.0.1:1055"

func Open(ctx context.Context, path string) (*App, error) {
	store, e := database.Open(path)
	if e != nil {
		return nil, e
	}
	lock, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		store.Close()
		return nil, e
	}
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		lock.Close()
		store.Close()
		return nil, errors.New("another Controller owns this database")
	}
	opened := false
	defer func() {
		if !opened {
			_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
			_ = lock.Close()
		}
	}()
	d, e := store.Load(ctx)
	if e != nil {
		store.Close()
		return nil, e
	}
	if e = store.Save(ctx, d); e != nil {
		store.Close()
		return nil, e
	}
	var counters dnsserver.CounterSnapshot
	if e = store.LoadStatistics(ctx, "dns", &counters); e != nil {
		store.Close()
		return nil, fmt.Errorf("load DNS statistics: %w", e)
	}
	a := &App{ruleTests: make(chan struct{}, 2), lockFile: lock, config: d, store: store, bus: events.New(), security: security.New(d.Settings.Security), ctx: ctx, dir: filepath.Dir(path), webChanged: make(chan struct{}, 1)}
	a.dnsCounters.Restore(counters)
	a.remote = remote.New(func() []config.Backend { return a.Configuration().Settings.RemoteBackends })
	a.eventRetention.Store(int64(d.Settings.Logging.EventRetention))
	a.setLogging(d.Settings.Logging.Level)
	notificationCtx, notificationCancel := context.WithCancel(ctx)
	a.notificationCancel = notificationCancel
	a.notificationDone = make(chan struct{})
	a.notifications = notifications.New(a.event)
	go func() { defer close(a.notificationDone); a.notifications.Run(notificationCtx) }()
	a.control = &api.API{Service: a, Store: a.store, Bus: a.bus}
	go a.control.RunRouteFeedScheduler(ctx)
	var savedDecisions decisionCacheSnapshot
	if err := store.LoadDecisionCache(ctx, &savedDecisions); err != nil {
		// Cache corruption must not prevent management or fallback DNS startup.
		slog.Warn("Saved decision cache could not be loaded; starting empty", "error", err)
		savedDecisions = decisionCacheSnapshot{}
	}
	rt, e := a.build(d, &savedDecisions)
	if e != nil {
		a.notificationCancel()
		<-a.notificationDone
		store.Close()
		return nil, e
	}
	a.runtime = rt
	statsCtx, statsCancel := context.WithCancel(context.Background())
	a.statsCancel = statsCancel
	a.statsDone = make(chan struct{})
	go a.persistStatistics(statsCtx, counters)
	opened = true
	a.event("INFO", "startup", "Controller management service started")
	return a, nil
}
func (a *App) event(severity, kind, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ev, e := a.store.Event(ctx, severity, kind, message, int(a.eventRetention.Load()))
	if e != nil {
		slog.Error("operational event persistence failed")
		return
	}
	a.bus.Publish(ev)
	level := slog.LevelInfo
	if severity == "WARNING" {
		level = slog.LevelWarn
	} else if severity == "ERROR" || severity == "CRITICAL" {
		level = slog.LevelError
	}
	slog.Log(ctx, level, "controller event", "severity", severity, "kind", kind, "message", message)
}
func (a *App) Configuration() config.Document {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config.Clone()
}
func (a *App) build(d config.Document, saved ...*decisionCacheSnapshot) (rt *Runtime, err error) {
	if err = d.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	rt = &Runtime{cancel: cancel, done: make(chan struct{}), routingWake: make(chan struct{}, 1), settings: d, started: time.Now(), machine: state.New(d.Settings.Health, d.Settings.General.Mode), health: state.NewHealth(d.Settings.Health.Successes, d.Settings.Health.Failures)}
	failed := true
	// Error returns assign nil to the named result. Keep the allocated runtime
	// separately so partial initialization is cleaned up without panicking.
	defer func(cleanup *Runtime) {
		if failed {
			cancel()
			if cleanup.server != nil {
				cleanup.server.Close()
			}
			if cleanup.pfLease != nil {
				_ = cleanup.pfLease.Close()
			}
			if cleanup.pf != nil {
				_ = cleanup.pf.Close()
			}
			if cleanup.routes != nil {
				_ = cleanup.routes.Close(context.Background())
			}
			if cleanup.engine != nil {
				cleanup.engine.Close()
			}
		}
	}(rt)
	s := d.Settings
	cli, err := surge.NewCLI(s.Surge.CLIPath, time.Duration(s.Cache.TimeoutMS)*time.Millisecond)
	if err != nil {
		// Surge absence must not take down management or fallback DNS. Run
		// still attempts the real executable and reports the health failure.
		cli = &surge.CLI{Path: s.Surge.CLIPath, Timeout: time.Duration(s.Cache.TimeoutMS) * time.Millisecond}
	}
	cli.UserName = s.Surge.UserName
	fakeWire, _ := resolver.NewWireTransport(&net.Dialer{Timeout: time.Duration(s.DNS.TimeoutMS) * time.Millisecond}, time.Duration(s.DNS.TimeoutMS)*time.Millisecond)
	rt.adapter, err = surge.New(cli, fakeWire, resolver.ResolverEndpoint{Protocol: "udp", Address: s.Surge.DNS, Enabled: true})
	if err != nil {
		return nil, err
	}
	rt.engine, err = classifier.New(ctx, rt.adapter, classifier.Config{ExpireDecisions: s.Cache.ExpireDecisions, TrustDirect: s.Surge.TrustDirect, Capacity: s.Cache.Capacity, Workers: s.Cache.Workers, QueueSize: s.Cache.Queue, Timeout: time.Duration(s.Cache.TimeoutMS) * time.Millisecond, DirectTTL: time.Duration(s.Cache.DirectTTLSeconds) * time.Second, SurgeTTL: time.Duration(s.Cache.SurgeTTLSeconds) * time.Second, UnknownTTL: time.Duration(s.Cache.UnknownTTLSeconds) * time.Second, ErrorTTL: time.Duration(s.Cache.ErrorTTLSeconds) * time.Second})
	if err != nil {
		return nil, err
	}
	rt.policy = &policy{engine: rt.engine, realIPDomains: rt.adapter}
	rt.policy.update(s.General.Mode, d.Overrides)
	rt.decisionScope = decisionCacheScope(d)
	if len(saved) > 0 && saved[0] != nil && saved[0].Version == 1 && saved[0].Scope == rt.decisionScope && saved[0].Digest != "" {
		copy := *saved[0]
		rt.pendingDecisions = &copy
	}
	rt.hosts = localdns.New(d.Hosts)
	maxFakeTTL := uint32(s.Health.GraceSeconds)
	rt.dispatcher = &dnsserver.Dispatcher{SharedCounters: &a.dnsCounters, Local: rt.hosts, RealIPDomains: rt.adapter, FakeTTLMax: &maxFakeTTL, Classifier: rt.policy, Surge: &observedSurge{SurgeAdapter: rt.adapter, rt: rt}, Gate: rt.machine, FakeIssuer: rt.machine, History: dnsserver.NewHistory(s.Logging.HistoryCapacity, s.Logging.QueryHistory && s.Logging.LogDomains)}
	rt.previousQueries.Store(a.dnsCounters.Queries.Load())
	for _, p := range s.Surge.FakePrefixes {
		rt.dispatcher.FakePrefixes = append(rt.dispatcher.FakePrefixes, netip.MustParsePrefix(p))
	}
	parse := func(v string) netip.Addr { ip, _ := netip.ParseAddr(v); return ip }
	s4, s6 := parse(s.DNS.Source4), parse(s.DNS.Source6)
	for _, source := range []netip.Addr{s4, s6} {
		if source.IsValid() {
			rt.dispatcher.ForbiddenClients = append(rt.dispatcher.ForbiddenClients, source)
		}
	}
	if s.DNS.EgressInterface == "" {
		rt.fault = "Configure a pinned DNS egress interface/source before enabling DNS"
	} else {
		dial, e := resolver.NewPinnedDialer(s.DNS.EgressInterface, s4, s6, time.Duration(s.DNS.TimeoutMS)*time.Millisecond, s.DNS.SourcePortFirst, s.DNS.SourcePortLast)
		if e != nil {
			return nil, e
		}
		rt.wire, _ = resolver.NewWireTransport(dial, time.Duration(s.DNS.TimeoutMS)*time.Millisecond)
		rt.real, err = resolver.NewReal(rt.wire, s.DNS.Upstreams, s.DNS.PreserveOriginal, s.Cache.ResponseCapacity)
		if err != nil {
			return nil, err
		}
		rt.dispatcher.Real = &observedReal{real: rt.real, rt: rt}
		policyPools := 0
		for _, rule := range s.DNS.PolicyUpstreams {
			if rule.Enabled {
				policyPools++
				if rule.Fallback == "upstreams" {
					policyPools++
				}
			}
		}
		for _, rule := range s.DNS.PolicyUpstreams {
			if !rule.Enabled {
				continue
			}
			capacity := max(1, s.Cache.ResponseCapacity/max(1, policyPools))
			upstream, e := resolver.NewReal(rt.wire, rule.Upstreams, false, capacity)
			if e != nil {
				return nil, e
			}
			rt.policyResolvers = append(rt.policyResolvers, upstream)
			binding := dnsserver.PolicyDNSBinding{Field: rule.Field, Policy: rule.Policy, Resolver: &observedReal{real: upstream, rt: rt}, Fallback: rule.Fallback, AttemptTimeout: time.Duration(s.DNS.TimeoutMS) * time.Millisecond}
			if rule.Fallback == "upstreams" {
				backup, err := resolver.NewReal(rt.wire, rule.FallbackUpstreams, false, capacity)
				if err != nil {
					return nil, err
				}
				rt.policyResolvers = append(rt.policyResolvers, backup)
				binding.FallbackResolver = &observedReal{real: backup, rt: rt}
			}
			rt.dispatcher.PolicyDNS = append(rt.dispatcher.PolicyDNS, binding)
		}
		ingresses := []dnsserver.Ingress{}
		for _, b := range s.DNS.Listeners {
			ingresses = append(ingresses, dnsserver.Ingress{Bind: netip.MustParseAddrPort(b)})
		}
		if s.DNS.InterceptionEnabled {
			rt.pf, err = dnsserver.OpenPF()
			if err != nil {
				return nil, err
			}
			for _, b := range s.DNS.Transparent {
				ingresses = append(ingresses, dnsserver.Ingress{Bind: netip.MustParseAddrPort(b), Intercepted: true, Lookup: rt.pf})
			}
		}
		sc := dnsserver.DefaultServerConfig()
		sc.QueryTimeout = dnsQueryTimeout(s)
		sc.ResponseBarrier = rt.machine
		rt.server, err = dnsserver.Listen(ctx, ingresses, sc, func(ctx context.Context, q model.DNSQueryContext, msg *dns.Msg) *dns.Msg {
			begin := time.Now()
			reply := rt.dispatcher.Dispatch(ctx, q, msg)
			elapsed := time.Since(begin)
			rt.latencySum.Add(uint64(elapsed))
			for i, b := range []time.Duration{time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond, time.Second, time.Duration(1<<63 - 1)} {
				if elapsed <= b {
					rt.latency[i].Add(1)
				}
			}
			return reply
		})
		if err != nil {
			return nil, err
		}
		if s.DNS.InterceptionEnabled {
			pc := dnsserver.PFConfig{IngressInterfaces: s.DNS.IngressInterfaces, EgressInterface: s.DNS.EgressInterface, Source4: s4, Source6: s6, Gateway4: parse(s.DNS.Gateway4), Gateway6: parse(s.DNS.Gateway6), SourcePortFirst: s.DNS.SourcePortFirst, SourcePortLast: s.DNS.SourcePortLast}
			for _, r := range d.Interception {
				if r.Enabled {
					pc.Prefixes = append(pc.Prefixes, netip.MustParsePrefix(r.Prefix))
				}
			}
			for _, b := range s.DNS.Transparent {
				addr := netip.MustParseAddrPort(b)
				if addr.Addr().Is4() {
					pc.Listener4 = addr
				} else {
					pc.Listener6 = addr
				}
			}
			rt.pfLease, err = dnsserver.LoadPF(ctx, pc, a.dir)
			if err != nil {
				return nil, err
			}
		}
	}
	rt.routes = routing.New(ctx, d, func(severity, message string) { a.event(severity, "routing", message) })
	failed = false
	go a.monitor(ctx, rt)
	return rt, nil
}

// Each resolution stage receives its own bounded window so a timed-out primary
// cannot consume the backup's budget. Classification retains its existing limit.
func dnsQueryTimeout(s config.Settings) time.Duration {
	stages := 1
	for _, rule := range s.DNS.PolicyUpstreams {
		if rule.Enabled && rule.Fallback != "" && rule.Fallback != "none" {
			stages = 2
			break
		}
	}
	return time.Duration(stages*s.DNS.TimeoutMS+s.Cache.TimeoutMS) * time.Millisecond
}

type observedReal struct {
	real *resolver.Real
	rt   *Runtime
}

func (o *observedReal) Resolve(ctx context.Context, r model.ResolveRequest) (*dns.Msg, error) {
	o.rt.realQueries.Add(1)
	m, e := o.real.Resolve(ctx, r)
	if e != nil {
		o.rt.realErrors.Add(1)
	}
	return m, e
}

type observedSurge struct {
	model.SurgeAdapter
	rt *Runtime
}

func (o *observedSurge) ResolveFakeIP(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	o.rt.fakeQueries.Add(1)
	r, e := o.SurgeAdapter.ResolveFakeIP(ctx, m)
	if e != nil {
		o.rt.fakeErrors.Add(1)
	}
	return r, e
}

func (a *App) monitor(ctx context.Context, rt *Runtime) {
	defer close(rt.done)
	s := rt.settings.Settings
	interval := time.Duration(s.Health.IntervalMS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	previousState := ""
	rateAt := time.Now()
	lastPeers := ""
	for {
		begin := time.Now()
		probeCtx, cancel := context.WithTimeout(ctx, time.Duration(s.DNS.TimeoutMS)*time.Millisecond+time.Second)
		type probe struct {
			name    string
			ok      bool
			latency time.Duration
		}
		results := make(chan probe, 5)
		var wg sync.WaitGroup
		run := func(name string, fn func() bool) {
			wg.Add(1)
			go func() { defer wg.Done(); t := time.Now(); results <- probe{name, fn(), time.Since(t)} }()
		}
		run("Surge CLI", func() bool {
			snapshot, e := rt.adapter.Refresh(probeCtx)
			reason, restored := rt.observeDecisionRevision(snapshot, e)
			if reason != "" {
				a.event("INFO", "cache_invalidation", reason)
			}
			if restored > 0 {
				a.event("INFO", "cache_restore", fmt.Sprintf("已恢复 %d 条持久化决策缓存", restored))
			}
			if reason != "" || restored > 0 {
				if err := a.saveDecisionCache(); err != nil {
					slog.Error("Decision cache persistence failed", "error", err)
				}
			}
			return e == nil
		})
		run("Surge Fake-IP DNS", func() bool {
			q := new(dns.Msg)
			q.SetQuestion(dns.Fqdn(s.Health.ProbeDomain), dns.TypeA)
			r, e := rt.adapter.ResolveFakeIP(probeCtx, q)
			if e != nil || r == nil || r.Rcode != dns.RcodeSuccess {
				return false
			}
			found := false
			for _, rr := range r.Answer {
				if v, ok := rr.(*dns.A); ok {
					addr, _ := netip.AddrFromSlice(v.A)
					valid := false
					for _, p := range rt.dispatcher.FakePrefixes {
						valid = valid || p.Contains(addr.Unmap())
					}
					if !valid {
						return false
					}
					found = true
				}
			}
			return found
		})
		run("Real DNS", func() bool {
			if rt.wire == nil {
				return false
			}
			q := new(dns.Msg)
			q.SetQuestion(dns.Fqdn(s.Health.ProbeDomain), dns.TypeA)
			for _, ep := range s.DNS.Upstreams {
				if !ep.Enabled {
					continue
				}
				r, e := rt.wire.Exchange(probeCtx, ep, q)
				if e == nil && r != nil && !dnsserver.ContainsFakeAddress(r, rt.dispatcher.FakePrefixes) && (r.Rcode == dns.RcodeSuccess || r.Rcode == dns.RcodeNameError) {
					return true
				}
			}
			return false
		})
		run("Database", func() bool { return a.store.DB.PingContext(probeCtx) == nil })
		run("Surge API", func() bool {
			if s.Surge.APIURL == "" {
				return true
			}
			return surge.ProbeAPI(probeCtx, s.Surge.APIURL, s.Surge.APIToken)
		})
		wg.Wait()
		close(results)
		up := map[string]bool{}
		for p := range results {
			up[p.name] = rt.health.Observe(p.name, p.ok, p.latency, "component probe failed")
		}
		cancel()
		if s.Surge.APIURL == "" {
			rt.health.Disabled("Surge API")
		}
		// The optional API is not a prerequisite when CLI integration is used.
		// Disabled resets its health counters, so it cannot converge to UP.
		surgeReady := up["Surge CLI"] && (s.Surge.APIURL == "" || up["Surge API"])
		controller := rt.server != nil
		dnsReady := controller
		if rt.server != nil {
			select {
			case <-rt.server.Errors():
				dnsReady = false
				rt.SetFault("DNS listener failed")
			default:
			}
			if rt.Fault() != "" {
				dnsReady = false
			}
			if iface, e := net.InterfaceByName(s.DNS.EgressInterface); e != nil || iface.Flags&net.FlagUp == 0 {
				controller = false
			} else {
				addresses, _ := iface.Addrs()
				for _, source := range []string{s.DNS.Source4, s.DNS.Source6} {
					if source == "" {
						continue
					}
					assigned := false
					for _, value := range addresses {
						if prefix, e := netip.ParsePrefix(value.String()); e == nil && prefix.Addr().Unmap().String() == source {
							assigned = true
						}
					}
					if !assigned {
						controller = false
					}
				}
			}
		}
		rt.health.Observe("Controller", controller, time.Since(begin), "controller ingress/egress unavailable")
		rt.health.Observe("DNS listener", dnsReady, 0, "DNS listeners unavailable")
		rt.health.Observe("Transparent DNS", !s.DNS.InterceptionEnabled || dnsReady && rt.pfLease != nil, 0, "transparent listener/PF unavailable")
		if !s.DNS.InterceptionEnabled {
			rt.health.Disabled("Transparent DNS")
		}
		rt.routingMu.RLock()
		preCtx, preCancel := context.WithTimeout(ctx, 2*time.Second)
		preReady, _ := rt.routes.Refresh(preCtx)
		preCancel()
		plan := rt.machine.Step(time.Now(), state.Input{Controller: controller || rt.server == nil && rt.Fault() != "", DNS: dnsReady, Surge: surgeReady, FakeIP: up["Surge Fake-IP DNS"], Routes: preReady})
		routeCtx, routeCancel := context.WithTimeout(ctx, 2*time.Second)
		desired := rt.routes.Desired(controller && dnsReady && up["Database"], dnsReady && up["Real DNS"], plan.KeepSurgeRoutes)
		if !plan.KeepSurgeRoutes {
			rt.machine.DrainResponses()
		}
		e := rt.routes.Reconcile(routeCtx, desired)
		ready, re := rt.routes.Refresh(routeCtx)
		routeCancel()
		peers := rt.routes.Status()
		engineOK := peers.Enabled && e == nil && re == nil && peers.LastError == ""
		rt.health.Observe("BGP engine", engineOK, 0, "BGP disabled or not ready")
		rt.health.Observe("Fake-IP routing", ready, 0, "selected Fake-IP routing backend is not ready")
		if s.DNS.InterceptionEnabled {
			rt.health.Observe("DNS interception routing", peers.DNSInterceptionReady, 0, "selected DNS interception routing backend is not ready")
		} else {
			rt.health.Disabled("DNS interception routing")
		}
		if !peers.BGPRequested {
			rt.health.Disabled("BGP engine")
		}
		for _, p := range peers.Peers {
			rt.health.Observe("BGP peer "+p.Address, p.State == "ESTABLISHED", 0, "BGP session unavailable")
		}
		peerStates := []string{}
		for _, p := range peers.Peers {
			peerStates = append(peerStates, p.Address+"="+p.State)
		}
		sort.Strings(peerStates)
		peerJSON, _ := json.Marshal(peerStates)
		if string(peerJSON) != lastPeers {
			a.event("INFO", "bgp_peer", "BGP peer state changed")
			lastPeers = string(peerJSON)
		}
		plan = rt.machine.Step(time.Now(), state.Input{Controller: controller || rt.server == nil && rt.Fault() != "", DNS: dnsReady, Surge: surgeReady, FakeIP: up["Surge Fake-IP DNS"], Routes: ready})
		rt.routingMu.RUnlock()
		if plan.State != previousState {
			a.event("INFO", "system_state", "System state: "+plan.State)
			previousState = plan.State
		}
		if time.Since(rateAt) >= time.Minute {
			current := rt.dispatcher.Statistics().Queries.Load()
			previous := rt.previousQueries.Swap(current)
			rt.queryRate.Store(current - previous)
			rateAt = time.Now()
		}
		if a.notifications != nil && ctx.Err() == nil {
			document := a.Configuration()
			values := rt.health.Values()
			activePeers := map[string]bool{}
			for _, peer := range document.Peers {
				if peer.Enabled {
					activePeers["BGP peer "+peer.Address] = true
				}
			}
			filtered := values[:0]
			for _, value := range values {
				if strings.HasPrefix(value.Name, "BGP peer ") && (!document.Settings.Routing.UsesBGP() || !document.Settings.BGP.Enabled || !activePeers[value.Name]) {
					continue
				}
				filtered = append(filtered, value)
			}
			a.notifications.Observe(document.Settings.General.Name, document.Settings.Notifications, filtered)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-rt.routingWake:
		}
	}
}

func (rt *Runtime) close(ctx context.Context, waitTTL bool) error {
	rt.closeOnce.Do(func() { rt.closeErr = rt.shutdown(ctx, waitTTL) })
	return rt.closeErr
}
func (rt *Runtime) shutdown(ctx context.Context, waitTTL bool) error {
	expiry := rt.machine.Stop()
	rt.cancel()
	if rt.server != nil {
		rt.server.Close()
	}
	<-rt.done
	rt.machine.DrainResponses()
	if waitTTL && time.Now().Before(expiry) {
		// DNS listeners are now down: withdraw their Controller/DNS bindings
		// immediately while keeping previously issued Fake-IP paths until expiry.
		desired := rt.routes.Desired(false, false, true)
		_ = rt.routes.Reconcile(context.Background(), desired)
		// A disconnected management caller does not interrupt safety cleanup.
		timer := time.NewTimer(time.Until(expiry))
		<-timer.C
	}

	e := rt.routes.Close(context.Background())
	if rt.pfLease != nil {
		if pe := rt.pfLease.Close(); e == nil {
			e = pe
		}
	}
	if rt.pf != nil {
		_ = rt.pf.Close()
	}
	rt.engine.Close()
	return e
}
func (a *App) Apply(ctx context.Context, d config.Document) error {
	d = d.Clone()
	a.transition.Lock()
	defer a.transition.Unlock()
	if a.closing.Load() {
		return errors.New("Controller is stopping")
	}
	if e := d.Validate(); e != nil {
		a.event("CRITICAL", "configuration_rejected", "Configuration rejected by safety validation")
		return e
	}
	old := a.Configuration()
	if d.Settings.DNS.InterceptionEnabled {
		// Check access before stopping working DNS. Opening the state device does
		// not install PF rules; the normal build still owns the eventual lease.
		pf, e := dnsserver.OpenPF()
		if e != nil {
			if errors.Is(e, os.ErrPermission) {
				return errors.New("透明 DNS 需要管理员权限：请在状态栏菜单安装网络后台服务后重试；当前配置未更改")
			}
			return errors.New("无法访问系统 PF 设备，透明 DNS 未启用；当前配置未更改")
		}
		_ = pf.Close()
	}
	d.Settings.Routing = d.Settings.Routing.Effective()
	oldRouting := old.Settings.Routing.Effective()
	if !reflect.DeepEqual(old.Settings.Surge.FakePrefixes, d.Settings.Surge.FakePrefixes) || old.Settings.Surge.NextHop4 != d.Settings.Surge.NextHop4 || old.Settings.Surge.NextHop6 != d.Settings.Surge.NextHop6 || oldRouting.Mode != d.Settings.Routing.Mode || oldRouting.FakeIPBackend != d.Settings.Routing.FakeIPBackend {
		d.Settings.Routing.StaticFakeIPConfigured = false
	}
	if !reflect.DeepEqual(old.Interception, d.Interception) || old.Settings.DNS.Source4 != d.Settings.DNS.Source4 || old.Settings.DNS.Source6 != d.Settings.DNS.Source6 || oldRouting.Mode != d.Settings.Routing.Mode || oldRouting.DNSBackend != d.Settings.Routing.DNSBackend || oldRouting.StaticDNSInterception != d.Settings.Routing.StaticDNSInterception {
		d.Settings.Routing.StaticDNSConfigured = false
	}
	webChange := d.Settings.General.WebListen != old.Settings.General.WebListen || d.Settings.Security.TLSCert != old.Settings.Security.TLSCert || d.Settings.Security.TLSKey != old.Settings.Security.TLSKey
	if webChange {
		if d.Settings.Security.TLSCert != "" {
			if _, e := tls.LoadX509KeyPair(d.Settings.Security.TLSCert, d.Settings.Security.TLSKey); e != nil {
				return errors.New("management TLS certificate/key could not be loaded")
			}
		}
		if d.Settings.General.WebListen != old.Settings.General.WebListen && d.Settings.General.WebListen != managementRecoveryAddress {
			listener, e := net.Listen("tcp", d.Settings.General.WebListen)
			if e != nil {
				return errors.New("management address cannot be bound")
			}
			_ = listener.Close()
		}
	}
	routingChanged := old.Settings.Surge.NextHop4 != d.Settings.Surge.NextHop4 || old.Settings.Surge.NextHop6 != d.Settings.Surge.NextHop6 || !reflect.DeepEqual(old.Settings.Routing, d.Settings.Routing) || !reflect.DeepEqual(old.Settings.BGP, d.Settings.BGP) || !reflect.DeepEqual(old.Peers, d.Peers) || !reflect.DeepEqual(old.Routes, d.Routes)
	oldRuntime, newRuntime := old, d
	oldRuntime.Settings.RemoteBackends = nil
	newRuntime.Settings.RemoteBackends = nil
	oldRuntime.Settings.General = config.General{}
	newRuntime.Settings.General = config.General{}
	oldRuntime.Settings.Security = config.Security{}
	newRuntime.Settings.Security = config.Security{}
	oldRuntime.Settings.Notifications = config.Notifications{}
	newRuntime.Settings.Notifications = config.Notifications{}
	oldRuntime.Settings.Logging.Level = ""
	newRuntime.Settings.Logging.Level = ""
	oldRuntime.Settings.Logging.EventRetention = 0
	newRuntime.Settings.Logging.EventRetention = 0
	oldRuntime.Settings.Routing = config.Routing{}
	newRuntime.Settings.Routing = config.Routing{}
	oldRuntime.Settings.Surge.NextHop4 = ""
	oldRuntime.Settings.Surge.NextHop6 = ""
	newRuntime.Settings.Surge.NextHop4 = ""
	newRuntime.Settings.Surge.NextHop6 = ""
	oldRuntime.Settings.BGP = config.BGP{}
	newRuntime.Settings.BGP = config.BGP{}
	oldRuntime.Peers = nil
	newRuntime.Peers = nil
	oldRuntime.Routes = nil
	newRuntime.Routes = nil
	oldRuntime.RouteFeeds = nil
	newRuntime.RouteFeeds = nil
	oldRuntime.Hosts = nil
	newRuntime.Hosts = nil
	oldRuntime.Overrides = nil
	newRuntime.Overrides = nil
	peerOnlyRoutingChange := !reflect.DeepEqual(old.Peers, d.Peers) &&
		reflect.DeepEqual(old.Settings.BGP, d.Settings.BGP) &&
		reflect.DeepEqual(old.Settings.Routing, d.Settings.Routing) &&
		reflect.DeepEqual(old.Routes, d.Routes) &&
		old.Settings.Surge.NextHop4 == d.Settings.Surge.NextHop4 &&
		old.Settings.Surge.NextHop6 == d.Settings.Surge.NextHop6
	routesOnlyRoutingChange := !reflect.DeepEqual(old.Routes, d.Routes) &&
		reflect.DeepEqual(old.Peers, d.Peers) &&
		reflect.DeepEqual(old.Settings.BGP, d.Settings.BGP) &&
		reflect.DeepEqual(old.Settings.Routing, d.Settings.Routing) &&
		old.Settings.Surge.NextHop4 == d.Settings.Surge.NextHop4 &&
		old.Settings.Surge.NextHop6 == d.Settings.Surge.NextHop6
	if !reflect.DeepEqual(oldRuntime, newRuntime) {
		a.mu.RLock()
		previous := a.runtime
		a.mu.RUnlock()
		if e := previous.close(ctx, true); e != nil {
			return e
		}
		next, e := a.build(d)
		if e != nil {
			restored, re := a.build(old)
			if re == nil {
				a.mu.Lock()
				a.runtime = restored
				a.mu.Unlock()
				return fmt.Errorf("DNS 配置无法启用，已恢复原配置；原因：%v", e)
			}
			return fmt.Errorf("DNS 配置无法启用，恢复也失败；启用失败：%v；恢复失败：%v", e, re)
		}
		if e = a.store.Save(ctx, d); e != nil {
			_ = next.close(context.Background(), false)
			restored, _ := a.build(old)
			a.mu.Lock()
			a.runtime = restored
			a.mu.Unlock()
			return errors.New("configuration transaction failed")
		}
		a.mu.Lock()
		a.runtime = next
		a.config = d
		a.mu.Unlock()
	} else {
		if routingChanged {
			rt := a.runtime
			rt.routingMu.Lock()
			canSyncPeers := peerOnlyRoutingChange && (!d.Settings.BGP.Enabled || rt.routes.Status().Enabled)
			canUpdateRoutes := routesOnlyRoutingChange && (!d.Settings.BGP.Enabled || rt.routes.Status().Enabled)
			if canSyncPeers {
				if e := rt.routes.SyncPeers(ctx, d); e != nil {
					rt.routingMu.Unlock()
					return fmt.Errorf("BGP neighbor update failed: %w", e)
				}
				if e := a.store.Save(ctx, d); e != nil {
					_ = rt.routes.SyncPeers(context.Background(), old)
					rt.routingMu.Unlock()
					return errors.New("routing configuration transaction failed")
				}
				rt.routingMu.Unlock()
			} else if canUpdateRoutes {
				if e := a.store.Save(ctx, d); e != nil {
					rt.routingMu.Unlock()
					return errors.New("routing configuration transaction failed")
				}
				rt.routes.UpdateDocument(d)
				rt.routingMu.Unlock()
				select {
				case rt.routingWake <- struct{}{}:
				default:
				}
			} else {
				expiry := rt.machine.SuspendRouting()
				rt.machine.DrainResponses()
				if old.Settings.Routing.Effective().FakeIPBackend == "bgp" && time.Now().Before(expiry) {
					timer := time.NewTimer(time.Until(expiry))
					<-timer.C
				}
				if e := rt.routes.Close(context.Background()); e != nil {
					rt.routingMu.Unlock()
					return errors.New("old BGP backend could not be released; DNS remains running")
				}
				next := routing.New(a.ctx, d, func(severity, message string) { a.event(severity, "routing", message) })
				if e := a.store.Save(ctx, d); e != nil {
					next.Close(context.Background())
					restored := routing.New(a.ctx, old, nil)
					a.mu.Lock()
					rt.routes = restored
					a.mu.Unlock()
					rt.routingMu.Unlock()
					return errors.New("routing configuration transaction failed")
				}
				a.mu.Lock()
				rt.routes = next
				a.mu.Unlock()
				rt.routingMu.Unlock()
			}
		} else if e := a.store.Save(ctx, d); e != nil {
			return errors.New("configuration transaction failed")
		}
		a.mu.Lock()
		a.config = d
		hostsChanged := !reflect.DeepEqual(old.Hosts, d.Hosts)
		policyChanged := old.Settings.General.Mode != d.Settings.General.Mode || !reflect.DeepEqual(old.Overrides, d.Overrides)
		if hostsChanged || policyChanged {
			a.runtime.decisionMu.Lock()
			a.runtime.pendingDecisions = nil
			a.runtime.decisionScope = decisionCacheScope(d)
		}
		if hostsChanged {
			a.runtime.hosts.Update(d.Hosts)
		}
		if policyChanged {
			a.runtime.policy.update(d.Settings.General.Mode, d.Overrides)
		} else if hostsChanged {
			a.runtime.engine.Cache.FlushReason("本地 DNS 条目已更新")
		}
		if hostsChanged || policyChanged {
			a.runtime.decisionMu.Unlock()
		}
		if old.Settings.General.Mode != d.Settings.General.Mode {
			a.runtime.machine.SetMode(d.Settings.General.Mode)
		}
		a.mu.Unlock()
	}
	if !reflect.DeepEqual(old.Settings.Security, d.Settings.Security) {
		a.security.Update(d.Settings.Security)
	}
	a.eventRetention.Store(int64(d.Settings.Logging.EventRetention))
	a.setLogging(d.Settings.Logging.Level)
	if err := a.saveDecisionCache(); err != nil {
		slog.Error("Configuration applied, but decision cache persistence failed", "error", err)
	}
	a.event("INFO", "configuration", "Configuration applied")
	if webChange {
		select {
		case a.webChanged <- struct{}{}:
		default:
		}
	}
	return nil
}
func (a *App) View(ctx context.Context, key string) (any, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	rt := a.runtime
	if rt == nil {
		return nil, errors.New("runtime unavailable")
	}
	switch key {
	case "status":
		p := rt.machine.Plan()
		return map[string]any{"software": buildinfo.Current(), "name": a.config.Settings.General.Name, "mode": a.config.Settings.General.Mode, "state": p.State, "fake_ip_enabled": p.AllowFakeIP, "queries_per_minute": rt.queryRate.Load(), "counters": rt.dispatcher.Statistics().Snapshot(), "warning": rt.Fault(), "uptime_seconds": int(time.Since(rt.started).Seconds())}, nil
	case "health":
		values := rt.health.Values()
		sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
		return values, nil
	case "dns/queries":
		return rt.dispatcher.History.Records(), nil
	case "dns/cache":
		return a.cacheViewLocked(rt, 0, 1000, "", ""), nil
	case "surge/hosts":
		select {
		case a.ruleTests <- struct{}{}:
			defer func() { <-a.ruleTests }()
		default:
			return nil, errors.New("Surge reader capacity exhausted")
		}
		return surge.ReadHosts(ctx, rt.adapter.Runner)
	case "surge/status":
		snapshot, observed := rt.adapter.Snapshot()
		return map[string]any{"snapshot": snapshot, "observed_at": observed, "fresh": snapshot.Digest != "" && time.Since(observed) <= rt.adapter.MaxSnapshotAge}, nil
	case "bgp/status", "routing/status":
		return rt.routes.Status(), nil
	}
	return nil, errors.New("unknown view")
}
func (a *App) CacheView(ctx context.Context, offset, limit int, domain, decision string) (any, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cacheViewLocked(a.runtime, offset, limit, domain, decision), nil
}
func (a *App) cacheViewLocked(rt *Runtime, offset, limit int, domain, decision string) any {
	rt.decisionMu.Lock()
	pending := rt.pendingDecisions != nil
	rt.decisionMu.Unlock()
	rows, total := rt.engine.Cache.Page(offset, limit, func(d model.DecisionResult) bool {
		return (domain == "" || strings.Contains(d.Domain, strings.ToLower(domain))) && (decision == "" || string(d.Decision) == decision)
	})
	return map[string]any{"stats": rt.engine.Cache.Stats(), "entries": rows, "total": total, "offset": offset, "limit": limit, "capacity": rt.settings.Settings.Cache.Capacity, "ttl": rt.settings.Settings.Cache, "restoration_pending": pending}
}
func (a *App) Flush(ctx context.Context, domain string) error {
	a.transition.Lock()
	defer a.transition.Unlock()
	if a.closing.Load() {
		return errors.New("Controller is stopping")
	}
	if domain != "" {
		var err error
		domain, err = model.NormalizeDomain(domain)
		if err != nil {
			return err
		}
	}
	a.mu.RLock()
	rt := a.runtime
	rt.decisionMu.Lock()
	if domain == "" {
		rt.engine.Cache.FlushReason("用户手动清理全部决策缓存")
		rt.pendingDecisions = nil
	} else {
		if e := rt.engine.FlushDomain(domain); e != nil {
			rt.decisionMu.Unlock()
			a.mu.RUnlock()
			return e
		}
		if pending := rt.pendingDecisions; pending != nil {
			kept := make([]decisionRecord, 0, len(pending.Entries))
			for _, record := range pending.Entries {
				if record.Key.Domain != domain {
					kept = append(kept, record)
				}
			}
			pending.Entries = kept
		}
	}
	rt.decisionMu.Unlock()
	if rt.real != nil {
		rt.real.Cache.Flush()
	}
	for _, upstream := range rt.policyResolvers {
		upstream.Cache.Flush()
	}
	a.mu.RUnlock()
	if err := a.saveDecisionCache(); err != nil {
		return err
	}
	a.event("INFO", "cache_flush", "DNS caches invalidated")
	return nil
}
func (a *App) Test(ctx context.Context, body json.RawMessage) (any, error) {
	select {
	case a.ruleTests <- struct{}{}:
		defer func() { <-a.ruleTests }()
	default:
		return nil, errors.New("rule tester capacity exhausted")
	}
	var input struct {
		Domain   string `json:"domain"`
		ClientIP string `json:"client_ip"`
		Protocol string `json:"protocol"`
		Port     uint16 `json:"port"`
	}
	if json.Unmarshal(body, &input) != nil {
		return nil, errors.New("invalid rule test input")
	}
	domain, e := model.NormalizeDomain(input.Domain)
	client, ce := netip.ParseAddr(input.ClientIP)
	if e != nil || ce != nil || input.Port == 0 || (input.Protocol != "tcp" && input.Protocol != "udp") {
		return nil, errors.New("domain, client IP, TCP/UDP and valid port required")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	rt := a.runtime
	explain, e := rt.adapter.ExplainConnection(ctx, domain, client, input.Protocol, input.Port)
	if e != nil {
		return nil, e
	}
	decision, de := rt.policy.Classify(ctx, model.DNSQueryContext{ClientIP: client, Question: dns.Question{Name: domain, Qtype: dns.TypeA, Qclass: dns.ClassINET}})
	if de != nil {
		return nil, errors.New("DNS classification unavailable")
	}
	return map[string]any{"match": explain, "decision": decision, "dns_behavior": decision.Decision, "context": input, "fake_ip_allowed": rt.machine.Snapshot().AllowFakeIP}, nil
}

func (a *App) Metrics() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	rt := a.runtime
	if rt == nil {
		return ""
	}
	c := rt.dispatcher.Statistics().Snapshot()
	cache := rt.engine.Cache.Stats()
	bs := rt.routes.Status()
	values := map[string]uint64{"dns_queries_total": c.Queries, "dns_direct_total": c.Direct, "dns_surge_total": c.Surge, "dns_reject_total": c.Reject, "dns_intercepted_total": c.Intercepted, "dns_direct_addressed_total": c.Addressed, "decision_cache_hits_total": cache.Hits, "decision_cache_misses_total": cache.Misses, "fake_ip_queries_total": rt.fakeQueries.Load(), "fake_ip_errors_total": rt.fakeErrors.Load(), "real_dns_queries_total": rt.realQueries.Load(), "real_dns_errors_total": rt.realErrors.Load(), "bgp_routes_announced": uint64(bs.Advertised), "surge_rule_match_errors_total": rt.engine.RuleErrors.Load()}
	var b strings.Builder
	keys := []string{}
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, values[k])
	}
	for _, p := range bs.Peers {
		v := 0
		if p.State == "ESTABLISHED" {
			v = 1
		}
		fmt.Fprintf(&b, "bgp_session_up{peer=%q} %d\n", p.Address, v)
	}
	fmt.Fprintf(&b, "surge_rule_match_duration_seconds_sum %f\nsurge_rule_match_duration_seconds_count %d\n", float64(rt.engine.RuleDurationNS.Load())/float64(time.Second), rt.engine.RuleQueries.Load())
	current := rt.machine.Plan().State
	for _, s := range []string{state.Starting, state.Recovering, state.Healthy, state.SurgeDegraded, state.DirectFallback, state.Stopping} {
		v := 0
		if s == current {
			v = 1
		}
		fmt.Fprintf(&b, "controller_state{state=%q} %d\n", s, v)
	}
	for i, bound := range []string{"0.001", "0.005", "0.01", "0.05", "0.1", "0.5", "1", "+Inf"} {
		fmt.Fprintf(&b, "dns_query_duration_seconds_bucket{le=%q} %d\n", bound, rt.latency[i].Load())
	}
	fmt.Fprintf(&b, "dns_query_duration_seconds_sum %f\ndns_query_duration_seconds_count %d\n", float64(rt.latencySum.Load())/float64(time.Second), rt.latency[7].Load())
	return b.String()
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/management/relay/", a.remote)
	mux.Handle("/api/", a.control.Handler())
	mux.Handle("/metrics", a.control.Handler())
	mux.Handle("/", web.Handler())
	requests := make(chan struct{}, 128)
	bounded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- struct{}{}:
			defer func() { <-requests }()
			mux.ServeHTTP(w, r)
		default:
			http.Error(w, "management request capacity exhausted", 503)
		}
	})
	return a.security.Wrap(bounded)
}
func (rt *Runtime) Fault() string { rt.faultMu.RLock(); defer rt.faultMu.RUnlock(); return rt.fault }
func (rt *Runtime) SetFault(message string) {
	rt.faultMu.Lock()
	defer rt.faultMu.Unlock()
	rt.fault = message
}
func (a *App) Serve(ctx context.Context) error {
	previous := a.Configuration().Settings
	// Configuration applied before the initial listener needs no restart.
	select {
	case <-a.webChanged:
	default:
	}
	for {
		c := a.Configuration().Settings
		listener, e := net.Listen("tcp", c.General.WebListen)
		if e != nil {
			if c.General.WebListen == previous.General.WebListen {
				return e
			}
			a.transition.Lock()
			a.mu.Lock()
			a.config.Settings.General.WebListen = previous.General.WebListen
			a.config.Settings.Security.TLSCert = previous.Security.TLSCert
			a.config.Settings.Security.TLSKey = previous.Security.TLSKey
			fallback := a.config
			a.mu.Unlock()
			_ = a.store.Save(context.Background(), fallback)
			a.transition.Unlock()
			a.event("CRITICAL", "management_bind", "Management address failed; previous listener restored")
			continue
		}
		listener = netutil.LimitListener(listener, 256)
		server := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		result := make(chan error, 1)
		go func() {
			if c.Security.TLSCert != "" {
				result <- server.ServeTLS(listener, c.Security.TLSCert, c.Security.TLSKey)
			} else {
				result <- server.Serve(listener)
			}
		}()
		slog.Info("management ready", "address", c.General.WebListen)
		var recoveryListener net.Listener
		var recoveryServer *http.Server
		if c.General.WebListen != managementRecoveryAddress {
			recoveryListener, e = net.Listen("tcp", managementRecoveryAddress)
			if e != nil {
				slog.Warn("local management recovery listener unavailable", "address", managementRecoveryAddress, "error", e)
			} else {
				recoveryListener = netutil.LimitListener(recoveryListener, 64)
				recoveryServer = &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
				go func() {
					if serveErr := recoveryServer.Serve(recoveryListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
						slog.Error("local management recovery listener stopped", "error", serveErr)
					}
				}()
				slog.Info("local management recovery ready", "address", managementRecoveryAddress)
			}
		}
		restart := false
		select {
		case <-ctx.Done():
		case <-a.webChanged:
			restart = true
		case e = <-result:
			if errors.Is(e, http.ErrServerClosed) {
				e = nil
			}
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = server.Shutdown(shutdown)
		// Shutdown can run before the Serve goroutine registers the listener.
		// Close our listener explicitly so a fast restart cannot leak the port.
		_ = listener.Close()
		if recoveryServer != nil {
			_ = recoveryServer.Shutdown(shutdown)
			_ = recoveryListener.Close()
		}
		cancel()
		if !restart {
			return e
		}
		previous = c
	}
}
func (a *App) Close() error {
	a.closing.Store(true)
	if a.remote != nil {
		a.remote.Close()
	}
	a.transition.Lock()
	defer a.transition.Unlock()
	a.mu.Lock()
	rt := a.runtime
	a.mu.Unlock()
	var runtimeErr error
	if rt != nil {
		runtimeErr = rt.close(context.Background(), false)
	}
	// DNS workers have drained before the final snapshot is written. Join the
	// periodic writer so an older snapshot cannot overwrite this final value.
	var statsErr error
	if a.statsCancel != nil {
		a.statsCancel()
		<-a.statsDone
		statsErr = a.saveDNSStatistics(a.dnsCounters.Snapshot())
	}
	cacheErr := a.saveDecisionCache()
	if a.notificationCancel != nil {
		a.notificationCancel()
		<-a.notificationDone
	}
	if runtimeErr != nil {
		a.event("CRITICAL", "shutdown_cleanup", "Controller stopped; managed route/PF cleanup requires inspection")
	} else {
		a.event("INFO", "shutdown", "Controller stopped; managed routing resources released; external static routes unchanged")
	}
	e := errors.Join(runtimeErr, statsErr, cacheErr, a.store.Close())
	if a.lockFile != nil {
		_ = syscall.Flock(int(a.lockFile.Fd()), syscall.LOCK_UN)
		_ = a.lockFile.Close()
	}
	return e
}
func (a *App) setLogging(level string) {
	v := slog.LevelInfo
	switch level {
	case "debug":
		v = slog.LevelDebug
	case "warn":
		v = slog.LevelWarn
	case "error":
		v = slog.LevelError
	}
	a.logLevel.Set(v)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &a.logLevel})))
}
func Run(ctx context.Context, path string, listen ...string) (err error) {
	absolute, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	a, e := Open(ctx, absolute)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, a.Close()) }()
	if len(listen) > 0 && listen[0] != "" {
		d := a.Configuration()
		d.Settings.General.WebListen = listen[0]
		if e = a.Apply(ctx, d); e != nil {
			return e
		}
	}
	return a.Serve(ctx)
}
