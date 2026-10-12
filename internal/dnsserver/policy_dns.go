package dnsserver

import (
	"context"
	"errors"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
)

// PolicyDNSBinding is immutable for the lifetime of a configured runtime.
type PolicyDNSBinding struct {
	Field            string
	Policy           string
	Resolver         model.Resolver
	Fallback         string
	FallbackResolver model.Resolver
	AttemptTimeout   time.Duration
}

func (d *Dispatcher) policyDNS(result model.DecisionResult) (*PolicyDNSBinding, *model.PolicyDNSMatch) {
	for _, field := range []string{"final_policy", "policy"} {
		name := result.FinalPolicy
		if field == "policy" {
			name = result.Policy
		}
		if name == "" {
			continue
		}
		for i := range d.PolicyDNS {
			binding := &d.PolicyDNS[i]
			if binding.Field == field && binding.Policy == name && binding.Resolver != nil {
				return binding, &model.PolicyDNSMatch{Field: field, Policy: name, Fallback: binding.Fallback}
			}
		}
	}
	return nil, nil
}

func policyAttemptContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func (d *Dispatcher) resolvePolicyDNS(ctx context.Context, q model.DNSQueryContext, m *dns.Msg, binding *PolicyDNSBinding, match *model.PolicyDNSMatch, gate SafetySnapshot) *dns.Msg {
	trace, _ := ctx.Value(resolutionKey{}).(*resolution)
	started := time.Now()
	defer func() { trace.Duration = time.Since(started) }()
	reply, err := d.policyRealAttempt(ctx, q, m, binding.Resolver, binding.AttemptTimeout, "primary", match)
	if err == nil {
		return reply
	}
	trace.Error = err.Error()
	if ctx.Err() != nil || binding.Fallback == "" || binding.Fallback == "none" {
		return failure(m, dns.RcodeServerFailure)
	}
	match.FallbackUsed = true
	trace.Error = ""
	trace.ResolutionTrace = model.ResolutionTrace{}
	trace.Destination = ""
	if binding.Fallback == "surge" {
		attemptCtx, cancel := policyAttemptContext(ctx, binding.AttemptTimeout)
		defer cancel()
		begin := time.Now()
		reply = d.resolveSurge(attemptCtx, q, m, gate)
		code := reply.Rcode
		if err := resolver.ResolutionFailure(reply); err != nil && trace.Error == "" {
			trace.Error = err.Error()
		}
		match.Attempts = append(match.Attempts, model.PolicyDNSAttempt{Source: "surge", Resolver: trace.Endpoint, Rcode: &code, Error: trace.Error, LatencyMS: float64(time.Since(begin)) / float64(time.Millisecond)})
		if trace.Error != "" {
			return failure(m, dns.RcodeServerFailure)
		}
		return reply
	}
	upstream := binding.FallbackResolver
	if binding.Fallback == "real" {
		upstream = d.Real
	}
	reply, err = d.policyRealAttempt(ctx, q, m, upstream, binding.AttemptTimeout, binding.Fallback, match)
	if err != nil {
		trace.Error = err.Error()
		return failure(m, dns.RcodeServerFailure)
	}
	return reply
}

func (d *Dispatcher) policyRealAttempt(ctx context.Context, q model.DNSQueryContext, m *dns.Msg, upstream model.Resolver, timeout time.Duration, source string, match *model.PolicyDNSMatch) (*dns.Msg, error) {
	attemptCtx, cancel := policyAttemptContext(ctx, timeout)
	defer cancel()
	begin := time.Now()
	record := model.ResolutionTrace{}
	var reply *dns.Msg
	var err error
	if upstream == nil {
		err = errors.New("policy DNS resolver unavailable")
	} else {
		trace, _ := ctx.Value(resolutionKey{}).(*resolution)
		trace.Destination = "real"
		reply, err = upstream.Resolve(attemptCtx, model.ResolveRequest{Query: q, Message: m, Trace: &record, UseConfiguredUpstreams: true, RetryDNSFailures: true})
	}
	if err == nil && attemptCtx.Err() != nil {
		err = attemptCtx.Err()
	}
	if err == nil {
		err = resolver.ValidateResponse(m, reply)
	}
	if err == nil {
		err = resolver.ResolutionFailure(reply)
	}
	if err == nil && ContainsFakeAddress(reply, d.FakePrefixes) {
		err = errors.New("真实 DNS 上游返回 Fake-IP，请检查源地址绕行策略及 Surge DNS 劫持")
	}
	attempt := model.PolicyDNSAttempt{Source: source, Resolver: record.Endpoint, CacheHit: record.CacheHit, LatencyMS: float64(time.Since(begin)) / float64(time.Millisecond)}
	if reply != nil {
		code := reply.Rcode
		attempt.Rcode = &code
	}
	if err != nil {
		attempt.Error = err.Error()
	}
	match.Attempts = append(match.Attempts, attempt)
	trace, _ := ctx.Value(resolutionKey{}).(*resolution)
	trace.ResolutionTrace = record
	return reply, err
}
