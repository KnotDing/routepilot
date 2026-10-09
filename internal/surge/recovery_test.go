package surge

import (
	"context"
	"errors"
	"reflect"
	"surge-controller/internal/config"
	"testing"
	"time"
)

func restartFixture() config.SurgeRestart {
	return config.SurgeRestart{Enabled: true, DNS: true, Failures: 2, DurationSeconds: 5, StartupGraceSeconds: 10, CooldownSeconds: 30, MaxPerHour: 2}
}
func TestRecoveryToleranceAndBudget(t *testing.T) {
	var r Recovery
	s := restartFixture()
	started := time.Now()
	bad := map[string]bool{"DNS": false}
	if r.Take(started, started, s, bad) != "" || r.Take(started.Add(6*time.Second), started, s, bad) != "" {
		t.Fatal("startup grace ignored")
	}
	if r.Take(started.Add(10*time.Second), started, s, bad) != "DNS" {
		t.Fatal("sustained failure not restarted")
	}
	if r.Take(started.Add(11*time.Second), started, s, bad) != "" {
		t.Fatal("overlapping restart")
	}
	r.Complete(errors.New("fixture"))
	if r.Status().LastError != "fixture" {
		t.Fatal("error lost")
	}
	if r.Take(started.Add(20*time.Second), started, s, bad) != "" {
		t.Fatal("cooldown ignored")
	}
	if r.Take(started.Add(41*time.Second), started, s, bad) != "DNS" {
		t.Fatal("second attempt missing")
	}
	r.Complete(nil)
	if r.Take(started.Add(90*time.Second), started, s, bad) != "" {
		t.Fatal("hourly limit ignored")
	}
	if r.Take(started.Add(2*time.Hour), started, s, bad) != "DNS" {
		t.Fatal("hourly budget did not recover")
	}
}
func TestRecoverySeparateConditionsAndDisable(t *testing.T) {
	var r Recovery
	s := restartFixture()
	started := time.Now().Add(-time.Minute)
	now := time.Now()
	r.Take(now, started, s, map[string]bool{"API": false, "DNS": true})
	if r.Take(now.Add(6*time.Second), started, s, map[string]bool{"API": true, "DNS": false}) != "" {
		t.Fatal("different failures combined")
	}
	s.Enabled = false
	r.Take(now.Add(7*time.Second), started, s, map[string]bool{"DNS": false})
	s.Enabled = true
	if r.Take(now.Add(12*time.Second), started, s, map[string]bool{"DNS": false}) != "" {
		t.Fatal("disable retained failures")
	}
	if r.Take(now.Add(18*time.Second), started, s, map[string]bool{"DNS": false}) != "DNS" {
		t.Fatal("failure not detected")
	}
}

type probeRunner struct {
	args []string
	raw  string
	err  error
}

func (r *probeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	r.args = args
	return []byte(r.raw), r.err
}
func TestPolicyProbeAndRestartCommands(t *testing.T) {
	r := &probeRunner{raw: `{"status":200,"duration-ms":80.5}`}
	latency, err := ProbePolicy(context.Background(), r, "https://example.test", "group ; literal")
	if err != nil || latency != 80500*time.Microsecond || !reflect.DeepEqual(r.args, []string{"http", "probe", "https://example.test", "group ; literal"}) {
		t.Fatal(latency, err, r.args)
	}
	for _, raw := range []string{`{}`, `invalid`, `{"status":500}`, `{"status":200,"error":"failed"}`, `{"status":200,"duration-ms":-1}`} {
		r.raw = raw
		if _, err = ProbePolicy(context.Background(), r, "https://example.test", "Node"); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	r.raw = `{}`
	if RestartEngine(context.Background(), r) != nil || !reflect.DeepEqual(r.args, []string{"restart-engine"}) {
		t.Fatal("unsafe restart command", r.args)
	}
	r.err = errors.New("unavailable")
	if RestartEngine(context.Background(), r) == nil {
		t.Fatal("restart error hidden")
	}
}

func TestRecoveryNewProbeURLDoesNotReuseOldFailures(t *testing.T) {
	var r Recovery
	s := restartFixture()
	now := time.Now()
	started := now.Add(-time.Minute)
	r.Take(now, started, s, map[string]bool{"Node": false})
	s.ProbeURL = "https://new.example.test"
	if r.Take(now.Add(6*time.Second), started, s, map[string]bool{"Node": false}) != "" {
		t.Fatal("new probe URL reused old failure history")
	}
	if r.Take(now.Add(12*time.Second), started, s, map[string]bool{"Node": false}) != "Node" {
		t.Fatal("new failure series ignored")
	}
}
func TestRecoveryCLIUsesIndependentTimeout(t *testing.T) {
	cli := &CLI{Path: "/fixture", Timeout: 500 * time.Millisecond, UserName: "fixture"}
	copy := recoveryRunner(cli, 15*time.Second).(*CLI)
	if copy == cli || copy.Timeout != 15*time.Second || cli.Timeout != 500*time.Millisecond || copy.UserName != "fixture" {
		t.Fatal("recovery changed live classification settings")
	}
}
