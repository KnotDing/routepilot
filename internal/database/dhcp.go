package database

import (
	"context"
	"surge-controller/internal/dhcpserver"
)

func (s *Store) LoadDHCP(ctx context.Context) (dhcpserver.State, error) {
	var state dhcpserver.State
	err := s.LoadStatistics(ctx, "dhcp_leases", &state)
	return state, err
}
func (s *Store) SaveDHCP(ctx context.Context, state dhcpserver.State) error {
	return s.SaveStatistics(ctx, "dhcp_leases", state)
}
