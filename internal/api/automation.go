package api

import (
	"errors"
	"net/http"
	"surge-controller/internal/appautomation"
)

func (a *API) automation(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "PUT" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if a.AppAutomation == nil {
		write(w, http.StatusNotImplemented, appautomation.Result{Error: "CLI and skill management is not supported on this backend"})
		return
	}
	var change *appautomation.Change
	if r.Method == "PUT" {
		change = &appautomation.Change{}
		if err := decode(w, r, change); err != nil {
			fail(w, 400, err)
			return
		}
		if err := change.Validate(); err != nil {
			fail(w, 422, err)
			return
		}
	}
	result, err := a.AppAutomation.Call(r.Context(), a.Service.Configuration().Settings.Surge.UserName, change)
	if err != nil {
		fail(w, 503, err)
		return
	}
	status := http.StatusOK
	if result.Error != "" {
		switch {
		case result.Code == "automation_conflict" || result.Code == "automation_busy":
			status = http.StatusConflict
		case !result.Supported && change != nil:
			status = http.StatusNotImplemented
		case result.Supported:
			status = http.StatusInternalServerError
		}
	}
	if change != nil && !result.Supported && result.Error == "" {
		fail(w, 501, errors.New("CLI and skill management is not supported on this backend"))
		return
	}
	write(w, status, result)
}
