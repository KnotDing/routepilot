package surge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"surge-controller/internal/model"
)

var errRuleDNS = errors.New("Surge rule DNS resolution failed")

type ruleDNSNotes struct {
	required, completed bool
	failure             string
}

func inspectRuleDNS(notes []string) ruleDNSNotes {
	var state ruleDNSNotes
	for _, note := range notes {
		lower := strings.ToLower(strings.TrimSpace(note))
		if strings.Contains(lower, "requires dns") || strings.Contains(lower, "requires a dns") {
			state.required = true
		}
		if strings.HasPrefix(lower, "dns completed") {
			state.completed = true
			state.failure = ""
			if strings.Contains(lower, "ipv4 address for matching: n/a") && strings.Contains(lower, "ipv6 address for matching: n/a") {
				state.failure = "DNS completed without usable IP addresses"
			}
		}
		for _, phrase := range []string{"dns lookup failed", "dns resolution failed", "dns query failed", "dns request failed", "dns failed", "dns error", "dns timeout", "dns lookup timed out", "dns resolution timed out", "failed to resolve"} {
			if strings.Contains(lower, phrase) {
				state.failure = note
				break
			}
		}
	}
	return state
}

// Surge normally resolves IP-dependent rules during explain itself. Only a
// pending/failed resolution needs an explicit lookup. Use Surge's internal DNS
// client so the subsequent evaluation sees its cache and DNS mapping rules;
// querying the Fake-IP listener or an unrelated resolver cannot achieve that.
// All three operations share the caller's deadline, with at most one retry.
func (a *Adapter) explainWithDNS(ctx context.Context, domain string, args []string) (model.RuleMatchResult, error) {
	if cli, ok := a.Runner.(*CLI); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cli.Timeout)
		defer cancel()
	}
	read := func() (model.RuleMatchResult, error) {
		raw, err := a.Runner.Run(ctx, args...)
		if err != nil {
			return model.RuleMatchResult{}, err
		}
		var m model.RuleMatchResult
		err = json.Unmarshal(raw, &m)
		return m, err
	}
	m, err := read()
	if err != nil {
		return m, err
	}
	state := inspectRuleDNS(m.Notes)
	if state.required || state.completed || state.failure != "" {
		m.RuleDNS = &model.RuleDNSResult{State: "completed"}
		if state.failure != "" || state.required && !state.completed {
			trace := &model.RuleDNSResult{State: "failed", LookupPerformed: true}
			m.RuleDNS = trace
			if _, err = a.Runner.Run(ctx, "dns", "lookup", domain); err != nil {
				trace.Reason = "Surge rule DNS lookup failed"
				if ctx.Err() != nil {
					trace.Reason = "Surge rule DNS lookup timed out or was canceled"
				}
				m.SafetyReason = trace.Reason
				return m, errors.Join(errRuleDNS, err)
			}
			trace.ReEvaluated = true
			retried, retryErr := read()
			if retryErr != nil {
				trace.Reason = "Surge rule re-evaluation failed after DNS lookup"
				m.SafetyReason = trace.Reason
				return m, errors.Join(errRuleDNS, retryErr)
			}
			m = retried
			m.RuleDNS = trace
			state = inspectRuleDNS(m.Notes)
			if state.failure != "" || state.required && !state.completed {
				trace.State = "incomplete"
				trace.Reason = "Surge rule DNS resolution did not complete after lookup"
				if state.failure != "" {
					trace.State = "failed"
					trace.Reason = "Surge rule DNS resolution failed after lookup: " + state.failure
				}
				m.SafetyReason = trace.Reason
				return m, errRuleDNS
			}
			trace.State = "completed"
		}
	}
	if m.FinalPolicy == "" || m.FinalType == "" {
		return m, errors.New("Surge explanation lacks final policy and type")
	}
	return m, nil
}
