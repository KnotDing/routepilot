package config

import (
	"math"
	"testing"
)

func TestNotificationAnomalyDefaultsAndValidation(t *testing.T) {
	defaults := (NotificationAnomalies{}).Effective()
	if defaults.CacheMismatchPercent != 5 || defaults.CacheMinimumCompared != 100 || defaults.CacheWindowSeconds != 60 || defaults.BurstQPS != 500 || defaults.BurstWindowSeconds != 10 || defaults.SustainedQPS != 100 || defaults.SustainedSeconds != 300 {
		t.Fatal(defaults)
	}
	if err := (NotificationAnomalies{}).Validate(); err != nil {
		t.Fatal("older settings incompatible", err)
	}
	for _, change := range []func(*NotificationAnomalies){func(s *NotificationAnomalies) { s.CacheMismatchPercent = -1 }, func(s *NotificationAnomalies) { s.CacheMismatchPercent = 101 }, func(s *NotificationAnomalies) { s.CacheMismatchPercent = math.NaN() }, func(s *NotificationAnomalies) { s.CacheWindowSeconds = 3601 }, func(s *NotificationAnomalies) { s.CacheMinimumCompared = -1 }, func(s *NotificationAnomalies) { s.BurstQPS = -1 }, func(s *NotificationAnomalies) { s.BurstWindowSeconds = -1 }, func(s *NotificationAnomalies) { s.SustainedQPS = 1000001 }, func(s *NotificationAnomalies) { s.SustainedSeconds = 86401 }} {
		value := defaults
		change(&value)
		if err := value.Validate(); err == nil {
			t.Fatal("invalid thresholds accepted", value)
		}
	}
	s := Default().Settings.Notifications
	s.Errors = []string{"cache_mismatch", "dns_request_burst", "dns_request_sustained"}
	for _, name := range []string{"DNS cache mismatch decision", "DNS cache mismatch response", "DNS request burst", "DNS sustained requests"} {
		if !s.Selected(name) {
			t.Fatal("new event not selectable", name)
		}
	}
}
