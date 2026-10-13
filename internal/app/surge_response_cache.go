package app

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	"surge-controller/internal/dnsserver"
	"surge-controller/internal/model"
	"surge-controller/internal/resolver"
)

// Health probes use the adapter directly. Only actual DNS lookups and bounded
// refreshes enter this transport; refreshes never acquire an issuance lease.
type surgeResponseTransport struct{ rt *Runtime }

func (t *surgeResponseTransport) Exchange(ctx context.Context, _ resolver.ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
	before := t.rt.machine.Snapshot()
	if !before.AllowFakeIP {
		return nil, errors.New("Surge Fake-IP issuance is not ready")
	}
	t.rt.fakeQueries.Add(1)
	r, err := t.rt.adapter.ResolveFakeIP(ctx, q)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = resolver.ValidateResponse(q, r)
	}
	if err == nil && !dnsserver.FakeAddressesValid(r, t.rt.dispatcher.FakePrefixes) {
		err = errors.New("Surge DNS returned an address outside the configured Fake-IP prefixes")
	}
	after := t.rt.machine.Snapshot()
	if err == nil && (!after.AllowFakeIP || after.Generation != before.Generation) {
		err = errors.New("Surge Fake-IP safety generation changed during lookup")
	}
	if err != nil {
		t.rt.fakeErrors.Add(1)
		return nil, err
	}
	return r, nil
}
func (o *observedSurge) ResolveSurge(ctx context.Context, req model.ResolveRequest) (*dns.Msg, error) {
	if o.rt.surgeResponses != nil {
		return o.rt.surgeResponses.Resolve(ctx, req)
	}
	return o.ResolveFakeIP(ctx, req.Message)
}
