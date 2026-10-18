package api

import (
	"errors"
	"fmt"

	"surge-controller/internal/config"
)

const configurationBackupFormatVersion = 1

var configurationBackupSections = []string{
	"general", "dhcp", "dns", "surge", "routing", "bgp", "cache", "health", "logging", "notifications", "security", "updates",
	"peers", "routes", "route_feeds", "interception", "overrides", "hosts",
}

type configurationBackup struct {
	FormatVersion int             `json:"format_version"`
	ConfigVersion int             `json:"config_version"`
	Sections      []string        `json:"sections"`
	Configuration config.Document `json:"configuration"`
}

type configurationRestore struct {
	Backup   configurationBackup `json:"backup"`
	Sections []string            `json:"sections"`
}

func selectBackupSections(sections []string) (map[string]bool, error) {
	if len(sections) == 0 {
		return nil, errors.New("select at least one configuration section")
	}
	allowed := make(map[string]bool, len(configurationBackupSections))
	for _, section := range configurationBackupSections {
		allowed[section] = true
	}
	selected := make(map[string]bool, len(sections))
	for _, section := range sections {
		if !allowed[section] {
			return nil, fmt.Errorf("unknown configuration section %q", section)
		}
		if selected[section] {
			return nil, fmt.Errorf("duplicate configuration section %q", section)
		}
		selected[section] = true
	}
	return selected, nil
}

func makeConfigurationBackup(document config.Document, sections []string) (configurationBackup, error) {
	selected, err := selectBackupSections(sections)
	if err != nil {
		return configurationBackup{}, err
	}
	filtered := config.Document{Version: document.Version}
	if selected["dhcp"] {
		filtered.Settings.DHCP = document.Settings.DHCP.Clone()
	}
	if selected["general"] {
		filtered.Settings.General = document.Settings.General
		filtered.Settings.RemoteBackends = document.Settings.RemoteBackends
	}
	if selected["dns"] {
		filtered.Settings.DNS = document.Settings.DNS
	}
	if selected["surge"] {
		filtered.Settings.Surge = document.Settings.Surge
	}
	if selected["routing"] {
		filtered.Settings.Routing = document.Settings.Routing
	}
	if selected["bgp"] {
		filtered.Settings.BGP = document.Settings.BGP
	}
	if selected["cache"] {
		filtered.Settings.Cache = document.Settings.Cache
	}
	if selected["health"] {
		filtered.Settings.Health = document.Settings.Health
	}
	if selected["logging"] {
		filtered.Settings.Logging = document.Settings.Logging
	}
	if selected["notifications"] {
		filtered.Settings.Notifications = document.Settings.Notifications
	}
	if selected["security"] {
		filtered.Settings.Security = document.Settings.Security
	}
	if selected["updates"] {
		filtered.Settings.Updates = document.Settings.Updates
	}
	if selected["peers"] {
		filtered.Peers = document.Peers
	}
	if selected["routes"] {
		filtered.Routes = document.Routes
	}
	if selected["route_feeds"] {
		filtered.RouteFeeds = document.RouteFeeds
	}
	if selected["interception"] {
		filtered.Interception = document.Interception
	}
	if selected["overrides"] {
		filtered.Overrides = document.Overrides
	}
	if selected["hosts"] {
		filtered.Hosts = document.Hosts
	}
	return configurationBackup{FormatVersion: configurationBackupFormatVersion, ConfigVersion: document.Version, Sections: append([]string(nil), sections...), Configuration: filtered.Redacted()}, nil
}

func (backup configurationBackup) validate() error {
	if backup.FormatVersion != configurationBackupFormatVersion {
		return fmt.Errorf("unsupported backup format version %d", backup.FormatVersion)
	}
	if backup.ConfigVersion != 1 || backup.Configuration.Version != backup.ConfigVersion {
		return fmt.Errorf("configuration version %d is incompatible with this backend", backup.ConfigVersion)
	}
	if _, err := selectBackupSections(backup.Sections); err != nil {
		return err
	}
	return nil
}

func mergeBackup(current, incoming config.Document, sections []string) (config.Document, error) {
	selected, err := selectBackupSections(sections)
	if err != nil {
		return config.Document{}, err
	}
	merged := current.Clone()
	if selected["dhcp"] {
		merged.Settings.DHCP = incoming.Settings.DHCP.Clone()
	}
	if selected["general"] {
		merged.Settings.General = incoming.Settings.General
		merged.Settings.RemoteBackends = incoming.Settings.RemoteBackends
	}
	if selected["dns"] {
		merged.Settings.DNS = incoming.Settings.DNS
	}
	if selected["surge"] {
		merged.Settings.Surge = incoming.Settings.Surge
	}
	if selected["routing"] {
		merged.Settings.Routing = incoming.Settings.Routing
	}
	if selected["bgp"] {
		merged.Settings.BGP = incoming.Settings.BGP
	}
	if selected["cache"] {
		merged.Settings.Cache = incoming.Settings.Cache
	}
	if selected["health"] {
		merged.Settings.Health = incoming.Settings.Health
	}
	if selected["logging"] {
		merged.Settings.Logging = incoming.Settings.Logging
	}
	if selected["notifications"] {
		merged.Settings.Notifications = incoming.Settings.Notifications
	}
	if selected["security"] {
		merged.Settings.Security = incoming.Settings.Security
	}
	if selected["updates"] {
		merged.Settings.Updates = incoming.Settings.Updates
	}
	if selected["peers"] {
		merged.Peers = incoming.Peers
	}
	if selected["routes"] {
		merged.Routes = incoming.Routes
	}
	if selected["route_feeds"] {
		merged.RouteFeeds = incoming.RouteFeeds
	}
	if selected["interception"] {
		merged.Interception = incoming.Interception
	}
	if selected["overrides"] {
		merged.Overrides = incoming.Overrides
	}
	if selected["hosts"] {
		merged.Hosts = incoming.Hosts
	}
	return merged, nil
}
