//go:build !darwin && !linux

package dhcpserver

import (
	"errors"
	"net"
)

func bindInterface(_ *net.UDPConn, _ *net.Interface) error {
	return errors.New("DHCP interface binding is unsupported on this platform")
}
