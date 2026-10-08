package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/events"
	"surge-controller/internal/frr"
	"surge-controller/internal/security"
	"sync"
	"time"
)

type Service interface {
	Configuration() config.Document
	Apply(context.Context, config.Document) error
	View(context.Context, string) (any, error)
	CacheView(context.Context, int, int, string, string) (any, error)
	Test(context.Context, json.RawMessage) (any, error)
	Flush(context.Context, string) error
	Metrics() string
	ImportHosts(context.Context) (any, error)
	VerifyRoute(context.Context, json.RawMessage) (any, error)
}
type API struct {
	Service  Service
	Store    *database.Store
	Bus      *events.Bus
	mutation sync.Mutex
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, e error) {
	write(w, status, map[string]string{"error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, dest any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024))
	dec.DisallowUnknownFields()
	if e := dec.Decode(dest); e != nil {
		return errors.New("invalid request document")
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return errors.New("one JSON document required")
	}
	return nil
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(w, a.Service.Metrics())
	})
	mux.HandleFunc("/api/v1/", a.serve)
	return mux
}
func (a *API) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if path == "bgp/export-frr" && r.Method == "POST" {
		var input frr.Request
		if e := decode(w, r, &input); e != nil {
			fail(w, 400, e)
			return
		}
		value, e := frr.Generate(a.Service.Configuration(), input)
		if e != nil {
			fail(w, 422, e)
			return
		}
		write(w, 200, value)
		return
	}
	if path == "events/stream" && r.Method == "GET" {
		a.stream(w, r)
		return
	}
	if path == "events" && r.Method == "GET" {
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		v, e := a.Store.Events(r.Context(), after)
		if e != nil {
			fail(w, 503, errors.New("event database unavailable"))
			return
		}
		write(w, 200, v)
		return
	}
	if path == "dns/hosts/import-surge" && r.Method == "POST" {
		a.mutation.Lock()
		defer a.mutation.Unlock()
		value, e := a.Service.ImportHosts(r.Context())
		if e != nil {
			fail(w, 409, e)
			return
		}
		write(w, 200, value)
		return
	}
	if path == "routing/verify" && r.Method == "POST" {
		var body json.RawMessage
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		v, e := a.Service.VerifyRoute(r.Context(), body)
		if e != nil {
			fail(w, 400, e)
			return
		}
		write(w, 200, v)
		return
	}
	if path == "test/domain" && r.Method == "POST" {
		var body json.RawMessage
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		v, e := a.Service.Test(r.Context(), body)
		if e != nil {
			fail(w, 400, e)
			return
		}
		write(w, 200, v)
		return
	}
	if path == "dns/cache/flush" && r.Method == "POST" {
		var body struct {
			Domain string `json:"domain"`
		}
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		if e := a.Service.Flush(r.Context(), body.Domain); e != nil {
			fail(w, 400, e)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "GET" {
		d := a.Service.Configuration().Redacted()
		switch path {
		case "management/backends":
			write(w, 200, d.Settings.RemoteBackends)
		case "settings":
			write(w, 200, d.Settings)
		case "configuration/export":
			w.Header().Set("Content-Disposition", "attachment; filename=surge-controller-backup.json")
			write(w, 200, d)
		case "mode":
			write(w, 200, map[string]string{"mode": d.Settings.General.Mode})
		case "dns/interception":
			write(w, 200, d.Interception)
		case "bgp/peers":
			write(w, 200, d.Peers)
		case "bgp/routes":
			write(w, 200, d.Routes)
		case "overrides":
			write(w, 200, d.Overrides)
		case "dns/hosts":
			write(w, 200, d.Hosts)
		case "dns/cache":
			offset, limit := 0, 100
			query := r.URL.Query()
			var err error
			if query.Has("offset") {
				offset, err = strconv.Atoi(query.Get("offset"))
				if err != nil || offset < 0 {
					fail(w, 400, errors.New("invalid cache offset"))
					return
				}
			}
			if query.Has("limit") {
				limit, err = strconv.Atoi(query.Get("limit"))
				if err != nil || limit < 0 || limit > 1000 {
					fail(w, 400, errors.New("cache limit must be 0-1000"))
					return
				}
			}
			decision := query.Get("decision")
			if decision != "" && decision != "DIRECT" && decision != "SURGE" && decision != "UNKNOWN" && decision != "REJECT" {
				fail(w, 400, errors.New("invalid cache decision"))
				return
			}
			v, e := a.Service.CacheView(r.Context(), offset, limit, query.Get("domain"), decision)
			if e != nil {
				fail(w, 503, e)
			} else {
				write(w, 200, v)
			}
		case "status", "health", "dns/queries", "surge/status", "surge/hosts", "bgp/status", "routing/status":
			v, e := a.Service.View(r.Context(), path)
			if e != nil {
				fail(w, 503, e)
			} else {
				write(w, 200, v)
			}
		default:
			fail(w, 404, errors.New("unknown API endpoint"))
		}
		return
	}
	if r.Method != "POST" && r.Method != "PUT" && r.Method != "DELETE" {
		w.WriteHeader(405)
		return
	}
	a.mutation.Lock()
	defer a.mutation.Unlock()
	d := a.Service.Configuration()
	old := d
	created := false
	switch path {
	case "settings":
		if r.Method != "PUT" {
			w.WriteHeader(405)
			return
		}
		var settings config.Settings
		if e := decode(w, r, &settings); e != nil {
			fail(w, 400, e)
			return
		}
		d.Settings = settings
		if e := mergeSecrets(&d, &old); e != nil {
			fail(w, 400, e)
			return
		}
	case "mode":
		if r.Method != "PUT" {
			w.WriteHeader(405)
			return
		}
		var body struct {
			Mode string `json:"mode"`
		}
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		d.Settings.General.Mode = body.Mode
	case "configuration/import":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var imported config.Document
		if e := decode(w, r, &imported); e != nil {
			fail(w, 400, e)
			return
		}
		d = imported
		if e := mergeSecrets(&d, &old); e != nil {
			fail(w, 400, e)
			return
		}
	default:
		parts := strings.Split(path, "/")
		key, id := path, ""
		if len(parts) > 1 {
			candidate := strings.Join(parts[:len(parts)-1], "/")
			if candidate == "dns/interception" || candidate == "bgp/peers" || candidate == "bgp/routes" || candidate == "overrides" || candidate == "dns/hosts" || candidate == "management/backends" {
				key, id = candidate, parts[len(parts)-1]
			}
		}
		if r.Method == "POST" && id != "" || r.Method != "POST" && id == "" {
			fail(w, 400, errors.New("invalid resource method/path"))
			return
		}
		if r.Method == "POST" {
			id = security.Token()[:22]
			created = true
		}
		var e error
		switch key {
		case "management/backends":
			d.Settings.RemoteBackends, e = mutate(w, r, d.Settings.RemoteBackends, id, func(v *config.Backend) { v.ID = id })
		case "dns/interception":
			d.Interception, e = mutate(w, r, d.Interception, id, func(v *config.Interception) { v.ID = id })
		case "bgp/peers":
			d.Peers, e = mutate(w, r, d.Peers, id, func(v *config.Peer) { v.ID = id })
		case "bgp/routes":
			d.Routes, e = mutate(w, r, d.Routes, id, func(v *config.Route) {
				v.ID = id
				if v.Binding == "" {
					v.Binding = "none"
				}
			})
		case "dns/hosts":
			d.Hosts, e = mutate(w, r, d.Hosts, id, func(v *config.Host) {
				v.ID = id
				if v.Source == "" {
					v.Source = "manual"
				}
			})
		case "overrides":
			d.Overrides, e = mutate(w, r, d.Overrides, id, func(v *config.Override) { v.ID = id })
		default:
			fail(w, 404, errors.New("unknown API endpoint"))
			return
		}
		if e != nil {
			fail(w, 400, e)
			return
		}
	}
	if e := d.Validate(); e != nil {
		severity := "WARNING"
		if strings.Contains(e.Error(), "hard BGP route limit") {
			severity = "CRITICAL"
		}
		if event, err := a.Store.Event(r.Context(), severity, "configuration_rejected", e.Error(), a.Service.Configuration().Settings.Logging.EventRetention); err == nil {
			a.Bus.Publish(event)
		}
		fail(w, 422, e)
		return
	}
	if e := a.Service.Apply(r.Context(), d); e != nil {
		fail(w, 409, e)
		return
	}
	code := 200
	if created {
		code = 201
	}
	write(w, code, a.Service.Configuration().Redacted())
}
func mergeSecrets(d, old *config.Document) error {
	if d.Settings.Security.PasswordHash != "" {
		return errors.New("password hash cannot be supplied through the API")
	}
	if d.Settings.Surge.APIToken == "" {
		d.Settings.Surge.APIToken = old.Settings.Surge.APIToken
	}
	d.Settings.Security.PasswordHash = old.Settings.Security.PasswordHash
	if p := d.Settings.Security.Password; p != "" {
		if len(p) < 12 || len(p) > 72 {
			return errors.New("password must contain 12–72 bytes")
		}
		hash, e := security.Hash(p)
		if e != nil {
			return errors.New("password hashing failed")
		}
		d.Settings.Security.PasswordHash = hash
	}
	d.Settings.Security.Password = ""
	d.Settings.Surge.TokenConfigured = false
	d.Settings.Security.PasswordConfigured = false
	return nil
}
func mutate[T any](w http.ResponseWriter, r *http.Request, items []T, id string, set func(*T)) ([]T, error) {
	index := -1
	for i, item := range items {
		b, _ := json.Marshal(item)
		var entry struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(b, &entry)
		if entry.ID == id {
			index = i
			break
		}
	}
	if r.Method != "POST" && index < 0 {
		return items, errors.New("resource not found")
	}
	if r.Method == "DELETE" {
		out := append([]T{}, items[:index]...)
		return append(out, items[index+1:]...), nil
	}
	var v T
	if e := decode(w, r, &v); e != nil {
		return items, e
	}
	set(&v)
	out := append([]T{}, items...)
	if index >= 0 {
		out[index] = v
	} else {
		out = append(out, v)
	}
	return out, nil
}
func (a *API) stream(w http.ResponseWriter, r *http.Request) {
	ch, ok := a.Bus.Subscribe()
	if !ok {
		http.Error(w, "event stream capacity exhausted", 503)
		return
	}
	defer a.Bus.Unsubscribe(ch)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(w)
	send := func(s string) bool {
		_ = control.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, e := io.WriteString(w, s)
		if e != nil {
			return false
		}
		return control.Flush() == nil
	}
	if !send(": connected\n\n") {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			if !send(fmt.Sprintf("id: %d\nevent: update\ndata: %s\n\n", ev.ID, b)) {
				return
			}
		case <-ticker.C:
			if !send(": heartbeat\n\n") {
				return
			}
		}
	}
}
