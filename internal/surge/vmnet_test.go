package surge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type vmnetRunner func(context.Context, ...string) ([]byte, error)

func (r vmnetRunner) Run(ctx context.Context, args ...string) ([]byte, error) { return r(ctx, args...) }

func TestVMNextHopUsesOnlyRunningVMSelfIP(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"running":1,"self-ip":"192.168.20.141","router-ip":"192.168.20.1","primaryAddress":"192.168.20.2"}`, "192.168.20.141"},
		{`{"running":0,"self-ip":"192.168.20.141"}`, ""},
		{`{"self-ip":"192.168.20.141"}`, ""},
		{`{"running":1,"router-ip":"192.168.20.1"}`, ""},
		{`{"running":true,"self-ip":"192.168.20.141"}`, ""},
		{`{"running":1,"self-ip":"::ffff:192.168.20.141"}`, ""},
		{`{"running":1,"self-ip":"127.0.0.1"}`, ""},
		{`{"running":1,"self-ip":"169.254.1.1"}`, ""},
		{`{"running":1,"self-ip":"0.0.0.0"}`, ""},
		{`{"running":1,"self-ip":"255.255.255.255"}`, ""},
		{`{"running":1,"self-ip":"224.0.0.1"}`, ""},
		{`{"running":1,"self-ip":"192.168.20.141"} garbage`, ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			address, err := ReadVMNextHop4(context.Background(), vmnetRunner(func(ctx context.Context, args ...string) ([]byte, error) {
				if !reflect.DeepEqual(args, []string{"vmnet", "status"}) {
					t.Fatal(args)
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 2*time.Second {
					t.Fatal("unbounded probe")
				}
				return []byte(tc.raw), nil
			}))
			if address != tc.want || (err == nil) != (tc.want != "") {
				t.Fatal(address, err)
			}
		})
	}
	_, err := ReadVMNextHop4(context.Background(), vmnetRunner(func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("private command output")
	}))
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
}
