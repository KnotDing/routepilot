package resolver

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/miekg/dns"
)

type ResolverEndpoint struct {
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	URL      string `json:"url,omitempty"`
	TLSName  string `json:"tls_name,omitempty"`
}

func (e ResolverEndpoint) Validate() error {
	ap, err := netip.ParseAddrPort(e.Address)
	if err != nil || ap.Port() == 0 || ap.Addr().IsUnspecified() || ap.Addr().IsMulticast() {
		return errors.New("resolver address must be a literal IP:port")
	}
	switch e.Protocol {
	case "udp", "tcp":
	case "dot":
		if e.TLSName == "" {
			return errors.New("DoT requires a TLS server name")
		}
	case "doh":
		u, err := url.Parse(e.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return errors.New("DoH requires an HTTPS URL without credentials or fragment")
		}
	default:
		return errors.New("unsupported resolver protocol")
	}
	return nil
}

type UpstreamTransport interface {
	Exchange(context.Context, ResolverEndpoint, *dns.Msg) (*dns.Msg, error)
}

// EgressDialer is the only path from forwarding to a socket. A transparent
// deployment supplies a pinned dialer, never the system-default routing path.
type EgressDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}
type WireTransport struct {
	Dialer    EgressDialer
	Timeout   time.Duration
	TLSConfig *tls.Config
}

func NewWireTransport(d EgressDialer, timeout time.Duration) (*WireTransport, error) {
	if d == nil || timeout <= 0 {
		return nil, errors.New("explicit egress dialer and timeout required")
	}
	return &WireTransport{Dialer: d, Timeout: timeout}, nil
}
func (t *WireTransport) Exchange(ctx context.Context, e ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	request := q.Copy()
	request.Id = dns.Id()
	var reply *dns.Msg
	var err error
	if e.Protocol == "doh" {
		reply, err = t.doh(ctx, e, request)
	} else {
		reply, err = t.wire(ctx, e, request)
	}
	if err != nil {
		return nil, err
	}
	if err = ValidateResponse(request, reply); err != nil {
		return nil, err
	}
	reply.Id = q.Id
	return reply, nil
}
func ValidateResponse(q, r *dns.Msg) error {
	if r == nil || !r.Response || r.Id != q.Id || r.Opcode != q.Opcode || len(r.Question) != len(q.Question) {
		return errors.New("upstream response does not match query")
	}
	for i := range q.Question {
		a, b := q.Question[i], r.Question[i]
		if !equalQuestion(a, b) {
			return errors.New("upstream question mismatch")
		}
	}
	return nil
}
func equalQuestion(a, b dns.Question) bool {
	return dns.CanonicalName(a.Name) == dns.CanonicalName(b.Name) && a.Qtype == b.Qtype && a.Qclass == b.Qclass
}
func (t *WireTransport) wire(ctx context.Context, e ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
	network := "tcp"
	if e.Protocol == "udp" {
		network = "udp"
	}
	conn, err := t.Dialer.DialContext(ctx, network, e.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if e.Protocol == "dot" {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: e.TLSName}
		if t.TLSConfig != nil {
			cfg = t.TLSConfig.Clone()
			cfg.ServerName = e.TLSName
			cfg.MinVersion = tls.VersionTLS12
		}
		secure := tls.Client(conn, cfg)
		if err = secure.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = secure
	}
	dc := &dns.Conn{Conn: conn, UDPSize: dns.MaxMsgSize}
	if err = dc.WriteMsg(q); err != nil {
		return nil, err
	}
	r, err := dc.ReadMsg()
	if err != nil {
		return nil, err
	}
	if err = ValidateResponse(q, r); err != nil {
		return nil, err
	}
	if r.Truncated && e.Protocol == "udp" {
		e.Protocol = "tcp"
		return t.wire(ctx, e, q)
	}
	return r, nil
}
func (t *WireTransport) doh(ctx context.Context, e ResolverEndpoint, q *dns.Msg) (*dns.Msg, error) {
	raw, err := q.Pack()
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(e.URL)
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	if t.TLSConfig != nil {
		cfg = t.TLSConfig.Clone()
		cfg.ServerName = u.Hostname()
		cfg.MinVersion = tls.VersionTLS12
	}
	// The URL hostname is used for TLS/Host only. Bootstrap uses a configured
	// literal address, so DoH bootstrapping never invokes system DNS.
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: cfg, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return t.Dialer.DialContext(ctx, network, e.Address)
	}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: t.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("DoH redirects disabled") }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("DoH status %d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "application/dns-message" {
		return nil, errors.New("invalid DoH content type")
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, dns.MaxMsgSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > dns.MaxMsgSize {
		return nil, errors.New("DoH response exceeds DNS packet limit")
	}
	r := new(dns.Msg)
	if err = r.Unpack(raw); err != nil {
		return nil, err
	}
	return r, nil
}
