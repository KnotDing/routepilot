package config

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// ManagementNetwork accepts a single IP or a canonical CIDR. Zone identifiers,
// unspecified/multicast addresses, and default routes are not identities.
func ManagementNetwork(raw string) (netip.Prefix, error) {
	if a, err := netip.ParseAddr(raw); err == nil {
		if a.Zone() == "" {
			a = a.Unmap()
		}
		if a.Zone() == "" && !a.IsUnspecified() && !a.IsMulticast() {
			return netip.PrefixFrom(a, a.BitLen()), nil
		}
	}
	p, err := netip.ParsePrefix(raw)
	if err != nil || p != p.Masked() || p.Bits() == 0 || p.Addr().IsMulticast() || p.Addr().Is4In6() {
		return netip.Prefix{}, errors.New("management network must be an IP or canonical non-default CIDR")
	}
	return p, nil
}

// ManagementOrigin accepts a concrete HTTP(S) origin, including a DNS name
// used by a reverse proxy. Paths and embedded credentials are never accepted.
func ManagementOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || len(raw) > 512 || u.Host == "" || strings.HasSuffix(u.Host, ":") || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.HasSuffix(raw, "#") {
		return nil, errors.New("management origin must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	host := u.Hostname()
	if a, e := netip.ParseAddr(host); e == nil {
		if a.Zone() != "" || a.Unmap().IsUnspecified() || a.Unmap().IsMulticast() {
			return nil, errors.New("invalid management origin host")
		}
	} else {
		if len(host) > 253 || strings.ContainsAny(host, "[]:") || strings.ContainsAny(u.Host, "[]") {
			return nil, errors.New("invalid management origin host")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, errors.New("invalid management origin host")
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
					return nil, errors.New("invalid management origin host")
				}
			}
		}
	}
	if port := u.Port(); port != "" {
		if n, e := strconv.Atoi(port); e != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid management origin port")
		}
	}
	u.Host = strings.ToLower(u.Host)
	// Default ports and a trailing slash denote the same browser origin.
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		u.Host = strings.TrimSuffix(u.Host, ":"+u.Port())
	}
	u.Path = ""
	return u, nil
}

func (s Security) ValidateIPAccess() error {
	switch s.ProxyClientIPHeader {
	case "", "X-Forwarded-For", "X-Real-IP", "Forwarded":
	default:
		return errors.New("invalid management proxy client IP header")
	}
	for _, list := range []struct {
		values []string
		name   string
	}{{s.AuthBypassNetworks, "invalid management authentication bypass network"}, {s.TrustedProxies, "invalid management trusted proxy"}} {
		if len(list.values) > 128 {
			return errors.New(list.name)
		}
		for _, value := range list.values {
			if _, err := ManagementNetwork(value); err != nil {
				return errors.New(list.name)
			}
		}
	}
	if len(s.TrustedProxies) > 0 && !s.Authentication {
		return errors.New("trusted management proxies require authentication")
	}
	if len(s.ProxyOrigins) > 32 || len(s.ProxyOrigins) > 0 && len(s.TrustedProxies) == 0 {
		return errors.New("management proxy origins require trusted proxies (maximum 32)")
	}
	for _, value := range s.ProxyOrigins {
		if _, err := ManagementOrigin(value); err != nil {
			return errors.New("invalid management proxy origin")
		}
	}
	return nil
}
