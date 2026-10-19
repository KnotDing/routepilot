package routingapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"surge-controller/internal/bgp"
	"surge-controller/internal/config"
)

func advertisement(prefix, nextHop, kind string) bgp.Advertisement {
	return bgp.Advertisement{Prefix: prefix, NextHop: nextHop, Kind: kind, Binding: "surge"}
}

func collectEvents() (*[]string, func(string, string)) {
	events := &[]string{}
	return events, func(severity, message string) {
		*events = append(*events, severity+" "+message)
	}
}

// ubusServer emulates the OpenWrt JSON-RPC surface used by the preset.
type ubusServer struct {
	mu       sync.Mutex
	sessions map[string]bool
	sections map[string]map[string]any
	requests []string
}

func newUbusServer() *ubusServer {
	return &ubusServer{sessions: map[string]bool{}, sections: map[string]map[string]any{}}
}

func (s *ubusServer) record(format string, args ...any) {
	s.requests = append(s.requests, fmt.Sprintf(format, args...))
}

func (s *ubusServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if len(body.Params) < 3 {
		w.WriteHeader(400)
		return
	}
	var session, object, method string
	_ = json.Unmarshal(body.Params[0], &session)
	_ = json.Unmarshal(body.Params[1], &object)
	_ = json.Unmarshal(body.Params[2], &method)
	s.mu.Lock()
	defer s.mu.Unlock()
	write := func(payload any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
	if object == "session" && method == "login" {
		s.sessions["sid"] = true
		s.record("login")
		write(map[string]any{"result": []any{0, map[string]any{"ubus_rpc_session": "sid", "timeout": 3600}}})
		return
	}
	if !s.sessions[session] {
		write(map[string]any{"result": []any{6}})
		return
	}
	var params map[string]any
	if len(body.Params) > 3 {
		_ = json.Unmarshal(body.Params[3], &params)
	}
	switch object + "." + method {
	case "uci.get":
		s.record("uci.get")
		write(map[string]any{"result": []any{0, map[string]any{"values": s.sections}}})
	case "uci.add":
		name, _ := params["name"].(string)
		values, _ := params["values"].(map[string]any)
		entry := map[string]any{".type": params["type"]}
		for k, v := range values {
			entry[k] = v
		}
		s.sections[name] = entry
		s.record("uci.add %s %v", name, values)
		write(map[string]any{"result": []any{0, map[string]any{"section": name}}})
	case "uci.delete":
		section, _ := params["section"].(string)
		delete(s.sections, section)
		s.record("uci.delete %s", section)
		write(map[string]any{"result": []any{0, map[string]any{}}})
	case "uci.commit":
		s.record("uci.commit")
		write(map[string]any{"result": []any{0, map[string]any{}}})
	case "network.reload":
		s.record("network.reload")
		write(map[string]any{"result": []any{0, map[string]any{}}})
	default:
		write(map[string]any{"result": []any{6}})
	}
}

func openWrtConfig(base string) config.RoutingAPI {
	return config.RoutingAPI{Preset: "openwrt", BaseURL: base, Username: "root", Password: "secret"}
}

func TestOpenWrtPublishUpdateAndWithdraw(t *testing.T) {
	server := newUbusServer()
	api := httptest.NewServer(server)
	defer api.Close()
	events, notify := collectEvents()
	m, err := New(openWrtConfig(api.URL), notify)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	ctx := context.Background()
	desired := []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.2", "fake-ip")}
	if e := m.Reconcile(ctx, desired); e != nil {
		t.Fatalf("publish failed: %v", e)
	}
	if !strings.Contains(strings.Join(server.requests, ";"), "uci.commit") {
		t.Fatalf("expected commit after publish, got %v", server.requests)
	}
	ready, e := m.Refresh(ctx)
	if e != nil || !ready {
		status := m.Status()
		t.Fatalf("expected ready after publish: ready=%v err=%v last=%v", ready, e, status.LastError)
	}
	status := m.Status()
	if status.Published != 1 || len(status.Routes) != 1 || !status.Routes[0].Present {
		t.Fatalf("unexpected status: %+v", status)
	}
	// Next hop change must rewrite the section instead of duplicating it.
	updated := []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.9", "fake-ip")}
	if e := m.Reconcile(ctx, updated); e != nil {
		t.Fatalf("update failed: %v", e)
	}
	ready, e = m.Refresh(ctx)
	if e != nil || !ready {
		t.Fatalf("expected ready after update: %v %v", ready, e)
	}
	// Withdraw everything; leftover rp_ sections must disappear.
	if e := m.Reconcile(ctx, nil); e != nil {
		t.Fatalf("withdraw failed: %v", e)
	}
	if len(server.sections) != 0 {
		t.Fatalf("expected no sections left, got %v", server.sections)
	}
	if len(*events) == 0 {
		t.Fatal("expected notify events")
	}
}

func TestOpenWrtIPv6Prefix(t *testing.T) {
	server := newUbusServer()
	api := httptest.NewServer(server)
	defer api.Close()
	m, err := New(openWrtConfig(api.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	ctx := context.Background()
	if e := m.Reconcile(ctx, []bgp.Advertisement{advertisement("fd00:1234::/64", "fe80::1", "fake-ip")}); e != nil {
		t.Fatalf("publish failed: %v", e)
	}
	found := false
	for section, entry := range server.sections {
		if !strings.HasPrefix(section, "rp_") {
			continue
		}
		if entry[".type"] != "route6" || entry["target"] != "fd00:1234::/64" || entry["gateway"] != "fe80::1" {
			t.Fatalf("unexpected route6 values: %s %+v", section, entry)
		}
		found = true
	}
	if !found {
		t.Fatalf("route6 section missing: %+v", server.sections)
	}
	ready, e := m.Refresh(ctx)
	if e != nil || !ready {
		t.Fatalf("expected ready: %v %v", ready, e)
	}
}

func TestOpenWrtRejectedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":[6]}`))
	}))
	defer server.Close()
	m, err := New(openWrtConfig(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	if e := m.Reconcile(context.Background(), []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.2", "fake-ip")}); e == nil {
		t.Fatal("expected login failure to surface")
	}
	if m.Status().LastError == "" {
		t.Fatal("expected status error")
	}
}

// unifiServer emulates the UniFi OS Integration API.
type unifiServer struct {
	mu     sync.Mutex
	routes []map[string]any
	nextID int
}

func (s *unifiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-API-KEY") != "key123" {
		w.WriteHeader(401)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" {
		_ = json.NewEncoder(w).Encode(s.routes)
		return
	}
	if r.Method == "DELETE" {
		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/integration/v1/routes/")
		for i, row := range s.routes {
			if row["routeId"] == id {
				s.routes = append(s.routes[:i], s.routes[i+1:]...)
				break
			}
		}
		_, _ = w.Write([]byte(`{}`))
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.nextID++
	body["routeId"] = fmt.Sprintf("rt%d", s.nextID)
	s.routes = append(s.routes, body)
	_, _ = w.Write([]byte(`{}`))
}

func TestUniFiLifecycle(t *testing.T) {
	server := &unifiServer{}
	api := httptest.NewServer(server)
	defer api.Close()
	cfg := config.RoutingAPI{Preset: "unifi", BaseURL: api.URL, HeaderValue: "key123"}
	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	ctx := context.Background()
	if e := m.Reconcile(ctx, []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.2", "fake-ip"), advertisement("203.0.113.0/24", "192.0.2.3", "dns-interception")}); e != nil {
		t.Fatalf("publish failed: %v", e)
	}
	if len(server.routes) != 2 {
		t.Fatalf("expected two routes, got %+v", server.routes)
	}
	ready, e := m.Refresh(ctx)
	if e != nil || !ready {
		t.Fatalf("expected ready: %v %v", ready, e)
	}
	if e := m.Reconcile(ctx, []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.2", "fake-ip")}); e != nil {
		t.Fatalf("withdraw failed: %v", e)
	}
	if len(server.routes) != 1 || server.routes[0]["name"] != RouteName("fake-ip", "198.18.0.0/15") {
		t.Fatalf("unexpected remaining routes: %+v", server.routes)
	}
	status := m.Status()
	if status.Published != 1 {
		t.Fatalf("expected one published route, got %+v", status)
	}
}

func customConfig(base string) config.RoutingAPI {
	return config.RoutingAPI{
		Preset: "custom", Auth: "header", BaseURL: base, HeaderName: "X-API-KEY", HeaderValue: "key123",
		Add:    config.RoutingAPIRequest{Method: "POST", Path: "/routes", Body: `{"name":"{{name}}","network":"{{prefix}}","gateway":"{{next_hop}}"}`},
		Remove: config.RoutingAPIRequest{Method: "DELETE", Path: "/routes/{{id}}"},
		List:   config.RoutingAPIRequest{Method: "GET", Path: "/routes", ItemsPath: "items", IDField: "id", NameField: "name", PrefixField: "network", NextHopField: "gateway"},
	}
}

func TestCustomTemplateLifecycle(t *testing.T) {
	type entry map[string]string
	state := struct {
		sync.Mutex
		rows  map[string]entry
		order []string
	}{rows: map[string]entry{}}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "key123" {
			w.WriteHeader(401)
			return
		}
		state.Lock()
		defer state.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET":
			items := []entry{}
			for _, id := range state.order {
				items = append(items, state.rows[id])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		case r.Method == "POST":
			var body entry
			_ = json.NewDecoder(r.Body).Decode(&body)
			id := fmt.Sprintf("id%d", len(state.rows)+1)
			body["id"] = id
			state.rows[id] = body
			state.order = append(state.order, id)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == "DELETE":
			id := strings.TrimPrefix(r.URL.Path, "/routes/")
			delete(state.rows, id)
			for i, v := range state.order {
				if v == id {
					state.order = append(state.order[:i], state.order[i+1:]...)
				}
			}
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer api.Close()
	m, err := New(customConfig(api.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	ctx := context.Background()
	if e := m.Reconcile(ctx, []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.2", "fake-ip")}); e != nil {
		t.Fatalf("publish failed: %v", e)
	}
	ready, e := m.Refresh(ctx)
	if e != nil || !ready {
		t.Fatalf("expected ready: %v %v", ready, e)
	}
	// Next hop change: removal by id then re-add with the same name.
	if e := m.Reconcile(ctx, []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.9", "fake-ip")}); e != nil {
		t.Fatalf("update failed: %v", e)
	}
	ready, e = m.Refresh(ctx)
	if e != nil || !ready {
		t.Fatalf("expected ready after update: %v %v", ready, e)
	}
	if e := m.Reconcile(ctx, nil); e != nil {
		t.Fatalf("withdraw failed: %v", e)
	}
	if len(state.rows) != 0 {
		t.Fatalf("expected empty router, got %v", state.rows)
	}
}

func TestSteadyStateThrottling(t *testing.T) {
	server := &unifiServer{}
	api := httptest.NewServer(server)
	defer api.Close()
	cfg := config.RoutingAPI{Preset: "unifi", BaseURL: api.URL, HeaderValue: "key123"}
	m, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	ctx := context.Background()
	desired := []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.2", "fake-ip")}
	if e := m.Reconcile(ctx, desired); e != nil {
		t.Fatal(e)
	}
	routesOnRouter := func() (rows []map[string]any) {
		server.mu.Lock()
		defer server.mu.Unlock()
		return append(rows, server.routes...)
	}
	published := routesOnRouter()
	// Identical desired sets and immediate refreshes must reuse the cached
	// snapshot instead of hammering the router every health cycle.
	for i := 0; i < 3; i++ {
		if e := m.Reconcile(ctx, desired); e != nil {
			t.Fatal(e)
		}
		if _, e := m.Refresh(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if now := routesOnRouter(); len(now) != len(published) {
		t.Fatalf("steady state must not re-issue requests, routes=%d want=%d", len(now), len(published))
	}
	ready, e := m.Refresh(ctx)
	if e != nil || !ready {
		t.Fatalf("cached refresh must stay ready: %v %v", ready, e)
	}
	// A changed desired set bypasses the interval immediately.
	changed := []bgp.Advertisement{advertisement("198.18.0.0/15", "192.0.2.7", "fake-ip")}
	if e := m.Reconcile(ctx, changed); e != nil {
		t.Fatal(e)
	}
	nextHop := ""
	for _, row := range routesOnRouter() {
		if row["name"] == RouteName("fake-ip", "198.18.0.0/15") {
			nextHop, _ = row["nextHop"].(string)
		}
	}
	if nextHop != "192.0.2.7" {
		t.Fatalf("changed desired set must republish, nextHop=%s", nextHop)
	}
}

func TestRouteNameAndFacts(t *testing.T) {
	if got := RouteName("fake-ip", "198.18.0.0/15"); got != "routepilot-fakeip-198.18.0.0-15" {
		t.Fatalf("unexpected name: %s", got)
	}
	facts := routeFactsFor("198.18.0.0/15", "192.0.2.2")
	if facts.routeType != "route" || facts.target != "198.18.0.0" || facts.netmask != "255.254.0.0" {
		t.Fatalf("unexpected v4 facts: %+v", facts)
	}
	if facts.values != `"target":"198.18.0.0","netmask":"255.254.0.0","gateway":"192.0.2.2"` {
		t.Fatalf("unexpected values: %s", facts.values)
	}
	v6 := routeFactsFor("fd00::/64", "fe80::1")
	if v6.routeType != "route6" || v6.target != "fd00::/64" || v6.netmask != "" {
		t.Fatalf("unexpected v6 facts: %+v", v6)
	}
}
