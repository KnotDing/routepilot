package api

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"time"

	"surge-controller/internal/dnsserver"
)

type monitoringService interface {
	DNSMonitoringView(context.Context, string) (dnsserver.DNSMonitoringSnapshot, error)
}

func (a *API) serveDNSMonitoring(w http.ResponseWriter, r *http.Request, path string) {
	source := r.URL.Query().Get("source")
	if source != "" && source != "unknown" && source != "other" && source != "legacy" {
		ip, err := netip.ParseAddr(source)
		if err != nil {
			fail(w, 400, errors.New("invalid monitoring source"))
			return
		}
		source = ip.Unmap().String()
	}
	if path == "dns/monitoring" {
		service, ok := a.Service.(monitoringService)
		if !ok {
			fail(w, 501, errors.New("monitoring statistics are unsupported"))
			return
		}
		view, err := service.DNSMonitoringView(r.Context(), source)
		if err != nil {
			fail(w, 404, err)
			return
		}
		write(w, 200, view)
		return
	}
	if source != "" {
		if service, ok := a.Service.(monitoringService); ok {
			if _, err := service.DNSMonitoringView(r.Context(), source); err != nil {
				fail(w, 404, err)
				return
			}
		}
	}
	if a.Store == nil {
		fail(w, 503, errors.New("monitoring history storage is unavailable"))
		return
	}
	from, err := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		fail(w, 400, errors.New("invalid monitoring history start"))
		return
	}
	to, err := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if err != nil {
		fail(w, 400, errors.New("invalid monitoring history end"))
		return
	}
	if !to.After(from) || to.Sub(from) > 7*24*time.Hour || !from.Equal(from.Truncate(time.Minute)) || !to.Equal(to.Truncate(time.Minute)) {
		fail(w, 400, errors.New("monitoring history range must use whole minutes and span at most 7 days"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	view, err := a.Store.DNSMonitoringHistory(ctx, from.UTC(), to.UTC(), source)
	if err != nil {
		fail(w, 503, err)
		return
	}
	write(w, 200, view)
}
