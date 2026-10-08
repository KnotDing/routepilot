//go:build darwin

package dnsserver

import (
	"encoding/binary"
	"golang.org/x/sys/unix"
	"net/netip"
	"surge-controller/internal/model"
	"testing"
)

func TestDarwinNATLookProtocolVariants(t *testing.T) {
	for _, pair := range [][2]string{{"172.30.31.130:36822", "192.168.20.2:1054"}, {"[2001:db8::1]:36822", "[2001:db8::2]:1054"}} {
		for _, proto := range []model.DNSProtocol{model.UDP, model.TCP} {
			b, err := natLookRequest(proto, netip.MustParseAddrPort(pair[0]), netip.MustParseAddrPort(pair[1]))
			if err != nil {
				t.Fatal(err)
			}
			wantProto, wantVariant := byte(unix.IPPROTO_UDP), byte(1)
			if proto == model.TCP {
				wantProto, wantVariant = unix.IPPROTO_TCP, 0
			}
			if b[81] != wantProto || b[82] != wantVariant || b[83] != 2 {
				t.Fatalf("incorrect PF state-key discriminator: %v", b[80:])
			}
			if binary.BigEndian.Uint16(b[64:66]) != 36822 || binary.BigEndian.Uint16(b[68:70]) != 1054 {
				t.Fatal("incorrect port layout or byte order")
			}
		}
	}
}
