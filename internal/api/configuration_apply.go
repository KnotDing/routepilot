package api

import (
	"context"
	"errors"
	"net/http"
	"surge-controller/internal/config"
	"surge-controller/internal/surge"
)

// Every configuration mutation uses durable save when the backend supports it.
// Runtime activation is independently observed through configuration/status.
func (a *API) saveConfiguration(ctx context.Context, d config.Document) error {
	if service, ok := a.Service.(interface {
		SaveConfiguration(context.Context, config.Document) error
	}); ok {
		return service.SaveConfiguration(ctx, d)
	}
	return a.Service.Apply(ctx, d)
}
func (a *API) configurationStatus(w http.ResponseWriter, r *http.Request) {
	value, err := a.Service.View(r.Context(), "configuration/status")
	if err != nil {
		fail(w, 501, err)
		return
	}
	write(w, 200, value)
}

// Read the Surge profile without occupying the configuration mutation lock;
// merge the imported hosts into the latest document only when committing.
func (a *API) importHosts(ctx context.Context) (any, error) {
	if service, ok := a.Service.(interface {
		FetchHosts(context.Context) (surge.HostImport, error)
		CommitHostsImport(context.Context, surge.HostImport) (any, error)
	}); ok {
		imported, err := service.FetchHosts(ctx)
		if err != nil {
			return nil, err
		}
		if !a.mutation.TryLock() {
			return nil, errors.New("another configuration save is in progress; try again shortly")
		}
		defer a.mutation.Unlock()
		return service.CommitHostsImport(ctx, imported)
	}
	if !a.mutation.TryLock() {
		return nil, errors.New("another configuration save is in progress; try again shortly")
	}
	defer a.mutation.Unlock()
	return a.Service.ImportHosts(ctx)
}
