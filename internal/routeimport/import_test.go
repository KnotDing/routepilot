package routeimport

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"reflect"
	"strings"
	"surge-controller/internal/config"
	"testing"
)

func TestParseAddressesCommentsDuplicatesAndInvalidLines(t *testing.T) {
	d := config.Default()
	d.Routes = []config.Route{{Prefix: "91.108.4.0/22"}}
	d.Interception = []config.Interception{{Prefix: "8.8.8.8/32"}}
	v, err := Parse([]byte("\ufeff# Telegram\r\n91.108.56.0/22\r\n2001:b28:f23d::/48\n149.154.160.1 # host\n91.108.56.0/22\n91.108.4.0/22\n8.8.8.8\n198.18.0.0/15\nnot-a-route\n91.108.56.1/22\n0.0.0.0/0\n// comment\n"), d)
	if err != nil || !reflect.DeepEqual(v.Prefixes, []string{"91.108.56.0/22", "2001:b28:f23d::/48", "149.154.160.1/32"}) || len(v.Skipped) != 7 {
		t.Fatal(v, err)
	}
	if v.Skipped[0].Line != 5 || v.Skipped[4].Reason != "invalid route destination" {
		t.Fatal(v.Skipped)
	}
}

func TestAppendUsesSeparateNextHopsAndValidatesAtomically(t *testing.T) {
	d := config.Default()
	req := Request{Prefixes: []string{"91.108.56.0/22", "2001:b28:f23d::/48"}, NextHop4: "192.168.20.5", NextHop6: "fd00:20::5", Enabled: true, Binding: "surge", MED: 42, LocalPreference: 100, Communities: []string{"64512:7"}, Description: "Telegram"}
	out, result, err := Append(d, req)
	if err != nil || result.Imported != 2 || len(d.Routes) != 0 || out.Routes[0].NextHop != req.NextHop4 || out.Routes[1].NextHop != req.NextHop6 {
		t.Fatal(out.Routes, result, err)
	}
	if out.Routes[0].Binding != "surge" || out.Routes[0].MED != 42 || out.Routes[0].Description != "Telegram" {
		t.Fatal(out.Routes)
	}
	// Repeated imports preserve edits to existing routes and do not create duplicates.
	out.Routes[0].NextHop = "192.168.20.9"
	again, result, err := Append(out, req)
	if err != nil || result.Imported != 0 || result.Skipped != 2 || again.Routes[0].NextHop != "192.168.20.9" {
		t.Fatal(again.Routes, result, err)
	}
	req.NextHop6 = ""
	failed, _, err := Append(d, req)
	if err == nil || len(failed.Routes) != 0 || len(d.Routes) != 0 {
		t.Fatal("partial import on missing IPv6 next hop", err)
	}
	req.NextHop6 = "fd00:20::5"
	d.Settings.BGP.RouteLimit = 2 // One Fake-IP route plus two new routes exceeds this limit.
	failed, _, err = Append(d, req)
	if err == nil || len(failed.Routes) != 0 {
		t.Fatal("partial import on route limit", err)
	}
	req.Enabled = false
	if _, _, err = Append(d, req); err != nil {
		t.Fatal("disabled routes count as advertisements", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchBoundedSnapshotAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"telegram", "91.108.56.0/22\n2001:b28:f23d::/48\n", 200, true},
		{"status", "91.108.56.0/22", 404, false},
		{"oversized", strings.Repeat("a", MaxBytes+1), 200, false},
		{"binary", "\xff", 200, false},
		{"too many lines", strings.Repeat("#\n", 8193), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.Header.Get("Accept") != "text/plain" {
					t.Fatal(r)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			v, err := Fetch(context.Background(), client, "https://core.telegram.org/resources/cidr.txt", config.Default())
			if (err == nil) != tc.valid || tc.valid && len(v.Prefixes) != 2 {
				t.Fatal(v, err)
			}
		})
	}
}

func TestURLAccessAndRedirectRestrictions(t *testing.T) {
	for _, raw := range []string{"file:///etc/hosts", "http://127.0.0.1/list", "http://[::1]/list", "http://192.168.20.1/list", "https://user:pass@example.com/list", "https://example.com/list#fragment", "http://localhost/list", "http://[::ffff:127.0.0.1]/"} {
		if _, err := validateURL(raw); err == nil {
			t.Fatal("unsafe URL accepted", raw)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.0.1", "::1", "fe80::1", "fc00::1", "169.254.169.254"} {
		if publicAddress(netip.MustParseAddr(ip)) {
			t.Fatal(ip)
		}
	}
	client := NewClient()
	first, _ := http.NewRequest("GET", "https://example.com/list", nil)
	for _, raw := range []string{"http://example.com/list", "https://127.0.0.1/list"} {
		r, _ := http.NewRequest("GET", raw, nil)
		if err := client.CheckRedirect(r, []*http.Request{first}); err == nil {
			t.Fatal("unsafe redirect accepted", raw)
		}
	}
}
