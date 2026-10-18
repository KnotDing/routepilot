package app

import (
	"context"
	"surge-controller/internal/config"
	"surge-controller/internal/dhcpserver"
)

func (a *App) reportDHCPWarnings() {
	if report := a.dhcp.Snapshot().Preflight; report != nil {
		for _, warning := range report.Warnings {
			a.event("WARNING", "dhcp_preflight", warning)
		}
	}
}
func (a *App) CheckDHCP(ctx context.Context, d config.DHCP) (dhcpserver.CheckResult, error) {
	return a.dhcp.Check(ctx, d)
}
