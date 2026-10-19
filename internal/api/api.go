package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"surge-controller/internal/appautomation"
	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/events"
	"surge-controller/internal/frr"
	"surge-controller/internal/routeimport"
	"surge-controller/internal/security"
	"surge-controller/internal/softwareupdate"
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
	Service           Service
	Store             *database.Store
	Bus               *events.Bus
	mutation          sync.Mutex
	operationMu       sync.Mutex
	operations        map[string]operation
	RouteImportClient *http.Client
	SoftwareUpdates   softwareUpdateController
	AppAutomation     interface {
		Call(context.Context, string, *appautomation.Change) (appautomation.Result, error)
	}
	updateMu          sync.Mutex
	updateCheckMu     sync.Mutex
	updateState       updateAutomationState
	updateStateLoaded bool
	updateStateError  string
}

// updatePlan is one scheduler tick of the configured automatic update policy.
type updatePlan struct {
	check, install bool
}

// planUpdateAutomation decides one tick of the update policy. The scheduled
// install is attempted at most once a day and only after the local update time;
// a missed slot is picked up at the next tick of the same day.
func planUpdateAutomation(now time.Time, policy config.Updates, lastCheck time.Time, attemptedDay string) updatePlan {
	var plan updatePlan
	if !policy.AutomaticCheck {
		return plan
	}
	if lastCheck.IsZero() || now.Sub(lastCheck) >= time.Duration(policy.CheckIntervalHours)*time.Hour {
		plan.check = true
	}
	if !policy.AutomaticInstall {
		return plan
	}
	hour, minute, ok := config.ParseUpdateTime(policy.UpdateTime)
	if !ok {
		return plan
	}
	if now.Format("2006-01-02") != attemptedDay && now.Hour()*60+now.Minute() >= hour*60+minute {
		plan.install = true
	}
	return plan
}

// RunUpdateScheduler applies the configured automatic update policy to the Mac
// running this backend: periodic release checks and at most one scheduled
// install attempt a day. Manual checks and installs stay available via the
// updates API.

// RunRouteFeedScheduler keeps URL-managed route snapshots current while the
// controller service is running. All changes pass through the same mutation
// lock and configuration transaction as interactive edits.
func (a *API) RunRouteFeedScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshDueRouteFeeds(ctx)
		}
	}
}

func (a *API) refreshDueRouteFeeds(ctx context.Context) {
	d := a.Service.Configuration()
	now := time.Now().UTC()
	for _, feed := range d.RouteFeeds {
		if !feed.Enabled || !feed.LastAttempt.IsZero() && now.Sub(feed.LastAttempt) < time.Duration(feed.IntervalMinutes)*time.Minute {
			continue
		}
		// Fetch outside the save lock. A slow rule source must not block forms.
		snapshot := a.Service.Configuration()
		fetchDoc := snapshot.Clone()
		fetchDoc.Routes = filterFeedRoutes(fetchDoc.Routes, feed.ID)
		ctxFetch, cancel := context.WithTimeout(ctx, 25*time.Second)
		preview, err := routeimport.Fetch(ctxFetch, a.RouteImportClientOrDefault(), feed.URL, fetchDoc)
		cancel()
		a.mutation.Lock()
		current := a.Service.Configuration()
		if !reflect.DeepEqual(snapshot, current) {
			a.mutation.Unlock()
			continue
		}

		idx := -1
		for i := range current.RouteFeeds {
			if current.RouteFeeds[i].ID == feed.ID {
				idx = i
				break
			}
		}
		if idx < 0 || !current.RouteFeeds[idx].Enabled {
			a.mutation.Unlock()
			continue
		}
		feed = current.RouteFeeds[idx]
		base := current.Clone()
		ownedIDs := make(map[string]string)
		for _, route := range base.Routes {
			if route.SourceID == feed.ID {
				ownedIDs[route.Prefix] = route.ID
			}
		}
		base.Routes = filterFeedRoutes(base.Routes, feed.ID)
		candidate := base
		if err == nil {
			if len(preview.Prefixes) == 0 {
				err = errors.New("URL contains no usable route destinations")
			}
			prefixes := make([]string, 0, len(preview.Prefixes))
			for _, prefix := range preview.Prefixes {
				if strings.Contains(prefix, ":") && !feed.SyncIPv6 || !strings.Contains(prefix, ":") && !feed.SyncIPv4 {
					continue
				}
				prefixes = append(prefixes, prefix)
			}
			if err == nil {
				candidate, _, err = routeimport.Append(base, routeimport.Request{Prefixes: prefixes, SourceID: feed.ID, NextHop4: feed.NextHop4, NextHop6: feed.NextHop6, MED: feed.MED, LocalPreference: feed.LocalPreference, Communities: feed.Communities, Binding: feed.Binding, Enabled: feed.RouteEnabled, Description: feed.Description})
				if err == nil {
					for i := range candidate.Routes {
						if id := ownedIDs[candidate.Routes[i].Prefix]; id != "" {
							candidate.Routes[i].ID = id
						}
					}
				}
			}
		}
		updated := current.RouteFeeds[idx]
		updated.LastAttempt = now
		updated.LastError = ""
		if err != nil {
			updated.LastError = err.Error()
			candidate = current
		} else {
			updated.LastUpdated = now
		}
		candidate.RouteFeeds[idx] = updated
		if applyErr := a.saveConfiguration(ctx, candidate); applyErr != nil {
			// Preserve the existing installed route set and expose the failed attempt.
			current.RouteFeeds[idx].LastAttempt = now
			current.RouteFeeds[idx].LastError = "automatic route update could not be applied"
			_ = a.saveConfiguration(ctx, current)
		}
		a.mutation.Unlock()
	}
}

func filterFeedRoutes(routes []config.Route, sourceID string) []config.Route {
	out := make([]config.Route, 0, len(routes))
	for _, route := range routes {
		if route.SourceID != sourceID {
			out = append(out, route)
		}
	}
	return out
}

func (a *API) RouteImportClientOrDefault() *http.Client {
	if a.RouteImportClient != nil {
		return a.RouteImportClient
	}
	return routeimport.NewClient()
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
	if path == "app/automation" {
		a.automation(w, r)
		return
	}
	if path == "operations/status" && r.Method == "GET" {
		a.operationStatus(w, r)
		return
	}
	if path == "configuration/status" && r.Method == "GET" {
		a.configurationStatus(w, r)
		return
	}
	if strings.HasPrefix(path, "updates/") {
		if a.SoftwareUpdates == nil {
			fail(w, 501, errors.New("Backend online updates are not supported"))
			return
		}
		switch {
		case path == "updates/status" && r.Method == "GET":
			write(w, 200, a.updateStatus(r.Context()))
		case path == "updates/check" && r.Method == "GET":
			if r.Header.Get("Prefer") == "respond-async" {
				a.startOperation(w, path, func(ctx context.Context) (any, error) {
					return a.checkUpdates(ctx)
				})
				return
			}
			status, err := a.checkUpdates(r.Context())
			if err != nil {
				fail(w, 502, err)
			} else {
				write(w, 200, status)
			}
		case path == "updates/skip" && r.Method == "POST":
			var body struct {
				Version string `json:"version"`
			}
			if err := decode(w, r, &body); err != nil {
				fail(w, 400, err)
				return
			}
			if body.Version != "" {
				if err := softwareupdate.ValidateVersion(body.Version); err != nil {
					fail(w, 400, err)
					return
				}
				body.Version = "v" + strings.TrimPrefix(body.Version, "v")
			}
			if err := a.skipUpdate(r.Context(), body.Version); err != nil {
				fail(w, 409, err)
			} else {
				write(w, 200, a.updateStatus(r.Context()))
			}
		case path == "updates/restart-app" && r.Method == "POST":
			if !a.Service.Configuration().Settings.Security.Authentication {
				fail(w, 403, errors.New("Enable management authentication before restarting the remote App"))
				return
			}
			restarter, ok := a.SoftwareUpdates.(interface {
				ForceRestartApp(string) (softwareupdate.Status, error)
			})
			if !ok {
				fail(w, 501, errors.New("This backend does not support forcing the remote App to restart"))
				return
			}
			var body struct {
				Version string `json:"version"`
			}
			if err := decode(w, r, &body); err != nil {
				fail(w, 400, err)
				return
			}
			status, err := restarter.ForceRestartApp(body.Version)
			if err != nil {
				fail(w, 409, err)
			} else {
				write(w, 202, status)
			}
		case path == "updates/install" && r.Method == "POST":
			if !a.Service.Configuration().Settings.Security.Authentication {
				fail(w, 403, errors.New("Enable management authentication before online backend updates"))
				return
			}
			var body struct {
				Version string `json:"version"`
			}
			if err := decode(w, r, &body); err != nil {
				fail(w, 400, err)
				return
			}
			status, err := a.installUpdate(r.Context(), body.Version)
			if err != nil {
				fail(w, 409, err)
			} else {
				write(w, 202, status)
			}
		default:
			fail(w, 404, errors.New("unknown update endpoint"))
		}
		return
	}
	if path == "resources/bulk-delete" && r.Method == "POST" {
		var body struct {
			Resource string   `json:"resource"`
			IDs      []string `json:"ids"`
		}
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		if len(body.IDs) == 0 || len(body.IDs) > 512 {
			fail(w, 400, errors.New("select between 1 and 512 rows to delete"))
			return
		}
		if !a.mutation.TryLock() {
			fail(w, 409, errors.New("another configuration save is in progress; try again shortly"))
			return
		}
		defer a.mutation.Unlock()
		d := a.Service.Configuration()
		var deleted int
		var e error
		switch body.Resource {
		case "dns/interception":
			d.Interception, deleted, e = removeIDs(d.Interception, body.IDs, func(v config.Interception) string { return v.ID })
		case "bgp/peers":
			d.Peers, deleted, e = removeIDs(d.Peers, body.IDs, func(v config.Peer) string { return v.ID })
		case "bgp/routes":
			d.Routes, deleted, e = removeIDs(d.Routes, body.IDs, func(v config.Route) string { return v.ID })
		case "dns/hosts":
			d.Hosts, deleted, e = removeIDs(d.Hosts, body.IDs, func(v config.Host) string { return v.ID })
		case "overrides":
			d.Overrides, deleted, e = removeIDs(d.Overrides, body.IDs, func(v config.Override) string { return v.ID })
		default:
			fail(w, 400, errors.New("unsupported bulk delete resource"))
			return
		}
		if e != nil {
			fail(w, 404, e)
			return
		}
		if e = a.saveConfiguration(r.Context(), d); e != nil {
			fail(w, 409, e)
			return
		}
		write(w, 200, map[string]int{"deleted": deleted})
		return
	}
	if path == "bgp/routes/import-url" && r.Method == "POST" {
		var body struct {
			URL string `json:"url"`
		}
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		client := a.RouteImportClient
		if client == nil {
			client = routeimport.NewClient()
		}
		if r.Header.Get("Prefer") == "respond-async" {
			doc := a.Service.Configuration()
			a.startOperation(w, path, func(ctx context.Context) (any, error) { return routeimport.Fetch(ctx, client, body.URL, doc) })
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		v, e := routeimport.Fetch(ctx, client, body.URL, a.Service.Configuration())
		if e != nil {
			fail(w, 422, e)
			return
		}
		write(w, 200, v)
		return
	}
	if path == "bgp/routes/import" && r.Method == "POST" {
		var body routeimport.Request
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		if !a.mutation.TryLock() {
			fail(w, 409, errors.New("another configuration save is in progress; try again shortly"))
			return
		}
		defer a.mutation.Unlock()
		d, result, e := routeimport.Append(a.Service.Configuration(), body)
		if e != nil {
			fail(w, 422, e)
			return
		}
		if result.Imported > 0 {
			if e = a.saveConfiguration(r.Context(), d); e != nil {
				fail(w, 409, e)
				return
			}
		}
		write(w, 200, result)
		return
	}
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
		if r.Header.Get("Prefer") == "respond-async" {
			a.startOperation(w, path, a.importHosts)
			return
		}
		value, e := a.importHosts(r.Context())
		if e != nil {
			fail(w, 409, e)
			return
		}
		write(w, 200, value)
		return
	}
	if path == "dhcp/check" && r.Method == "POST" {
		a.checkDHCP(w, r)
		return
	}
	if path == "routing/verify" && r.Method == "POST" {
		var body json.RawMessage
		if e := decode(w, r, &body); e != nil {
			fail(w, 400, e)
			return
		}
		if r.Header.Get("Prefer") == "respond-async" {
			a.startOperation(w, path, func(ctx context.Context) (any, error) { return a.Service.VerifyRoute(ctx, body) })
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
		if r.Header.Get("Prefer") == "respond-async" {
			a.startOperation(w, path, func(ctx context.Context) (any, error) { return a.Service.Test(ctx, body) })
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
		if r.Header.Get("Prefer") == "respond-async" {
			a.startOperation(w, path, func(ctx context.Context) (any, error) {
				if err := a.Service.Flush(ctx, body.Domain); err != nil {
					return nil, err
				}
				return map[string]bool{"ok": true}, nil
			})
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
		case "dhcp/configuration":
			write(w, 200, d.Settings.DHCP)
		case "dhcp/devices":
			write(w, 200, d.Settings.DHCP.Devices)
		case "configuration/export":
			w.Header().Set("Content-Disposition", "attachment; filename=surge-controller-backup.json")
			sections := r.URL.Query()["sections"]
			if len(sections) == 0 {
				sections = configurationBackupSections
			}
			backup, err := makeConfigurationBackup(d, sections)
			if err != nil {
				fail(w, 400, err)
				return
			}
			write(w, 200, backup)
		case "mode":
			write(w, 200, map[string]string{"mode": d.Settings.General.Mode})
		case "dns/interception":
			write(w, 200, d.Interception)
		case "bgp/peers":
			write(w, 200, d.Peers)
		case "bgp/routes":
			write(w, 200, d.Routes)
		case "bgp/routes/feeds":
			write(w, 200, d.RouteFeeds)
		case "dns/client-settings":
			write(w, 200, d.Settings.DNS.Clients)
		case "overrides":
			write(w, 200, d.Overrides)
		case "dns/hosts":
			write(w, 200, d.Hosts)
		case "dns/monitoring", "dns/monitoring/history":
			a.serveDNSMonitoring(w, r, path)
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
		case "dhcp/status", "status", "health", "dns/queries", "dns/clients", "surge/status", "surge/hosts", "bgp/status", "routing/status":
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
	if !a.mutation.TryLock() {
		fail(w, 409, errors.New("another configuration save is in progress; try again shortly"))
		return
	}
	defer a.mutation.Unlock()
	d := a.Service.Configuration()
	old := d
	created := false
	switch path {
	case "dhcp/devices":
		if r.Method != "PUT" {
			w.WriteHeader(405)
			return
		}
		if e := decode(w, r, &d.Settings.DHCP.Devices); e != nil {
			fail(w, 400, e)
			return
		}
	case "dhcp/configuration":
		if r.Method != "PUT" {
			w.WriteHeader(405)
			return
		}
		if e := decode(w, r, &d.Settings.DHCP); e != nil {
			fail(w, 400, e)
			return
		}
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
		// Client settings have a dedicated resource. Older settings forms must
		// not erase or overwrite that independently edited configuration.
		settings.DNS.Clients = d.Settings.DNS.Clients
		settings.DHCP = d.Settings.DHCP
		d.Settings = settings
		if e := mergeSecrets(&d, &old); e != nil {
			fail(w, 400, e)
			return
		}
	case "configuration/retry":
		if r.Method != "POST" {
			w.WriteHeader(405)
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
		var payload json.RawMessage
		if e := decode(w, r, &payload); e != nil {
			fail(w, 400, e)
			return
		}
		var restore configurationRestore
		if e := json.Unmarshal(payload, &restore); e != nil {
			fail(w, 400, e)
			return
		}
		if restore.Backup.FormatVersion == 0 {
			// Accept the original bare-document backup only at the exact schema
			// version understood by this backend.
			var legacy config.Document
			if e := json.Unmarshal(payload, &legacy); e != nil || legacy.Version != 1 {
				fail(w, 422, errors.New("configuration version is incompatible with this backend"))
				return
			}
			restore.Backup = configurationBackup{FormatVersion: configurationBackupFormatVersion, ConfigVersion: legacy.Version, Sections: append([]string(nil), configurationBackupSections...), Configuration: legacy.Redacted()}
			if len(restore.Sections) == 0 {
				restore.Sections = append([]string(nil), configurationBackupSections...)
			}
		}
		if e := restore.Backup.validate(); e != nil {
			fail(w, 422, e)
			return
		}
		available, _ := selectBackupSections(restore.Backup.Sections)
		if len(restore.Sections) == 0 {
			fail(w, 400, errors.New("select at least one configuration section to restore"))
			return
		}
		for _, section := range restore.Sections {
			if !available[section] {
				fail(w, 400, fmt.Errorf("configuration section %q is not present in this backup", section))
				return
			}
		}
		var err error
		d, err = mergeBackup(d, restore.Backup.Configuration, restore.Sections)
		if err != nil {
			fail(w, 400, err)
			return
		}
		if e := mergeSecrets(&d, &old); e != nil {
			fail(w, 400, e)
			return
		}
	default:
		parts := strings.Split(path, "/")
		key, id := path, ""
		if len(parts) > 1 && path != "bgp/routes/feeds" {
			candidate := strings.Join(parts[:len(parts)-1], "/")
			if candidate == "dns/client-settings" || candidate == "dns/interception" || candidate == "bgp/peers" || candidate == "bgp/routes" || candidate == "bgp/routes/feeds" || candidate == "overrides" || candidate == "dns/hosts" || candidate == "management/backends" {
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
		case "dns/client-settings":
			d.Settings.DNS.Clients, e = mutate(w, r, d.Settings.DNS.Clients, id, func(v *config.Client) {
				v.ID = id
				if ip, err := netip.ParseAddr(v.IP); err == nil {
					v.IP = ip.Unmap().String()
				}
				v.Name = strings.TrimSpace(v.Name)
			})
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
		case "bgp/routes/feeds":
			d.RouteFeeds, e = mutate(w, r, d.RouteFeeds, id, func(v *config.RouteFeed) { v.ID = id })
			if r.Method == "DELETE" && e == nil {
				for i := range d.Routes {
					if d.Routes[i].SourceID == id {
						d.Routes[i].SourceID = ""
					}
				}
			}
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
	if e := a.saveConfiguration(r.Context(), d); e != nil {
		fail(w, 409, e)
		return
	}
	code := 200
	if created {
		code = 201
	}
	write(w, code, a.Service.Configuration().Redacted())
}

func removeIDs[T any](items []T, ids []string, idOf func(T) string) ([]T, int, error) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || selected[id] {
			return items, 0, errors.New("invalid or duplicate row ID")
		}
		selected[id] = true
	}
	keep := make([]T, 0, len(items))
	for _, item := range items {
		if selected[idOf(item)] {
			delete(selected, idOf(item))
			continue
		}
		keep = append(keep, item)
	}
	if len(selected) != 0 {
		return items, 0, errors.New("one or more selected rows no longer exist")
	}
	return keep, len(ids), nil
}
func mergeSecrets(d, old *config.Document) error {
	if d.Settings.Notifications.ClearWebhook {
		d.Settings.Notifications.WebhookURL = ""
	} else if d.Settings.Notifications.WebhookURL == "" {
		d.Settings.Notifications.WebhookURL = old.Settings.Notifications.WebhookURL
	}
	d.Settings.Notifications.ClearWebhook = false
	d.Settings.Notifications.WebhookConfigured = false
	if d.Settings.Security.PasswordHash != "" {
		return errors.New("password hash cannot be supplied through the API")
	}
	if d.Settings.Surge.APIToken == "" {
		d.Settings.Surge.APIToken = old.Settings.Surge.APIToken
	}
	if d.Settings.RoutingAPI.ClearPassword {
		d.Settings.RoutingAPI.Password = ""
	} else if d.Settings.RoutingAPI.Password == "" {
		d.Settings.RoutingAPI.Password = old.Settings.RoutingAPI.Password
	}
	d.Settings.RoutingAPI.ClearPassword = false
	d.Settings.RoutingAPI.PasswordConfigured = false
	if d.Settings.RoutingAPI.ClearHeader {
		d.Settings.RoutingAPI.HeaderValue = ""
	} else if d.Settings.RoutingAPI.HeaderValue == "" {
		d.Settings.RoutingAPI.HeaderValue = old.Settings.RoutingAPI.HeaderValue
	}
	d.Settings.RoutingAPI.ClearHeader = false
	d.Settings.RoutingAPI.HeaderConfigured = false
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
