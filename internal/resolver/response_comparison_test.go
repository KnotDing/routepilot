package resolver

import (
	"testing"

	"github.com/miekg/dns"
)

func TestResponseFingerprintIgnoresWireAndRecordOrderingChanges(t *testing.T) {
	first := new(dns.Msg)
	first.Id = 12
	first.Answer = []dns.RR{
		mustRR(t, "Example.test. 60 IN A 192.0.2.1"),
		mustRR(t, "example.test. 60 IN AAAA 2001:db8::1"),
	}
	second := new(dns.Msg)
	second.Id = 99
	second.Answer = []dns.RR{
		mustRR(t, "EXAMPLE.TEST. 5 IN AAAA 2001:db8::1"),
		mustRR(t, "example.TEST. 5 IN A 192.0.2.1"),
	}
	if !sameResponse(response{Fingerprint: responseFingerprint(first)}, response{Fingerprint: responseFingerprint(second)}) {
		t.Fatal("wire ID, TTL, owner case, or answer ordering changed response identity")
	}
}

func TestResponseFingerprintPreservesOpaqueTXTChanges(t *testing.T) {
	first := new(dns.Msg)
	first.Answer = []dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60}, Txt: []string{"CaseSensitive"}}}
	second := new(dns.Msg)
	second.Answer = []dns.RR{&dns.TXT{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60}, Txt: []string{"casesensitive"}}}
	if responseFingerprint(first) == responseFingerprint(second) {
		t.Fatal("opaque TXT content change was treated as an equal response")
	}
}

func mustRR(t *testing.T, text string) dns.RR {
	t.Helper()
	record, err := dns.NewRR(text)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
