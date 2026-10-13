package resolver

// CacheIdentity distinguishes DoH paths and TLS server names on a shared IP.
func (e ResolverEndpoint) CacheIdentity() string {
	key := e.Protocol + "://" + e.Address
	if e.Protocol == "doh" {
		key += "|" + e.URL
	}
	if e.Protocol == "dot" {
		key += "|" + e.TLSName
	}
	return key
}
func (r *Real) allowsOptimistic(endpoint string) bool {
	return r.optimistic && r.policies[endpoint].OptimisticEnabled()
}
func (r *Real) policyForResponse(v response, stale bool) bool {
	return !stale || r.allowsOptimistic(v.Endpoint)
}
func (r *Real) storedEndpoint(record ResponseCacheRecord) string {
	endpoints := record.Endpoints
	if len(endpoints) == 0 {
		endpoints = r.endpoints
	}
	identity := ""
	for _, ep := range endpoints {
		if record.Endpoint == ep.CacheIdentity() {
			return record.Endpoint
		}
		if record.Endpoint == ep.Protocol+"://"+ep.Address {
			if identity != "" && identity != ep.CacheIdentity() {
				return ""
			}
			identity = ep.CacheIdentity()
		}
	}
	// Original-destination UDP/TCP records from old snapshots remain identifiable.
	if identity == "" && record.Original {
		identity = record.Endpoint
	}
	return identity
}
func (r *Real) configureCacheGroups() {
	r.Cache.ConfigureGroups(func(_ string, v response) string { return v.Endpoint }, func(v response) bool { return r.allowsOptimistic(v.Endpoint) })
}
