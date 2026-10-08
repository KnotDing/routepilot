package surge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type realIPRunner struct {
	*runner
	profile string
	err     error
	reads   int
}

func (r *realIPRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if strings.Join(args, " ") == "dump profile effective" {
		r.reads++
		if r.err != nil {
			return nil, r.err
		}
		return json.Marshal(map[string]string{"profile": r.profile})
	}
	return r.runner.Run(ctx, args...)
}

func TestAlwaysRealIPHostListOrderAndWildcardMatching(t *testing.T) {
	entries, err := ParseAlwaysRealIP("[General]\nalways-real-ip = -private.*.test, *.test, time?.example.com, <simple-hostname> # comment\nsecret = must-not-expose\n[Proxy]\nalways-real-ip = *\n")
	if err != nil || !reflect.DeepEqual(entries, []string{"-private.*.test", "*.test", "time?.example.com", "<simple-hostname>"}) {
		t.Fatalf("wrong effective list: %v %v", entries, err)
	}
	a := &Adapter{realIPEntries: entries}
	for domain, want := range map[string]bool{"PRIVATE.foo.test.": false, "foo.test": true, "a.b.test": true, "time1.example.com": true, "time12.example.com": false, "nas": true, "example.com": false} {
		if _, got := a.MatchAlwaysRealIP(domain); got != want {
			t.Errorf("%s: matched=%v, want=%v", domain, got, want)
		}
	}
	// An exclusion after a positive match must not reverse the earlier match.
	a.realIPEntries = []string{"*.test", "-private.test"}
	if _, ok := a.MatchAlwaysRealIP("private.test"); !ok {
		t.Fatal("host-list order lost")
	}
}

func TestAlwaysRealIPInvalidExclusionsCannotBroadenBypass(t *testing.T) {
	for _, list := range []string{"-bad[pattern].test, *", "-*.example.com:443, *", "%APPEND% *.test", "https://example.com/private?token=must-not-expose"} {
		entries, err := ParseAlwaysRealIP("[General]\nalways-real-ip = " + list)
		if err == nil || len(entries) != 0 || strings.Contains(err.Error(), "must-not-expose") {
			t.Fatalf("invalid entry leaked or widened a bypass: %v %v", entries, err)
		}
	}
	entries, err := ParseAlwaysRealIP("[General]\nalways-real-ip = *.old.test\nalways-real-ip = EXAMPLE.COM.:0, *.test:53\n")
	if err != nil || !reflect.DeepEqual(entries, []string{"example.com", "*.test"}) {
		t.Fatalf("last effective assignment or port normalization failed: %v %v", entries, err)
	}
}

func TestAlwaysRealIPRefreshRetainsLastConfirmedListThenRemovesIt(t *testing.T) {
	r := &realIPRunner{runner: &runner{rules: []string{"FINAL,Proxy"}}, profile: "[General]\nalways-real-ip = *.test\n"}
	a := &Adapter{Runner: r}
	first, err := a.Refresh(context.Background())
	if err != nil || first.AlwaysRealIPCount != 1 || first.AlwaysRealIPReadError != "" || first.AlwaysRealIPObservedAt.IsZero() {
		t.Fatalf("initial list unavailable: %+v %v", first, err)
	}
	for i := 0; i < 50; i++ {
		if _, ok := a.MatchAlwaysRealIP("example.test"); !ok {
			t.Fatal("confirmed list did not match")
		}
	}
	if r.reads != 1 {
		t.Fatal("matching performed per-query profile reads")
	}
	view, _ := a.Snapshot()
	if !reflect.DeepEqual(view.AlwaysRealIP, []string{"*.test"}) {
		t.Fatal("management snapshot omitted active always-real-ip entries", view)
	}
	view.AlwaysRealIP[0] = "*.modified.test"
	view, _ = a.Snapshot()
	if !reflect.DeepEqual(view.AlwaysRealIP, []string{"*.test"}) {
		t.Fatal("snapshot list shared mutable state", view)
	}
	r.err = errors.New("Surge profile unavailable")
	a.realIPCheckedAt = time.Time{}
	failed, err := a.Refresh(context.Background())
	if err != nil || failed.Digest != first.Digest || failed.AlwaysRealIPCount != 1 || failed.AlwaysRealIPReadError == "" {
		t.Fatalf("failed read invalidated a confirmed list: %+v %v", failed, err)
	}
	a.invalidate()
	view, observed := a.Snapshot()
	if !reflect.DeepEqual(view.AlwaysRealIP, []string{"*.test"}) || view.AlwaysRealIPReadError == "" || !observed.IsZero() {
		t.Fatal("offline management view hid a list still used by DNS", view)
	}
	r.err = nil
	r.profile = "[General]\ndns-server = 192.0.2.53\n"
	a.realIPCheckedAt = time.Time{}
	empty, err := a.Refresh(context.Background())
	if err != nil || empty.Digest == first.Digest || empty.AlwaysRealIPCount != 0 || empty.AlwaysRealIPReadError != "" {
		t.Fatalf("confirmed removal did not replace the list: %+v %v", empty, err)
	}
	if _, ok := a.MatchAlwaysRealIP("example.test"); ok {
		t.Fatal("removed entry still bypasses Surge")
	}
}
