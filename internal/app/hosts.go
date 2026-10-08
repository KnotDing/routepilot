package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"surge-controller/internal/config"
	"surge-controller/internal/surge"
)

func (a *App) ImportHosts(ctx context.Context) (any, error) {
	select {
	case a.ruleTests <- struct{}{}:
		defer func() { <-a.ruleTests }()
	default:
		return nil, errors.New("Surge reader capacity exhausted")
	}
	a.mu.RLock()
	runner := a.runtime.adapter.Runner
	a.mu.RUnlock()
	imported, e := surge.ReadHosts(ctx, runner)
	if e != nil {
		return nil, e
	}
	d := a.Configuration()
	retained := []config.Host{}
	for _, h := range d.Hosts {
		if h.Source != "surge" {
			retained = append(retained, h)
		}
	}
	count := 0
	for _, h := range imported.Entries {
		fake := false
		for _, ip := range h.Addresses {
			for _, prefix := range d.Settings.Surge.FakePrefixes {
				fake = fake || netip.MustParsePrefix(prefix).Contains(netip.MustParseAddr(ip))
			}
		}
		if fake {
			imported.Skipped = append(imported.Skipped, surge.HostSkip{Line: h.Order, Reason: "static mapping belongs to configured Surge Fake-IP network"})
			continue
		}
		retained = append(retained, h)
		count++
	}
	d.Hosts = retained
	if e = a.Apply(ctx, d); e != nil {
		return nil, e
	}
	a.event("INFO", "hosts_import", "Static local DNS mappings imported from effective Surge profile")
	return map[string]any{"imported": count, "skipped": imported.Skipped, "total": imported.Total}, nil
}

func (a *App) VerifyRoute(ctx context.Context, raw json.RawMessage) (any, error) {
	select {
	case a.ruleTests <- struct{}{}:
		defer func() { <-a.ruleTests }()
	default:
		return nil, errors.New("connectivity probe capacity exhausted")
	}
	var input struct {
		Prefix string `json:"prefix"`
		Target string `json:"target"`
		Port   int    `json:"port"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return nil, errors.New("invalid connectivity probe")
	}
	a.mu.RLock()
	manager := a.runtime.routes
	a.mu.RUnlock()
	return manager.Verify(ctx, input.Prefix, input.Target, input.Port)
}
