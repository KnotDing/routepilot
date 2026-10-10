package api

import (
	"context"
	"errors"
	"fmt"
	"surge-controller/internal/softwareupdate"
	"time"
)

type softwareUpdateController interface {
	Status() softwareupdate.Status
	Check(context.Context, bool) (softwareupdate.Status, error)
	StartForChannel(string, bool) (softwareupdate.Status, error)
	MarkReady()
}

// Stored independently of configuration drafts so a settings form cannot
// overwrite a skip or the daily limit. The SQLite record survives updates and
// rollback; record the install attempt before handing control to the worker.
type updateAutomationState struct {
	SkippedVersion string    `json:"skipped_version,omitempty"`
	LastCheck      time.Time `json:"last_check,omitempty"`
	CheckBeta      bool      `json:"check_beta"`
	CheckError     string    `json:"check_error,omitempty"`
	InstallError   string    `json:"install_error,omitempty"`
	NextRetryAt    time.Time `json:"next_retry_at,omitempty"`
	CompletedDay   string    `json:"completed_day,omitempty"`
	AttemptedDay   string    `json:"attempted_day,omitempty"`
	AttemptedTag   string    `json:"attempted_tag,omitempty"`
}

const updateAutomationKey = "software_update_automation"
const updateRetryDelay = 5 * time.Minute

// Caller holds updateMu. A failed read is retried, never treated as empty state.
func (a *API) loadUpdateState(ctx context.Context) error {
	if a.updateStateLoaded {
		return nil
	}
	if a.Store != nil {
		var state updateAutomationState
		if err := a.Store.LoadStatistics(ctx, updateAutomationKey, &state); err != nil {
			a.updateStateError = "Cannot load automatic update state: " + err.Error()
			return errors.New(a.updateStateError)
		}
		a.updateState = state
	}
	a.updateStateLoaded = true
	a.updateStateError = ""
	return nil
}

func (a *API) saveUpdateState(ctx context.Context, state updateAutomationState) error {
	if a.Store != nil {
		if err := a.Store.SaveStatistics(ctx, updateAutomationKey, state); err != nil {
			a.updateStateError = "Cannot save automatic update state: " + err.Error()
			return errors.New(a.updateStateError)
		}
	}
	a.updateState, a.updateStateError = state, ""
	return nil
}

func (a *API) updateStatus(ctx context.Context) softwareupdate.Status {
	status := a.SoftwareUpdates.Status()
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	_ = a.loadUpdateState(ctx)
	status.SkippedVersion = a.updateState.SkippedVersion
	if a.updateState.CheckError != "" {
		status.CheckError = a.updateState.CheckError
	}
	status.AutomationError, status.NextRetryAt = a.updateState.InstallError, a.updateState.NextRetryAt
	if a.updateStateError != "" {
		status.AutomationError = a.updateStateError
	}
	return status
}

func (a *API) checkUpdates(ctx context.Context) (softwareupdate.Status, error) {
	a.updateCheckMu.Lock()
	defer a.updateCheckMu.Unlock()
	policy := a.Service.Configuration().Settings.Updates
	_, checkErr := a.SoftwareUpdates.Check(ctx, policy.CheckBeta)
	// A timeout or a disconnected client must not erase the check outcome.
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	a.updateMu.Lock()
	err := a.loadUpdateState(stateCtx)
	if err == nil {
		state := a.updateState
		if checkErr != nil {
			state.CheckError = checkErr.Error()
			state.NextRetryAt = time.Now().Add(updateRetryDelay)
		} else {
			state.LastCheck, state.CheckBeta = time.Now(), policy.CheckBeta
			state.CheckError, state.NextRetryAt = "", time.Time{}
		}
		err = a.saveUpdateState(stateCtx, state)
	}
	a.updateMu.Unlock()
	return a.updateStatus(ctx), errors.Join(checkErr, err)
}

func (a *API) skipUpdate(ctx context.Context, version string) error {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	if a.Store == nil {
		return errors.New("Persistent update storage is unavailable")
	}
	if err := a.loadUpdateState(ctx); err != nil {
		return err
	}
	phase := a.SoftwareUpdates.Status().Phase
	if phase == "downloading" || phase == "validating" || phase == "restarting" {
		return errors.New("A software update is already in progress")
	}
	state := a.updateState
	state.SkippedVersion = version
	// An explicit unskip can schedule work today, but cannot erase an actual
	// installation attempt or bypass the once-per-day limit.
	if version == "" && state.AttemptedDay != time.Now().Format("2006-01-02") {
		state.CompletedDay = ""
	}
	return a.saveUpdateState(ctx, state)
}

func (a *API) installUpdate(ctx context.Context, version string) (softwareupdate.Status, error) {
	a.updateMu.Lock()
	if err := a.loadUpdateState(ctx); err != nil {
		a.updateMu.Unlock()
		return a.updateStatus(ctx), err
	}
	// Choosing immediate installation explicitly overrides a previous skip.
	state := a.updateState
	if state.SkippedVersion == version {
		state.SkippedVersion = ""
	}
	state.InstallError, state.NextRetryAt = "", time.Time{}
	err := a.saveUpdateState(ctx, state)
	if err == nil {
		_, err = a.SoftwareUpdates.StartForChannel(version, a.Service.Configuration().Settings.Updates.CheckBeta)
	}
	a.updateMu.Unlock()
	return a.updateStatus(ctx), err
}

func (a *API) RunUpdateScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := a.updateSchedulerTick(ctx, now); err != nil && a.Store != nil {
				event, writeErr := a.Store.Event(ctx, "WARNING", "software-update", err.Error(), a.Service.Configuration().Settings.Logging.EventRetention)
				if writeErr == nil && a.Bus != nil {
					a.Bus.Publish(event)
				}
			}
		}
	}
}

func (a *API) updateSchedulerTick(ctx context.Context, now time.Time) error {
	if a.SoftwareUpdates == nil || !a.SoftwareUpdates.Status().Supported {
		return nil
	}
	if a.Store == nil {
		return errors.New("Persistent update storage is unavailable")
	}
	policy := a.Service.Configuration().Settings.Updates
	a.updateMu.Lock()
	if err := a.loadUpdateState(ctx); err != nil {
		a.updateMu.Unlock()
		return err
	}
	state := a.updateState
	lastCheck := state.LastCheck
	if state.CheckBeta != policy.CheckBeta {
		lastCheck = time.Time{}
	}
	plan := planUpdateAutomation(now, policy, lastCheck, state.CompletedDay)
	a.updateMu.Unlock()
	if now.Before(state.NextRetryAt) || !plan.check && !plan.install {
		return nil
	}
	phase := a.SoftwareUpdates.Status().Phase
	if phase == "downloading" || phase == "validating" || phase == "restarting" {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, time.Minute)
	_, err := a.checkUpdates(checkCtx)
	cancel()
	if err != nil || !plan.install {
		return err
	}
	// Checking does network I/O outside the lock. Re-read policy and skip state
	// before installing; another client may have changed either in the meantime.
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	if a.Service.Configuration().Settings.Updates != policy {
		return nil
	}
	state = a.updateState
	day := now.Format("2006-01-02")
	if state.CompletedDay == day || state.AttemptedDay == day {
		return nil
	}
	status := a.SoftwareUpdates.Status()
	next := state
	next.CompletedDay, next.InstallError, next.NextRetryAt = day, "", time.Time{}
	if !status.Available || status.Latest == "" || status.Latest == state.SkippedVersion {
		return a.saveUpdateState(ctx, next)
	}
	next.AttemptedDay, next.AttemptedTag = day, status.Latest
	if err := a.saveUpdateState(ctx, next); err != nil {
		return err // Never launch a worker without its durable daily reservation.
	}
	if _, err := a.SoftwareUpdates.StartForChannel(status.Latest, policy.CheckBeta); err != nil {
		// No worker was accepted, so retry after a delay without consuming the day.
		state.InstallError, state.NextRetryAt = err.Error(), now.Add(updateRetryDelay)
		return errors.Join(fmt.Errorf("Automatic installation could not start: %w", err), a.saveUpdateState(ctx, state))
	}
	return nil
}
