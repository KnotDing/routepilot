package resolver

import "errors"

func (r *Real) endpointIdentity(endpoint ResolverEndpoint) string {
	if r.sourceIdentity != "" {
		return r.sourceIdentity
	}
	return endpoint.CacheIdentity()
}

// Cache invalidation and the epoch observation are serialized so concurrent
// lookups cannot revive answers from a previous health/routing generation.
func (r *Real) currentGeneration() (uint64, error) {
	if r.scope == nil {
		return r.Cache.Generation(), nil
	}
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	epoch, allowed := r.scope()
	if epoch != r.scopeGeneration {
		r.Cache.FlushReason("Surge Fake-IP safety generation changed")
		r.scopeGeneration = epoch
	}
	if !allowed {
		return r.Cache.Generation(), errors.New("Surge Fake-IP issuance is not ready")
	}
	return r.Cache.Generation(), nil
}
