package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
)

type operation struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	StartedAt time.Time `json:"started_at"`
	Result    any       `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Long diagnostics/import reads acknowledge a task instead of holding a relay
// request open. Their actual result is never reported before the work succeeds.
func (a *API) startOperation(w http.ResponseWriter, kind string, work func(context.Context) (any, error)) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		fail(w, 500, err)
		return
	}
	job := operation{ID: hex.EncodeToString(token[:]), Kind: kind, State: "running", StartedAt: time.Now().UTC()}
	a.operationMu.Lock()
	if a.operations == nil {
		a.operations = make(map[string]operation)
	}
	active := 0
	for _, value := range a.operations {
		if value.State == "running" {
			active++
		}
	}
	if active >= 4 {
		a.operationMu.Unlock()
		fail(w, 429, errors.New("background operation capacity exhausted; try again later"))
		return
	}
	if len(a.operations) >= 32 {
		oldest := ""
		for id, value := range a.operations {
			if value.State != "running" && (oldest == "" || value.StartedAt.Before(a.operations[oldest].StartedAt)) {
				oldest = id
			}
		}
		delete(a.operations, oldest)
	}
	a.operations[job.ID] = job
	a.operationMu.Unlock()
	write(w, 202, map[string]any{"operation": job})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := work(ctx)
		job.State = "succeeded"
		if err != nil {
			job.State = "failed"
			job.Error = err.Error()
		} else {
			job.Result = result
		}
		a.operationMu.Lock()
		a.operations[job.ID] = job
		a.operationMu.Unlock()
	}()
}

func (a *API) operationStatus(w http.ResponseWriter, r *http.Request) {
	a.operationMu.Lock()
	job, ok := a.operations[r.URL.Query().Get("id")]
	a.operationMu.Unlock()
	if !ok {
		fail(w, 404, errors.New("background operation not found; check whether it completed before submitting again"))
		return
	}
	write(w, 200, job)
}
