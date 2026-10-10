//go:build !darwin && !linux

package app

import "errors"

func processRSSBytes() (uint64, error) {
	return 0, errors.New("process RSS is not supported on this platform")
}
