//go:build darwin

package dnsserver

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
	"surge-controller/internal/model"
)

// Darwin's private PF ABI: four 16-byte pf_addr unions, four 4-byte
// pf_state_xport unions, af/proto/proto_variant/direction, total 84 bytes.
// This is NOT the older OpenBSD 76-byte structure used by many examples.
// See docs/architecture.md for the Apple source and the required runtime test.
const natLookSize = 84
const diOCNATLOOK = uintptr(0xc0544417)

type PF struct {
	mu   sync.Mutex
	file *os.File
}

func OpenPF() (*PF, error) {
	f, err := os.OpenFile("/dev/pf", os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open PF state device: %w", err)
	}
	return &PF{file: f}, nil
}
func (p *PF) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	return err
}
func (p *PF) Lookup(ctx context.Context, proto model.DNSProtocol, client, local netip.AddrPort) (netip.AddrPort, error) {
	if err := ctx.Err(); err != nil {
		return netip.AddrPort{}, err
	}
	if !client.IsValid() || !local.IsValid() || client.Port() == 0 || local.Port() == 0 || client.Addr().Unmap().Is4() != local.Addr().Unmap().Is4() {
		return netip.AddrPort{}, errors.New("invalid PF lookup tuple")
	}
	b, err := natLookRequest(proto, client, local)
	if err != nil {
		return netip.AddrPort{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		return netip.AddrPort{}, os.ErrClosed
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, p.file.Fd(), diOCNATLOOK, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(b)
	if errno != 0 {
		return netip.AddrPort{}, fmt.Errorf("PF original destination: %w", errno)
	}
	var addr netip.Addr
	if client.Addr().Unmap().Is4() {
		addr = netip.AddrFrom4([4]byte(b[48:52]))
	} else {
		addr = netip.AddrFrom16([16]byte(b[48:64]))
	}
	return netip.AddrPortFrom(addr, binary.BigEndian.Uint16(b[76:78])), nil
}

func natLookRequest(proto model.DNSProtocol, client, local netip.AddrPort) ([natLookSize]byte, error) {
	var b [natLookSize]byte
	source, dest := client.Addr().Unmap(), local.Addr().Unmap()
	copy(b[0:16], source.AsSlice())
	copy(b[16:32], dest.AsSlice())
	binary.BigEndian.PutUint16(b[64:66], client.Port())
	binary.BigEndian.PutUint16(b[68:70], local.Port())
	b[80] = unix.AF_INET
	if source.Is6() {
		b[80] = unix.AF_INET6
	}
	b[81] = unix.IPPROTO_UDP
	// UDP state keys use PF_EXTFILTER_APD (1), not the zero variant used
	// by TCP. Our rdr rules use the default address-and-port-dependent filter.
	// Asking for variant zero cannot find a Darwin UDP state.
	b[82] = 1
	if proto == model.TCP {
		b[81] = unix.IPPROTO_TCP
		b[82] = 0
	} else if proto != model.UDP {
		return b, errors.New("unsupported PF protocol")
	}
	// PF_OUT searches the reverse of the translated client -> listener tuple.
	// Apple pfioctl_ioc_natlook returns sk->gwy (original external destination).
	b[83] = 2
	return b, nil
}
