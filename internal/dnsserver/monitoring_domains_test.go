package dnsserver

import (
	"fmt"
	"testing"
)

func TestDomainBuffersAreBoundedAndCapacityReopensAfterFlush(t *testing.T) {
	m := NewMonitoring(DNSMonitoringSnapshot{})
	for i := 0; i < monitoringDomainBufferCapacity+500; i++ {
		recordDimension(m, "192.0.2.1", fmt.Sprintf("host-%d.test", i), "real", "DIRECT", "DIRECT", "NOERROR")
	}
	if len(m.domainBuffer) != monitoringDomainBufferCapacity+1 {
		t.Fatal("unbounded pending domains", len(m.domainBuffer))
	}
	select {
	case <-m.DomainWake():
	default:
		t.Fatal("full buffer did not request early save")
	}
	pending := m.PendingDomains()
	if m.PendingDomains() != pending {
		t.Fatal("retry changed frozen batch")
	}
	for i := 0; i < 100; i++ {
		recordDimension(m, "192.0.2.1", "late-hot.test", "real", "DIRECT", "DIRECT", "NOERROR")
	}
	buffer := m.BufferedDomains()
	var total, missing, hot uint64
	for _, row := range buffer.Rows {
		total += row.Queries
		if row.Other {
			missing += row.Queries
		}
		if row.Domain == "late-hot.test" {
			hot += row.Queries
		}
	}
	if total != monitoringDomainBufferCapacity+600 || missing != 500 || hot != 100 {
		t.Fatal("lost request or failed to admit later domain", total, missing, hot)
	}
	m.CommitDomains(pending)
	buffer = m.BufferedDomains()
	if len(buffer.Rows) != 1 || buffer.Rows[0].Domain != "late-hot.test" || buffer.Rows[0].Queries != 100 {
		t.Fatal("acknowledgement discarded new writes", buffer)
	}
}
