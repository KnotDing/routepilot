package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"surge-controller/internal/config"
	"surge-controller/internal/database"
	"surge-controller/internal/softwareupdate"
	"testing"
	"time"
)

type updateStub struct {
	status                 softwareupdate.Status
	checkError, startError error
	checks, starts         int
	onCheck                func()
}

func (u *updateStub) Status() softwareupdate.Status { return u.status }
func (u *updateStub) MarkReady()                    {}
func (u *updateStub) Check(context.Context, bool) (softwareupdate.Status, error) {
	u.checks++
	if u.onCheck != nil {
		u.onCheck()
	}
	return u.status, u.checkError
}
func (u *updateStub) StartForChannel(string, bool) (softwareupdate.Status, error) {
	u.starts++
	if u.startError == nil {
		u.status.Phase = "downloading"
	}
	return u.status, u.startError
}

func updateFixture(t *testing.T) (*API, *updateStub) {
	t.Helper()
	store, err := database.Open(filepath.Join(t.TempDir(), "private", "controller.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	d := config.Default()
	d.Settings.Updates.AutomaticInstall = true
	d.Settings.Updates.UpdateTime = "00:00"
	u := &updateStub{status: softwareupdate.Status{Supported: true, Available: true, Latest: "v9.0.0", Phase: "idle"}}
	return &API{Service: &routeService{doc: d}, Store: store, SoftwareUpdates: u}, u
}

func TestSkippedVersionSurvivesRestartAndPreventsAutomaticInstall(t *testing.T) {
	a, u := updateFixture(t)
	ctx, now := context.Background(), time.Now()
	if err := a.skipUpdate(ctx, "v9.0.0"); err != nil {
		t.Fatal(err)
	}
	restarted := &API{Service: a.Service, Store: a.Store, SoftwareUpdates: u}
	if restarted.updateStatus(ctx).SkippedVersion != "v9.0.0" {
		t.Fatal("skip was not durable")
	}
	if err := restarted.updateSchedulerTick(ctx, now); err != nil || u.starts != 0 {
		t.Fatal("installed a skipped release", err, u.starts)
	}
	if err := restarted.skipUpdate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := restarted.updateSchedulerTick(ctx, now.Add(time.Minute)); err != nil || u.starts != 1 {
		t.Fatal("unskip did not restore automatic installation", err, u.starts)
	}
}

func TestAutomaticInstallDailyReservationSurvivesRollback(t *testing.T) {
	a, u := updateFixture(t)
	ctx, now := context.Background(), time.Now()
	if err := a.updateSchedulerTick(ctx, now); err != nil || u.starts != 1 {
		t.Fatal(err, u.starts)
	}
	// A rolled-back service still sees the release as available, but its durable
	// daily reservation must prevent a second worker after restart.
	u.status.Phase = "failed"
	restarted := &API{Service: a.Service, Store: a.Store, SoftwareUpdates: u}
	if err := restarted.updateSchedulerTick(ctx, now); err != nil || u.starts != 1 {
		t.Fatal("rollback repeated today's installation", err, u.starts)
	}
	if err := restarted.updateSchedulerTick(ctx, now.Add(24*time.Hour)); err != nil || u.starts != 2 {
		t.Fatal("next day's installation did not run", err, u.starts)
	}
}

func TestFailedScheduledCheckRetriesWithoutConsumingDailyInstall(t *testing.T) {
	a, u := updateFixture(t)
	ctx, now := context.Background(), time.Now()
	u.checkError = errors.New("release server offline")
	if err := a.updateSchedulerTick(ctx, now); err == nil {
		t.Fatal("lost check failure")
	}
	status := a.updateStatus(ctx)
	if status.CheckError != u.checkError.Error() || status.NextRetryAt.IsZero() || a.updateState.CompletedDay != "" {
		t.Fatal(status, a.updateState)
	}
	restarted := &API{Service: a.Service, Store: a.Store, SoftwareUpdates: u}
	if err := restarted.updateSchedulerTick(ctx, now.Add(time.Minute)); err != nil || u.checks != 1 {
		t.Fatal("restart bypassed retry backoff", err, u.checks)
	}
	u.checkError = nil
	if err := restarted.updateSchedulerTick(ctx, now.Add(6*time.Minute)); err != nil || u.starts != 1 || restarted.updateStatus(ctx).CheckError != "" {
		t.Fatal("check recovery did not install", err, u.starts)
	}
}

func TestFailedWorkerStartIsVisibleAndRetries(t *testing.T) {
	a, u := updateFixture(t)
	ctx, now := context.Background(), time.Now()
	u.startError = errors.New("worker is busy")
	if err := a.updateSchedulerTick(ctx, now); err == nil {
		t.Fatal("ignored start error")
	}
	status := a.updateStatus(ctx)
	if status.AutomationError != "worker is busy" || a.updateState.AttemptedDay != "" {
		t.Fatal(status, a.updateState)
	}
	u.startError = nil
	if err := a.updateSchedulerTick(ctx, now.Add(time.Minute)); err != nil || u.starts != 1 {
		t.Fatal("no backoff after rejected start", err, u.starts)
	}
	if err := a.updateSchedulerTick(ctx, now.Add(6*time.Minute)); err != nil || u.starts != 2 || a.updateStatus(ctx).AutomationError != "" {
		t.Fatal(err, u.starts)
	}
}

func TestAutomaticInstallRechecksPolicyAndSkipAfterSlowCheck(t *testing.T) {
	for _, change := range []string{"skip", "channel", "disabled"} {
		t.Run(change, func(t *testing.T) {
			a, u := updateFixture(t)
			u.onCheck = func() {
				switch change {
				case "skip":
					if err := a.skipUpdate(context.Background(), "v9.0.0"); err != nil {
						t.Fatal(err)
					}
				case "channel":
					a.Service.(*routeService).doc.Settings.Updates.CheckBeta = true
				case "disabled":
					a.Service.(*routeService).doc.Settings.Updates.AutomaticInstall = false
				}
			}
			if err := a.updateSchedulerTick(context.Background(), time.Now()); err != nil || u.starts != 0 {
				t.Fatal("installed after policy changed", err, u.starts)
			}
		})
	}
}

func TestAutomaticInstallFailsClosedWhenStateCannotBeReadOrWritten(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		a, u := updateFixture(t)
		a.updateStateLoaded = loaded
		_ = a.Store.Close()
		if err := a.updateSchedulerTick(context.Background(), time.Now()); err == nil || u.starts != 0 || a.updateStatus(context.Background()).AutomationError == "" {
			t.Fatal("started an untracked worker", loaded, err, u.starts)
		}
	}
}

func TestSkipAPIValidatesVersionAndPersists(t *testing.T) {
	a, _ := updateFixture(t)
	call := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/updates/skip", strings.NewReader(body)))
		return w
	}
	for _, body := range []string{`{"version":"../anything"}`, `{"version":"v1.2.3-rc.1"}`, `{"version":5}`, `{"version":"v9.0.0","url":"https://evil.test"}`} {
		if w := call(body); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := call(`{"version":"9.0.0"}`)
	var status softwareupdate.Status
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || w.Code != 200 || status.SkippedVersion != "v9.0.0" {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	if w := call(`{"version":""}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestExplicitInstallOverridesSkip(t *testing.T) {
	a, u := updateFixture(t)
	ctx := context.Background()
	if err := a.skipUpdate(ctx, "v9.0.0"); err != nil {
		t.Fatal(err)
	}
	if status, err := a.installUpdate(ctx, "v9.0.0"); err != nil || u.starts != 1 || status.SkippedVersion != "" {
		t.Fatal(status, err, u.starts)
	}
	restarted := &API{Service: a.Service, Store: a.Store, SoftwareUpdates: u}
	if restarted.updateStatus(ctx).SkippedVersion != "" {
		t.Fatal("skip was not cleared durably")
	}
}

func TestCancelledCheckStillPersistsItsFailure(t *testing.T) {
	a, u := updateFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u.checkError = context.Canceled
	if _, err := a.checkUpdates(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	restarted := &API{Service: a.Service, Store: a.Store, SoftwareUpdates: u}
	if status := restarted.updateStatus(context.Background()); status.CheckError != context.Canceled.Error() || status.NextRetryAt.IsZero() {
		t.Fatal("lost the cancelled check's persistent outcome", status)
	}
}
