package routingapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"time"

	"surge-controller/internal/config"
)

// openWrt manages routes through the router's ubus JSON-RPC endpoint
// (POST /ubus). Sections are named rp_<hash> inside /etc/config/network so
// leftovers stay recognizable; changes are committed and applied with a
// network reload. Uncommitted deltas survive across calls, so an interrupted
// reconciliation resumes on the next health cycle.
type openWrt struct {
	http     *httpClient
	username string
	password string
	notify   func(severity, message string)
	session  string
	expires  time.Time
}

func newOpenWrt(cfg config.RoutingAPI, notify func(string, string)) backend {
	return &openWrt{http: mustHTTP(cfg), username: cfg.Username, password: cfg.Password, notify: notify}
}

func mustHTTP(cfg config.RoutingAPI) *httpClient {
	c, err := newHTTPClient(cfg.BaseURL, cfg.VerifyTLS)
	if err != nil {
		return nil
	}
	return c
}

type ubusResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (o *openWrt) call(ctx context.Context, session, object, method string, params map[string]any) (json.RawMessage, error) {
	return o.callRetry(ctx, session, object, method, params, true)
}

// callRetry re-authenticates at most once per call. A second rejection is an
// error, which keeps permission failures from looping forever.
func (o *openWrt) callRetry(ctx context.Context, session, object, method string, params map[string]any, retry bool) (json.RawMessage, error) {
	if o.http == nil {
		return nil, errors.New("routing API base URL is invalid")
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "call",
		"params": []any{session, object, method, params},
	})
	status, data, err := o.http.do(ctx, "POST", "/ubus", nil, body)
	if err != nil {
		return nil, err
	}
	var reply ubusResponse
	if e := decodeJSON(status, data, &reply); e != nil {
		return nil, errors.New("OpenWrt ubus call failed: " + e.Error())
	}
	if reply.Error != nil {
		return nil, errors.New("OpenWrt ubus error " + itoa(reply.Error.Code) + ": " + reply.Error.Message)
	}
	var arr []json.RawMessage
	if e := json.Unmarshal(reply.Result, &arr); e != nil || len(arr) == 0 {
		return nil, errors.New("OpenWrt ubus response missing status code")
	}
	var code int
	if e := json.Unmarshal(arr[0], &code); e != nil {
		return nil, errors.New("OpenWrt ubus response missing status code")
	}
	if code == 6 && session != "" && retry {
		// Session expired; log in once and retry the original call.
		o.session = ""
		if e := o.login(ctx); e != nil {
			return nil, e
		}
		return o.callRetry(ctx, o.session, object, method, params, false)
	}
	if code != 0 {
		return nil, errors.New("OpenWrt ubus call rejected with code " + itoa(code))
	}
	if len(arr) > 1 {
		return arr[1], nil
	}
	return json.RawMessage("{}"), nil
}

func (o *openWrt) login(ctx context.Context) error {
	if o.session != "" && time.Now().Before(o.expires) {
		return nil
	}
	raw, err := o.call(ctx, "", "session", "login", map[string]any{"username": o.username, "password": o.password, "timeout": 3600})
	if err != nil {
		return err
	}
	var session struct {
		UbusRPCSession string `json:"ubus_rpc_session"`
		Timeout        int    `json:"timeout"`
	}
	if e := json.Unmarshal(raw, &session); e != nil || session.UbusRPCSession == "" {
		return errors.New("OpenWrt session login rejected the credentials")
	}
	o.session = session.UbusRPCSession
	o.expires = time.Now().Add(time.Duration(session.Timeout) * time.Second)
	return nil
}

func (o *openWrt) authorized(ctx context.Context, object, method string, params map[string]any) (json.RawMessage, error) {
	if e := o.login(ctx); e != nil {
		return nil, e
	}
	return o.call(ctx, o.session, object, method, params)
}

type uciSection struct {
	Type    string `json:".type"`
	Target  string `json:"target"`
	Netmask string `json:"netmask"`
	Gateway string `json:"gateway"`
}

func (o *openWrt) readSections(ctx context.Context) (map[string]uciSection, error) {
	raw, err := o.authorized(ctx, "uci", "get", map[string]any{"config": "network"})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Values map[string]uciSection `json:"values"`
	}
	if e := json.Unmarshal(raw, &payload); e != nil {
		return nil, errors.New("OpenWrt uci get response malformed")
	}
	return payload.Values, nil
}

// sectionPrefix rebuilds the announced prefix stored in a uci section.
func sectionPrefix(s uciSection) (string, bool) {
	if s.Target == "" {
		return "", false
	}
	if strings.Contains(s.Target, "/") {
		return s.Target, true
	}
	addr, e := netip.ParseAddr(s.Target)
	if e != nil {
		return "", false
	}
	if addr.Is6() {
		return s.Target + "/128", true
	}
	if s.Netmask == "" {
		return s.Target + "/32", true
	}
	mask, e := netip.ParseAddr(s.Netmask)
	if e != nil {
		return "", false
	}
	bits, valid := maskLength(mask)
	if !valid {
		return "", false
	}
	return netip.PrefixFrom(addr, bits).String(), true
}

func maskLength(mask netip.Addr) (int, bool) {
	b := mask.As4()
	bits, zeroSeen := 0, false
	for i := 0; i < 32; i++ {
		set := b[i/8]&(1<<(7-uint(i%8))) != 0
		if set {
			if zeroSeen {
				return 0, false
			}
			bits++
		} else {
			zeroSeen = true
		}
	}
	return bits, true
}

func (o *openWrt) peek(ctx context.Context, desired []desiredRoute) ([]RouteState, error) {
	sections, err := o.readSections(ctx)
	if err != nil {
		return nil, err
	}
	return o.statesFrom(sections, desired), nil
}

func (o *openWrt) statesFrom(sections map[string]uciSection, desired []desiredRoute) []RouteState {
	states := []RouteState{}
	wanted := map[string]desiredRoute{}
	for _, d := range desired {
		wanted[openWrtSection(d.name)] = d
	}
	for _, d := range desired {
		states = append(states, RouteState{Name: d.name, Prefix: d.adv.Prefix, NextHop: d.adv.NextHop, Present: sectionMatches(sections[openWrtSection(d.name)], d)})
	}
	for section, raw := range sections {
		if !strings.HasPrefix(section, "rp_") || raw.Type != "route" && raw.Type != "route6" {
			continue
		}
		if _, ok := wanted[section]; ok {
			continue
		}
		prefix, ok := sectionPrefix(raw)
		if !ok {
			prefix = "unknown"
		}
		states = append(states, RouteState{Name: section, Prefix: prefix, NextHop: raw.Gateway, Managed: true})
	}
	return states
}

func sectionMatches(s uciSection, d desiredRoute) bool {
	if s.Type != "route" && s.Type != "route6" {
		return false
	}
	prefix, ok := sectionPrefix(s)
	if !ok || prefix != d.adv.Prefix {
		return false
	}
	return s.Gateway == d.adv.NextHop
}

func (o *openWrt) sync(ctx context.Context, desired []desiredRoute) ([]RouteState, error) {
	if e := o.login(ctx); e != nil {
		return nil, e
	}
	sections, err := o.readSections(ctx)
	if err != nil {
		return nil, err
	}
	changed := false
	wanted := map[string]desiredRoute{}
	for _, d := range desired {
		wanted[openWrtSection(d.name)] = d
	}
	for section, raw := range sections {
		if !strings.HasPrefix(section, "rp_") || raw.Type != "route" && raw.Type != "route6" {
			continue
		}
		if _, keep := wanted[section]; keep {
			continue
		}
		if _, err := o.call(ctx, o.session, "uci", "delete", map[string]any{"config": "network", "section": section}); err != nil {
			return nil, err
		}
		changed = true
		if o.notify != nil {
			o.notify("INFO", "Router API route withdrawn: "+section)
		}
	}
	for _, d := range desired {
		section := openWrtSection(d.name)
		if sectionMatches(sections[section], d) {
			continue
		}
		if _, exists := sections[section]; exists {
			if _, err := o.call(ctx, o.session, "uci", "delete", map[string]any{"config": "network", "section": section}); err != nil {
				return nil, err
			}
		}
		facts := routeFactsFor(d.adv.Prefix, d.adv.NextHop)
		values := map[string]any{"target": facts.target, "gateway": d.adv.NextHop}
		if facts.netmask != "" {
			values["netmask"] = facts.netmask
		}
		if _, err := o.call(ctx, o.session, "uci", "add", map[string]any{"config": "network", "type": facts.routeType, "name": section, "values": values}); err != nil {
			return nil, err
		}
		changed = true
		if o.notify != nil {
			o.notify("INFO", "Router API route published: "+d.adv.Prefix+" via "+d.adv.NextHop)
		}
	}
	if changed {
		if _, err := o.call(ctx, o.session, "uci", "commit", map[string]any{"config": "network"}); err != nil {
			return nil, err
		}
		if _, err := o.call(ctx, o.session, "network", "reload", map[string]any{}); err != nil {
			return nil, err
		}
	}
	sections, err = o.readSections(ctx)
	if err != nil {
		return nil, err
	}
	return o.statesFrom(sections, desired), nil
}

func (o *openWrt) close(ctx context.Context) {
	if o.session != "" {
		_, _ = o.call(ctx, o.session, "session", "destroy", map[string]any{"ubus_rpc_session": o.session})
		o.session = ""
	}
}
