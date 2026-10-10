package classifier

import (
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"surge-controller/internal/model"
)

func (e *Engine) isIdle() bool {
	return e.ctx.Err() == nil && e.activeJobs.Load() == 0 && len(e.jobs) == 0 &&
		time.Since(time.Unix(0, e.lastQuery.Load())) >= time.Second
}

// At most one idle evaluation runs, and at most four start each second.
// The cursor rotates across stale entries so failed candidates do not starve
// other domains. Foreground requests cancel this maintenance immediately.
func (e *Engine) refreshIdle() {
	defer e.workers.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	offset := 0
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
		}
		if !e.isIdle() {
			continue
		}
		records, generation := e.Cache.StaleRecords(offset, 1)
		if len(records) == 0 {
			offset = 0
			continue
		}
		key := records[0].Key
		qtype, qclass, ok := strings.Cut(key.QueryClass, "/")
		typeID, typeErr := strconv.ParseUint(qtype, 10, 16)
		classID, classErr := strconv.ParseUint(qclass, 10, 16)
		client, clientErr := netip.ParseAddr(key.ClientGroup)
		if !ok || typeErr != nil || classErr != nil || clientErr != nil || typeID == 0 || classID == 0 {
			offset++
			continue
		}
		query := model.DNSQueryContext{ClientIP: client,
			Question: dns.Question{Name: key.Domain, Qtype: uint16(typeID), Qclass: uint16(classID)}}
		if e.refresh(query, key, generation, true) {
			offset++
		}
	}
}
