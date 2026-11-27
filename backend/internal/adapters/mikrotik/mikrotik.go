// Package mikrotik reads the ARP table and the DHCP leases of MikroTik
// routers through the RouterOS API (api-ssl, a read-only user is enough).
//
// It plays two roles: a discoverer for subnets the Vartija host has no
// interface on (the router's ARP table lists who talked recently), and a
// hostname source (the host-name each client sent in its DHCP request).
package mikrotik

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/go-routeros/routeros/v3"

	"github.com/renatotecchio/vartija/backend/internal/domain"
)

// Router is one RouterOS device.
type Router struct {
	Name    string
	Address string // host:port of api-ssl, usually :8729
}

// Client queries every router and caches the result for a short while, so
// one scan cycle (several subnets, then the hostname lookups) reads each
// router once.
type Client struct {
	routers  []Router
	user     string
	password string
	ttl      time.Duration
	log      *slog.Logger

	mu       sync.Mutex
	snapshot *snapshot
}

type arpEntry struct {
	ip  netip.Addr
	mac string
}

type lease struct {
	ip       netip.Addr
	mac      string
	hostname string
	comment  string
	bound    bool
}

type snapshot struct {
	at       time.Time
	networks []netip.Prefix
	arp      []arpEntry
	leases   []lease
}

// New returns a client for the routers.
func New(routers []Router, user, password string, log *slog.Logger) *Client {
	return &Client{routers: routers, user: user, password: password, ttl: 60 * time.Second, log: log}
}

// Name implements ports.Discoverer.
func (c *Client) Name() string { return "mikrotik" }

func (c *Client) load(ctx context.Context) *snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot != nil && time.Since(c.snapshot.at) < c.ttl {
		return c.snapshot
	}
	s := &snapshot{at: time.Now()}
	for _, r := range c.routers {
		if err := c.read(ctx, r, s); err != nil {
			c.log.Warn("mikrotik read failed", "router", r.Name, "error", err)
		}
	}
	c.snapshot = s
	return s
}

func (c *Client) read(ctx context.Context, r Router, s *snapshot) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// The routers use a self-signed certificate: encrypted, not verified
	// (RouterOS ships one by default)
	conn, err := routeros.DialTLSContext(ctx, r.Address, c.user, c.password, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
	if err != nil {
		return err
	}
	defer conn.Close()

	addrs, err := conn.RunContext(ctx, "/ip/address/print", "=.proplist=address,disabled")
	if err != nil {
		return fmt.Errorf("/ip/address: %w", err)
	}
	for _, re := range addrs.Re {
		if re.Map["disabled"] == "true" {
			continue
		}
		if p, err := netip.ParsePrefix(re.Map["address"]); err == nil {
			s.networks = append(s.networks, p.Masked())
		}
	}

	arp, err := conn.RunContext(ctx, "/ip/arp/print", "=.proplist=address,mac-address,status,complete,invalid")
	if err != nil {
		return fmt.Errorf("/ip/arp: %w", err)
	}
	for _, re := range arp.Re {
		m := re.Map
		if m["mac-address"] == "" || m["invalid"] == "true" || m["complete"] == "false" ||
			m["status"] == "failed" || m["status"] == "incomplete" {
			continue
		}
		ip, err := netip.ParseAddr(m["address"])
		if err != nil {
			continue
		}
		s.arp = append(s.arp, arpEntry{ip: ip, mac: m["mac-address"]})
	}

	leases, err := conn.RunContext(ctx, "/ip/dhcp-server/lease/print", "=.proplist=address,active-address,mac-address,active-mac-address,host-name,comment,status")
	if err != nil {
		// A router without a DHCP server still gives its ARP table
		c.log.Debug("mikrotik leases", "router", r.Name, "error", err)
		return nil
	}
	for _, re := range leases.Re {
		m := re.Map
		mac := firstNonEmpty(m["active-mac-address"], m["mac-address"])
		ip, _ := netip.ParseAddr(firstNonEmpty(m["active-address"], m["address"]))
		if mac == "" {
			continue
		}
		s.leases = append(s.leases, lease{
			ip: ip, mac: mac, hostname: m["host-name"], comment: m["comment"], bound: m["status"] == "bound",
		})
	}
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Covers is true when a router has an address on the subnet.
func (c *Client) Covers(ctx context.Context, p netip.Prefix) bool {
	for _, n := range c.load(ctx).networks {
		if n.Contains(p.Addr()) && n.Bits() <= p.Bits() {
			return true
		}
	}
	return false
}

// Discover returns the router ARP entries and bound DHCP leases on the
// subnet. Unlike an ARP sweep, it only lists devices that talked recently.
func (c *Client) Discover(ctx context.Context, p netip.Prefix) ([]domain.Sighting, error) {
	s := c.load(ctx)
	byMAC := map[string]domain.Sighting{}
	for _, e := range s.arp {
		if p.Contains(e.ip) {
			byMAC[strings.ToLower(e.mac)] = domain.Sighting{MAC: e.mac, IP: e.ip.String()}
		}
	}
	for _, l := range s.leases {
		if !l.bound || !l.ip.IsValid() || !p.Contains(l.ip) {
			continue
		}
		sg := byMAC[strings.ToLower(l.mac)]
		sg.MAC, sg.IP = l.mac, l.ip.String()
		if name := leaseName(l); name != "" {
			sg.Hostname, sg.HostnameSource = name, "dhcp"
		}
		byMAC[strings.ToLower(l.mac)] = sg
	}
	out := make([]domain.Sighting, 0, len(byMAC))
	for _, sg := range byMAC {
		out = append(out, sg)
	}
	return out, nil
}

func leaseName(l lease) string {
	return firstNonEmpty(strings.TrimSpace(l.hostname), strings.TrimSpace(l.comment))
}

// Resolve implements ports.HostnameResolver: the host-name of the MAC's
// DHCP lease, or the lease's comment.
func (c *Client) Resolve(ctx context.Context, _ netip.Addr, mac string) (string, string) {
	for _, l := range c.load(ctx).leases {
		if strings.EqualFold(l.mac, mac) {
			if name := leaseName(l); name != "" {
				return name, "dhcp"
			}
		}
	}
	return "", ""
}
