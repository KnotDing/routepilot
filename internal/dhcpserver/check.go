package dhcpserver

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
	"surge-controller/internal/config"
)

type DiscoveredServer struct {
	Address string `json:"address"`
	Source  string `json:"source"`
}
type CheckResult struct {
	Interface    string             `json:"interface"`
	CheckedAt    time.Time          `json:"checked_at"`
	AddressMode  string             `json:"address_mode"`
	AddressError string             `json:"address_error,omitempty"`
	ProbeError   string             `json:"probe_error,omitempty"`
	Servers      []DiscoveredServer `json:"servers"`
	Warnings     []string           `json:"warnings"`
}

// Check uses DHCPINFORM/ACK from an already configured interface, without
// acquiring a lease or modifying interface configuration.
func (s *Service) Check(ctx context.Context, d config.DHCP) (CheckResult, error) {
	iface, err := net.InterfaceByName(d.Interface)
	if err != nil {
		return CheckResult{}, fmt.Errorf("DHCP interface: %w", err)
	}
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
		return CheckResult{}, fmt.Errorf("DHCP needs an active broadcast-capable LAN interface")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	r := CheckResult{Interface: d.Interface, CheckedAt: time.Now().UTC(), AddressMode: "unknown", Servers: []DiscoveredServer{}, Warnings: []string{}}
	s.mu.Lock()
	own := ""
	if s.current != nil {
		own = s.current.config.ServerIP
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		mode, e := s.addressCheck(ctx, d.Interface)
		r.AddressMode = mode
		if e != nil {
			r.AddressError = e.Error()
		}
	}()
	go func() {
		defer wg.Done()
		servers, e := s.discoverCheck(ctx, iface, own)
		r.Servers = servers
		if e != nil {
			r.ProbeError = e.Error()
		}
	}()
	wg.Wait()
	if r.AddressMode == "dhcp" {
		r.Warnings = append(r.Warnings, "The selected interface obtains its IPv4 address using DHCP; configure a static address before using it as a DHCP server.")
	}
	if r.AddressMode == "unknown" {
		r.Warnings = append(r.Warnings, "The selected interface's IPv4 configuration method could not be verified.")
	}
	if len(r.Servers) > 0 {
		r.Warnings = append(r.Warnings, "Another DHCP server responded on the selected interface. Multiple DHCP servers may distribute conflicting addresses and options.")
	}
	if r.ProbeError != "" {
		r.Warnings = append(r.Warnings, "DHCP discovery could not be completed. Absence of another DHCP server has not been established.")
	}
	return r, nil
}

func discover(ctx context.Context, iface *net.Interface, own string) ([]DiscoveredServer, error) {
	servers := []DiscoveredServer{}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 68})
	if err != nil {
		return servers, fmt.Errorf("DHCP probe UDP 68: %w", err)
	}
	defer conn.Close()
	if err = bindInterface(conn, iface); err != nil {
		return servers, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return servers, err
	}
	var source net.IP
	for _, value := range addrs {
		ip, _, e := net.ParseCIDR(value.String())
		if e == nil && ip.To4() != nil && !ip.IsUnspecified() {
			source = ip.To4()
			break
		}
	}
	if source == nil {
		return servers, fmt.Errorf("DHCP discovery requires an IPv4 address on the selected interface")
	}
	pc := ipv4.NewPacketConn(conn)
	if err = pc.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return servers, err
	}
	data := make([]byte, 300)
	if _, err = rand.Read(data[4:8]); err != nil {
		return servers, err
	}
	if _, err = rand.Read(data[28:34]); err != nil {
		return servers, err
	}
	data[28] = (data[28] | 2) & 0xfe
	data[0] = 1
	data[1] = 1
	data[2] = 6
	data[10] = 0x80
	copy(data[236:240], []byte{99, 130, 83, 99})
	copy(data[12:16], source)
	copy(data[240:], []byte{53, 1, 8, 255})
	xid := binary.BigEndian.Uint32(data[4:8])
	buf := make([]byte, 4096)
	seen := map[string]bool{}
	for attempt := 0; attempt < 2; attempt++ {
		if err = ctx.Err(); err != nil {
			return servers, err
		}
		if _, err = pc.WriteTo(data, &ipv4.ControlMessage{IfIndex: iface.Index, Src: source}, &net.UDPAddr{IP: net.IPv4bcast, Port: 67}); err != nil {
			return servers, err
		}
		deadline := time.Now().Add(750 * time.Millisecond)
		if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
			deadline = limit
		}
		if err = conn.SetReadDeadline(deadline); err != nil {
			return servers, err
		}
		for {
			n, cm, peer, e := pc.ReadFrom(buf)
			if e != nil {
				if ne, ok := e.(net.Error); ok && ne.Timeout() {
					break
				}
				return servers, e
			}
			remote, ok := peer.(*net.UDPAddr)
			if !ok || remote.Port != 67 || cm == nil || cm.IfIndex != iface.Index {
				continue
			}
			address := probeReply(buf[:n], xid, data[28:34])
			if address == "" || address == own || seen[address] {
				continue
			}
			seen[address] = true
			servers = append(servers, DiscoveredServer{Address: address, Source: remote.IP.String()})
			if len(servers) >= 32 {
				return servers, nil
			}
		}
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Address < servers[j].Address })
	return servers, nil
}
func probeReply(b []byte, xid uint32, mac []byte) string {
	if len(b) < 240 || b[0] != 2 || b[1] != 1 || b[2] != 6 || binary.BigEndian.Uint32(b[4:8]) != xid || string(b[28:34]) != string(mac) || string(b[236:240]) != string([]byte{99, 130, 83, 99}) {
		return ""
	}
	var kind byte
	address := ""
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
			return ""
		}
		n := int(b[i])
		i++
		v := b[i : i+n]
		i += n
		if code == 53 && n == 1 {
			kind = v[0]
		}
		if code == 54 && n == 4 {
			address = net.IP(v).String()
		}
	}
	if kind != 5 {
		return ""
	}
	return address
}
