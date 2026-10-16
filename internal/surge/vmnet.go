package surge

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"time"
)

// ReadVMNextHop4 reads the VM's own address, not the Mac's primary address or
// VMNET's upstream router-ip. It runs in the backend's Surge GUI user context.
func ReadVMNextHop4(ctx context.Context, runner Runner) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	raw, err := runner.Run(ctx, "vmnet", "status")
	if err != nil {
		return "", errors.New("Surge VMNET status unavailable; check Surge Enhanced Mode and CLI support")
	}
	var status struct {
		Running *int   `json:"running"`
		SelfIP  string `json:"self-ip"`
	}
	if len(raw) > 64<<10 || json.Unmarshal(raw, &status) != nil || status.Running == nil || *status.Running != 1 {
		return "", errors.New("Surge VMNET is not running or returned invalid status")
	}
	ip, err := netip.ParseAddr(status.SelfIP)
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.String() == "255.255.255.255" {
		return "", errors.New("Surge VMNET did not return a usable IPv4 self-ip")
	}
	return ip.String(), nil
}
