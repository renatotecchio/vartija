package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/domain"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// --- fakes ---

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

type memDevices struct {
	next uint
	rows map[uint]domain.Device
}

func newMemDevices() *memDevices { return &memDevices{rows: map[uint]domain.Device{}} }

func (m *memDevices) List(_ context.Context, f ports.DeviceFilter) ([]domain.Device, error) {
	var out []domain.Device
	for _, d := range m.rows {
		if f.Status != "" && d.Status != f.Status {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *memDevices) Get(_ context.Context, id uint) (domain.Device, error) {
	d, ok := m.rows[id]
	if !ok {
		return d, ports.ErrNotFound
	}
	return d, nil
}
func (m *memDevices) GetByMAC(_ context.Context, mac string) (domain.Device, error) {
	for _, d := range m.rows {
		if d.MAC == mac {
			return d, nil
		}
	}
	return domain.Device{}, ports.ErrNotFound
}
func (m *memDevices) Create(_ context.Context, d *domain.Device) error {
	m.next++
	d.ID = m.next
	m.rows[d.ID] = *d
	return nil
}
func (m *memDevices) Save(_ context.Context, d *domain.Device) error { m.rows[d.ID] = *d; return nil }
func (m *memDevices) Delete(_ context.Context, id uint) error        { delete(m.rows, id); return nil }
func (m *memDevices) CountByStatus(context.Context) (map[domain.DeviceStatus]int64, error) {
	out := map[domain.DeviceStatus]int64{}
	for _, d := range m.rows {
		out[d.Status]++
	}
	return out, nil
}

type memSubnets struct{ rows map[uint]domain.Subnet }

func (m *memSubnets) List(context.Context) ([]domain.Subnet, error) {
	var out []domain.Subnet
	for _, s := range m.rows {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *memSubnets) Get(_ context.Context, id uint) (domain.Subnet, error) {
	s, ok := m.rows[id]
	if !ok {
		return s, ports.ErrNotFound
	}
	return s, nil
}
func (m *memSubnets) Create(_ context.Context, s *domain.Subnet) error {
	s.ID = uint(len(m.rows) + 1)
	m.rows[s.ID] = *s
	return nil
}
func (m *memSubnets) Save(_ context.Context, s *domain.Subnet) error { m.rows[s.ID] = *s; return nil }
func (m *memSubnets) Delete(_ context.Context, id uint) error        { delete(m.rows, id); return nil }

type memEvents struct{ rows []domain.Event }

func (m *memEvents) Add(_ context.Context, e *domain.Event) error {
	m.rows = append(m.rows, *e)
	return nil
}
func (m *memEvents) List(context.Context, int) ([]domain.Event, error) { return m.rows, nil }
func (m *memEvents) Prune(context.Context, time.Time) (int64, error)   { return 0, nil }
func (m *memEvents) count(t domain.EventType) int {
	n := 0
	for _, e := range m.rows {
		if e.Type == t {
			n++
		}
	}
	return n
}

type fakeDiscoverer struct {
	covers    netip.Prefix
	sightings []domain.Sighting
	err       error
}

func (f *fakeDiscoverer) Name() string { return "fake" }
func (f *fakeDiscoverer) Covers(_ context.Context, p netip.Prefix) bool {
	return f.covers == p
}
func (f *fakeDiscoverer) Discover(context.Context, netip.Prefix) ([]domain.Sighting, error) {
	return f.sightings, f.err
}

type fakeResolver struct{ calls atomic.Int32 }

func (f *fakeResolver) Resolve(_ context.Context, ip netip.Addr, _ string) (string, string) {
	f.calls.Add(1)
	return "host-" + strings.ReplaceAll(ip.String(), ".", "-"), "dns"
}

type fakeNotifier struct {
	alerts    []ports.NewDeviceAlert
	baselines []ports.BaselineSummary
}

func (f *fakeNotifier) NewDevice(_ context.Context, a ports.NewDeviceAlert) error {
	f.alerts = append(f.alerts, a)
	return nil
}
func (f *fakeNotifier) Baseline(_ context.Context, s ports.BaselineSummary) error {
	f.baselines = append(f.baselines, s)
	return nil
}

type fixture struct {
	clock    *fakeClock
	devices  *memDevices
	subnets  *memSubnets
	events   *memEvents
	disc     *fakeDiscoverer
	resolver *fakeResolver
	notifier *fakeNotifier
	scan     *ScanService
	subnet   domain.Subnet
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		clock:    &fakeClock{t: time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)},
		devices:  newMemDevices(),
		subnets:  &memSubnets{rows: map[uint]domain.Subnet{}},
		events:   &memEvents{},
		disc:     &fakeDiscoverer{covers: netip.MustParsePrefix("192.168.1.0/24")},
		resolver: &fakeResolver{},
		notifier: &fakeNotifier{},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.scan = NewScanService(f.devices, f.subnets, f.events, []ports.Discoverer{f.disc}, f.resolver, f.notifier, f.clock, log)
	sn, err := NewSubnetService(f.subnets, f.clock).Create(context.Background(), "192.168.1.0/24", "LAN")
	if err != nil {
		t.Fatal(err)
	}
	f.subnet = sn
	return f
}

// --- tests ---

func TestFirstScanIsABaselineWithoutPerDeviceAlerts(t *testing.T) {
	f := newFixture(t)
	f.disc.sightings = []domain.Sighting{
		{MAC: "AA:BB:CC:00:00:01", IP: "192.168.1.10"},
		{MAC: "aa-bb-cc-00-00-02", IP: "192.168.1.11"},
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"}, // duplicate
	}
	if err := f.scan.ScanSubnet(context.Background(), f.subnet.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.devices.rows) != 2 {
		t.Fatalf("want 2 devices, got %d", len(f.devices.rows))
	}
	for _, d := range f.devices.rows {
		if d.Status != domain.StatusNew {
			t.Errorf("%s should start as new", d.MAC)
		}
		if d.Hostname == "" || d.HostnameSource != "dns" {
			t.Errorf("%s has no hostname", d.MAC)
		}
	}
	if len(f.notifier.alerts) != 0 {
		t.Errorf("baseline must not alert per device, got %d", len(f.notifier.alerts))
	}
	if len(f.notifier.baselines) != 1 || f.notifier.baselines[0].Devices != 2 {
		t.Errorf("want one baseline summary with 2 devices, got %+v", f.notifier.baselines)
	}
	sn := f.subnets.rows[f.subnet.ID]
	if !sn.Baselined || sn.LastScanFound != 2 || sn.LastScanError != "" {
		t.Errorf("subnet not updated: %+v", sn)
	}
}

func TestNewMACAfterBaselineAlerts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.disc.sightings = []domain.Sighting{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"}}
	_ = f.scan.ScanSubnet(ctx, f.subnet.ID)

	f.disc.sightings = append(f.disc.sightings, domain.Sighting{
		MAC: "aa:bb:cc:00:00:09", IP: "192.168.1.99", Hostname: "phone", HostnameSource: "dhcp",
	})
	if err := f.scan.ScanSubnet(ctx, f.subnet.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.notifier.alerts) != 1 {
		t.Fatalf("want 1 alert, got %d", len(f.notifier.alerts))
	}
	a := f.notifier.alerts[0]
	if a.Device.MAC != "aa:bb:cc:00:00:09" || a.Device.Hostname != "phone" || a.Subnet.CIDR != "192.168.1.0/24" {
		t.Errorf("alert: %+v", a)
	}
	if len(f.notifier.baselines) != 1 {
		t.Errorf("the baseline is sent once, got %d", len(f.notifier.baselines))
	}
}

func TestKnownDeviceIPChangeIsRecordedNotAlerted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.disc.sightings = []domain.Sighting{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"}}
	_ = f.scan.ScanSubnet(ctx, f.subnet.ID)
	f.clock.t = f.clock.t.Add(5 * time.Minute)
	f.disc.sightings[0].IP = "192.168.1.20"
	_ = f.scan.ScanSubnet(ctx, f.subnet.ID)

	d, _ := f.devices.GetByMAC(ctx, "aa:bb:cc:00:00:01")
	if d.IP != "192.168.1.20" || !d.LastSeen.Equal(f.clock.t) {
		t.Errorf("device not updated: %+v", d)
	}
	if f.events.count(domain.EventIPChanged) != 1 || len(f.notifier.alerts) != 0 {
		t.Errorf("want 1 ip_changed and no alert")
	}
	if d.Hostname != "host-192-168-1-20" {
		t.Errorf("hostname should be looked up again after an IP change, got %q", d.Hostname)
	}
}

func TestHostnameIsNotLookedUpEveryScan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.disc.sightings = []domain.Sighting{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"}}
	for i := 0; i < 5; i++ {
		_ = f.scan.ScanSubnet(ctx, f.subnet.ID)
		f.clock.t = f.clock.t.Add(5 * time.Minute)
	}
	if f.resolver.calls.Load() != 1 {
		t.Errorf("want 1 lookup, got %d", f.resolver.calls.Load())
	}
	f.clock.t = f.clock.t.Add(hostnameRefresh)
	_ = f.scan.ScanSubnet(ctx, f.subnet.ID)
	if f.resolver.calls.Load() != 2 {
		t.Errorf("want a refresh after %v, got %d lookups", hostnameRefresh, f.resolver.calls.Load())
	}
}

func TestScanFailureIsRecordedOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.disc.err = errors.New("permission denied")
	for i := 0; i < 3; i++ {
		if err := f.scan.ScanSubnet(ctx, f.subnet.ID); err == nil {
			t.Fatal("want an error")
		}
	}
	if n := f.events.count(domain.EventScanFailed); n != 1 {
		t.Errorf("want 1 scan_failed event, got %d", n)
	}
	sn := f.subnets.rows[f.subnet.ID]
	if sn.Baselined || !strings.Contains(sn.LastScanError, "permission denied") {
		t.Errorf("subnet: %+v", sn)
	}
}

func TestUncoveredSubnet(t *testing.T) {
	f := newFixture(t)
	sn, _ := NewSubnetService(f.subnets, f.clock).Create(context.Background(), "10.99.0.0/24", "")
	err := f.scan.ScanSubnet(context.Background(), sn.ID)
	if !errors.Is(err, ErrNoDiscoverer) {
		t.Fatalf("want ErrNoDiscoverer, got %v", err)
	}
}

func TestAcknowledgeAllNew(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.disc.sightings = []domain.Sighting{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"},
		{MAC: "aa:bb:cc:00:00:02", IP: "192.168.1.11"},
	}
	_ = f.scan.ScanSubnet(ctx, f.subnet.ID)
	svc := NewDeviceService(f.devices, f.events, f.clock)
	n, err := svc.Acknowledge(ctx, nil)
	if err != nil || n != 2 {
		t.Fatalf("acknowledged %d, %v", n, err)
	}
	counts, _ := svc.Counts(ctx)
	if counts[domain.StatusKnown] != 2 || counts[domain.StatusNew] != 0 {
		t.Errorf("counts: %v", counts)
	}
}

func TestDeviceUpdate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.disc.sightings = []domain.Sighting{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"}}
	_ = f.scan.ScanSubnet(ctx, f.subnet.ID)
	svc := NewDeviceService(f.devices, f.events, f.clock)
	nick := "  TV da sala "
	known := domain.StatusKnown
	d, err := svc.Update(ctx, 1, DeviceUpdate{Nickname: &nick, Status: &known})
	if err != nil {
		t.Fatal(err)
	}
	if d.Nickname != "TV da sala" || d.Status != domain.StatusKnown || d.AcknowledgedAt == nil {
		t.Errorf("got %+v", d)
	}
	long := strings.Repeat("x", maxNickname+1)
	if _, err := svc.Update(ctx, 1, DeviceUpdate{Nickname: &long}); !errors.Is(err, ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
	bad := domain.DeviceStatus("maybe")
	if _, err := svc.Update(ctx, 1, DeviceUpdate{Status: &bad}); !errors.Is(err, ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestAuth(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAuthService(map[string][]byte{"renato": hash}, []byte(strings.Repeat("s", 32)), time.Hour, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Login("renato", "wrong"); !errors.Is(err, ErrBadCredentials) {
		t.Error("wrong password accepted")
	}
	if _, _, err := a.Login("nobody", "correct horse battery"); !errors.Is(err, ErrBadCredentials) {
		t.Error("unknown user accepted")
	}
	tok, _, err := a.Login("renato", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if u, err := a.Verify(tok); err != nil || u != "renato" {
		t.Errorf("verify: %q %v", u, err)
	}
	if _, err := a.Verify(tok[:len(tok)-2] + "xx"); err == nil {
		t.Error("tampered token accepted")
	}
	now = now.Add(2 * time.Hour)
	if _, err := a.Verify(tok); err == nil {
		t.Error("expired token accepted")
	}
}

func TestParseHtpasswd(t *testing.T) {
	hash, _ := HashPassword("correct horse battery")
	users, err := ParseHtpasswd(strings.NewReader("# comment\n\nrenato:" + string(hash) + "\n"))
	if err != nil || len(users) != 1 {
		t.Fatalf("got %v, %v", users, err)
	}
	if _, err := ParseHtpasswd(strings.NewReader("renato:$apr1$abc$def\n")); err == nil {
		t.Error("MD5 hash accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Error("short password accepted")
	}
}
