package api

import (
	"context"
	"net/http"
	"surge-controller/internal/config"
	"surge-controller/internal/dhcpserver"
)

func (a *API) checkDHCP(w http.ResponseWriter, r *http.Request) {
	var d config.DHCP
	if err := decode(w, r, &d); err != nil {
		fail(w, 400, err)
		return
	}
	if err := d.Validate(); err != nil {
		fail(w, 422, err)
		return
	}
	service, ok := a.Service.(interface {
		CheckDHCP(context.Context, config.DHCP) (dhcpserver.CheckResult, error)
	})
	if !ok {
		http.Error(w, "DHCP checks unavailable", http.StatusNotImplemented)
		return
	}
	if r.Header.Get("Prefer") == "respond-async" {
		a.startOperation(w, "dhcp/check", func(ctx context.Context) (any, error) { return service.CheckDHCP(ctx, d) })
		return
	}
	v, err := service.CheckDHCP(r.Context(), d)
	if err != nil {
		fail(w, 503, err)
		return
	}
	write(w, 200, v)
}
