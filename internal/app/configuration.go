package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	"surge-controller/internal/config"
)

type ApplicationStatus struct {
	Revision        uint64    `json:"revision"`
	AppliedRevision uint64    `json:"applied_revision"`
	State           string    `json:"state"`
	Error           string    `json:"error,omitempty"`
	SavedAt         time.Time `json:"saved_at"`
	FinishedAt      time.Time `json:"finished_at"`
}

func (a *App) activeConfiguration() config.Document {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.applied.Clone()
}

// SaveConfiguration acknowledges the durable desired configuration. Applying
// listeners/routes is owned by a single worker and never holds this save lock.
func (a *App) SaveConfiguration(ctx context.Context, d config.Document) error {
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	if a.closing.Load() {
		return errors.New("Controller is stopping")
	}
	d = d.Clone()
	old := a.Configuration()
	d.Settings.Routing = d.Settings.Routing.Effective()
	previousRouting := old.Settings.Routing.Effective()
	if !reflect.DeepEqual(old.Settings.Surge.FakePrefixes, d.Settings.Surge.FakePrefixes) || old.Settings.Surge.NextHop4 != d.Settings.Surge.NextHop4 || old.Settings.Surge.NextHop6 != d.Settings.Surge.NextHop6 || previousRouting.Mode != d.Settings.Routing.Mode || previousRouting.FakeIPBackend != d.Settings.Routing.FakeIPBackend {
		d.Settings.Routing.StaticFakeIPConfigured = false
		d.Settings.Routing.StaticFakeIPNextHop4 = ""
	}
	if !d.Settings.Routing.StaticFakeIPConfigured || d.Settings.Surge.NextHop4 != "" {
		d.Settings.Routing.StaticFakeIPNextHop4 = ""
	} else if d.Settings.Routing.FakeIPBackend == "static" && d.Settings.Routing.StaticFakeIPNextHop4 == "" && hasIPv4FakePrefix(d) {
		// The empty metadata field requests an explicit new confirmation. Other
		// saves preserve its address, so an IP change cannot silently re-confirm.
		a.mu.RLock()
		routes := a.runtime.routes
		a.mu.RUnlock()
		next := routes.NextHop4()
		if !next.Automatic || next.Address == "" || next.Address != next.DetectedAddress || next.Error != "" {
			return errors.New("请等待 Surge IPv4 下一跳自动探测成功后，再确认静态 Fake-IP 路由")
		}
		d.Settings.Routing.StaticFakeIPNextHop4 = next.Address
	}
	if !reflect.DeepEqual(old.Interception, d.Interception) || old.Settings.DNS.Source4 != d.Settings.DNS.Source4 || old.Settings.DNS.Source6 != d.Settings.DNS.Source6 || previousRouting.Mode != d.Settings.Routing.Mode || previousRouting.DNSBackend != d.Settings.Routing.DNSBackend || previousRouting.StaticDNSInterception != d.Settings.Routing.StaticDNSInterception {
		d.Settings.Routing.StaticDNSConfigured = false
	}
	if err := d.Validate(); err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := a.store.Save(writeCtx, d); err != nil {
		return errors.New("configuration transaction failed")
	}
	a.mu.Lock()
	a.config = d
	a.application.Revision++
	a.application.State = "pending"
	a.application.Error = ""
	a.application.SavedAt = time.Now().UTC()
	a.application.FinishedAt = time.Time{}
	a.mu.Unlock()
	select {
	case a.applyWake <- struct{}{}:
	default:
	}
	return nil
}

func (a *App) runConfigurationWorker(ctx context.Context) {
	defer close(a.applyDone)
	var processed uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.applyWake:
		}
		for {
			if ctx.Err() != nil {
				return
			}
			a.mu.Lock()
			d, revision := a.config.Clone(), a.application.Revision
			if revision == processed {
				a.mu.Unlock()
				break
			}
			old := a.applied.Settings
			a.application.State = "applying"
			a.mu.Unlock()
			err := a.activateConfiguration(ctx, d)
			if err == nil {
				checkpointCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				checkpointErr := a.store.SaveActiveConfiguration(checkpointCtx, a.activeConfiguration())
				cancel()
				if checkpointErr != nil {
					a.event("ERROR", "configuration_checkpoint_failed", "Active configuration checkpoint could not be saved")
				}
			}
			processed = revision
			a.mu.Lock()
			if err == nil {
				a.application.AppliedRevision = revision
			}
			latest := a.application.Revision == revision
			if latest {
				a.application.State = "succeeded"
				a.application.Error = ""
				a.application.FinishedAt = time.Now().UTC()
				if err != nil {
					a.application.State = "failed"
					a.application.Error = err.Error()
				}
			}
			a.mu.Unlock()
			if err == nil && (old.General.WebListen != d.Settings.General.WebListen || old.Security.TLSCert != d.Settings.Security.TLSCert || old.Security.TLSKey != d.Settings.Security.TLSKey) {
				select {
				case a.webChanged <- struct{}{}:
				default:
				}
			}
			if err != nil {
				a.event("ERROR", "configuration_apply_failed", "Configuration saved, but runtime application failed: "+err.Error())
			}
			if latest {
				break
			}
		}
	}
}

// Internal startup callers may still wait for activation. HTTP mutations use
// SaveConfiguration directly so success means the transaction is committed.
func (a *App) Apply(ctx context.Context, d config.Document) error {
	if err := a.SaveConfiguration(ctx, d); err != nil {
		return err
	}
	a.mu.RLock()
	revision := a.application.Revision
	a.mu.RUnlock()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		a.mu.RLock()
		s := a.application
		a.mu.RUnlock()
		if s.AppliedRevision >= revision {
			return nil
		}
		if s.Revision >= revision && s.State == "failed" {
			return errors.New(s.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
