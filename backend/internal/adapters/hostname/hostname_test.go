package hostname

import (
	"context"
	"encoding/binary"
	"net/netip"
	"testing"
)

type static struct{ name, src string }

func (s static) Resolve(context.Context, netip.Addr, string) (string, string) { return s.name, s.src }

func TestChainFirstNameWins(t *testing.T) {
	c := Chain{static{}, nil, static{"printer.", "dns"}, static{"other", "mdns"}}
	name, src := c.Resolve(context.Background(), netip.MustParseAddr("10.0.0.1"), "")
	if name != "printer" || src != "dns" {
		t.Fatalf("got %q %q", name, src)
	}
}

func TestReverseName(t *testing.T) {
	n, err := reverseName(netip.MustParseAddr("192.168.1.7"))
	if err != nil || n != "7.1.168.192.in-addr.arpa." {
		t.Fatalf("got %q %v", n, err)
	}
}

func TestParseNBSTAT(t *testing.T) {
	resp := make([]byte, 56)
	binary.BigEndian.PutUint16(resp[6:8], 1) // one answer
	resp = append(resp, 2)                   // two names
	group := append([]byte("WORKGROUP      "), 0x00, 0x84, 0x00)
	host := append([]byte("DESKTOP-ABC    "), 0x00, 0x04, 0x00)
	resp = append(resp, group...)
	resp = append(resp, host...)
	if got := parseNBSTAT(resp); got != "DESKTOP-ABC" {
		t.Fatalf("got %q", got)
	}
	if got := parseNBSTAT(resp[:20]); got != "" {
		t.Fatalf("short packet gave %q", got)
	}
}

func TestNBSTATRequestShape(t *testing.T) {
	if len(nbstatRequest) != 50 {
		t.Fatalf("request has %d bytes", len(nbstatRequest))
	}
	if string(nbstatRequest[13:15]) != "CK" { // '*' = 0x2a -> 'C','K'
		t.Fatalf("bad name encoding %q", nbstatRequest[13:15])
	}
}
