// Package ports declares what the application core needs from the outside
// world. The adapters implement these interfaces; the core never imports an
// adapter.
package ports

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/domain"
)

// ErrNotFound is returned by repositories when the record doesn't exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a unique key (a subnet's CIDR) already exists.
var ErrConflict = errors.New("already exists")

// DeviceFilter narrows a device listing.
type DeviceFilter struct {
	Status   domain.DeviceStatus // empty = any
	SubnetID uint                // 0 = any
	Search   string              // MAC, IP, hostname, nickname, notes or vendor, case insensitive
}

// DeviceRepository stores devices.
type DeviceRepository interface {
	List(ctx context.Context, f DeviceFilter) ([]domain.Device, error)
	Get(ctx context.Context, id uint) (domain.Device, error)
	GetByMAC(ctx context.Context, mac string) (domain.Device, error)
	Create(ctx context.Context, d *domain.Device) error
	Save(ctx context.Context, d *domain.Device) error
	Delete(ctx context.Context, id uint) error
	CountByStatus(ctx context.Context) (map[domain.DeviceStatus]int64, error)
}

// SubnetRepository stores subnets.
type SubnetRepository interface {
	List(ctx context.Context) ([]domain.Subnet, error)
	Get(ctx context.Context, id uint) (domain.Subnet, error)
	Create(ctx context.Context, s *domain.Subnet) error
	Save(ctx context.Context, s *domain.Subnet) error
	Delete(ctx context.Context, id uint) error
}

// EventRepository stores the history.
type EventRepository interface {
	Add(ctx context.Context, e *domain.Event) error
	List(ctx context.Context, limit int) ([]domain.Event, error)
	// Prune deletes events older than the cutoff.
	Prune(ctx context.Context, before time.Time) (int64, error)
}

// Discoverer finds the MACs answering on a subnet.
type Discoverer interface {
	// Name identifies the discoverer in logs and in the scan status.
	Name() string
	// Covers reports whether this discoverer can sweep the subnet (an ARP
	// sweep needs an interface on it; a router needs the subnet in its ARP
	// table).
	Covers(ctx context.Context, subnet netip.Prefix) bool
	Discover(ctx context.Context, subnet netip.Prefix) ([]domain.Sighting, error)
}

// HostnameResolver finds a name for an address. It returns "" when it has
// none. source says which resolver answered (dns, mdns, netbios...).
type HostnameResolver interface {
	Resolve(ctx context.Context, ip netip.Addr, mac string) (name, source string)
}

// NewDeviceAlert is what a Notifier sends when a MAC shows up for the first
// time after the subnet's baseline.
type NewDeviceAlert struct {
	Device domain.Device
	Subnet domain.Subnet
}

// BaselineSummary is sent once per subnet, after its first scan.
type BaselineSummary struct {
	Subnet  domain.Subnet
	Devices int
}

// Notifier sends alerts (Slack). Errors are logged, never fatal to a scan.
type Notifier interface {
	NewDevice(ctx context.Context, a NewDeviceAlert) error
	Baseline(ctx context.Context, s BaselineSummary) error
}

// VendorLookup names the manufacturer of a MAC ("" when unknown).
type VendorLookup interface {
	Vendor(mac string) string
}

// Clock lets tests control time.
type Clock interface {
	Now() time.Time
}
