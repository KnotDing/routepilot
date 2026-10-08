package app

import (
	"surge-controller/internal/config"
	"testing"
	"time"
)

func TestPolicyDNSFallbackQueryTimeout(t *testing.T) {
	s := config.Default().Settings
	base := time.Duration(s.DNS.TimeoutMS+s.Cache.TimeoutMS) * time.Millisecond
	if dnsQueryTimeout(s) != base {
		t.Fatal("default timeout changed")
	}
	s.DNS.PolicyUpstreams = []config.PolicyUpstream{{Enabled: true, Fallback: "real"}}
	if dnsQueryTimeout(s) != base+time.Duration(s.DNS.TimeoutMS)*time.Millisecond {
		t.Fatal("backup has no query budget")
	}
	s.DNS.PolicyUpstreams[0].Enabled = false
	if dnsQueryTimeout(s) != base {
		t.Fatal("disabled backup changed timeout")
	}
}
