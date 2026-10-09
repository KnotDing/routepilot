package config

import (
	_ "embed"
	"encoding/json"
	"strings"
)

type notificationEvent struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix bool   `json:"prefix"`
}

//go:embed notification-events.json
var notificationEventJSON []byte

var notificationEvents = func() []notificationEvent {
	var events []notificationEvent
	if err := json.Unmarshal(notificationEventJSON, &events); err != nil {
		panic("invalid built-in notification event catalog")
	}
	return events
}()

func (s Notifications) Selected(name string) bool {
	// nil preserves the category choices from older configurations; [] explicitly
	// selects no errors. New selections use stable IDs rather than error text.
	if s.Errors != nil {
		for _, event := range notificationEvents {
			if event.Name == name || event.Prefix && strings.HasPrefix(name, event.Name) {
				for _, id := range s.Errors {
					if id == event.ID {
						return true
					}
				}
				return false
			}
		}
		return false
	}
	switch {
	case strings.HasPrefix(name, "Surge"):
		return s.Surge
	case strings.HasPrefix(name, "BGP") || strings.Contains(name, "routing"):
		return s.BGP
	case strings.Contains(name, "DNS"):
		return s.DNS
	default:
		return s.Controller
	}
}

func validNotificationErrors(ids []string) bool {
	if len(ids) > len(notificationEvents) {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		known := false
		for _, event := range notificationEvents {
			known = known || event.ID == id
		}
		if !known || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
