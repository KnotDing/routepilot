//go:build !darwin

package dhcpserver

import (
	"context"
	"errors"
)

func addressMode(_ context.Context, _ string) (string, error) {
	return "unknown", errors.New("automatic IPv4 configuration method detection is only available on macOS")
}
