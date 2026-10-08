//go:build !darwin

package dnsserver

import (
	"context"
	"errors"
	"net/netip"
	"surge-controller/internal/model"
)

type PF struct{}

func OpenPF() (*PF, error) { return nil, errors.New("transparent PF ingress requires macOS") }
func (*PF) Close() error   { return nil }
func (*PF) Lookup(context.Context, model.DNSProtocol, netip.AddrPort, netip.AddrPort) (netip.AddrPort, error) {
	return netip.AddrPort{}, errors.New("PF ingress is unavailable")
}
