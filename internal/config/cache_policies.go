package config

import (
	"fmt"
	"strings"
	"surge-controller/internal/cache"
	"surge-controller/internal/model"
)

func (c Cache) validatePolicies() error {
	for key := range c.Decisions {
		switch model.Decision(key) {
		case model.Direct, model.Surge, model.Reject, model.Unknown:
		default:
			return fmt.Errorf("unsupported cache decision: %s", key)
		}
	}
	if len(c.Upstreams) > 1024 {
		return fmt.Errorf("at most 1024 upstream cache policies are allowed")
	}
	for key := range c.Upstreams {
		if len(key) > 2048 || strings.ContainsAny(key, "\r\n\x00") || !(strings.HasPrefix(key, "udp://") || strings.HasPrefix(key, "tcp://") || strings.HasPrefix(key, "dot://") || strings.HasPrefix(key, "doh://") || strings.HasPrefix(key, "surge://")) {
			return fmt.Errorf("invalid upstream cache identity")
		}
	}
	return nil
}
func cloneCachePolicies(c Cache) Cache {
	c.Decisions = cache.ClonePolicies(c.Decisions)
	c.Upstreams = cache.ClonePolicies(c.Upstreams)
	return c
}
