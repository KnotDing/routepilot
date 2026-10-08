// Package routeimport imports a bounded, operator-reviewed snapshot of route destinations.
package routeimport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/security"
	"time"
	"unicode/utf8"
)

const MaxBytes = 1024 * 1024
const MaxPrefixes = 1024

type Skipped struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}
type Preview struct {
	Prefixes []string  `json:"prefixes"`
	Skipped  []Skipped `json:"skipped"`
}
type Request struct {
	URL             string   `json:"url"`
	AutoUpdate      bool     `json:"auto_update"`
	IntervalMinutes int      `json:"interval_minutes"`
	SyncIPv4        bool     `json:"sync_ipv4"`
	SyncIPv6        bool     `json:"sync_ipv6"`
	SourceID        string   `json:"source_id,omitempty"`
	Prefixes        []string `json:"prefixes"`
	NextHop4        string   `json:"next_hop4"`
	NextHop6        string   `json:"next_hop6"`
	MED             uint32   `json:"med"`
	LocalPreference uint32   `json:"local_preference"`
	Communities     []string `json:"communities"`
	Binding         string   `json:"health_binding"`
	Enabled         bool     `json:"enabled"`
	Description     string   `json:"description"`
}
type Result struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

func existing(d config.Document) map[string]bool {
	seen := map[string]bool{}
	for _, p := range d.Settings.Surge.FakePrefixes {
		seen[p] = true
	}
	for _, r := range d.Routes {
		seen[r.Prefix] = true
	}
	for _, r := range d.Interception {
		seen[r.Prefix] = true
	}
	return seen
}

func destination(s string) (netip.Prefix, error) {
	if a, err := netip.ParseAddr(s); err == nil && a.Zone() == "" && !a.Is4In6() {
		s = netip.PrefixFrom(a, a.BitLen()).String()
	}
	p, err := config.Prefix(s)
	if err != nil || p.Addr().Is4In6() || p.Addr().Zone() != "" {
		return netip.Prefix{}, errors.New("invalid route destination")
	}
	return p, nil
}

func Parse(data []byte, d config.Document) (Preview, error) {
	out := Preview{Prefixes: []string{}, Skipped: []Skipped{}}
	if len(data) > MaxBytes {
		return out, errors.New("route list exceeds 1 MiB")
	}
	if !utf8.Valid(data) {
		return out, errors.New("route list must be UTF-8 text")
	}
	seen := existing(d)
	scanner := bufio.NewScanner(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	scanner.Buffer(make([]byte, 4096), 64*1024)
	line := 0
	for scanner.Scan() {
		line++
		if line > 8192 {
			return Preview{}, errors.New("route list exceeds 8192 lines")
		}
		s := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		p, err := destination(s)
		if err != nil {
			out.Skipped = append(out.Skipped, Skipped{line, "invalid route destination"})
			continue
		}
		key := p.String()
		if seen[key] {
			out.Skipped = append(out.Skipped, Skipped{line, "duplicate route destination"})
			continue
		}
		seen[key] = true
		out.Prefixes = append(out.Prefixes, key)
		if len(out.Prefixes) > MaxPrefixes {
			return Preview{}, errors.New("route list exceeds 1024 destinations")
		}
	}
	if scanner.Err() != nil {
		return Preview{}, errors.New("route list line is too long")
	}
	return out, nil
}

// Append clones and validates the complete candidate. The caller applies it once.
func Append(d config.Document, req Request) (config.Document, Result, error) {
	out, result := d.Clone(), Result{}
	if len(req.Prefixes) == 0 && req.SourceID == "" || len(req.Prefixes) > MaxPrefixes {
		return d, result, errors.New("select 1-1024 route destinations")
	}
	seen := existing(d)
	if req.Binding == "" {
		req.Binding = "none"
	}
	if req.AutoUpdate {
		if _, err := validateURL(strings.TrimSpace(req.URL)); err != nil || req.IntervalMinutes < 15 || req.IntervalMinutes > 10080 || !req.SyncIPv4 && !req.SyncIPv6 {
			return d, result, errors.New("automatic URL updates require a valid public URL and an interval from 15 minutes to 7 days")
		}
		req.SourceID = security.Token()[:22]
		now := time.Now().UTC()
		out.RouteFeeds = append(out.RouteFeeds, config.RouteFeed{ID: req.SourceID, URL: strings.TrimSpace(req.URL), IntervalMinutes: req.IntervalMinutes, SyncIPv4: req.SyncIPv4, SyncIPv6: req.SyncIPv6, NextHop4: req.NextHop4, NextHop6: req.NextHop6, MED: req.MED, LocalPreference: req.LocalPreference, Communities: append([]string{}, req.Communities...), Binding: req.Binding, Enabled: true, RouteEnabled: req.Enabled, Description: req.Description, LastAttempt: now, LastUpdated: now})
	}
	for _, s := range req.Prefixes {
		p, err := destination(s)
		if err != nil {
			return d, Result{}, err
		}
		if seen[p.String()] {
			result.Skipped++
			continue
		}
		seen[p.String()] = true
		next := strings.TrimSpace(req.NextHop4)
		if p.Addr().Is6() {
			next = strings.TrimSpace(req.NextHop6)
		}
		out.Routes = append(out.Routes, config.Route{ID: security.Token()[:22], Prefix: p.String(), NextHop: next, MED: req.MED, LocalPreference: req.LocalPreference, Communities: append([]string{}, req.Communities...), Binding: req.Binding, Enabled: req.Enabled, Description: req.Description, SourceID: req.SourceID})
		result.Imported++
	}
	if err := out.Validate(); err != nil {
		return d, Result{}, err
	}
	return out, result, nil
}

func publicAddress(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast()
}

func validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.EqualFold(u.Hostname(), "localhost") {
		return nil, errors.New("route URL must be a public HTTP or HTTPS URL without credentials or a fragment")
	}
	if a, e := netip.ParseAddr(u.Hostname()); e == nil && !publicAddress(a) {
		return nil, errors.New("route URL cannot access local or private addresses")
	}
	return u, nil
}

func NewClient() *http.Client {
	transport := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, DisableKeepAlives: true}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, errors.New("route URL cannot access local or private addresses")
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		for _, ip := range ips {
			conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
			err = e
		}
		if err == nil {
			err = errors.New("route URL host has no addresses")
		}
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many route URL redirects")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("route URL redirect cannot downgrade HTTPS")
		}
		_, err := validateURL(req.URL.String())
		return err
	}}
}

func Fetch(ctx context.Context, client *http.Client, raw string, d config.Document) (Preview, error) {
	u, err := validateURL(strings.TrimSpace(raw))
	if err != nil {
		return Preview{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Preview{}, errors.New("invalid route URL")
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "RoutePilot route importer")
	resp, err := client.Do(req)
	if err != nil {
		return Preview{}, errors.New("route URL fetch failed; check connectivity and the URL")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Preview{}, fmt.Errorf("route URL returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return Preview{}, errors.New("route URL response could not be read")
	}
	return Parse(data, d)
}
