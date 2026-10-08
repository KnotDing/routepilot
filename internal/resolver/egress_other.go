//go:build !darwin

package resolver

import (
	"errors"
	"syscall"
)

func interfaceControl(int, bool) func(string, string, syscall.RawConn) error {
	return func(string, string, syscall.RawConn) error {
		return errors.New("pinned egress is supported only on macOS")
	}
}
