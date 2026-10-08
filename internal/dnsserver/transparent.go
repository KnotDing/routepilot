package dnsserver

import (
	"context"
	"errors"
	"net/netip"

	"surge-controller/internal/model"
)

type OriginalDestinationLookup interface {
	Lookup(context.Context, model.DNSProtocol, netip.AddrPort, netip.AddrPort) (netip.AddrPort, error)
}
type Ingress struct {
	Bind        netip.AddrPort
	Intercepted bool
	Lookup      OriginalDestinationLookup
}

func (i Ingress) Validate() error {
	if !i.Bind.IsValid() || i.Bind.Addr().IsUnspecified() || i.Bind.Addr().IsMulticast() {
		return errors.New("DNS listeners require a concrete unicast bind address")
	}
	if i.Intercepted && (i.Lookup == nil || i.Bind.Port() == 0) {
		return errors.New("transparent listener requires an original-destination lookup and fixed port")
	}
	return nil
}
func (i Ingress) QueryContext(ctx context.Context, protocol model.DNSProtocol, client, local netip.AddrPort) (model.DNSQueryContext, error) {
	dest := local
	if i.Intercepted {
		var err error
		dest, err = i.Lookup.Lookup(ctx, protocol, client, local)
		if err != nil {
			return model.DNSQueryContext{}, ErrOriginalDestination
		}
		if !dest.IsValid() || dest.Port() != 53 || dest.Addr().Unmap() == local.Addr().Unmap() {
			return model.DNSQueryContext{}, ErrOriginalDestination
		}
	}
	return model.DNSQueryContext{ClientIP: client.Addr().Unmap(), ClientPort: client.Port(), OriginalDestinationIP: dest.Addr().Unmap(), OriginalDestinationPort: dest.Port(), Transport: protocol, Intercepted: i.Intercepted}, nil
}
