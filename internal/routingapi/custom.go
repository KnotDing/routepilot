package routingapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"surge-controller/internal/config"
)

// custom renders operator-provided request templates. The list template is
// mandatory: reconciliation compares the router's answer with the desired
// set, which is what makes restarts and leftovers safe. Templates must use
// {{name}} as the route identity; entries whose name starts with
// "routepilot-" but no longer match the desired set are withdrawn.
type custom struct {
	http    *httpClient
	cfg     config.RoutingAPI
	notify  func(severity, message string)
	session string
	expires time.Time
}

func newCustom(cfg config.RoutingAPI, notify func(string, string)) backend {
	return &custom{http: mustHTTP(cfg), cfg: cfg, notify: notify}
}

type templateVars map[string]string

func (v templateVars) render(tmpl string) string {
	out := tmpl
	for key, value := range v {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
		out = strings.ReplaceAll(out, "{{ "+key+" }}", value)
	}
	return out
}

func (c *custom) baseVars(ctx context.Context) (templateVars, error) {
	vars := templateVars{}
	switch c.cfg.Auth {
	case "ubus":
		if e := c.login(ctx); e != nil {
			return nil, e
		}
		vars["session"] = c.session
	case "header":
		vars["api_key"] = c.cfg.HeaderValue
	}
	return vars, nil
}

func (c *custom) login(ctx context.Context) error {
	if c.session != "" && time.Now().Before(c.expires) {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "call",
		"params": []any{"00000000000000000000000000000000", "session", "login", map[string]any{"username": c.cfg.Username, "password": c.cfg.Password, "timeout": 3600}},
	})
	status, data, err := c.http.do(ctx, "POST", "/ubus", nil, body)
	if err != nil {
		return err
	}
	var reply struct {
		Result []json.RawMessage `json:"result"`
	}
	if e := decodeJSON(status, data, &reply); e != nil || len(reply.Result) < 2 {
		return errors.New("custom routing API login failed")
	}
	var session struct {
		UbusRPCSession string `json:"ubus_rpc_session"`
		Timeout        int    `json:"timeout"`
	}
	if e := json.Unmarshal(reply.Result[1], &session); e != nil || session.UbusRPCSession == "" {
		return errors.New("custom routing API login rejected the credentials")
	}
	c.session = session.UbusRPCSession
	c.expires = time.Now().Add(time.Duration(session.Timeout) * time.Second)
	return nil
}

func (c *custom) request(ctx context.Context, q config.RoutingAPIRequest, vars templateVars) (int, []byte, error) {
	if c.http == nil {
		return 0, nil, errors.New("routing API base URL is invalid")
	}
	headers := map[string]string{}
	for k, v := range q.Headers {
		headers[k] = vars.render(v)
	}
	if c.cfg.Auth == "header" && c.cfg.HeaderName != "" {
		headers[c.cfg.HeaderName] = c.cfg.HeaderValue
	}
	body := vars.render(q.Body)
	var payload []byte
	if strings.TrimSpace(body) != "" {
		if !json.Valid([]byte(body)) {
			return 0, nil, errors.New("rendered routing API body is not valid JSON")
		}
		payload = []byte(body)
	}
	return c.http.do(ctx, vars.render(q.Method), vars.render(q.Path), headers, payload)
}

type listedRoute struct {
	id, name, prefix, nextHop string
}

func (c *custom) list(ctx context.Context, vars templateVars) ([]listedRoute, error) {
	q := c.cfg.List
	status, data, err := c.request(ctx, q, vars)
	if err != nil {
		return nil, err
	}
	var parsed any
	if e := decodeJSON(status, data, &parsed); e != nil {
		return nil, errors.New("custom routing API list failed: " + e.Error())
	}
	items := parsed
	if q.ItemsPath != "" {
		for _, segment := range strings.Split(strings.Trim(q.ItemsPath, "/."), ".") {
			object, ok := items.(map[string]any)
			if !ok {
				return nil, errors.New("custom routing API list items path does not match the response")
			}
			items, ok = object[segment]
			if !ok {
				return nil, errors.New("custom routing API list items path does not match the response")
			}
		}
	}
	rows := []listedRoute{}
	collect := func(entry map[string]any, keyID string) {
		field := func(name, fallback string) string {
			if name != "" {
				if v, ok := entry[name].(string); ok {
					return v
				}
			}
			return fallback
		}
		id := field(q.IDField, keyID)
		name := field(q.NameField, "")
		if name == "" {
			name = id
		}
		rows = append(rows, listedRoute{id: id, name: name, prefix: field(q.PrefixField, ""), nextHop: field(q.NextHopField, "")})
	}
	switch items := items.(type) {
	case []any:
		for _, item := range items {
			if entry, ok := item.(map[string]any); ok {
				collect(entry, "")
			}
		}
	case map[string]any:
		for key, item := range items {
			if entry, ok := item.(map[string]any); ok {
				collect(entry, key)
			}
		}
	default:
		return nil, errors.New("custom routing API list response must be an array or object")
	}
	return rows, nil
}

func (c *custom) routeVars(vars templateVars, d desiredRoute) templateVars {
	facts := routeFactsFor(d.adv.Prefix, d.adv.NextHop)
	merged := templateVars{}
	for k, v := range vars {
		merged[k] = v
	}
	merged["name"] = d.name
	merged["section"] = openWrtSection(d.name)
	merged["prefix"] = d.adv.Prefix
	merged["target"] = facts.target
	merged["netmask"] = facts.netmask
	merged["next_hop"] = d.adv.NextHop
	merged["route_type"] = facts.routeType
	merged["values"] = facts.values
	return merged
}

func (c *custom) statesFrom(rows []listedRoute, desired []desiredRoute) []RouteState {
	states := []RouteState{}
	for _, d := range desired {
		state := RouteState{Name: d.name, Prefix: d.adv.Prefix, NextHop: d.adv.NextHop}
		for _, row := range rows {
			if row.name != d.name {
				continue
			}
			state.ID = row.id
			state.Present = row.prefix == d.adv.Prefix && (row.nextHop == d.adv.NextHop || row.nextHop == "")
		}
		states = append(states, state)
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.name, "routepilot-") {
			continue
		}
		claimed := false
		for _, d := range desired {
			if d.name == row.name {
				claimed = true
				break
			}
		}
		if !claimed {
			states = append(states, RouteState{Name: row.name, Prefix: row.prefix, NextHop: row.nextHop, ID: row.id, Managed: true})
		}
	}
	return states
}

func (c *custom) peek(ctx context.Context, desired []desiredRoute) ([]RouteState, error) {
	vars, err := c.baseVars(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := c.list(ctx, vars)
	if err != nil {
		return nil, err
	}
	return c.statesFrom(rows, desired), nil
}

func (c *custom) sync(ctx context.Context, desired []desiredRoute) ([]RouteState, error) {
	vars, err := c.baseVars(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := c.list(ctx, vars)
	if err != nil {
		return nil, err
	}
	current := map[string]listedRoute{}
	for _, row := range rows {
		if strings.HasPrefix(row.name, "routepilot-") {
			current[row.name] = row
		}
	}
	for name, row := range current {
		wanted := false
		for _, d := range desired {
			if d.name == name {
				wanted = d.adv.Prefix == row.prefix && (row.nextHop == d.adv.NextHop || row.nextHop == "")
				break
			}
		}
		if wanted {
			continue
		}
		removeVars := templateVars{}
		for k, v := range vars {
			removeVars[k] = v
		}
		removeVars["id"] = row.id
		removeVars["name"] = row.name
		removeVars["prefix"] = row.prefix
		removeVars["next_hop"] = row.nextHop
		status, data, err := c.request(ctx, c.cfg.Remove, removeVars)
		if err != nil {
			return nil, err
		}
		if e := decodeJSON(status, data, nil); e != nil {
			return nil, errors.New("custom routing API removal failed: " + e.Error())
		}
		if c.notify != nil {
			c.notify("INFO", "Router API route withdrawn: "+row.prefix)
		}
	}
	for _, d := range desired {
		if row, exists := current[d.name]; exists && row.prefix == d.adv.Prefix && (row.nextHop == d.adv.NextHop || row.nextHop == "") {
			continue
		}
		status, data, err := c.request(ctx, c.cfg.Add, c.routeVars(vars, d))
		if err != nil {
			return nil, err
		}
		if e := decodeJSON(status, data, nil); e != nil {
			return nil, errors.New("custom routing API publication failed: " + e.Error())
		}
		if c.notify != nil {
			c.notify("INFO", "Router API route published: "+d.adv.Prefix+" via "+d.adv.NextHop)
		}
	}
	return c.peek(ctx, desired)
}

func (c *custom) close(ctx context.Context) {
	// Sessions expire server-side; dropping the reference avoids reuse after
	// the backend is replaced.
	c.session = ""
}
