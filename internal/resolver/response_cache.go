package resolver

import (
	"strings"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/cache"
)

// Wire data (including the binary query key) uses byte slices so JSON preserves
// it exactly. Message IDs are restored from the new request when serving a hit.
type ResponseCacheRecord struct {
	Key       []byte    `json:"key"`
	Message   []byte    `json:"message"`
	Stored    time.Time `json:"stored"`
	ExpiresAt time.Time `json:"expires_at"`
	Endpoint  string    `json:"endpoint"`
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
			out = append(out, ResponseCacheRecord{[]byte(record.Key), raw, record.Value.Stored, record.ExpiresAt, record.Value.Endpoint})
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
		values = append(values, cache.Record[string, response]{Key: key,
			Value: response{Msg: msg, Stored: record.Stored, Endpoint: record.Endpoint}, ExpiresAt: expiry, Stale: r.optimistic})
	}
	return r.Cache.Restore(values)
}
