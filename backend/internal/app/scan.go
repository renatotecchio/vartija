// Package app is the application core: the use cases, written against the
// ports only.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
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
	var batch []domain.Sighting
	seen := map[string]bool{}
	for _, sg := range sightings {
		mac, err := domain.NormalizeMAC(sg.MAC)
		if err != nil || seen[mac] || mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
			continue
		}
		seen[mac] = true
		sg.MAC = mac
		batch = append(batch, sg)
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
			Message:  fmt.Sprintf("Primeira varredura de %s: %d dispositivos", sn.CIDR, len(seen)),
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
			SubnetID:  sn.ID,
			Status:    domain.StatusNew,
			RandomMAC: domain.IsRandomMAC(sg.MAC),
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
			Message:  fmt.Sprintf("Novo dispositivo %s (%s) em %s", dev.DisplayName(), dev.IP, sn.CIDR),
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

	ipChanged := sg.IP != "" && sg.IP != dev.IP
	if ipChanged {
		s.addEvent(ctx, &domain.Event{
			Type:     domain.EventIPChanged,
			DeviceID: &dev.ID,
			SubnetID: &sn.ID,
			Message:  fmt.Sprintf("%s mudou de IP: %s → %s", dev.DisplayName(), dev.IP, sg.IP),
			At:       now,
		})
		dev.IP = sg.IP
	}
	dev.SubnetID = sn.ID
	dev.LastSeen = now
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
		ip, err := netip.ParseAddr(sg.IP)
		if err != nil {
			continue
		}
		dev, err := s.devices.GetByMAC(ctx, sg.MAC)
		force := err != nil || dev.IP != sg.IP || dev.Hostname == ""
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
			Message:  fmt.Sprintf("Falha na varredura de %s: %v", sn.CIDR, cause),
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
