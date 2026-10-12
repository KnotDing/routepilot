package config

import (
	"errors"
	"net/netip"
	"strings"
	"surge-controller/internal/model"
	"unicode"
	"unicode/utf8"
)

func validateClients(clients []Client) error {
	if len(clients) > 4096 {
		return errors.New("too many client settings")
	}
	ips, ids := map[netip.Addr]bool{}, map[string]bool{}
	for _, c := range clients {
		ip, err := netip.ParseAddr(c.IP)
		if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("client IP must be a concrete IPv4 or IPv6 address")
		}
		ip = ip.Unmap()
		if (c.ID == "" || len(c.ID) > 64 || strings.ContainsAny(c.ID, "/\\.%?# ")) || ids[c.ID] || ips[ip] {
			return errors.New("duplicate or invalid client setting")
		}
		ids[c.ID], ips[ip] = true, true
		if !utf8.ValidString(c.Name) || utf8.RuneCountInString(c.Name) > 80 || strings.ContainsFunc(c.Name, unicode.IsControl) {
			return errors.New("client name must contain at most 80 characters without control characters")
		}
		if c.Decision != "" && c.Decision != model.Direct && c.Decision != model.Surge && c.Decision != model.Reject {
			return errors.New("client policy must be automatic, DIRECT, SURGE or REJECT")
		}
	}
	return nil
}
