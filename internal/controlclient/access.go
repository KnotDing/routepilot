package controlclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// CLI access is a local App preference, independent of the remote target and
// the cookie store. --session-dir therefore cannot override the disabled flag.
// Standalone installations without an App-managed flag continue to work.
func CheckAccess() error {
	root, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "RoutePilot", "CLI")
	path := filepath.Join(dir, "integration.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := private(dir, true); err != nil {
		return err
	}
	if err := private(path, false); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("cannot read local CLI access settings")
	}
	if len(data) > 64<<10 {
		return errors.New("invalid local CLI access settings")
	}
	var state struct {
		Enabled *bool `json:"cli_enabled"`
	}
	if json.Unmarshal(data, &state) != nil || state.Enabled == nil {
		return errors.New("invalid local CLI access settings")
	}
	if !*state.Enabled {
		return &Error{Code: "cli_disabled", Message: "CLI is disabled on this Mac; enable it in RoutePilot Settings → CLI and skill"}
	}
	return nil
}
