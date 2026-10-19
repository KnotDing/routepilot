package routingapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"surge-controller/internal/config"
)

// uniFi manages routes through the UniFi OS Integration API
// (/proxy/network/integration/v1/routes) using an API key header. The route
// name carries the RoutePilot prefix so entries created earlier can still be
// recognized after a restart. UniFi gateways without UniFi OS or firmware
// without the Integration API are out of scope for this preset.
type uniFi struct {
	http   *httpClient
	apiKey string
	notify func(severity, message string)
}

func newUniFi(cfg config.RoutingAPI, notify func(string, string)) backend {
	return &uniFi{http: mustHTTP(cfg), apiKey: cfg.HeaderValue, notify: notify}
}

const uniFiRoutesPath = "/proxy/network/integration/v1/routes"

type uniFiRoute struct {
	ID      string `json:"routeId"`
	Name    string `json:"name"`
	Network string `json:"network"`
	NextHop string `json:"nextHop"`
}

func (u *uniFi) headers() map[string]string {
	return map[string]string{"X-API-KEY": u.apiKey}
}

func (u *uniFi) list(ctx context.Context) ([]uniFiRoute, error) {
	if u.http == nil {
		return nil, errors.New("routing API base URL is invalid")
	}
	status, data, err := u.http.do(ctx, "GET", uniFiRoutesPath, u.headers(), nil)
	if err != nil {
		return nil, err
	}
	var rows []uniFiRoute
	if e := decodeJSON(status, data, &rows); e != nil {
		return nil, errors.New("UniFi route listing failed: " + e.Error())
	}
	return rows, nil
}

func (u *uniFi) statesFrom(rows []uniFiRoute, desired []desiredRoute) []RouteState {
	states := []RouteState{}
	for _, d := range desired {
		state := RouteState{Name: d.name, Prefix: d.adv.Prefix, NextHop: d.adv.NextHop}
		for _, row := range rows {
			if row.Name != d.name {
				continue
			}
			state.ID = row.ID
			state.Present = row.Network == d.adv.Prefix && (row.NextHop == d.adv.NextHop || row.NextHop == "")
		}
		states = append(states, state)
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.Name, "routepilot-") {
			continue
		}
		claimed := false
		for _, d := range desired {
			if d.name == row.Name {
				claimed = true
				break
			}
		}
		if !claimed {
			states = append(states, RouteState{Name: row.Name, Prefix: row.Network, NextHop: row.NextHop, ID: row.ID, Managed: true})
		}
	}
	return states
}

func (u *uniFi) peek(ctx context.Context, desired []desiredRoute) ([]RouteState, error) {
	rows, err := u.list(ctx)
	if err != nil {
		return nil, err
	}
	return u.statesFrom(rows, desired), nil
}

func (u *uniFi) sync(ctx context.Context, desired []desiredRoute) ([]RouteState, error) {
	rows, err := u.list(ctx)
	if err != nil {
		return nil, err
	}
	current := map[string]uniFiRoute{}
	for _, row := range rows {
		if strings.HasPrefix(row.Name, "routepilot-") {
			current[row.Name] = row
		}
	}
	for name, row := range current {
		wanted := false
		for _, d := range desired {
			if d.name == name {
				wanted = d.adv.Prefix == row.Network && (row.NextHop == d.adv.NextHop || row.NextHop == "")
				break
			}
		}
		if wanted {
			continue
		}
		if row.ID == "" {
			return nil, errors.New("UniFi route entry is missing an id and cannot be withdrawn")
		}
		status, data, err := u.http.do(ctx, "DELETE", uniFiRoutesPath+"/"+row.ID, u.headers(), nil)
		if err != nil {
			return nil, err
		}
		if e := decodeJSON(status, data, nil); e != nil {
			return nil, errors.New("UniFi route withdrawal failed: " + e.Error())
		}
		if u.notify != nil {
			u.notify("INFO", "Router API route withdrawn: "+row.Network)
		}
	}
	for _, d := range desired {
		if row, exists := current[d.name]; exists && row.Network == d.adv.Prefix && (row.NextHop == d.adv.NextHop || row.NextHop == "") {
			continue
		}
		body, _ := json.Marshal(map[string]any{"name": d.name, "network": d.adv.Prefix, "nextHop": d.adv.NextHop, "enabled": true})
		status, data, err := u.http.do(ctx, "POST", uniFiRoutesPath, u.headers(), body)
		if err != nil {
			return nil, err
		}
		if e := decodeJSON(status, data, nil); e != nil {
			return nil, errors.New("UniFi route publication failed: " + e.Error())
		}
		if u.notify != nil {
			u.notify("INFO", "Router API route published: "+d.adv.Prefix+" via "+d.adv.NextHop)
		}
	}
	return u.peek(ctx, desired)
}

func (u *uniFi) close(ctx context.Context) {}
