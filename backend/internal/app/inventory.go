package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/renatotecchio/vartija/backend/internal/domain"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// ErrValidation wraps input the use cases reject.
var ErrValidation = errors.New("validation")

const (
	maxNickname = 100
	maxNotes    = 1000
	maxName     = 100
)

// DeviceService is the owner's review of the devices.
type DeviceService struct {
	devices ports.DeviceRepository
	events  ports.EventRepository
	clock   ports.Clock
}

// NewDeviceService wires the device use cases.
func NewDeviceService(d ports.DeviceRepository, e ports.EventRepository, c ports.Clock) *DeviceService {
	return &DeviceService{devices: d, events: e, clock: c}
}

// List returns the devices matching the filter.
func (s *DeviceService) List(ctx context.Context, f ports.DeviceFilter) ([]domain.Device, error) {
	if f.Status != "" && !f.Status.Valid() {
		return nil, fmt.Errorf("%w: unknown status %q", ErrValidation, f.Status)
	}
	return s.devices.List(ctx, f)
}

// Counts returns how many devices are new and known.
func (s *DeviceService) Counts(ctx context.Context) (map[domain.DeviceStatus]int64, error) {
	return s.devices.CountByStatus(ctx)
}

// DeviceUpdate is a partial update: nil fields are left as they are.
type DeviceUpdate struct {
	Nickname *string
	Notes    *string
	Status   *domain.DeviceStatus
}

// Update changes the owner's fields of a device.
func (s *DeviceService) Update(ctx context.Context, id uint, u DeviceUpdate) (domain.Device, error) {
	dev, err := s.devices.Get(ctx, id)
	if err != nil {
		return dev, err
	}
	if u.Nickname != nil {
		n := strings.TrimSpace(*u.Nickname)
		if len([]rune(n)) > maxNickname {
			return dev, fmt.Errorf("%w: nickname longer than %d characters", ErrValidation, maxNickname)
		}
		dev.Nickname = n
	}
	if u.Notes != nil {
		n := strings.TrimSpace(*u.Notes)
		if len([]rune(n)) > maxNotes {
			return dev, fmt.Errorf("%w: notes longer than %d characters", ErrValidation, maxNotes)
		}
		dev.Notes = n
	}
	if u.Status != nil {
		if !u.Status.Valid() {
			return dev, fmt.Errorf("%w: unknown status %q", ErrValidation, *u.Status)
		}
		if *u.Status != dev.Status {
			if *u.Status == domain.StatusKnown {
				s.acknowledge(ctx, &dev)
			} else {
				dev.Reopen()
			}
		}
	}
	return dev, s.devices.Save(ctx, &dev)
}

// Acknowledge marks the given devices as known. With no ids, every new
// device is marked (the "mark all" after the first scan).
func (s *DeviceService) Acknowledge(ctx context.Context, ids []uint) (int, error) {
	var targets []domain.Device
	if len(ids) == 0 {
		var err error
		targets, err = s.devices.List(ctx, ports.DeviceFilter{Status: domain.StatusNew})
		if err != nil {
			return 0, err
		}
	} else {
		for _, id := range ids {
			d, err := s.devices.Get(ctx, id)
			if errors.Is(err, ports.ErrNotFound) {
				continue
			}
			if err != nil {
				return 0, err
			}
			targets = append(targets, d)
		}
	}
	n := 0
	for i := range targets {
		if targets[i].Status == domain.StatusKnown {
			continue
		}
		s.acknowledge(ctx, &targets[i])
		if err := s.devices.Save(ctx, &targets[i]); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *DeviceService) acknowledge(ctx context.Context, d *domain.Device) {
	now := s.clock.Now()
	d.Acknowledge(now)
	_ = s.events.Add(ctx, &domain.Event{
		Type:     domain.EventAcknowledged,
		DeviceID: &d.ID,
		Message:  fmt.Sprintf("%s marked as known", d.DisplayName()),
		Data:     map[string]string{"name": d.DisplayName(), "mac": d.MAC},
		At:       now,
	})
}

// Delete forgets a device: if it answers again it comes back as new.
func (s *DeviceService) Delete(ctx context.Context, id uint) error {
	return s.devices.Delete(ctx, id)
}

// SubnetService manages the monitored subnets.
type SubnetService struct {
	subnets ports.SubnetRepository
	clock   ports.Clock
}

// NewSubnetService wires the subnet use cases.
func NewSubnetService(s ports.SubnetRepository, c ports.Clock) *SubnetService {
	return &SubnetService{subnets: s, clock: c}
}

// List returns every subnet.
func (s *SubnetService) List(ctx context.Context) ([]domain.Subnet, error) {
	return s.subnets.List(ctx)
}

// Create adds a subnet, enabled.
func (s *SubnetService) Create(ctx context.Context, cidr, name string) (domain.Subnet, error) {
	p, err := domain.NormalizeCIDR(cidr)
	if err != nil {
		return domain.Subnet{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) > maxName {
		return domain.Subnet{}, fmt.Errorf("%w: name longer than %d characters", ErrValidation, maxName)
	}
	sn := domain.Subnet{CIDR: p.String(), Name: name, Enabled: true, CreatedAt: s.clock.Now()}
	return sn, s.subnets.Create(ctx, &sn)
}

// SubnetUpdate is a partial update: nil fields are left as they are.
type SubnetUpdate struct {
	Name    *string
	Enabled *bool
}

// Update renames or enables/disables a subnet.
func (s *SubnetService) Update(ctx context.Context, id uint, u SubnetUpdate) (domain.Subnet, error) {
	sn, err := s.subnets.Get(ctx, id)
	if err != nil {
		return sn, err
	}
	if u.Name != nil {
		n := strings.TrimSpace(*u.Name)
		if len([]rune(n)) > maxName {
			return sn, fmt.Errorf("%w: name longer than %d characters", ErrValidation, maxName)
		}
		sn.Name = n
	}
	if u.Enabled != nil {
		sn.Enabled = *u.Enabled
	}
	return sn, s.subnets.Save(ctx, &sn)
}

// Get returns one subnet.
func (s *SubnetService) Get(ctx context.Context, id uint) (domain.Subnet, error) {
	return s.subnets.Get(ctx, id)
}

// Delete removes a subnet. Its devices stay, with their history.
func (s *SubnetService) Delete(ctx context.Context, id uint) error {
	return s.subnets.Delete(ctx, id)
}
