// Package hostname finds a name for an IP address, trying several sources
// in order: the router's DHCP leases (another package), reverse DNS, a
// unicast mDNS query (Apple devices, Linux with Avahi, printers, ESPs with
// mDNS) and a NetBIOS node status query (Windows, Samba).
package hostname

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// Chain asks each resolver in turn and returns the first name.
type Chain []ports.HostnameResolver

// Resolve implements ports.HostnameResolver.
func (c Chain) Resolve(ctx context.Context, ip netip.Addr, mac string) (string, string) {
	for _, r := range c {
		if r == nil {
			continue
		}
		if name, src := r.Resolve(ctx, ip, mac); name != "" {
			return clean(name), src
		}
	}
	return "", ""
}

func clean(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

// DNS does reverse lookups (PTR), on the system resolver or on Server.
type DNS struct {
	Server  string // host:port; empty = the system resolver
	Timeout time.Duration
	// Suffixes stripped from the answer ("lan.example" makes
	// "printer.lan.example" show as "printer").
	Suffixes []string
}

// Resolve implements ports.HostnameResolver.
func (d DNS) Resolve(ctx context.Context, ip netip.Addr, _ string) (string, string) {
	r := net.DefaultResolver
	if d.Server != "" {
		r = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, d.Server)
			},
		}
	}
	ctx, cancel := context.WithTimeout(ctx, orDefault(d.Timeout, 2*time.Second))
	defer cancel()
	names, err := r.LookupAddr(ctx, ip.String())
	if err != nil || len(names) == 0 {
		return "", ""
	}
	name := strings.TrimSuffix(names[0], ".")
	for _, s := range d.Suffixes {
		s = "." + strings.Trim(s, ".")
		if strings.HasSuffix(name, s) && len(name) > len(s) {
			name = strings.TrimSuffix(name, s)
			break
		}
	}
	return name, "dns"
}

// MDNS sends a unicast PTR query to the device's port 5353.
type MDNS struct{ Timeout time.Duration }

// Resolve implements ports.HostnameResolver.
func (m MDNS) Resolve(ctx context.Context, ip netip.Addr, _ string) (string, string) {
	arpa, err := reverseName(ip)
	if err != nil {
		return "", ""
	}
	name, err := dnsmessage.NewName(arpa)
	if err != nil {
		return "", ""
	}
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 0},
		Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}},
	}
	req, err := msg.Pack()
	if err != nil {
		return "", ""
	}
	resp, err := udpExchange(ctx, netip.AddrPortFrom(ip, 5353), req, orDefault(m.Timeout, time.Second))
	if err != nil {
		return "", ""
	}
	var p dnsmessage.Parser
	if _, err := p.Start(resp); err != nil {
		return "", ""
	}
	_ = p.SkipAllQuestions()
	for {
		h, err := p.AnswerHeader()
		if err != nil {
			return "", ""
		}
		if h.Type != dnsmessage.TypePTR {
			_ = p.SkipAnswer()
			continue
		}
		ptr, err := p.PTRResource()
		if err != nil {
			return "", ""
		}
		host := strings.TrimSuffix(strings.TrimSuffix(ptr.PTR.String(), "."), ".local")
		if host != "" {
			return host, "mdns"
		}
	}
}

func reverseName(ip netip.Addr) (string, error) {
	if !ip.Is4() {
		return "", errors.New("IPv4 only")
	}
	b := ip.As4()
	return net.IPv4(b[3], b[2], b[1], b[0]).String() + ".in-addr.arpa.", nil
}

// NetBIOS sends a node status request (NBSTAT) to UDP 137 and returns the
// machine's workstation name.
type NetBIOS struct{ Timeout time.Duration }

// nbstatRequest is a node status query for the wildcard name "*".
var nbstatRequest = func() []byte {
	b := []byte{
		0x56, 0x4a, // transaction id
		0x00, 0x00, // flags: query
		0x00, 0x01, // 1 question
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x20, // name length 32
	}
	// "*" padded with NULs to 16 bytes, first-level encoded
	name := make([]byte, 16)
	name[0] = '*'
	for _, c := range name {
		b = append(b, 'A'+(c>>4), 'A'+(c&0x0f))
	}
	b = append(b, 0x00, 0x00, 0x21, 0x00, 0x01) // end, NBSTAT, IN
	return b
}()

// Resolve implements ports.HostnameResolver.
func (n NetBIOS) Resolve(ctx context.Context, ip netip.Addr, _ string) (string, string) {
	resp, err := udpExchange(ctx, netip.AddrPortFrom(ip, 137), nbstatRequest, orDefault(n.Timeout, time.Second))
	if err != nil {
		return "", ""
	}
	if name := parseNBSTAT(resp); name != "" {
		return name, "netbios"
	}
	return "", ""
}

// parseNBSTAT returns the first unique workstation (suffix 0x00) name.
func parseNBSTAT(b []byte) string {
	// header 12 + name 34 + type/class/ttl 8 + rdlength 2 = 56
	const off = 56
	if len(b) < off+1 || binary.BigEndian.Uint16(b[6:8]) == 0 {
		return ""
	}
	count := int(b[off])
	p := off + 1
	for i := 0; i < count && p+18 <= len(b); i++ {
		raw := b[p : p+15]
		suffix := b[p+15]
		flags := binary.BigEndian.Uint16(b[p+16 : p+18])
		p += 18
		group := flags&0x8000 != 0
		if suffix == 0x00 && !group {
			return strings.TrimRight(string(raw), " \x00")
		}
	}
	return ""
}

func udpExchange(ctx context.Context, to netip.AddrPort, req []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp4", to.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func orDefault(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}
