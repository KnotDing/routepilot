package config

import (
	"fmt"
	"strings"
	"surge-controller/internal/cache"
	"surge-controller/internal/model"
)

func (c Cache) validatePolicies() error {
	if !c.DecisionCacheEnabled() && c.Optimistic {
		return fmt.Errorf("optimistic decision cache requires decision caching")
	}
	if !c.ResponseCacheEnabled() && c.OptimisticResponses {
		return fmt.Errorf("optimistic response cache requires response caching")
	}
	for _, policies := range []map[string]cache.Policy{c.Decisions, c.Upstreams} {
		for _, policy := range policies {
			if !policy.CacheEnabled() && policy.Optimistic != nil && *policy.Optimistic {
				return fmt.Errorf("optimistic cache policy requires caching for the same item")
			}
		}
	}
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
	if c.DecisionsEnabled != nil {
		v := *c.DecisionsEnabled
		c.DecisionsEnabled = &v
	}
	if c.ResponsesEnabled != nil {
		v := *c.ResponsesEnabled
		c.ResponsesEnabled = &v
	}
	c.Decisions = cache.ClonePolicies(c.Decisions)
	c.Upstreams = cache.ClonePolicies(c.Upstreams)
	return c
}

func (c Cache) DecisionCacheEnabled() bool {
	return c.DecisionsEnabled == nil || *c.DecisionsEnabled
}

func (c Cache) ResponseCacheEnabled() bool {
	return c.ResponsesEnabled == nil || *c.ResponsesEnabled
}
