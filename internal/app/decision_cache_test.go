package app

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	"net/netip"
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
