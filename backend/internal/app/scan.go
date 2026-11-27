// Package app is the application core: the use cases, written against the
// ports only.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/domain"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// ErrNoDiscoverer is returned when no discoverer covers a subnet: the host
// has no interface on it and no router reports it.
var ErrNoDiscoverer = errors.New("no interface or router covers this subnet")

// ErrScanRunning is returned when a scan is asked while another runs.
var ErrScanRunning = errors.New("a scan is already running")

// hostnameRefresh is how often a known device's name is looked up again.
const hostnameRefresh = 6 * time.Hour

// ScanService sweeps subnets and records what it finds.
type ScanService struct {
	devices     ports.DeviceRepository
	subnets     ports.SubnetRepository
	events      ports.EventRepository
	discoverers []ports.Discoverer
	resolver    ports.HostnameResolver
	notifier    ports.Notifier
	vendors     ports.VendorLookup
	clock       ports.Clock
	log         *slog.Logger

	// One scan at a time: the sweeps share the interfaces, and a manual
	// scan must not race the scheduler on the same rows.
	mu sync.Mutex
	// When each MAC's name was last looked up (in memory: a restart only
	// costs one extra lookup per device).
	resolvedAt map[string]time.Time
	running    bool
	runningMu  sync.Mutex
}

// NewScanService wires the scan use case. resolver and notifier may be nil.
func NewScanService(
	devices ports.DeviceRepository,
	subnets ports.SubnetRepository,
	events ports.EventRepository,
	discoverers []ports.Discoverer,
	resolver ports.HostnameResolver,
	notifier ports.Notifier,
	clock ports.Clock,
	log *slog.Logger,
) *ScanService {
	return &ScanService{
		devices:     devices,
		subnets:     subnets,
		events:      events,
		discoverers: discoverers,
		resolver:    resolver,
		notifier:    notifier,
		clock:       clock,
		log:         log,
		resolvedAt:  map[string]time.Time{},
	}
}

// WithVendors sets the MAC vendor lookup (optional).
func (s *ScanService) WithVendors(v ports.VendorLookup) *ScanService {
	s.vendors = v
	return s
}

func (s *ScanService) vendorOf(mac string) string {
	if s.vendors == nil {
		return ""
	}
	return s.vendors.Vendor(mac)
}

// Running reports whether a scan is in progress.
func (s *ScanService) Running() bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	return s.running
}

func (s *ScanService) setRunning(v bool) {
	s.runningMu.Lock()
	s.running = v
	s.runningMu.Unlock()
}

// ScanAll sweeps every enabled subnet, one after the other. A failing subnet
// doesn't stop the others.
func (s *ScanService) ScanAll(ctx context.Context) error {
	subnets, err := s.subnets.List(ctx)
	if err != nil {
		return err
	}
	for _, sn := range subnets {
		if !sn.Enabled {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.ScanSubnet(ctx, sn.ID); err != nil {
			s.log.Warn("scan failed", "subnet", sn.CIDR, "error", err)
		}
	}
	return nil
}

// TryScanAll is ScanAll that gives up at once if a scan is running.
func (s *ScanService) TryScanAll(ctx context.Context) error {
	if s.Running() {
		return ErrScanRunning
	}
	return s.ScanAll(ctx)
}

// ScanSubnet sweeps one subnet and records every MAC that answered.
func (s *ScanService) ScanSubnet(ctx context.Context, subnetID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setRunning(true)
	defer s.setRunning(false)

	sn, err := s.subnets.Get(ctx, subnetID)
	if err != nil {
		return err
	}
	prefix, err := domain.NormalizeCIDR(sn.CIDR)
	if err != nil {
		return s.failScan(ctx, &sn, err)
	}

	disc := s.discovererFor(ctx, prefix)
	if disc == nil {
		return s.failScan(ctx, &sn, ErrNoDiscoverer)
	}

	started := s.clock.Now()
	sightings, err := disc.Discover(ctx, prefix)
	if err != nil {
		return s.failScan(ctx, &sn, fmt.Errorf("%s: %w", disc.Name(), err))
	}

	// Normalize and dedupe, then look the names up in parallel (each source
	// may wait up to a couple of seconds for a device that doesn't answer),
	// then write.
	// One entry per MAC, with every IP it answered for.
	var batch []domain.Sighting
	seen := map[string]int{} // MAC -> index in batch
	for _, sg := range sightings {
		mac, err := domain.NormalizeMAC(sg.MAC)
		if err != nil || mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
			continue
		}
		if i, ok := seen[mac]; ok {
			batch[i].IPs = append(batch[i].IPs, sg.IP)
			if batch[i].Hostname == "" && sg.Hostname != "" {
				batch[i].Hostname, batch[i].HostnameSource = sg.Hostname, sg.HostnameSource
			}
			continue
		}
		seen[mac] = len(batch)
		sg.MAC = mac
		sg.IPs = []string{sg.IP}
		batch = append(batch, sg)
	}
	for i := range batch {
		batch[i].IPs = domain.SortIPs(batch[i].IPs)
		if len(batch[i].IPs) > 0 {
			batch[i].IP = batch[i].IPs[0]
		}
	}
	names := s.resolveNames(ctx, batch)

	newDevices := 0
	for _, sg := range batch {
		if n, ok := names[sg.MAC]; ok {
			sg.Hostname, sg.HostnameSource = n.name, n.source
		}
		created, err := s.record(ctx, sn, sg)
		if err != nil {
			s.log.Error("record device", "mac", sg.MAC, "error", err)
			continue
		}
		if created {
			newDevices++
		}
	}

	now := s.clock.Now()
	if !sn.Baselined {
		sn.Baselined = true
		s.addEvent(ctx, &domain.Event{
			Type:     domain.EventBaseline,
			SubnetID: &sn.ID,
			Message:  fmt.Sprintf("First scan of %s: %d devices", sn.CIDR, len(seen)),
			Data:     map[string]string{"cidr": sn.CIDR, "count": strconv.Itoa(len(seen))},
			At:       now,
		})
		if s.notifier != nil {
			if err := s.notifier.Baseline(ctx, ports.BaselineSummary{Subnet: sn, Devices: len(seen)}); err != nil {
				s.log.Warn("baseline notification", "error", err)
			}
		}
	}
	sn.LastScanAt = &now
	sn.LastScanError = ""
	sn.LastScanFound = len(seen)
	if err := s.subnets.Save(ctx, &sn); err != nil {
		return err
	}
	s.log.Info("scan done", "subnet", sn.CIDR, "via", disc.Name(), "found", len(seen),
		"new", newDevices, "took", now.Sub(started).Round(time.Millisecond))
	return nil
}

func (s *ScanService) discovererFor(ctx context.Context, p netip.Prefix) ports.Discoverer {
	for _, d := range s.discoverers {
		if d.Covers(ctx, p) {
			return d
		}
	}
	return nil
}

// record creates or updates the device behind one sighting (its name
// already looked up). It returns true when the MAC was never seen before.
func (s *ScanService) record(ctx context.Context, sn domain.Subnet, sg domain.Sighting) (bool, error) {
	now := s.clock.Now()
	dev, err := s.devices.GetByMAC(ctx, sg.MAC)
	switch {
	case errors.Is(err, ports.ErrNotFound):
		dev = domain.Device{
			MAC:       sg.MAC,
			IP:        sg.IP,
			ExtraIPs:  sg.IPs[min(1, len(sg.IPs)):],
			SubnetID:  sn.ID,
			Status:    domain.StatusNew,
			RandomMAC: domain.IsRandomMAC(sg.MAC),
			Vendor:    s.vendorOf(sg.MAC),
			FirstSeen: now,
			LastSeen:  now,
		}
		dev.Hostname, dev.HostnameSource = sg.Hostname, sg.HostnameSource
		if err := s.devices.Create(ctx, &dev); err != nil {
			return false, err
		}
		s.addEvent(ctx, &domain.Event{
			Type:     domain.EventNewDevice,
			DeviceID: &dev.ID,
			SubnetID: &sn.ID,
			Message:  fmt.Sprintf("New device %s (%s) on %s", dev.DisplayName(), dev.IP, sn.CIDR),
			Data:     map[string]string{"name": dev.DisplayName(), "mac": dev.MAC, "ip": dev.IP, "cidr": sn.CIDR, "vendor": dev.Vendor},
			At:       now,
		})
		// Before the baseline every MAC is new: the summary covers them.
		if sn.Baselined && s.notifier != nil {
			if err := s.notifier.NewDevice(ctx, ports.NewDeviceAlert{Device: dev, Subnet: sn}); err != nil {
				s.log.Warn("new device notification", "mac", dev.MAC, "error", err)
			}
		}
		return true, nil
	case err != nil:
		return false, err
	}

	main, extra := domain.PickIP(dev.IP, sg.IPs)
	if main != "" && main != dev.IP {
		s.addEvent(ctx, &domain.Event{
			Type:     domain.EventIPChanged,
			DeviceID: &dev.ID,
			SubnetID: &sn.ID,
			Message:  fmt.Sprintf("%s changed IP: %s → %s", dev.DisplayName(), dev.IP, main),
			Data:     map[string]string{"name": dev.DisplayName(), "mac": dev.MAC, "old_ip": dev.IP, "ip": main},
			At:       now,
		})
		dev.IP = main
	}
	dev.ExtraIPs = extra
	dev.SubnetID = sn.ID
	dev.LastSeen = now
	if dev.Vendor == "" {
		dev.Vendor = s.vendorOf(dev.MAC)
	}
	if sg.Hostname != "" {
		dev.Hostname, dev.HostnameSource = sg.Hostname, sg.HostnameSource
	}
	return false, s.devices.Save(ctx, &dev)
}

type resolvedName struct{ name, source string }

// resolverWorkers bounds the parallel name lookups.
const resolverWorkers = 16

// resolveNames looks up the name of the sightings that need one: a MAC never
// seen, an IP change, a device without a name, or a name older than
// hostnameRefresh. Sightings that already carry a name (DHCP) are skipped.
func (s *ScanService) resolveNames(ctx context.Context, batch []domain.Sighting) map[string]resolvedName {
	out := map[string]resolvedName{}
	if s.resolver == nil {
		return out
	}
	now := s.clock.Now()
	type job struct {
		mac string
		ip  netip.Addr
	}
	var jobs []job
	for _, sg := range batch {
		if sg.Hostname != "" {
			s.resolvedAt[sg.MAC] = now
			continue
		}
		dev, err := s.devices.GetByMAC(ctx, sg.MAC)
		main := sg.IP
		if err == nil {
			main, _ = domain.PickIP(dev.IP, sg.IPs)
		}
		ip, perr := netip.ParseAddr(main)
		if perr != nil {
			continue
		}
		force := err != nil || dev.IP != main || dev.Hostname == ""
		if last, ok := s.resolvedAt[sg.MAC]; ok && !force && now.Sub(last) < hostnameRefresh {
			continue
		}
		s.resolvedAt[sg.MAC] = now
		jobs = append(jobs, job{mac: sg.MAC, ip: ip})
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolverWorkers)
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			if name, source := s.resolver.Resolve(ctx, j.ip, j.mac); name != "" {
				mu.Lock()
				out[j.mac] = resolvedName{name, source}
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
	return out
}

func (s *ScanService) failScan(ctx context.Context, sn *domain.Subnet, cause error) error {
	now := s.clock.Now()
	// One event per new error, not one per interval while it lasts
	if sn.LastScanError != cause.Error() {
		s.addEvent(ctx, &domain.Event{
			Type:     domain.EventScanFailed,
			SubnetID: &sn.ID,
			Message:  fmt.Sprintf("Scan of %s failed: %v", sn.CIDR, cause),
			Data:     map[string]string{"cidr": sn.CIDR, "error": cause.Error()},
			At:       now,
		})
	}
	sn.LastScanAt = &now
	sn.LastScanError = cause.Error()
	sn.LastScanFound = 0
	if err := s.subnets.Save(ctx, sn); err != nil {
		s.log.Error("save subnet", "error", err)
	}
	return cause
}

func (s *ScanService) addEvent(ctx context.Context, e *domain.Event) {
	if err := s.events.Add(ctx, e); err != nil {
		s.log.Error("add event", "error", err)
	}
}

// Coverage tells, per subnet, which discoverer would sweep it ("" = none).
func (s *ScanService) Coverage(ctx context.Context, p netip.Prefix) string {
	if d := s.discovererFor(ctx, p); d != nil {
		return d.Name()
	}
	return ""
}
