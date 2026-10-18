package dhcpserver

import "strings"

func networkService(order, iface string) string {
	service := ""
	for _, line := range strings.Split(order, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "(Hardware Port:") {
			if strings.Contains(line, ", Device: "+iface+")") {
				return service
			}
			continue
		}
		if strings.HasPrefix(line, "(") {
			if i := strings.Index(line, ") "); i >= 0 {
				service = line[i+2:]
			}
		}
	}
	return ""
}
func parseAddressMode(info string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "Manual Configuration" {
			return "static"
		}
		if line == "DHCP Configuration" || line == "Using DHCP" || line == "DHCP with manual address Configuration" {
			return "dhcp"
		}
	}
	return "unknown"
}
