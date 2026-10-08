package surge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHostImportStaticOrderAndRedactedSkips(t *testing.T) {
	profile := "[General]\nsecret = do-not-expose\n[Host]\n*.example.test = 192.0.2.1, 2001:db8::1\nnas.example.test = 192.0.2.2\nhome.example.test = server:system\nRULE-SET:https://example.test/private?token=do-not-expose = server:192.0.2.53\nlast.example.test = 192.0.2.3\n[Proxy]\nsecret=do-not-expose\n"
	result, e := ParseHosts(profile)
	if e != nil || len(result.Entries) != 2 || len(result.Skipped) != 3 || result.Total != 5 {
		t.Fatal(result, e)
	}
	if result.Entries[0].Order >= result.Entries[1].Order || len(result.Entries[0].Addresses) != 2 {
		t.Fatal("order or IP families lost")
	}
	again, _ := ParseHosts(profile)
	if again.Entries[0].ID != result.Entries[0].ID {
		t.Fatal("unstable identifiers")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "do-not-expose") || strings.Contains(string(raw), "https://") {
		t.Fatal("profile data leaked into import")
	}
}
func TestUnsupportedEarlierHostsDoNotBecomeStaticOverrides(t *testing.T) {
	result, e := ParseHosts("[Host]\n*.example.test = server:system\nnas.example.test = 192.0.2.2\nother.test = 192.0.2.3\n")
	if e != nil || len(result.Entries) != 1 || result.Entries[0].Domain != "other.test" {
		t.Fatal(result, e)
	}
	for _, domain := range []string{"", ".", "bad[host].test", "a;bad.test"} {
		if _, e := ParseHosts("[Host]\n" + domain + " = 192.0.2.1"); e != nil {
			t.Fatal(e)
		}
	}
}
