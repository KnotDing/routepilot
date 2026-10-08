//go:build linux

// Diagnostic only: inject one UDP query and one TCP SYN with the Controller's
// own source address from the isolated Docker test network. Never run on LAN.
package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/sys/unix"
)

func checksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) != 0 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}

func run() error {
	iface, err := net.InterfaceByName("eth0")
	if err != nil {
		return err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return err
	}
	isolated := false
	for _, a := range addrs {
		if a.String() == "172.30.31.130/24" {
			isolated = true
		}
	}
	if !isolated {
		return fmt.Errorf("requires the dedicated 172.30.31.130 Docker test container")
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_RAW)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1); err != nil {
		return err
	}
	query := new(dns.Msg)
	query.SetQuestion("routepilot-loop-test.invalid.", dns.TypeA)
	query.Id = 62001
	wire, err := query.Pack()
	if err != nil {
		return err
	}
	udp := make([]byte, 8+len(wire))
	binary.BigEndian.PutUint16(udp[0:2], 20050)
	binary.BigEndian.PutUint16(udp[2:4], 53)
	binary.BigEndian.PutUint16(udp[4:6], uint16(len(udp)))
	copy(udp[8:], wire)
	tcp := make([]byte, 20)
	binary.BigEndian.PutUint16(tcp[0:2], 20051)
	binary.BigEndian.PutUint16(tcp[2:4], 53)
	tcp[12], tcp[13] = 0x50, 2
	binary.BigEndian.PutUint16(tcp[14:16], 1024)
	src, dst := []byte{192, 168, 20, 2}, []byte{8, 8, 8, 8}
	pseudo := append(append([]byte{}, src...), dst...)
	pseudo = append(pseudo, 0, unix.IPPROTO_TCP, 0, 20)
	binary.BigEndian.PutUint16(tcp[16:18], checksum(append(pseudo, tcp...)))
	for i, body := range [][]byte{udp, tcp} {
		packet := make([]byte, 20+len(body))
		packet[0], packet[8] = 0x45, 32
		packet[9] = unix.IPPROTO_UDP
		if i == 1 {
			packet[9] = unix.IPPROTO_TCP
		}
		binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
		binary.BigEndian.PutUint16(packet[4:6], uint16(62001+i))
		copy(packet[12:16], src)
		copy(packet[16:20], dst)
		binary.BigEndian.PutUint16(packet[10:12], checksum(packet[:20]))
		copy(packet[20:], body)
		if err = unix.Sendto(fd, packet, 0, &unix.SockaddrInet4{Addr: [4]byte{8, 8, 8, 8}}); err != nil {
			return err
		}
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Injected exactly one self-origin UDP query and one TCP SYN; verify PF drop counters and zero Controller/upstream exchanges.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
