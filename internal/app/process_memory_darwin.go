//go:build darwin

package app

import (
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

func processRSSBytes() (uint64, error) {
	const (
		procInfoCallPIDInfo = 2
		procPIDTaskInfo     = 4
	)
	var info [256]byte
	n, _, errno := unix.Syscall6(
		unix.SYS_PROC_INFO,
		procInfoCallPIDInfo,
		uintptr(os.Getpid()),
		procPIDTaskInfo,
		0,
		uintptr(unsafe.Pointer(&info[0])),
		uintptr(len(info)),
	)
	runtime.KeepAlive(info)
	if errno != 0 {
		return 0, errno
	}
	if n < 16 {
		return 0, errors.New("macOS returned incomplete process memory information")
	}
	return binary.NativeEndian.Uint64(info[8:16]), nil
}
