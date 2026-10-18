package config

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// DHCP is a single explicitly selected IPv4 broadcast domain. An absent block
// in older documents is disabled. Device option pointers distinguish inheritance
// (nil) from an explicitly empty option.
type DHCP struct {
	Enabled   bool         `json:"enabled"`
	Interface string       `json:"interface"`
	ServerIP  string       `json:"server_ip"`
	Subnet    string       `json:"subnet"`
	PoolStart string       `json:"pool_start"`
	PoolEnd   string       `json:"pool_end"`
	Options   DHCPOptions  `json:"options"`
	Devices   []DHCPDevice `json:"devices"`
}
type DHCPRoute struct {
	Prefix  string `json:"prefix"`
	Gateway string `json:"gateway"`
}
type DHCPOptions struct {
	Gateway      string      `json:"gateway"`
	DNS          []string    `json:"dns"`
	Routes       []DHCPRoute `json:"routes"`
	NTP          []string    `json:"ntp"`
	BootFile     string      `json:"boot_file"`
	BootServer   string      `json:"boot_server"`
	TFTP         string      `json:"tftp"`
	LeaseSeconds int         `json:"lease_seconds"`
	Domain       string      `json:"domain"`
}
type DHCPOverrides struct {
	Gateway      *string      `json:"gateway,omitempty"`
	DNS          *[]string    `json:"dns,omitempty"`
	Routes       *[]DHCPRoute `json:"routes,omitempty"`
	NTP          *[]string    `json:"ntp,omitempty"`
	BootFile     *string      `json:"boot_file,omitempty"`
	BootServer   *string      `json:"boot_server,omitempty"`
	TFTP         *string      `json:"tftp,omitempty"`
	LeaseSeconds *int         `json:"lease_seconds,omitempty"`
	Domain       *string      `json:"domain,omitempty"`
}
type DHCPDevice struct {
	MAC     string        `json:"mac"`
	Name    string        `json:"name"`
	Address string        `json:"address"`
	Options DHCPOverrides `json:"options"`
}

func dhcpCopyPtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
func dhcpCopySlice[T any](v []T) []T {
	if v == nil {
		return nil
	}
	return append([]T{}, v...)
}
func dhcpCopyListPtr[T any](v *[]T) *[]T {
	if v == nil {
		return nil
	}
	c := dhcpCopySlice(*v)
	return &c
}
func (d DHCP) Clone() DHCP {
	c := d
	c.Options.DNS = dhcpCopySlice(d.Options.DNS)
	c.Options.NTP = dhcpCopySlice(d.Options.NTP)
	c.Options.Routes = dhcpCopySlice(d.Options.Routes)
	c.Devices = dhcpCopySlice(d.Devices)
	for i := range c.Devices {
		v := d.Devices[i].Options
		c.Devices[i].Options = DHCPOverrides{Gateway: dhcpCopyPtr(v.Gateway), DNS: dhcpCopyListPtr(v.DNS), Routes: dhcpCopyListPtr(v.Routes), NTP: dhcpCopyListPtr(v.NTP), BootFile: dhcpCopyPtr(v.BootFile), BootServer: dhcpCopyPtr(v.BootServer), TFTP: dhcpCopyPtr(v.TFTP), LeaseSeconds: dhcpCopyPtr(v.LeaseSeconds), Domain: dhcpCopyPtr(v.Domain)}
	}
	return c
}
func (d DHCP) ForDevice(mac string) (DHCPOptions, string) {
	o := d.Options
	if o.LeaseSeconds == 0 {
		o.LeaseSeconds = 86400
	}
	for _, device := range d.Devices {
		m, err := net.ParseMAC(device.MAC)
		if err != nil || !strings.EqualFold(m.String(), mac) {
			continue
		}
		v := device.Options
		if v.Gateway != nil {
			o.Gateway = *v.Gateway
		}
		if v.DNS != nil {
			o.DNS = *v.DNS
		}
		if v.Routes != nil {
			o.Routes = *v.Routes
		}
		if v.NTP != nil {
			o.NTP = *v.NTP
		}
		if v.BootFile != nil {
			o.BootFile = *v.BootFile
		}
		if v.BootServer != nil {
			o.BootServer = *v.BootServer
		}
		if v.TFTP != nil {
			o.TFTP = *v.TFTP
		}
		if v.LeaseSeconds != nil {
			o.LeaseSeconds = *v.LeaseSeconds
		}
		if v.Domain != nil {
			o.Domain = *v.Domain
		}
		return o, device.Address
	}
	return o, ""
}
func DHCPv4(raw string) (netip.Addr, error) {
	a, e := netip.ParseAddr(raw)
	if e != nil || !a.Is4() || a.IsUnspecified() || a.IsMulticast() || a.IsLoopback() || a == netip.MustParseAddr("255.255.255.255") {
		return netip.Addr{}, fmt.Errorf("invalid DHCP IPv4 address %q", raw)
	}
	return a, nil
}
func (d DHCP) Validate() error {
	fail := func(s string) error { return fmt.Errorf("DHCP: %s", s) }
	configured := d.Enabled || d.Subnet != "" || d.ServerIP != "" || d.PoolStart != "" || d.PoolEnd != ""
	var p netip.Prefix
	var first, last, server netip.Addr
	if configured {
		var err error
		p, err = netip.ParsePrefix(d.Subnet)
		if err != nil || !p.Addr().Is4() || p != p.Masked() || p.Bits() < 8 || p.Bits() > 30 {
			return fail("subnet must be a canonical IPv4 /8 through /30")
		}
		first, err = DHCPv4(d.PoolStart)
		if err != nil {
			return err
		}
		last, err = DHCPv4(d.PoolEnd)
		if err != nil {
			return err
		}
		server, err = DHCPv4(d.ServerIP)
		if err != nil {
			return err
		}
		if d.Interface == "" || len(d.Interface) > 64 || strings.ContainsAny(d.Interface, " /\\\t\r\n") {
			return fail("select an interface")
		}
		for _, a := range []netip.Addr{first, last, server} {
			if !dhcpHost(p, a) {
				return fail("server and address pool must be usable hosts in the subnet")
			}
		}
		if first.Compare(last) > 0 || ipv4Number(last)-ipv4Number(first) >= 65536 {
			return fail("pool must contain 1 through 65536 addresses")
		}
		if inDHCPPool(server, first, last) {
			return fail("server address must be outside the dynamic pool")
		}
	}
	if len(d.Devices) > 4096 {
		return fail("at most 4096 device configurations are supported")
	}
	macs, ips := map[string]bool{}, map[string]bool{}
	for _, v := range d.Devices {
		m, err := net.ParseMAC(v.MAC)
		if err != nil || len(m) != 6 || m[0]&1 != 0 || m.String() == "00:00:00:00:00:00" || macs[m.String()] {
			return fail("device MAC addresses must be unique unicast Ethernet addresses")
		}
		macs[m.String()] = true
		if len(v.Name) > 128 {
			return fail("device name is too long")
		}
		if v.Address != "" {
			a, err := DHCPv4(v.Address)
			if err != nil || !configured || !dhcpHost(p, a) || a == server || ips[v.Address] {
				return fail("reserved addresses must be unique usable hosts in the subnet")
			}
			ips[v.Address] = true
		}
		o, _ := d.ForDevice(m.String())
		if err := validateDHCPOptions(o, p, first, last); err != nil {
			return fmt.Errorf("DHCP device %s: %w", m.String(), err)
		}
	}
	infra := map[string]bool{d.ServerIP: true}
	all := []DHCPOptions{d.Options}
	for _, v := range d.Devices {
		m, _ := net.ParseMAC(v.MAC)
		o, _ := d.ForDevice(m.String())
		all = append(all, o)
	}
	for _, o := range all {
		if o.Gateway != "" {
			infra[o.Gateway] = true
		}
		for _, r := range o.Routes {
			infra[r.Gateway] = true
		}
	}
	for _, v := range d.Devices {
		if v.Address != "" && infra[v.Address] {
			return fail("reserved address conflicts with a server or gateway")
		}
	}
	o := d.Options
	if o.LeaseSeconds == 0 {
		o.LeaseSeconds = 86400
	}
	return validateDHCPOptions(o, p, first, last)
}
func validateDHCPOptions(o DHCPOptions, p netip.Prefix, first, last netip.Addr) error {
	if o.LeaseSeconds < 120 || o.LeaseSeconds > 31536000 {
		return fmt.Errorf("DHCP lease must be between 120 and 31536000 seconds")
	}
	checkGateway := func(raw string) error {
		a, err := DHCPv4(raw)
		if err != nil {
			return err
		}
		if p.IsValid() && (!dhcpHost(p, a) || inDHCPPool(a, first, last)) {
			return fmt.Errorf("DHCP gateways must be in the subnet and outside the dynamic pool")
		}
		return nil
	}
	if o.Gateway != "" {
		if err := checkGateway(o.Gateway); err != nil {
			return err
		}
	}
	for _, list := range [][]string{o.DNS, o.NTP} {
		if len(list) > 16 {
			return fmt.Errorf("DHCP DNS/NTP lists support at most 16 servers")
		}
		seen := map[string]bool{}
		for _, s := range list {
			if _, err := DHCPv4(s); err != nil {
				return err
			}
			if seen[s] {
				return fmt.Errorf("duplicate DHCP server address")
			}
			seen[s] = true
		}
	}
	if len(o.Routes) > 16 {
		return fmt.Errorf("DHCP supports at most 16 custom routes")
	}
	seen := map[string]bool{}
	for _, r := range o.Routes {
		prefix, err := netip.ParsePrefix(r.Prefix)
		if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() || prefix.Addr().IsMulticast() || seen[r.Prefix] {
			return fmt.Errorf("DHCP routes require unique canonical IPv4 CIDRs")
		}
		seen[r.Prefix] = true
		if err := checkGateway(r.Gateway); err != nil {
			return err
		}
	}
	if seen["0.0.0.0/0"] {
		return fmt.Errorf("DHCP default route is configured with the gateway field")
	}
	if o.BootServer != "" {
		if _, err := DHCPv4(o.BootServer); err != nil {
			return err
		}
	}
	for _, s := range []string{o.BootFile, o.TFTP, o.Domain} {
		if len(s) > 253 || strings.ContainsAny(s, "\x00\r\n") {
			return fmt.Errorf("DHCP string option exceeds 253 bytes or contains control characters")
		}
	}
	if o.Domain != "" {
		for _, label := range strings.Split(o.Domain, ".") {
			if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return fmt.Errorf("invalid DHCP domain")
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
					return fmt.Errorf("invalid DHCP domain")
				}
			}
		}
	}
	return nil
}
func dhcpHost(p netip.Prefix, a netip.Addr) bool {
	if !p.Contains(a) || a == p.Addr() {
		return false
	}
	n := ipv4Number(a)
	mask := uint32(0xffffffff) << uint(32-p.Bits())
	return n|mask != 0xffffffff
}
func ipv4Number(a netip.Addr) uint32 {
	b := a.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
func inDHCPPool(a, first, last netip.Addr) bool {
	return first.IsValid() && a.Compare(first) >= 0 && a.Compare(last) <= 0
}
