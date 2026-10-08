package classifier_test

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/classifier"
	"surge-controller/internal/model"
	"surge-controller/tests/mocks"
)

func query() model.DNSQueryContext {
	return model.DNSQueryContext{ClientIP: netip.MustParseAddr("192.0.2.10"), Question: dns.Question{Name: "Example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}}
}

func TestStableDecisionsPersistButUnknownAndErrorsRetry(t *testing.T) {
	for _, tc := range []struct {
		policy     string
		err        error
		persistent bool
	}{
		{"DIRECT", nil, true}, {"Proxy", nil, true}, {"REJECT", nil, true}, {"", nil, false}, {"DIRECT", errors.New("failed"), false},
	} {
		mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
			return model.RuleMatchResult{FinalPolicy: tc.policy, ContextSafe: true}, tc.err
		}}
		for _, expiry := range []bool{false, true} {
			cfg := classifier.DefaultConfig()
			cfg.ExpireDecisions = expiry
			e, err := classifier.New(context.Background(), mock, cfg)
			if err != nil {
				t.Fatal(err)
			}
			d, _ := e.Classify(context.Background(), query())
			e.Close()
			want := tc.persistent && !expiry
			if d.Persistent != want || d.ExpiresAt.IsZero() != want {
				t.Fatalf("%s expiry=%v: wrong lifetime %+v", tc.policy, expiry, d)
			}
		}
	}
}
func TestConservativeDecisions(t *testing.T) {
	for _, tc := range []struct {
		name, policy string
		safe         bool
		err          error
		want         model.Decision
	}{{"direct", "DIRECT", true, nil, model.Direct}, {"proxy", "Proxy", true, nil, model.Surge}, {"unknown", "", false, nil, model.Unknown}, {"ambiguous", "DIRECT", false, nil, model.Unknown}, {"error", "DIRECT", true, errors.New("failed"), model.Unknown}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
				return model.RuleMatchResult{FinalPolicy: tc.policy, ContextSafe: tc.safe}, tc.err
			}}
			e, err := classifier.New(context.Background(), mock, classifier.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			d, _ := e.Classify(context.Background(), query())
			if d.Decision != tc.want {
				t.Fatalf("got %s want %s", d.Decision, tc.want)
			}
		})
	}
}
func TestTimeoutAndClientIsolation(t *testing.T) {
	mock := &mocks.Surge{Match: func(ctx context.Context, r model.RuleMatchRequest) (model.RuleMatchResult, error) {
		<-ctx.Done()
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, ctx.Err()
	}}
	cfg := classifier.DefaultConfig()
	cfg.Timeout = 10 * time.Millisecond
	e, _ := classifier.New(context.Background(), mock, cfg)
	defer e.Close()
	d, _ := e.Classify(context.Background(), query())
	if d.Decision == model.Direct {
		t.Fatal("timeout became DIRECT")
	}
	q := query()
	q.ClientIP = netip.MustParseAddr("192.0.2.11")
	_, _ = e.Classify(context.Background(), q)
	if mock.Calls.Load() != 2 {
		t.Fatal("clients shared a decision")
	}
}
func TestSingleflightAndFlush(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	mock := &mocks.Surge{Match: func(ctx context.Context, _ model.RuleMatchRequest) (model.RuleMatchResult, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return model.RuleMatchResult{}, ctx.Err()
		}
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	}}
	e, _ := classifier.New(context.Background(), mock, classifier.DefaultConfig())
	defer e.Close()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, _ = e.Classify(context.Background(), query()) }()
	}
	close(start)
	<-entered
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if mock.Calls.Load() != 1 {
		t.Fatalf("singleflight evaluated %d times", mock.Calls.Load())
	}
	d, _ := e.Classify(context.Background(), query())
	if d.Source != "cache" {
		t.Fatal("not cached")
	}
	if err := e.FlushDomain("EXAMPLE.com"); err != nil {
		t.Fatal(err)
	}
	_, _ = e.Classify(context.Background(), query())
	if mock.Calls.Load() != 2 {
		t.Fatal("domain flush missed")
	}
}
func TestFlushRejectsInFlightDirect(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		close(started)
		<-release
		return model.RuleMatchResult{FinalPolicy: "DIRECT", ContextSafe: true}, nil
	}}
	e, _ := classifier.New(context.Background(), mock, classifier.DefaultConfig())
	defer e.Close()
	done := make(chan model.DecisionResult)
	go func() { d, _ := e.Classify(context.Background(), query()); done <- d }()
	<-started
	e.Cache.Flush()
	close(release)
	d := <-done
	if d.Decision == model.Direct {
		t.Fatal("stale in-flight DIRECT escaped generation guard")
	}
	if e.Cache.Stats().Entries != 0 {
		t.Fatal("stale decision cached")
	}
}

func TestConfiguredDirectDoesNotInventSuccessfulEvaluation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		trust      bool
		match      model.RuleMatchResult
		err        error
		want       model.Decision
		confidence model.Confidence
	}{
		{"conservative-default", false, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct"}, nil, model.Unknown, model.Uncertain},
		{"configured-direct", true, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct", SafetyReason: "requires connection context"}, nil, model.Direct, model.Configured},
		{"confirmed-direct", true, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct", ContextSafe: true}, nil, model.Direct, model.Certain},
		{"named-proxy", true, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "proxy"}, nil, model.Unknown, model.Uncertain},
		{"chain", true, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct", UnderlyingChain: []string{"Proxy"}}, nil, model.Unknown, model.Uncertain},
		{"error", true, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct"}, errors.New("failed"), model.Unknown, model.Uncertain},
		{"deadline", true, model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct"}, context.DeadlineExceeded, model.Unknown, model.Uncertain},
		{"proxy", true, model.RuleMatchResult{FinalPolicy: "Proxy", FinalType: "proxy"}, nil, model.Surge, model.Uncertain},
		{"empty", true, model.RuleMatchResult{}, nil, model.Unknown, model.Uncertain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) { return tc.match, tc.err }}
			cfg := classifier.DefaultConfig()
			cfg.TrustDirect = tc.trust
			e, err := classifier.New(context.Background(), mock, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			d, _ := e.Classify(context.Background(), query())
			if d.Decision != tc.want || d.Confidence != tc.confidence {
				t.Fatalf("unexpected result: %+v", d)
			}
			if d.Confidence == model.Configured && !strings.Contains(d.Reason, "按配置采用") {
				t.Fatal("missing explicit configuration reason")
			}
			cached, _ := e.Classify(context.Background(), query())
			if cached.Source != "cache" || cached.Confidence != d.Confidence {
				t.Fatal("cache lost decision provenance")
			}
		})
	}
}

func TestRuleDNSFailureCannotBecomeConfiguredDirectAndRetainsTrace(t *testing.T) {
	trace := &model.RuleDNSResult{State: "failed", LookupPerformed: true, Reason: "Surge rule DNS lookup failed"}
	mock := &mocks.Surge{Match: func(context.Context, model.RuleMatchRequest) (model.RuleMatchResult, error) {
		return model.RuleMatchResult{FinalPolicy: "DIRECT", FinalType: "direct", RuleDNS: trace, Notes: []string{"Rule evaluating requires DNS lookup"}}, errors.New("lookup failed")
	}}
	cfg := classifier.DefaultConfig()
	cfg.TrustDirect = true
	e, err := classifier.New(context.Background(), mock, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	d, err := e.Classify(context.Background(), query())
	if err == nil || d.Decision != model.Unknown || d.Persistent || d.RuleDNS == nil || d.RuleDNS.State != "failed" || d.Reason != trace.Reason || len(d.RuleNotes) != 1 {
		t.Fatalf("failed DNS classified as a successful policy or lost diagnostics: %+v %v", d, err)
	}
	cached, _ := e.Classify(context.Background(), query())
	if cached.Source != "cache" || cached.RuleDNS == nil || cached.RuleDNS.State != "failed" || len(cached.RuleNotes) != 1 {
		t.Fatalf("cached failure lost its DNS trace: %+v", cached)
	}
}
