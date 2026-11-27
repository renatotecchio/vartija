package arp

import (
	"encoding/binary"
	"net"
	"net/netip"
)

const (
	etherTypeARP = 0x0806
	frameLen     = 42 // 14 Ethernet + 28 ARP (no padding: the NIC adds it)
	opRequest    = 1
	opReply      = 2
)

var broadcast = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// request builds a broadcast "who has target, tell srcIP" frame.
func request(srcMAC net.HardwareAddr, srcIP, target netip.Addr) []byte {
	b := make([]byte, frameLen)
	copy(b[0:6], broadcast)
	copy(b[6:12], srcMAC)
	binary.BigEndian.PutUint16(b[12:14], etherTypeARP)
	a := b[14:]
	binary.BigEndian.PutUint16(a[0:2], 1)      // Ethernet
	binary.BigEndian.PutUint16(a[2:4], 0x0800) // IPv4
	a[4], a[5] = 6, 4
	binary.BigEndian.PutUint16(a[6:8], opRequest)
	copy(a[8:14], srcMAC)
	s4 := srcIP.As4()
	copy(a[14:18], s4[:])
	// target MAC (18:24) stays zero
	t4 := target.As4()
	copy(a[24:28], t4[:])
	return b
}

// parseReply returns the sender of an ARP reply frame.
func parseReply(b []byte) (netip.Addr, net.HardwareAddr, bool) {
	if len(b) < frameLen || binary.BigEndian.Uint16(b[12:14]) != etherTypeARP {
		return netip.Addr{}, nil, false
	}
	a := b[14:]
	if binary.BigEndian.Uint16(a[0:2]) != 1 || binary.BigEndian.Uint16(a[2:4]) != 0x0800 ||
		a[4] != 6 || a[5] != 4 || binary.BigEndian.Uint16(a[6:8]) != opReply {
		return netip.Addr{}, nil, false
	}
	mac := make(net.HardwareAddr, 6)
	copy(mac, a[8:14])
	ip := netip.AddrFrom4([4]byte(a[14:18]))
	return ip, mac, true
}
