//go:build !linux

package arp

import (
	"context"
	"errors"
	"net"
	"net/netip"
)

// sweep is only implemented on Linux (AF_PACKET).
func sweep(context.Context, attachment, []netip.Addr, *Scanner) (map[netip.Addr]net.HardwareAddr, error) {
	return nil, errors.New("ARP sweep is only supported on Linux")
}
