package surge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"surge-controller/internal/model"
	"time"
)

func ProbeAPI(ctx context.Context, base, token string) bool {
	u, e := url.Parse(base)
	if e != nil {
		return false
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/v1/outbound"
	r, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return false
	}
	r.Header.Set("X-Key", token)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(r)
	if e != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if e != nil || len(body) > 65536 {
		return false
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(body, &doc) != nil {
		return false
	}
	var mode string
	_ = json.Unmarshal(doc["mode"], &mode)
	return mode == "rule" || mode == "direct" || mode == "proxy"
}

// Explicit connection explanations use supplied metadata; DNS classification
// separately clears unavailable future metadata and applies its context guard.
func (a *Adapter) ExplainConnection(ctx context.Context, domain string, client netip.Addr, protocol string, port uint16) (model.RuleMatchResult, error) {
	name, e := model.NormalizeDomain(domain)
	if e != nil || name == "." || !client.IsValid() || port == 0 || (protocol != "udp" && protocol != "tcp") {
		return model.RuleMatchResult{}, errors.New("invalid connection test input")
	}
	args := []string{"rule", "explain", strings.TrimSuffix(name, "."), "source-address=" + client.String(), "source-port=0", "dest-port=" + strconv.Itoa(int(port)), "protocol=" + protocol, "is-local=off", "url=", "user-agent=", "process-path=", "sni=", "http-host=", "listen-port=0"}
	return a.explainWithDNS(ctx, strings.TrimSuffix(name, "."), args)
}
