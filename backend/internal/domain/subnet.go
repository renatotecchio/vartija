package domain

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// MinPrefixBits limits a subnet to /16 (65534 hosts): an ARP sweep of
// anything bigger takes too long and floods the network.
const MinPrefixBits = 16

// Subnet is one IPv4 network Vartija sweeps for MACs.
type Subnet struct {
	ID      uint
	CIDR    string
	Name    string
	Enabled bool
	// Baselined is set after the first successful scan. Devices found on that
	// scan don't raise one alert each, only a summary.
	Baselined     bool
	LastScanAt    *time.Time
	LastScanError string
	LastScanFound int
	CreatedAt     time.Time
}

// ErrInvalidSubnet is returned for CIDRs Vartija can't sweep.
var ErrInvalidSubnet = errors.New("invalid subnet")

// NormalizeCIDR parses an IPv4 CIDR and returns it masked to its network
// (192.168.1.7/24 becomes 192.168.1.0/24).
func NormalizeCIDR(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: %v", ErrInvalidSubnet, err)
	}
	if !p.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%w: only IPv4 is supported", ErrInvalidSubnet)
	}
	if p.Bits() < MinPrefixBits {
		return netip.Prefix{}, fmt.Errorf("%w: the largest subnet is /%d", ErrInvalidSubnet, MinPrefixBits)
	}
	if p.Bits() > 30 {
		return netip.Prefix{}, fmt.Errorf("%w: the smallest subnet is /30", ErrInvalidSubnet)
	}
	return p.Masked(), nil
}

// Hosts returns the subnet's usable addresses (no network or broadcast).
func Hosts(p netip.Prefix) []netip.Addr {
	p = p.Masked()
	var out []netip.Addr
	first := p.Addr().Next()
	for a := first; p.Contains(a); a = a.Next() {
		if !p.Contains(a.Next()) { // broadcast
			break
		}
		out = append(out, a)
	}
	return out
}
