//go:build darwin

package resolver

import (
	"golang.org/x/sys/unix"
	"syscall"
)

func interfaceControl(index int, ipv6 bool) func(string, string, syscall.RawConn) error {
	return func(_, _ string, raw syscall.RawConn) error {
		var result error
		err := raw.Control(func(fd uintptr) {
			if ipv6 {
				result = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, index)
			} else {
				result = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, index)
			}
		})
		if err != nil {
			return err
		}
		return result
	}
}
