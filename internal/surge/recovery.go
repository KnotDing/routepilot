package surge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"surge-controller/internal/config"
	"sync"
	"time"
)

type failure struct {
	Count int
	Since time.Time
}
type RecoveryStatus struct {
	Running          bool      `json:"running"`
	LastAttempt      time.Time `json:"last_attempt"`
	LastError        string    `json:"last_error,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	AttemptsLastHour int       `json:"attempts_last_hour"`
}

// Owned by App, so configuration/runtime rebuilds cannot reset the restart
// budget. Reserve an attempt before running the command; failures consume it too.
type Recovery struct {
	mu            sync.Mutex
	configuration config.SurgeRestart
	failures      map[string]failure
	attempts      []time.Time
	status        RecoveryStatus
}

func (r *Recovery) Take(now, started time.Time, settings config.SurgeRestart, checks map[string]bool) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := settings.Effective()
	if !reflect.DeepEqual(r.configuration, s) {
		r.failures = nil
		r.configuration = s
	}
	if !s.Enabled {
		r.failures = nil
		return ""
	}
	if r.failures == nil {
		r.failures = map[string]failure{}
	}
	for key := range r.failures {
		if _, ok := checks[key]; !ok {
			delete(r.failures, key)
		}
	}
	var reasons []string
	for key, ok := range checks {
		if ok {
			delete(r.failures, key)
			continue
		}
		f := r.failures[key]
		if f.Count == 0 {
			f.Since = now
		}
		f.Count++
		r.failures[key] = f
		if f.Count >= s.Failures && now.Sub(f.Since) >= time.Duration(s.DurationSeconds)*time.Second {
			reasons = append(reasons, key)
		}
	}
	r.prune(now)
	if r.status.Running || now.Sub(started) < time.Duration(s.StartupGraceSeconds)*time.Second || len(reasons) == 0 || len(r.attempts) >= s.MaxPerHour || !r.status.LastAttempt.IsZero() && now.Sub(r.status.LastAttempt) < time.Duration(s.CooldownSeconds)*time.Second {
		return ""
	}
	sort.Strings(reasons)
	reason := strings.Join(reasons, ", ")
	r.attempts = append(r.attempts, now)
	r.status = RecoveryStatus{Running: true, LastAttempt: now, Reason: reason, AttemptsLastHour: len(r.attempts)}
	r.failures = map[string]failure{}
	return reason
}
func (r *Recovery) Complete(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Running = false
	if err != nil {
		r.status.LastError = err.Error()
	}
}
func (r *Recovery) prune(now time.Time) {
	i := 0
	for i < len(r.attempts) && now.Sub(r.attempts[i]) >= time.Hour {
		i++
	}
	r.attempts = r.attempts[i:]
	r.status.AttemptsLastHour = len(r.attempts)
}
func (r *Recovery) Status() RecoveryStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prune(time.Now())
	return r.status
}

// HEAD probes exercise the configured policy/group through Surge itself.
// Only a successful HTTP status proves availability; malformed data fails closed.
func ProbePolicy(ctx context.Context, runner Runner, url, policy string) (time.Duration, error) {
	runner = recoveryRunner(runner, 15*time.Second)
	raw, err := runner.Run(ctx, "http", "probe", url, policy)
	if err != nil {
		return 0, err
	}
	var result struct {
		Status   int     `json:"status"`
		Duration float64 `json:"duration-ms"`
		Error    string  `json:"error"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Error != "" || result.Status < 200 || result.Status >= 400 {
		return 0, errors.New("Surge policy health probe failed")
	}
	if result.Duration < 0 || result.Duration > 60000 {
		return 0, errors.New("invalid Surge policy probe duration")
	}
	return time.Duration(result.Duration * float64(time.Millisecond)), nil
}
func RestartEngine(ctx context.Context, runner Runner) error {
	_, err := recoveryRunner(runner, 15*time.Second).Run(ctx, "restart-engine")
	return err
}

// Recovery commands need more time than DNS-time classification. Copy the CLI
// so extending this timeout never mutates concurrent classifier calls.
func recoveryRunner(runner Runner, timeout time.Duration) Runner {
	if cli, ok := runner.(*CLI); ok {
		copy := *cli
		copy.Timeout = timeout
		return &copy
	}
	return runner
}
