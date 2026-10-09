package database

import (
	"context"
	"encoding/json"
	"surge-controller/internal/config"
)

// Keep an encrypted last-working document independently of the desired config.
// Saving an unapplied configuration must not make management unrecoverable on boot.
func (s *Store) SaveActiveConfiguration(ctx context.Context, d config.Document) error {
	d = d.Clone()
	d.Settings.Security.Password = ""
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	encrypted, err := s.crypt(string(raw), true)
	if err != nil {
		return err
	}
	return s.SaveStatistics(ctx, "active_configuration", encrypted)
}
func (s *Store) LoadActiveConfiguration(ctx context.Context) (config.Document, error) {
	var encrypted string
	if err := s.LoadStatistics(ctx, "active_configuration", &encrypted); err != nil {
		return config.Document{}, err
	}
	if encrypted == "" {
		return config.Document{}, nil
	}
	raw, err := s.crypt(encrypted, false)
	if err != nil {
		return config.Document{}, err
	}
	var d config.Document
	if err = json.Unmarshal([]byte(raw), &d); err != nil {
		return d, err
	}
	return d, d.Validate()
}
