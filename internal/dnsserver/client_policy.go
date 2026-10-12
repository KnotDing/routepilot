package dnsserver

import (
	"context"
	"github.com/miekg/dns"
	"surge-controller/internal/model"
)

func (d *Dispatcher) resolveClientPolicy(ctx context.Context, q model.DNSQueryContext, m *dns.Msg, result *model.DecisionResult, decision model.Decision, gate SafetySnapshot) *dns.Msg {
	result.Decision, result.Confidence, result.Source = decision, model.Certain, "client-policy"
	result.Policy, result.FinalPolicy = string(decision), string(decision)
	result.MatchedRule = "CLIENT-IP," + q.ClientIP.Unmap().String() + "," + string(decision)
	result.Reason = "forced policy for configured client"
	switch decision {
	case model.Direct:
		d.Statistics().Direct.Add(1)
		return d.real(ctx, q, m)
	case model.Surge:
		d.Statistics().Surge.Add(1)
		return d.resolveSurge(ctx, q, m, gate)
	default:
		trace, _ := ctx.Value(resolutionKey{}).(*resolution)
		trace.Destination = "blocked"
		d.Statistics().Reject.Add(1)
		return failure(m, dns.RcodeRefused)
	}
}
