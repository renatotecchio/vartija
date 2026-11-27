// Package arp sweeps a directly attached IPv4 subnet with ARP requests and
// collects the replies. It needs CAP_NET_RAW and, in a container, the host
// network (an ARP request never leaves the host's own L2 segments).
package arp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/domain"
)

// Scanner is the ARP discoverer.
type Scanner struct {
	// Wait is how long to keep listening after the last request.
	Wait time.Duration
	// Rounds is how many requests each address gets (a sleeping phone may
	// miss the first).
	Rounds int
	// Gap paces the requests so a /24 doesn't burst 254 frames at once.
	Gap time.Duration
	// Interfaces restricts the sweep to these interface names (empty = any).
	Interfaces []string
}

// New returns a scanner with sensible defaults.
func New(interfaces []string) *Scanner {
	return &Scanner{Wait: 2 * time.Second, Rounds: 2, Gap: 2 * time.Millisecond, Interfaces: interfaces}
}

// Name implements ports.Discoverer.
func (s *Scanner) Name() string { return "arp" }

// attachment is the local interface that sits on the subnet.
type attachment struct {
	iface *net.Interface
	ip    netip.Addr
}

// Covers is true when an up interface has an address inside the subnet and
// its own network contains the whole subnet.
func (s *Scanner) Covers(_ context.Context, p netip.Prefix) bool {
	_, err := s.attach(p)
	return err == nil
}

func (s *Scanner) allowed(name string) bool {
	if len(s.Interfaces) == 0 {
		return true
	}
	for _, n := range s.Interfaces {
		if n == name {
			return true
		}
	}
	return false
}

var errNotAttached = errors.New("no local interface on this subnet")

func (s *Scanner) attach(p netip.Prefix) (attachment, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return attachment{}, err
	}
	for i := range ifaces {
		ifc := &ifaces[i]
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || len(ifc.HardwareAddr) != 6 || !s.allowed(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP.To4())
			if !ok {
				continue
			}
			ones, _ := ipn.Mask.Size()
			local := netip.PrefixFrom(ip, ones).Masked()
			if local.Contains(p.Addr()) && local.Bits() <= p.Bits() {
				return attachment{iface: ifc, ip: ip}, nil
			}
		}
	}
	return attachment{}, errNotAttached
}

// Discover sends ARP requests to every host of the subnet and returns the
// MACs that answered. The host's own address is included.
func (s *Scanner) Discover(ctx context.Context, p netip.Prefix) ([]domain.Sighting, error) {
	at, err := s.attach(p)
	if err != nil {
		return nil, err
	}
	replies, err := sweep(ctx, at, domain.Hosts(p), s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", at.iface.Name, err)
	}
	out := make([]domain.Sighting, 0, len(replies)+1)
	for ip, mac := range replies {
		if p.Contains(ip) {
			out = append(out, domain.Sighting{MAC: mac.String(), IP: ip.String()})
		}
	}
	out = append(out, domain.Sighting{MAC: at.iface.HardwareAddr.String(), IP: at.ip.String()})
	return out, nil
}
