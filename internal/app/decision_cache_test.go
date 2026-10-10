package app

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	"net/netip"
	"path/filepath"
	"strings"
	"surge-controller/internal/cache"
	"surge-controller/internal/classifier"
	"surge-controller/internal/config"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
	"surge-controller/internal/surge"
	"surge-controller/tests/mocks"
	"testing"
	"time"
)

func TestOptimisticRevisionRetainsAndPersistsPendingRefresh(t *testing.T) {
	for _, optimistic := range []bool{false, true} {
		d := config.Default()
		d.Settings.Cache.Optimistic = optimistic
		engine, err := classifier.New(context.Background(), &mocks.Surge{}, classifier.DefaultConfig())
		if err != nil {
			t.Fatal(err)
		}
		rt := &Runtime{engine: engine, settings: d, decisionRevision: decisionRevision{digest: "old", selection: "node"}, decisionScope: decisionCacheScope(d)}
		key := classifier.Key{Domain: "example.com.", ClientGroup: "192.0.2.1", QueryClass: "1/1"}
		value := model.DecisionResult{Domain: key.Domain, QueryType: 1, ClientIP: netip.MustParseAddr(key.ClientGroup), Decision: model.Direct, Persistent: true, CreatedAt: time.Now()}
		engine.Cache.Put(key, value, engine.Cache.Generation(), cache.Forever)
		reason, _ := rt.observeDecisionRevision(surge.Snapshot{Digest: "new", SelectionDigest: "node"}, nil)
		_, found, stale, _ := engine.Cache.GetState(key)
		if found != optimistic || stale != optimistic || strings.Contains(reason, "乐观缓存") != optimistic {
			t.Fatal("incorrect configuration update behavior", optimistic, found, stale, reason)
		}
		saved := rt.decisionSnapshot()
		if saved.Digest != "new" || (len(saved.Entries) == 1) != optimistic || optimistic && !saved.Entries[0].Stale {
			t.Fatal("pending refresh not persisted", saved)
		}
		engine.Close()
		if !optimistic {
			continue
		}
		next, _ := classifier.New(context.Background(), &mocks.Surge{}, classifier.DefaultConfig())
		restarted := &Runtime{engine: next, settings: d, pendingDecisions: &saved, decisionScope: rt.decisionScope}
		restarted.observeDecisionRevision(surge.Snapshot{}, errors.New("temporary Surge failure"))
		if restarted.pendingDecisions == nil || next.Cache.Stats().Entries != 0 {
			t.Fatal("startup failure discarded or served unconfirmed rows")
		}
		_, restored := restarted.observeDecisionRevision(surge.Snapshot{Digest: "newer", SelectionDigest: "node"}, nil)
		_, found, stale, _ = next.Cache.GetState(key)
		if restored != 1 || !found || !stale {
			t.Fatal("confirmed update after restart discarded optimistic rows")
		}
		next.Close()
	}
}

func TestOptimisticOptionDoesNotChangeDecisionScope(t *testing.T) {
	d := config.Default()
	before := decisionCacheScope(d)
	d.Settings.Cache.Optimistic = true
	if decisionCacheScope(d) != before {
		t.Fatal("enabling optimistic cache discards decisions")
	}
}

func TestOptimisticDecisionAndResponseOptionsAreIndependent(t *testing.T) {
	for _, decisions := range []bool{false, true} {
		for _, responses := range []bool{false, true} {
			d := config.Default()
			d.Settings.Cache.Optimistic = decisions
			d.Settings.Cache.OptimisticResponses = responses
			engine, _ := classifier.New(context.Background(), &mocks.Surge{}, classifier.DefaultConfig())
			tr := &mocks.Transport{Func: func(_ context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
				return mocks.AddressReply(q, "203.0.113.1"), nil
			}}
			real, _ := resolver.NewReal(tr, []resolver.ResolverEndpoint{{Enabled: true, Protocol: "udp", Address: "192.0.2.53:53"}}, false, 10, resolver.OptimisticOptions{Enabled: responses})
			q := new(dns.Msg)
			q.SetQuestion("example.com.", dns.TypeA)
			real.Resolve(context.Background(), model.ResolveRequest{Message: q})
			key := classifier.Key{Domain: "example.com.", ClientGroup: "192.0.2.1", QueryClass: "1/1"}
			engine.Cache.Put(key, model.DecisionResult{Decision: model.Direct}, engine.Cache.Generation(), cache.Forever)
			rt := &Runtime{engine: engine, real: real, settings: d, decisionRevision: decisionRevision{digest: "old", selection: "node"}}
			rt.observeDecisionRevision(surge.Snapshot{Digest: "new", SelectionDigest: "node"}, nil)
			if (engine.Cache.Stats().StaleEntries == 1) != decisions || (real.Cache.Stats().StaleEntries == 1) != responses {
				t.Fatal("optimistic options not independent", decisions, responses)
			}
			engine.Close()
			real.Close()
		}
	}
}

func TestDecisionsSurviveUnrelatedSavesDNSRebuildAndDiskRestart(t *testing.T) {
	a := fixtureApp(t)
	key := classifier.Key{Domain: "persist.example.", ClientGroup: "192.0.2.10", QueryClass: "1/1"}
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("decision cache did not reach expected state")
	}
	wait(func() bool { return a.runtime.decisionSnapshot().Digest != "" })
	engine := a.runtime.engine
	value := model.DecisionResult{Domain: key.Domain, QueryType: 1, ClientIP: netip.MustParseAddr(key.ClientGroup), Decision: model.Direct, Persistent: true, CreatedAt: time.Now()}
	if !engine.Cache.Put(key, value, engine.Cache.Generation(), cache.Forever) {
		t.Fatal("could not seed cache")
	}
	for _, change := range []string{"theme", "aaaa", "dns-timeout"} {
		d := a.Configuration()
		switch change {
		case "theme":
			d.Settings.General.Theme = "dark"
		case "aaaa":
			d.Settings.DNS.AAAAFallback = "block"
		case "dns-timeout":
			d.Settings.DNS.TimeoutMS += 100
		}
		if err := a.Apply(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		wait(func() bool { _, found := a.runtime.engine.Cache.Get(key); return found })
		if change != "dns-timeout" && a.runtime.engine != engine {
			t.Fatal("hot change rebuilt the classifier", change)
		}
		if change == "dns-timeout" && a.runtime.engine == engine {
			t.Fatal("test did not exercise a DNS rebuild")
		}
	}
	path := filepath.Join(a.dir, "c.sqlite")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	wait(func() bool { _, found := reopened.runtime.engine.Cache.Get(key); return found })
	if got, _ := reopened.runtime.engine.Cache.Get(key); !got.Persistent || !got.CreatedAt.Equal(value.CreatedAt) {
		t.Fatal("restart changed saved decision", got)
	}
	if err := reopened.Flush(context.Background(), key.Domain); err != nil {
		t.Fatal(err)
	}
	var saved decisionCacheSnapshot
	if err := reopened.store.LoadDecisionCache(context.Background(), &saved); err != nil || len(saved.Entries) != 0 {
		t.Fatal("manual deletion was not durable", saved, err)
	}
}

func TestRetainedDecisionsRejectMismatchedAndExpiredRecords(t *testing.T) {
	key := classifier.Key{Domain: "example.com.", ClientGroup: "192.0.2.1", QueryClass: "1/1"}
	value := model.DecisionResult{Domain: key.Domain, QueryType: 1, ClientIP: netip.MustParseAddr(key.ClientGroup), Decision: model.Direct, Persistent: true}
	base := decisionRecord{Key: key, Value: value}
	if len(retainedDecisions([]decisionRecord{base}, false)) != 1 {
		t.Fatal("valid persistent decision discarded")
	}
	for _, mutate := range []func(*decisionRecord){
		func(r *decisionRecord) { r.Value.Decision = model.Unknown },
		func(r *decisionRecord) { r.Key.Domain = "EXAMPLE.com." },
		func(r *decisionRecord) { r.Value.Domain = "other.test." },
		func(r *decisionRecord) { r.Key.ClientGroup = "192.0.2.2" },
		func(r *decisionRecord) { r.Key.QueryClass = "28/1" },
		func(r *decisionRecord) { r.Value.QueryType = 0 },
		func(r *decisionRecord) { r.Value.Persistent = false },
		func(r *decisionRecord) { r.ExpiresAt = time.Now().Add(time.Hour) },
	} {
		r := base
		mutate(&r)
		if len(retainedDecisions([]decisionRecord{r}, false)) != 0 {
			t.Fatal("invalid decision retained", r)
		}
	}
	timed := base
	timed.Value.Persistent = false
	timed.ExpiresAt = time.Now().Add(time.Minute)
	if len(retainedDecisions([]decisionRecord{timed}, true)) != 1 {
		t.Fatal("valid timed decision discarded")
	}
	for _, deadline := range []time.Time{{}, time.Now().Add(-time.Second)} {
		timed.ExpiresAt = deadline
		if len(retainedDecisions([]decisionRecord{timed}, true)) != 0 {
			t.Fatal("expired or permanent decision restored as timed")
		}
	}
}
