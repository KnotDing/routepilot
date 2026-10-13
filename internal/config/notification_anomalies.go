package config

import (
	"errors"
	"math"
)

type NotificationAnomalies struct {
	CacheMismatchPercent float64 `json:"cache_mismatch_percent"`
	CacheWindowSeconds   int     `json:"cache_window_seconds"`
	CacheMinimumCompared int     `json:"cache_minimum_compared"`
	BurstQPS             int     `json:"burst_qps"`
	BurstWindowSeconds   int     `json:"burst_window_seconds"`
	SustainedQPS         int     `json:"sustained_qps"`
	SustainedSeconds     int     `json:"sustained_seconds"`
}

// Missing/zero values keep older settings documents compatible.
func (s NotificationAnomalies) Effective() NotificationAnomalies {
	if s.CacheMismatchPercent == 0 {
		s.CacheMismatchPercent = 5
	}
	if s.CacheWindowSeconds == 0 {
		s.CacheWindowSeconds = 60
	}
	if s.CacheMinimumCompared == 0 {
		s.CacheMinimumCompared = 100
	}
	if s.BurstQPS == 0 {
		s.BurstQPS = 500
	}
	if s.BurstWindowSeconds == 0 {
		s.BurstWindowSeconds = 10
	}
	if s.SustainedQPS == 0 {
		s.SustainedQPS = 100
	}
	if s.SustainedSeconds == 0 {
		s.SustainedSeconds = 300
	}
	return s
}

func (s NotificationAnomalies) Validate() error {
	s = s.Effective()
	if math.IsNaN(s.CacheMismatchPercent) || math.IsInf(s.CacheMismatchPercent, 0) || s.CacheMismatchPercent <= 0 || s.CacheMismatchPercent > 100 {
		return errors.New("cache mismatch alert percentage must be above 0 and at most 100")
	}
	if s.CacheWindowSeconds < 1 || s.CacheWindowSeconds > 3600 || s.BurstWindowSeconds < 1 || s.BurstWindowSeconds > 3600 {
		return errors.New("notification sampling windows must be between 1 and 3600 seconds")
	}
	if s.CacheMinimumCompared < 1 || s.CacheMinimumCompared > 1000000 {
		return errors.New("cache mismatch alert minimum samples must be between 1 and 1000000")
	}
	if s.BurstQPS < 1 || s.BurstQPS > 1000000 || s.SustainedQPS < 1 || s.SustainedQPS > 1000000 {
		return errors.New("DNS request alert thresholds must be between 1 and 1000000 requests per second")
	}
	if s.SustainedSeconds < 1 || s.SustainedSeconds > 86400 {
		return errors.New("sustained DNS request alert duration must be between 1 and 86400 seconds")
	}
	return nil
}
