// Package domain holds Vartija's entities and their rules. It depends on
// nothing outside the standard library.
package domain

import (
	"errors"
	"fmt"
	"net"
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
	IP             string
	SubnetID       uint
	Status         DeviceStatus
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
