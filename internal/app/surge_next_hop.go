package app

import (
	"net/netip"
	"surge-controller/internal/config"
)

func hasIPv4FakePrefix(d config.Document) bool {
	for _, value := range d.Settings.Surge.FakePrefixes {
		if prefix, err := netip.ParsePrefix(value); err == nil && prefix.Addr().Is4() {
			return true
		}
	}
	return false
}
