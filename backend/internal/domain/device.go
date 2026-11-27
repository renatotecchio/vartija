// Package domain holds Vartija's entities and their rules. It depends on
// nothing outside the standard library.
package domain

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"
)

// DeviceStatus says whether the owner already reviewed a device.
type DeviceStatus string

const (
	// StatusNew is a MAC that nobody reviewed yet. Every MAC starts here.
	StatusNew DeviceStatus = "new"
	// StatusKnown is a MAC the owner marked as known.
	StatusKnown DeviceStatus = "known"
)

// Valid reports whether s is one of the known statuses.
func (s DeviceStatus) Valid() bool {
	return s == StatusNew || s == StatusKnown
}

// Device is one MAC address seen on a monitored subnet.
type Device struct {
	ID  uint
	MAC string
	// Nickname is the name the owner gives the device; it wins over Hostname
	// everywhere the device is shown.
	Nickname string
	Notes    string
	// Hostname is the best name found on the network, and HostnameSource
	// where it came from (dhcp, dns, mdns, netbios).
	Hostname       string
	HostnameSource string
	// Vendor is the manufacturer from the MAC's IEEE prefix ("" for random
	// MACs and unknown prefixes).
	Vendor string
	// IP is the main address; ExtraIPs the others the MAC answered for in
	// the last scan.
	IP       string
	ExtraIPs []string
	SubnetID uint
	Status   DeviceStatus
	// RandomMAC is true for locally administered MACs, the private addresses
	// phones and laptops make up per network.
	RandomMAC      bool
	FirstSeen      time.Time
	LastSeen       time.Time
	AcknowledgedAt *time.Time
}

// DisplayName is what the UI and the alerts call the device.
func (d Device) DisplayName() string {
	switch {
	case d.Nickname != "":
		return d.Nickname
	case d.Hostname != "":
		return d.Hostname
	default:
		return d.MAC
	}
}

// Acknowledge marks the device as known.
func (d *Device) Acknowledge(now time.Time) {
	d.Status = StatusKnown
	d.AcknowledgedAt = &now
}

// Reopen puts the device back in the review list.
func (d *Device) Reopen() {
	d.Status = StatusNew
	d.AcknowledgedAt = nil
}

// PickIP keeps current when the device still answers on it, so a host with
// several addresses doesn't "change IP" on every scan. Otherwise it takes
// the lowest. It returns the main IP and the others.
func PickIP(current string, ips []string) (string, []string) {
	if len(ips) == 0 {
		return current, nil
	}
	main := ips[0]
	for _, ip := range ips {
		if ip == current {
			main = current
			break
		}
	}
	extra := make([]string, 0, len(ips)-1)
	for _, ip := range ips {
		if ip != main {
			extra = append(extra, ip)
		}
	}
	return main, extra
}

// SortIPs orders addresses numerically (192.168.1.9 before 192.168.1.10)
// and drops duplicates and invalid ones.
func SortIPs(ips []string) []string {
	addrs := make([]netip.Addr, 0, len(ips))
	seen := map[netip.Addr]bool{}
	for _, s := range ips {
		a, err := netip.ParseAddr(s)
		if err != nil || seen[a] {
			continue
		}
		seen[a] = true
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Less(addrs[j]) })
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.String()
	}
	return out
}

// ErrInvalidMAC is returned for anything that is not a 48-bit MAC.
var ErrInvalidMAC = errors.New("invalid MAC address")

// NormalizeMAC returns the MAC in lower case, colon separated
// (aa:bb:cc:dd:ee:ff), whatever separator it came with.
func NormalizeMAC(s string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("%w: %q", ErrInvalidMAC, s)
	}
	return hw.String(), nil
}

// IsRandomMAC reports whether the MAC has the locally administered bit set.
// mac must already be normalized.
func IsRandomMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) == 0 {
		return false
	}
	return hw[0]&0x02 != 0
}
