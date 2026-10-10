//go:build linux

package app

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

func processRSSBytes() (uint64, error) {
	statm, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(statm))
	if len(fields) < 2 {
		return 0, errors.New("Linux returned incomplete process memory information")
	}
	residentPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return residentPages * uint64(os.Getpagesize()), nil
}
