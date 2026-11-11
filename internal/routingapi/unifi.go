package routingapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"surge-controller/internal/config"
)

// uniFi manages routes through the UniFi OS Network routing API. Site and
// gateway identifiers are read from the existing route list response.
type uniFi struct {
	http   *httpClient
	apiKey string
	notify func(severity, message string)
}

func newUniFi(cfg config.RoutingAPI, notify func(string, string)) backend {
	return &uniFi{http: mustHTTP(cfg), apiKey: cfg.HeaderValue, notify: notify}
}

const uniFiRoutesPath = "/proxy/network/api/s/default/rest/routing"

type uniFiRoute struct {
	ID            string `json:"_id"`
	SiteID        string `json:"site_id"`
	GatewayDevice string `json:"gateway_device"`
	Name          string `json:"name"`
	Network       string `json:"static-route_network"`
	NextHop       string `json:"static-route_nexthop"`
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
	var response struct {
		Data []uniFiRoute `json:"data"`
	}
	if e := decodeJSON(status, data, &response); e != nil {
		return nil, errors.New("UniFi route listing failed: " + e.Error())
	}
	if response.Data == nil {
		return nil, errors.New("UniFi route listing failed: response has no data array")
	}
	siteID, gatewayDevice := "", ""
	for _, row := range response.Data {
		if row.SiteID != "" {
			if siteID != "" && siteID != row.SiteID {
				return nil, errors.New("UniFi route listing contains conflicting site_id values")
			}
			siteID = row.SiteID
		}
		if row.GatewayDevice != "" {
			if gatewayDevice != "" && gatewayDevice != row.GatewayDevice {
				return nil, errors.New("UniFi route listing contains conflicting gateway_device values")
			}
			gatewayDevice = row.GatewayDevice
		}
	}
	return response.Data, nil
}

func uniFiRouteMetadata(rows []uniFiRoute) (siteID, gatewayDevice string) {
	for _, row := range rows {
		if siteID == "" && row.SiteID != "" {
			siteID = row.SiteID
		}
		if gatewayDevice == "" && row.GatewayDevice != "" {
			gatewayDevice = row.GatewayDevice
		}
	}
	return siteID, gatewayDevice
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
			state.Present = row.Network == d.adv.Prefix && row.NextHop == d.adv.NextHop
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
	siteID, gatewayDevice := uniFiRouteMetadata(rows)
	for name, row := range current {
		wanted := false
		for _, d := range desired {
			if d.name == name {
				wanted = d.adv.Prefix == row.Network && d.adv.NextHop == row.NextHop
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
		if row, exists := current[d.name]; exists && row.Network == d.adv.Prefix && row.NextHop == d.adv.NextHop {
			continue
		}
		payload := map[string]any{
			"static-route_nexthop":  d.adv.NextHop,
			"static-route_network":  d.adv.Prefix,
			"name":                  d.name,
			"gateway_type":          "default",
			"static-route_type":     "nexthop-route",
			"static-route_distance": 1,
			"type":                  "static-route",
			"enabled":               true,
		}
		if siteID != "" {
			payload["site_id"] = siteID
		}
		if gatewayDevice != "" {
			payload["gateway_device"] = gatewayDevice
		}
		body, _ := json.Marshal(payload)
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
