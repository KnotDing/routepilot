package resolver

import (
	"time"

	"surge-controller/internal/model"
)

func (r *Real) isIdle() bool {
	return r.ctx.Err() == nil && r.activeQueries.Load() == 0 &&
		time.Since(time.Unix(0, r.lastQuery.Load())) >= time.Second
}

func (r *Real) refreshIdle() {
	defer r.refreshWorkers.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	offset := 0
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
		if !r.isIdle() {
			continue
		}
		records, generation := r.Cache.StaleRecords(offset, 1)
		if len(records) == 0 {
			offset = 0
			continue
		}
		record := records[0]
		query := record.Value.RefreshQuery
		if query == nil || len(record.Value.RefreshEndpoints) == 0 {
			offset++
			continue
		}
		req := model.ResolveRequest{Query: model.DNSQueryContext{ClientIP: record.Value.RefreshClient}, Message: query.Copy(), RetryDNSFailures: true}
		if r.refresh(req, record.Value.RefreshEndpoints, record.Value.RefreshOriginal, record.Key, generation, true) {
			offset++
		}
	}
}
