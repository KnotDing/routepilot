package resolver

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"

	"github.com/miekg/dns"
)

// Compute once outside the cache lock. Compare DNS results, not wire IDs,
// elapsed TTLs, record ordering, EDNS transport options or server selection.
func responseFingerprint(msg *dns.Msg) [32]byte {
	hash := sha256.New()
	if msg == nil {
		return sha256.Sum256(nil)
	}
	fmt.Fprintf(hash, "%d/%t\n", msg.Rcode, msg.Truncated)
	for index, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		var records []string
		for _, record := range section {
			if record.Header().Rrtype == dns.TypeOPT {
				continue
			}
			copy := dns.Copy(record)
			copy.Header().Ttl = 0
			copy.Header().Name = dns.CanonicalName(copy.Header().Name)
			// DNS domain-valued RDATA is case-insensitive; TXT and other
			// opaque data must retain their original case and content.
			value := reflect.ValueOf(copy).Elem()
			for i := 0; i < value.NumField(); i++ {
				tag := value.Type().Field(i).Tag.Get("dns")
				if tag != "domain-name" && tag != "cdomain-name" {
					continue
				}
				field := value.Field(i)
				if field.Kind() == reflect.String {
					field.SetString(dns.CanonicalName(field.String()))
				} else if field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String {
					for j := 0; j < field.Len(); j++ {
						field.Index(j).SetString(dns.CanonicalName(field.Index(j).String()))
					}
				}
			}
			records = append(records, copy.String())
		}
		sort.Strings(records)
		fmt.Fprintf(hash, "section:%d/%d\n", index, len(records))
		for _, record := range records {
			fmt.Fprintf(hash, "%d:%s\n", len(record), record)
		}
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func sameResponse(old, next response) bool { return old.Fingerprint == next.Fingerprint }
