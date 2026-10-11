package security

import (
	"context"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"surge-controller/internal/config"
)

type secureRequestKey struct{}

func secureRequest(r *http.Request) bool {
	if secure, ok := r.Context().Value(secureRequestKey{}).(bool); ok {
		return secure
	}
	return r.TLS != nil
}

func matchesIP(ip netip.Addr, networks []string) bool {
	for _, raw := range networks {
		if prefix, err := netip.ParsePrefix(raw); err == nil && prefix.Contains(ip.Unmap()) {
			return true
		}
		if address, err := netip.ParseAddr(raw); err == nil && address.Unmap() == ip.Unmap() {
			return true
		}
	}
	return false
}

type clientIdentity struct {
	ip          netip.Addr
	trustedPeer bool
	forwarded   bool
	proto, host string
}

func proxyIP(raw string) (netip.Addr, error) {
	// Forwarded permits quoted IPv6 literals and IP:port. Never accept an
	// unknown/obfuscated node or a scoped address as an authentication identity.
	a, err := netip.ParseAddr(raw)
	if err != nil && strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		a, err = netip.ParseAddr(raw[1 : len(raw)-1])
		if err == nil && !a.Is6() {
			return netip.Addr{}, errors.New("invalid management proxy client IP")
		}
	}
	if err != nil {
		ap, e := netip.ParseAddrPort(raw)
		if e != nil || ap.Port() == 0 {
			return netip.Addr{}, errors.New("invalid management proxy client IP")
		}
		a = ap.Addr()
	}
	if a.Zone() != "" || a.Unmap().IsUnspecified() || a.Unmap().IsMulticast() {
		return netip.Addr{}, errors.New("invalid management proxy client IP")
	}
	return a.Unmap(), nil
}

// Split an HTTP list without treating separators inside quoted strings as
// separators. Header sizes and hop counts are bounded before parsing.
func proxyList(raw string) ([]string, error) {
	if len(raw) > 8192 {
		return nil, errors.New("invalid management proxy headers")
	}
	var parts []string
	quoted, escaped, start := false, false, 0
	for i, ch := range raw {
		if ch < 32 && ch != '\t' || ch == 127 {
			return nil, errors.New("invalid management proxy headers")
		}
		if escaped {
			escaped = false
		} else if quoted && ch == '\\' {
			escaped = true
		} else if ch == '"' {
			quoted = !quoted
		} else if !quoted && ch == ',' {
			parts = append(parts, strings.TrimSpace(raw[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(raw[start:]))
	if quoted || escaped || len(parts) > 64 {
		return nil, errors.New("invalid management proxy headers")
	}
	return parts, nil
}

func requestIdentity(r *http.Request, c config.Security) (clientIdentity, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return clientIdentity{}, errors.New("invalid management client IP")
	}
	ip, err := proxyIP(host)
	if err != nil {
		return clientIdentity{}, err
	}
	id := clientIdentity{ip: ip, trustedPeer: matchesIP(ip, c.TrustedProxies)}
	if !id.trustedPeer {
		// All forwarding headers from direct/untrusted clients are ignored.
		return id, nil
	}
	header := c.ProxyClientIPHeader
	if header == "" {
		header = "X-Forwarded-For"
	}
	values := r.Header.Values(header)
	if len(values) == 0 {
		// A proxy without a client identity can use password authentication,
		// but cannot borrow its own whitelisted address to bypass login.
		return id, nil
	}
	parts, err := proxyList(strings.Join(values, ","))
	if err != nil || header == "X-Real-IP" && len(parts) != 1 {
		return id, errors.New("invalid management proxy headers")
	}
	for i := len(parts) - 1; i >= 0 && matchesIP(id.ip, c.TrustedProxies); i-- {
		value := parts[i]
		if header == "Forwarded" {
			_, params, e := mime.ParseMediaType("forwarded;" + value)
			if e != nil || params["for"] == "" {
				return id, errors.New("invalid management proxy headers")
			}
			value = params["for"]
			id.proto, id.host = params["proto"], params["host"]
		}
		id.ip, err = proxyIP(value)
		if err != nil {
			return id, err
		}
		id.forwarded = true
	}
	if header != "Forwarded" {
		// Use the last value added by the immediate trusted proxy. Deployments
		// must overwrite these headers or append their own verified values.
		for name, target := range map[string]*string{"X-Forwarded-Proto": &id.proto, "X-Forwarded-Host": &id.host} {
			if values := r.Header.Values(name); len(values) > 0 {
				parts, e := proxyList(strings.Join(values, ","))
				if e != nil {
					return id, e
				}
				*target = parts[len(parts)-1]
			}
		}
	}
	return id, nil
}

func managementRequest(r *http.Request, c config.Security, id clientIdentity) (*http.Request, error) {
	scheme, host := "http", r.Host
	if r.TLS != nil {
		scheme = "https"
	}
	if id.forwarded {
		if id.proto != "" {
			scheme = strings.ToLower(id.proto)
		}
		if id.host != "" {
			host = id.host
		}
	}
	if strings.ContainsAny(host, "/?#@\\ \t\r\n") {
		return r, errors.New("invalid management host")
	}
	u, err := config.ManagementOrigin(scheme + "://" + host)
	if err != nil {
		return r, errors.New("invalid management host")
	}
	_, ipErr := netip.ParseAddr(u.Hostname())
	if id.forwarded || id.trustedPeer && ipErr != nil && u.Hostname() != "localhost" {
		// Explicit public origins permit domain-based proxies and TLS
		// termination, without relaxing Host validation for direct requests.
		if len(c.ProxyOrigins) > 0 {
			allowed := false
			for _, raw := range c.ProxyOrigins {
				origin, e := config.ManagementOrigin(raw)
				if e == nil && origin.String() == u.String() {
					allowed = true
				}
			}
			if !allowed {
				return r, errors.New("management proxy origin is not allowed")
			}
		} else if ipErr != nil && u.Hostname() != "localhost" {
			return r, errors.New("management proxy origin is not allowed")
		}
	} else if ipErr != nil && u.Hostname() != "localhost" {
		return r, errors.New("invalid management host")
	}
	if raw := r.Header.Get("Origin"); raw != "" {
		origin, e := config.ManagementOrigin(raw)
		if e != nil || origin.String() != u.String() {
			return r, errors.New("invalid origin")
		}
	}
	return r.WithContext(context.WithValue(r.Context(), secureRequestKey{}, scheme == "https")), nil
}
