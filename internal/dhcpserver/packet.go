// Package dhcpserver implements a local Ethernet DHCPv4 server (RFC 2131/2132).
// It serves one explicitly configured broadcast domain; relay agents and DHCPv6
// are deliberately rejected rather than allocated from an unrelated pool.
package dhcpserver

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"

	"surge-controller/internal/config"
)

type packet struct {
	raw      []byte
	options  map[byte][]byte
	mac, key string
	ci       netip.Addr
	kind     byte
}

func decode(b []byte) (packet, error) {
	var p packet
	if len(b) < 240 || len(b) > 4096 || b[0] != 1 || b[1] != 1 || b[2] != 6 || b[3] != 0 || !bytes.Equal(b[236:240], []byte{99, 130, 83, 99}) || !bytes.Equal(b[24:28], make([]byte, 4)) {
		return p, errors.New("unsupported DHCP packet")
	}
	p.raw = append([]byte(nil), b[:240]...)
	p.options = map[byte][]byte{}
	m := net.HardwareAddr(b[28:34])
	if m[0]&1 != 0 || bytes.Equal(m, make([]byte, 6)) {
		return p, errors.New("invalid client MAC")
	}
	p.mac = m.String()
	p.key = "mac:" + p.mac
	p.ci = netip.AddrFrom4([4]byte(b[12:16]))
	for i := 240; i < len(b); {
		code := b[i]
		i++
		if code == 255 {
			break
		}
		if code == 0 {
			continue
		}
		if i >= len(b) || i+1+int(b[i]) > len(b) {
			return p, errors.New("truncated DHCP option")
		}
		n := int(b[i])
		i++
		p.options[code] = append(p.options[code], b[i:i+n]...)
		i += n
	}
	if len(p.options[52]) != 0 {
		return p, errors.New("DHCP option overload is not supported")
	}
	if len(p.options[53]) != 1 {
		return p, errors.New("missing DHCP message type")
	}
	p.kind = p.options[53][0]
	if v := p.options[57]; len(v) != 0 && (len(v) != 2 || binary.BigEndian.Uint16(v) < 576) {
		return p, errors.New("invalid DHCP maximum message size")
	}
	for _, c := range []byte{50, 54} {
		if len(p.options[c]) != 0 && len(p.options[c]) != 4 {
			return p, errors.New("invalid DHCP address option")
		}
	}
	if id := p.options[61]; len(id) > 0 {
		if len(id) < 2 || len(id) > 128 {
			return p, errors.New("DHCP client ID is too long")
		}
		p.key = "id:" + hex.EncodeToString(id)
	}
	return p, nil
}
func optIP(v []byte) netip.Addr {
	if len(v) != 4 {
		return netip.Addr{}
	}
	return netip.AddrFrom4([4]byte(v))
}
func putOption(b []byte, code byte, v []byte) []byte {
	if len(v) == 0 {
		return b
	}
	for len(v) > 0 {
		n := len(v)
		if n > 255 {
			n = 255
		}
		b = append(b, code, byte(n))
		b = append(b, v[:n]...)
		v = v[n:]
	}
	return b
}
func ipBytes(s string) []byte { a := netip.MustParseAddr(s).As4(); return a[:] }
func uintBytes(n int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}
func addresses(list []string) []byte {
	var b []byte
	for _, s := range list {
		b = append(b, ipBytes(s)...)
	}
	return b
}

func reply(p packet, kind byte, address string, d config.DHCP, o config.DHCPOptions) []byte {
	b := make([]byte, 240)
	copy(b, p.raw)
	b[0] = 2
	b[3] = 0
	clear(b[8:12])
	clear(b[16:28])
	clear(b[44:236])
	if address != "" {
		copy(b[16:20], ipBytes(address))
	}
	b = putOption(b, 53, []byte{kind})
	b = putOption(b, 54, ipBytes(d.ServerIP))
	if id := p.options[61]; len(id) > 0 {
		b = putOption(b, 61, id)
	}
	if kind == 6 {
		clear(b[12:16])
		b[10] = 0x80
	}
	if kind != 6 {
		prefix := netip.MustParsePrefix(d.Subnet)
		mask := uint32(0xffffffff) << uint(32-prefix.Bits())
		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, mask)
		b = putOption(b, 1, v)
		if p.kind != 8 {
			b = putOption(b, 51, uintBytes(o.LeaseSeconds))
			b = putOption(b, 58, uintBytes(o.LeaseSeconds/2))
			b = putOption(b, 59, uintBytes(o.LeaseSeconds*7/8))
		}
		if o.Gateway != "" {
			b = putOption(b, 3, ipBytes(o.Gateway))
		}
		b = putOption(b, 6, addresses(o.DNS))
		b = putOption(b, 42, addresses(o.NTP))
		b = putOption(b, 15, []byte(o.Domain))
		b = putOption(b, 66, []byte(o.TFTP))
		b = putOption(b, 67, []byte(o.BootFile))
		if o.BootServer != "" {
			copy(b[20:24], ipBytes(o.BootServer))
		}
		if len(o.BootFile) <= 128 {
			copy(b[108:236], o.BootFile)
		}
		if len(o.Routes) > 0 {
			var routes []byte
			for _, r := range o.Routes {
				prefix := netip.MustParsePrefix(r.Prefix)
				a := prefix.Addr().As4()
				routes = append(routes, byte(prefix.Bits()))
				routes = append(routes, a[:(prefix.Bits()+7)/8]...)
				routes = append(routes, ipBytes(r.Gateway)...)
			}
			// RFC 3442 clients ignore option 3 in the presence of option 121.
			if o.Gateway != "" {
				routes = append(routes, 0)
				routes = append(routes, ipBytes(o.Gateway)...)
			}
			b = putOption(b, 121, routes)
		}
	}
	b = append(b, 255)
	for len(b) < 300 {
		b = append(b, 0)
	}
	return b
}
