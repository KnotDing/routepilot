package dhcpserver

import (
	"golang.org/x/sys/unix"
	"net"
)

func bindInterface(c *net.UDPConn, iface *net.Interface) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var socketErr error
	err = raw.Control(func(fd uintptr) {
		socketErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
		if socketErr == nil {
			socketErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, iface.Index)
		}
	})
	if err != nil {
		return err
	}
	return socketErr
}
