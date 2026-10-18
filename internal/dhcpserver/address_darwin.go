package dhcpserver

import (
	"context"
	"fmt"
	"os/exec"
)

func addressMode(ctx context.Context, iface string) (string, error) {
	order, err := exec.CommandContext(ctx, "/usr/sbin/networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return "unknown", fmt.Errorf("read macOS network service: %w", err)
	}
	service := networkService(string(order), iface)
	if service == "" {
		return "unknown", fmt.Errorf("no macOS network service found for %s", iface)
	}
	info, err := exec.CommandContext(ctx, "/usr/sbin/networksetup", "-getinfo", service).Output()
	if err != nil {
		return "unknown", fmt.Errorf("read IPv4 configuration: %w", err)
	}
	mode := parseAddressMode(string(info))
	if mode == "unknown" {
		return mode, fmt.Errorf("unrecognized IPv4 configuration method")
	}
	return mode, nil
}
