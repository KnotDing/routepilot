package config

import (
	"errors"
	"net/url"
	"strings"
)

// RoutingAPI describes a third-party router that RoutePilot manages through
// its HTTP API instead of BGP. Presets cover OpenWrt (ubus/uci) and UniFi OS
// (Integration API); the custom preset exposes request templates so operators
// can describe any REST endpoint. Secrets are merged on save and stored
// encrypted, never exported in backups.
type RoutingAPI struct {
	Preset    string `json:"preset"`
	Auth      string `json:"auth"`
	BaseURL   string `json:"base_url"`
	VerifyTLS bool   `json:"verify_tls"`

	Username           string `json:"username,omitempty"`
	Password           string `json:"password,omitempty"`
	PasswordConfigured bool   `json:"password_configured"`
	ClearPassword      bool   `json:"clear_password,omitempty"`

	HeaderName       string `json:"header_name,omitempty"`
	HeaderValue      string `json:"header_value,omitempty"`
	HeaderConfigured bool   `json:"header_configured"`
	ClearHeader      bool   `json:"clear_header,omitempty"`

	Add    RoutingAPIRequest `json:"add"`
	Remove RoutingAPIRequest `json:"remove"`
	List   RoutingAPIRequest `json:"list"`
}

// RoutingAPIRequest is one templated HTTP request. Templates substitute
// {{variables}} before the request is sent; the list request also describes
// how the response is decoded back into routes.
type RoutingAPIRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`

	// Response mapping for the list request. ItemsPath addresses the array
	// (or object keyed by identifier) inside the JSON response, for example
	// "data.routes". Fields name the members of each entry.
	ItemsPath    string `json:"items_path,omitempty"`
	IDField      string `json:"id_field,omitempty"`
	NameField    string `json:"name_field,omitempty"`
	PrefixField  string `json:"prefix_field,omitempty"`
	NextHopField string `json:"next_hop_field,omitempty"`
}

func (r RoutingAPI) Clone() RoutingAPI {
	r.Add.Headers = cloneRoutingAPIHeaders(r.Add.Headers)
	r.Remove.Headers = cloneRoutingAPIHeaders(r.Remove.Headers)
	r.List.Headers = cloneRoutingAPIHeaders(r.List.Headers)
	return r
}

func cloneRoutingAPIHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}

// Effective normalizes the preset-specific defaults. OpenWrt and UniFi
// presets carry fixed implementations; only their connection data is needed.
func (r RoutingAPI) Effective() RoutingAPI {
	switch r.Preset {
	case "openwrt":
		r.Auth = "ubus"
	case "unifi":
		r.Auth = "header"
		if r.HeaderName == "" {
			r.HeaderName = "X-API-KEY"
		}
	}
	return r
}

var routingAPIVariables = map[string]bool{
	"name": true, "section": true, "prefix": true, "target": true, "netmask": true,
	"next_hop": true, "route_type": true, "values": true, "id": true, "session": true, "api_key": true,
}

func checkRoutingAPITemplate(field, value string) error {
	if value == "" {
		return nil
	}
	if len(value) > 8192 {
		return errors.New("routing API " + field + " template exceeds 8 KiB")
	}
	for i := 0; i+len("{{") <= len(value); i++ {
		if value[i] != '{' || value[i+1] != '{' {
			continue
		}
		end := strings.Index(value[i:], "}}")
		if end < 0 {
			return errors.New("routing API " + field + " template has an unterminated {{variable}}")
		}
		name := strings.TrimSpace(value[i+2 : i+end])
		if !routingAPIVariables[name] {
			return errors.New("routing API " + field + " template uses unknown variable {{" + name + "}}")
		}
		i += end
	}
	return nil
}

func (q RoutingAPIRequest) Validate(role string) error {
	if q.Method == "" && q.Path == "" && q.Body == "" && len(q.Headers) == 0 {
		return errors.New("routing API " + role + " request template is required")
	}
	switch q.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return errors.New("routing API " + role + " request method must be GET, POST, PUT, PATCH or DELETE")
	}
	if !strings.HasPrefix(q.Path, "/") || len(q.Path) > 2048 {
		return errors.New("routing API " + role + " request path must start with /")
	}
	if e := checkRoutingAPITemplate(role+" path", q.Path); e != nil {
		return e
	}
	if e := checkRoutingAPITemplate(role+" body", q.Body); e != nil {
		return e
	}
	if len(q.Headers) > 16 {
		return errors.New("routing API " + role + " request has too many headers")
	}
	for name, value := range q.Headers {
		if name == "" || len(name) > 128 || len(value) > 4096 {
			return errors.New("routing API " + role + " request header exceeds limits")
		}
		if e := checkRoutingAPITemplate(role+" header "+name, value); e != nil {
			return e
		}
	}
	if role == "list" {
		for _, f := range []struct{ key, name string }{{q.ItemsPath, "items path"}, {q.IDField, "id field"}, {q.NameField, "name field"}} {
			if len(f.key) > 256 {
				return errors.New("routing API list " + f.name + " exceeds limits")
			}
		}
	}
	return nil
}

func (r RoutingAPI) Validate() error {
	r = r.Effective()
	switch r.Preset {
	case "openwrt", "unifi", "custom":
	default:
		return errors.New("routing API preset must be openwrt, unifi or custom")
	}
	u, e := url.Parse(r.BaseURL)
	if e != nil || r.BaseURL == "" || len(r.BaseURL) > 512 || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("routing API base URL must be an HTTP(S) URL without user info, query or fragment")
	}
	switch r.Auth {
	case "ubus":
		if r.Username == "" {
			return errors.New("routing API ubus authentication requires a username")
		}
		if r.Password == "" && !r.PasswordConfigured {
			return errors.New("routing API ubus authentication requires a password")
		}
	case "header":
		if r.HeaderName == "" {
			return errors.New("routing API header authentication requires a header name")
		}
		if r.HeaderValue == "" && !r.HeaderConfigured {
			return errors.New("routing API header authentication requires a value")
		}
	case "none":
	default:
		return errors.New("routing API authentication must be ubus, header or none")
	}
	if r.Preset == "custom" {
		if e := r.Add.Validate("add"); e != nil {
			return e
		}
		if e := r.Remove.Validate("remove"); e != nil {
			return e
		}
		if e := r.List.Validate("list"); e != nil {
			return e
		}
	}
	return nil
}
