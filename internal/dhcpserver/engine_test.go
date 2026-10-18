package dhcpserver

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"surge-controller/internal/config"
)

func testConfig() config.DHCP {
	return config.DHCP{Enabled: true, Interface: "test0", ServerIP: "192.0.2.10", Subnet: "192.0.2.0/24", PoolStart: "192.0.2.100", PoolEnd: "192.0.2.102", Options: config.DHCPOptions{Gateway: "192.0.2.1", DNS: []string{"1.1.1.1"}, LeaseSeconds: 3600}}
}
func clientPacket(kind byte, mac string, options map[byte][]byte) []byte {
	b := make([]byte, 240)
	b[0] = 1
	b[1] = 1
	b[2] = 6
	b[7] = 42
	m, _ := net.ParseMAC(mac)
	copy(b[28:34], m)
	copy(b[236:240], []byte{99, 130, 83, 99})
	b = putOption(b, 53, []byte{kind})
	for k, v := range options {
		b = putOption(b, k, v)
	}
	return append(b, 255)
}
func responseOptions(t *testing.T, b []byte) map[byte][]byte {
	t.Helper()
	if len(b) < 240 {
		t.Fatalf("missing reply: %x", b)
	}
	raw := append([]byte(nil), b...)
	raw[0] = 1
	p, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return p.options
}

const mac1 = "02:00:00:00:00:01"
const mac2 = "02:00:00:00:00:02"

func TestDHCPLeaseLifecycleAndRestart(t *testing.T) {
	d := testConfig()
	var disk State
	writes := 0
	e, err := New(d, State{}, func(s State) error { disk = s; writes++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	offer, dest, err := e.Handle(clientPacket(1, mac1, map[byte][]byte{61: {1, 255, 128}}))
	if err != nil || dest.Port != 68 || !dest.IP.Equal(net.IPv4bcast) {
		t.Fatalf("offer: %v %v", dest, err)
	}
	if writes != 0 || len(e.Snapshot().Leases) != 0 {
		t.Fatal("DISCOVER committed a lease")
	}
	ip := net.IP(offer[16:20]).String()
	if ip != "192.0.2.100" {
		t.Fatal(ip)
	}
	ack, _, err := e.Handle(clientPacket(3, mac1, map[byte][]byte{50: ipBytes(ip), 54: ipBytes(d.ServerIP), 61: {1, 255, 128}}))
	if err != nil {
		t.Fatal(err)
	}
	if responseOptions(t, ack)[53][0] != 5 || writes != 1 || len(disk.Leases) != 1 {
		t.Fatal("ACK was not committed")
	}
	// Client-ID bytes survive JSON persistence as hex, without UTF-8 replacement.
	if disk.Leases[0].ClientID != "id:01ff80" {
		t.Fatal(disk.Leases)
	}
	restarted, err := New(d, disk, func(s State) error { disk = s; return nil })
	if err != nil {
		t.Fatal(err)
	}
	renew := clientPacket(3, mac1, map[byte][]byte{61: {1, 255, 128}})
	copy(renew[12:16], ipBytes(ip))
	ack, dest, err = restarted.Handle(renew)
	if err != nil || !dest.IP.Equal(net.ParseIP(ip)) || responseOptions(t, ack)[53][0] != 5 {
		t.Fatalf("renew %v %v", dest, err)
	}
	release := clientPacket(7, mac1, map[byte][]byte{54: ipBytes(d.ServerIP), 61: {1, 255, 128}})
	copy(release[12:16], ipBytes(ip))
	if _, _, err = restarted.Handle(release); err != nil || len(disk.Leases) != 0 {
		t.Fatalf("release %v %+v", err, disk)
	}
}
func TestDHCPDeviceOptionsAndClasslessDefaultRoute(t *testing.T) {
	d := testConfig()
	gateway := "192.0.2.2"
	dns := []string{"9.9.9.9"}
	routes := []config.DHCPRoute{{Prefix: "91.104.0.0/13", Gateway: "192.0.2.2"}}
	ntp := []string{"192.0.2.3"}
	boot := "https://boot.example/start.ipxe"
	server := "192.0.2.4"
	tftp := "tftp.example"
	domain := "lan.example"
	duration := 7200
	d.Devices = []config.DHCPDevice{{MAC: mac1, Address: "192.0.2.120", Options: config.DHCPOverrides{Gateway: &gateway, DNS: &dns, Routes: &routes, NTP: &ntp, BootFile: &boot, BootServer: &server, TFTP: &tftp, Domain: &domain, LeaseSeconds: &duration}}}
	e, err := New(d, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := e.Handle(clientPacket(1, mac1, nil))
	if err != nil {
		t.Fatal(err)
	}
	o := responseOptions(t, b)
	if net.IP(b[16:20]).String() != "192.0.2.120" || net.IP(b[20:24]).String() != server {
		t.Fatal("reservation/boot server missing")
	}
	for code, want := range map[byte][]byte{3: ipBytes(gateway), 6: addresses(dns), 42: addresses(ntp), 66: []byte(tftp), 67: []byte(boot), 15: []byte(domain), 51: uintBytes(duration), 121: {13, 91, 104, 192, 0, 2, 2, 0, 192, 0, 2, 2}} {
		if !bytes.Equal(o[code], want) {
			t.Errorf("option %d got %v want %v", code, o[code], want)
		}
	}
	b, _, _ = e.Handle(clientPacket(1, mac2, nil))
	o = responseOptions(t, b)
	if !bytes.Equal(o[3], ipBytes("192.0.2.1")) || len(o[121]) != 0 {
		t.Fatal("device options leaked to another client")
	}
	empty := []string{}
	d.Devices[0].Options.DNS = &empty
	d.Devices[0].Options.Routes = nil
	d.Options.Routes = routes
	e, _ = New(d, State{}, nil)
	b, _, _ = e.Handle(clientPacket(1, mac1, nil))
	o = responseOptions(t, b)
	if len(o[6]) != 0 || len(o[121]) == 0 {
		t.Fatal("empty override/inherited routes lost")
	}
}
func TestDHCPPersistenceFailureNeverAcknowledges(t *testing.T) {
	e, _ := New(testConfig(), State{}, func(State) error { return errors.New("disk unavailable") })
	b, _, err := e.Handle(clientPacket(3, mac1, map[byte][]byte{50: ipBytes("192.0.2.100"), 54: ipBytes("192.0.2.10")}))
	if err == nil || b != nil || len(e.Snapshot().Leases) != 0 {
		t.Fatalf("unsafe ACK %x %v", b, err)
	}
}
func TestDHCPOwnershipDeclineAndExhaustion(t *testing.T) {
	d := testConfig()
	d.PoolEnd = d.PoolStart
	e, _ := New(d, State{}, nil)
	offer, _, _ := e.Handle(clientPacket(1, mac1, nil))
	ip := net.IP(offer[16:20]).String()
	if b, _, _ := e.Handle(clientPacket(1, mac2, nil)); b != nil {
		t.Fatal("offered one address twice")
	}
	if b, _, _ := e.Handle(clientPacket(3, mac1, map[byte][]byte{50: ipBytes(ip), 54: ipBytes("192.0.2.9")})); b != nil {
		t.Fatal("answered another server's selection")
	}
	e.Handle(clientPacket(1, mac1, nil))
	e.Handle(clientPacket(4, mac1, map[byte][]byte{50: ipBytes(ip), 54: ipBytes(d.ServerIP)}))
	if len(e.Snapshot().Leases) != 1 || e.Snapshot().Leases[0].State != "declined" {
		t.Fatal("decline not quarantined")
	}
	if b, _, _ := e.Handle(clientPacket(1, mac2, nil)); b != nil {
		t.Fatal("reused declined address")
	}
	e.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if b, _, _ := e.Handle(clientPacket(1, mac2, nil)); len(b) == 0 {
		t.Fatal("quarantine did not expire")
	}
}
func TestDHCPInformAndMalformedPackets(t *testing.T) {
	d := testConfig()
	e, _ := New(d, State{}, nil)
	b := clientPacket(8, mac1, nil)
	copy(b[12:16], ipBytes("192.0.2.111"))
	r, _, err := e.Handle(b)
	if err != nil || len(responseOptions(t, r)[51]) != 0 || len(e.Snapshot().Leases) != 0 {
		t.Fatal("INFORM allocated a lease")
	}
	for _, bad := range [][]byte{nil, b[:100], append(b[:len(b)-1], 6, 5, 1), func() []byte { c := append([]byte(nil), b...); c[24] = 10; return c }()} {
		if r, _, err = e.Handle(bad); r != nil || err != nil {
			t.Fatal("malformed/relay request was answered")
		}
	}
}
func TestProbeReplyRequiresMatchingTransaction(t *testing.T) {
	d := testConfig()
	request := clientPacket(1, mac1, nil)
	p, _ := decode(request)
	b := reply(p, 5, "", d, d.Options)
	xid := binary.BigEndian.Uint32(b[4:8])
	mac := b[28:34]
	if probeReply(b, xid, mac) != d.ServerIP || probeReply(b, xid+1, mac) != "" || probeReply(b[:200], xid, mac) != "" {
		t.Fatal("probe matched wrong server transaction")
	}
}

func TestDHCPReplySizeFailureDoesNotCommitLease(t *testing.T) {
	d := testConfig()
	d.Options.BootFile = strings.Repeat("x", 250)
	d.Options.TFTP = strings.Repeat("y", 250)
	writes := 0
	e, err := New(d, State{}, func(State) error { writes++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	b, dest, err := e.Handle(clientPacket(3, mac1, map[byte][]byte{50: ipBytes(d.PoolStart), 54: ipBytes(d.ServerIP)}))
	if err == nil || b != nil || dest != nil || writes != 0 || len(e.Snapshot().Leases) != 0 {
		t.Fatalf("oversized ACK committed a lease: writes=%d err=%v", writes, err)
	}
	// A client advertising enough receive space can receive all configured options.
	b, _, err = e.Handle(clientPacket(3, mac1, map[byte][]byte{50: ipBytes(d.PoolStart), 54: ipBytes(d.ServerIP), 57: {4, 0}}))
	if err != nil || writes != 1 || len(responseOptions(t, b)[67]) != 250 {
		t.Fatalf("advertised maximum size ignored: writes=%d err=%v", writes, err)
	}
}

func TestDHCPReservationsAndExpiredOffersRemainIsolated(t *testing.T) {
	d := testConfig()
	d.Devices = []config.DHCPDevice{{MAC: mac1, Address: d.PoolStart}}
	e, err := New(d, State{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := e.Handle(clientPacket(1, mac2, nil))
	if net.IP(b[16:20]).String() != "192.0.2.101" {
		t.Fatal("reservation was offered to another device")
	}
	// Selecting a different DHCP server releases the temporary hold immediately.
	e.Handle(clientPacket(3, mac2, map[byte][]byte{50: ipBytes("192.0.2.101"), 54: ipBytes("192.0.2.9")}))
	if len(e.held) != 0 || len(e.offers) != 0 {
		t.Fatal("foreign selection left an address held")
	}
	e.Handle(clientPacket(1, mac2, nil))
	e.now = func() time.Time { return time.Now().Add(61 * time.Second) }
	b, _, _ = e.Handle(clientPacket(1, "02:00:00:00:00:03", nil))
	if net.IP(b[16:20]).String() != "192.0.2.101" || len(e.held) != 1 || len(e.offers) != 1 {
		t.Fatal("expired offer was not released consistently")
	}
}
