package arp

import (
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
)

func TestRequestAndReply(t *testing.T) {
	src := net.HardwareAddr{0x02, 0, 0, 0, 0, 1}
	req := request(src, netip.MustParseAddr("192.168.1.2"), netip.MustParseAddr("192.168.1.7"))
	if len(req) != frameLen {
		t.Fatalf("frame has %d bytes", len(req))
	}
	if net.HardwareAddr(req[0:6]).String() != "ff:ff:ff:ff:ff:ff" {
		t.Error("request must be broadcast")
	}
	if _, _, ok := parseReply(req); ok {
		t.Error("a request is not a reply")
	}

	// Turn it into the reply 192.168.1.7 would send
	reply := make([]byte, len(req))
	copy(reply, req)
	a := reply[14:]
	binary.BigEndian.PutUint16(a[6:8], opReply)
	copy(a[8:14], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	copy(a[14:18], []byte{192, 168, 1, 7})
	ip, mac, ok := parseReply(reply)
	if !ok || ip.String() != "192.168.1.7" || mac.String() != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %v %v %v", ip, mac, ok)
	}
	if _, _, ok := parseReply(reply[:20]); ok {
		t.Error("short frame accepted")
	}
}
