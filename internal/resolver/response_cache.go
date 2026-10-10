package resolver

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/cache"
)

// Wire data (including the binary query key) uses byte slices so JSON preserves
// it exactly. Message IDs are restored from the new request when serving a hit.
type ResponseCacheRecord struct {
	Key       []byte             `json:"key"`
	Message   []byte             `json:"message"`
	Stored    time.Time          `json:"stored"`
	ExpiresAt time.Time          `json:"expires_at"`
	Endpoint  string             `json:"endpoint"`
	Query     []byte             `json:"query,omitempty"`
	Endpoints []ResolverEndpoint `json:"endpoints,omitempty"`
	Original  bool               `json:"original,omitempty"`
}

func (r *Real) ResponseSnapshot() []ResponseCacheRecord {
	records := r.Cache.Snapshot()
	out := make([]ResponseCacheRecord, 0, len(records))
	for _, record := range records {
		if record.Value.Msg == nil {
			continue
		}
		msg := record.Value.Msg.Copy()
		msg.Id = 0
		raw, err := msg.Pack()
		if err == nil {
			var query []byte
			if record.Value.RefreshQuery != nil {
				query, err = record.Value.RefreshQuery.Pack()
			}
			if err == nil {
				out = append(out, ResponseCacheRecord{
					Key: []byte(record.Key), Message: raw, Stored: record.Value.Stored,
					ExpiresAt: record.ExpiresAt, Endpoint: record.Value.Endpoint,
					Query: query, Endpoints: append([]ResolverEndpoint(nil), record.Value.RefreshEndpoints...),
					Original: record.Value.RefreshOriginal,
				})
			}
		}
	}
	return out
}

// Restore keeps original DNS deadlines, including expired responses when
// optimistic caching is enabled. Optimistic restores start stale for background
// refresh; ordinary restores retain only unexpired responses as fresh entries.
func (r *Real) RestoreResponses(records []ResponseCacheRecord) int {
	now := r.now()
	values := make([]cache.Record[string, response], 0, len(records))
	for _, record := range records {
		if record.Stored.IsZero() || record.Stored.After(now) || !record.ExpiresAt.After(record.Stored) {
			continue
		}
		// Keys begin with an endpoint partition followed by a zero-ID DNS query.
		key := string(record.Key)
		index := strings.Index(key, "\x00\x00")
		if index <= 0 || len(record.Key) > 65535+8192 || len(record.Message) > 65535 {
			continue
		}
		query, msg := new(dns.Msg), new(dns.Msg)
		if query.Unpack(record.Key[index:]) != nil || msg.Unpack(record.Message) != nil ||
			query.Response || query.CheckingDisabled || len(query.Question) != 1 ||
			msg.Id != 0 || ValidateResponse(query, msg) != nil {
			continue
		}
		if opt := query.IsEdns0(); opt != nil && len(opt.Option) > 0 {
			continue
		}
		ttl := ResponseTTL(msg)
		if ttl <= 0 {
			continue
		}
		expiry := record.ExpiresAt
		if deadline := record.Stored.Add(ttl); expiry.After(deadline) {
			expiry = deadline
		}
		if !r.optimistic && !now.Before(expiry) {
			continue
		}
		refreshQuery := query
		refreshEndpoints := []ResolverEndpoint(nil)
		refreshOriginal := false
		if len(record.Query) > 0 && len(record.Endpoints) > 0 {
			persistedQuery := new(dns.Msg)
			if persistedQuery.Unpack(record.Query) == nil && !persistedQuery.Response &&
				!persistedQuery.CheckingDisabled && len(persistedQuery.Question) == 1 {
				if _, validName := dns.IsDomainName(persistedQuery.Question[0].Name); validName {
					partition := makeEndpointPartition(record.Endpoints)
					if bytes.Equal(record.Key[:index], partition) && bytes.Equal(record.Key[index:], record.Query) {
						endpointsValid := true
						for _, endpoint := range record.Endpoints {
							if endpoint.Validate() != nil {
								endpointsValid = false
								break
							}
						}
						if endpointsValid {
							refreshQuery = persistedQuery
							refreshEndpoints = append([]ResolverEndpoint(nil), record.Endpoints...)
							refreshOriginal = record.Original
						}
					}
				}
			}
		} else if bytes.Equal(record.Key[:index], makeEndpointPartition(r.endpoints)) {
			refreshEndpoints = append([]ResolverEndpoint(nil), r.endpoints...)
		}
		values = append(values, cache.Record[string, response]{Key: key,
			Value: response{Msg: msg, Stored: record.Stored, Endpoint: record.Endpoint, RefreshQuery: refreshQuery,
				RefreshEndpoints: refreshEndpoints, RefreshOriginal: refreshOriginal}, ExpiresAt: expiry, Stale: r.optimistic})
	}
	return r.Cache.Restore(values)
}

func makeEndpointPartition(endpoints []ResolverEndpoint) []byte {
	var b strings.Builder
	for _, endpoint := range endpoints {
		fmt.Fprintf(&b, "%s:%s:%s|", endpoint.Protocol, endpoint.Address, endpoint.URL)
	}
	return []byte(b.String())
}
