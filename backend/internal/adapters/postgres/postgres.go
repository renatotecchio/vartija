// Package postgres implements the repositories with GORM on PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/renatotecchio/vartija/backend/internal/domain"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// --- table models (kept apart from the domain entities) ---

type deviceRow struct {
	ID             uint   `gorm:"primaryKey"`
	MAC            string `gorm:"column:mac;size:17;uniqueIndex;not null"`
	Nickname       string `gorm:"size:100;not null;default:''"`
	Notes          string `gorm:"size:1000;not null;default:''"`
	Hostname       string `gorm:"size:255;not null;default:''"`
	HostnameSource string `gorm:"size:20;not null;default:''"`
	IP             string `gorm:"column:ip;size:45;not null;default:''"`
	SubnetID       uint   `gorm:"index"`
	Status         string `gorm:"size:10;index;not null"`
	RandomMAC      bool   `gorm:"column:random_mac;not null;default:false"`
	FirstSeen      time.Time
	LastSeen       time.Time `gorm:"index"`
	AcknowledgedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (deviceRow) TableName() string { return "devices" }

type subnetRow struct {
	ID            uint   `gorm:"primaryKey"`
	CIDR          string `gorm:"column:cidr;size:43;uniqueIndex;not null"`
	Name          string `gorm:"size:100;not null;default:''"`
	Enabled       bool   `gorm:"not null;default:true"`
	Baselined     bool   `gorm:"not null;default:false"`
	LastScanAt    *time.Time
	LastScanError string `gorm:"size:500;not null;default:''"`
	LastScanFound int    `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (subnetRow) TableName() string { return "subnets" }

type eventRow struct {
	ID       uint   `gorm:"primaryKey"`
	Type     string `gorm:"size:20;not null"`
	DeviceID *uint  `gorm:"index"`
	SubnetID *uint
	Message  string    `gorm:"size:500;not null"`
	At       time.Time `gorm:"index;not null"`
}

func (eventRow) TableName() string { return "events" }

// Open connects and migrates the schema.
func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := db.AutoMigrate(&deviceRow{}, &subnetRow{}, &eventRow{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// Ping checks the connection (readiness).
func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ports.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return ports.ErrConflict
	default:
		return err
	}
}

// --- devices ---

// Devices is the device repository.
type Devices struct{ db *gorm.DB }

// NewDevices returns the device repository.
func NewDevices(db *gorm.DB) *Devices { return &Devices{db: db} }

func toDevice(r deviceRow) domain.Device {
	return domain.Device{
		ID: r.ID, MAC: r.MAC, Nickname: r.Nickname, Notes: r.Notes,
		Hostname: r.Hostname, HostnameSource: r.HostnameSource, IP: r.IP,
		SubnetID: r.SubnetID, Status: domain.DeviceStatus(r.Status), RandomMAC: r.RandomMAC,
		FirstSeen: r.FirstSeen, LastSeen: r.LastSeen, AcknowledgedAt: r.AcknowledgedAt,
	}
}

func fromDevice(d domain.Device) deviceRow {
	return deviceRow{
		ID: d.ID, MAC: d.MAC, Nickname: d.Nickname, Notes: d.Notes,
		Hostname: d.Hostname, HostnameSource: d.HostnameSource, IP: d.IP,
		SubnetID: d.SubnetID, Status: string(d.Status), RandomMAC: d.RandomMAC,
		FirstSeen: d.FirstSeen, LastSeen: d.LastSeen, AcknowledgedAt: d.AcknowledgedAt,
	}
}

// List returns the devices, new first, then by most recently seen.
func (r *Devices) List(ctx context.Context, f ports.DeviceFilter) ([]domain.Device, error) {
	q := r.db.WithContext(ctx).Model(&deviceRow{})
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.SubnetID != 0 {
		q = q.Where("subnet_id = ?", f.SubnetID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + escapeLike(strings.ToLower(s)) + "%"
		q = q.Where("lower(mac) LIKE ? OR ip LIKE ? OR lower(hostname) LIKE ? OR lower(nickname) LIKE ? OR lower(notes) LIKE ?",
			like, like, like, like, like)
	}
	var rows []deviceRow
	if err := q.Order("CASE WHEN status = 'new' THEN 0 ELSE 1 END").Order("last_seen DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Device, len(rows))
	for i, row := range rows {
		out[i] = toDevice(row)
	}
	return out, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Get returns one device.
func (r *Devices) Get(ctx context.Context, id uint) (domain.Device, error) {
	var row deviceRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	return toDevice(row), mapErr(err)
}

// GetByMAC returns the device with that (normalized) MAC.
func (r *Devices) GetByMAC(ctx context.Context, mac string) (domain.Device, error) {
	var row deviceRow
	err := r.db.WithContext(ctx).Where("mac = ?", mac).First(&row).Error
	return toDevice(row), mapErr(err)
}

// Create inserts a device and sets its ID.
func (r *Devices) Create(ctx context.Context, d *domain.Device) error {
	row := fromDevice(*d)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapErr(err)
	}
	d.ID = row.ID
	return nil
}

// Save updates every field of a device.
func (r *Devices) Save(ctx context.Context, d *domain.Device) error {
	row := fromDevice(*d)
	return mapErr(r.db.WithContext(ctx).Omit("created_at").Save(&row).Error)
}

// Delete removes a device and its events.
func (r *Devices) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ?", id).Delete(&eventRow{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&deviceRow{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ports.ErrNotFound
		}
		return nil
	})
}

// CountByStatus returns how many devices have each status.
func (r *Devices) CountByStatus(ctx context.Context) (map[domain.DeviceStatus]int64, error) {
	var rows []struct {
		Status string
		N      int64
	}
	err := r.db.WithContext(ctx).Model(&deviceRow{}).Select("status, count(*) AS n").Group("status").Scan(&rows).Error
	out := map[domain.DeviceStatus]int64{domain.StatusNew: 0, domain.StatusKnown: 0}
	for _, row := range rows {
		out[domain.DeviceStatus(row.Status)] = row.N
	}
	return out, err
}

// --- subnets ---

// Subnets is the subnet repository.
type Subnets struct{ db *gorm.DB }

// NewSubnets returns the subnet repository.
func NewSubnets(db *gorm.DB) *Subnets { return &Subnets{db: db} }

func toSubnet(r subnetRow) domain.Subnet {
	return domain.Subnet{
		ID: r.ID, CIDR: r.CIDR, Name: r.Name, Enabled: r.Enabled, Baselined: r.Baselined,
		LastScanAt: r.LastScanAt, LastScanError: r.LastScanError, LastScanFound: r.LastScanFound,
		CreatedAt: r.CreatedAt,
	}
}

func fromSubnet(s domain.Subnet) subnetRow {
	return subnetRow{
		ID: s.ID, CIDR: s.CIDR, Name: s.Name, Enabled: s.Enabled, Baselined: s.Baselined,
		LastScanAt: s.LastScanAt, LastScanError: truncate(s.LastScanError, 500), LastScanFound: s.LastScanFound,
		CreatedAt: s.CreatedAt,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// List returns every subnet by CIDR.
func (r *Subnets) List(ctx context.Context) ([]domain.Subnet, error) {
	var rows []subnetRow
	if err := r.db.WithContext(ctx).Order("cidr").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Subnet, len(rows))
	for i, row := range rows {
		out[i] = toSubnet(row)
	}
	return out, nil
}

// Get returns one subnet.
func (r *Subnets) Get(ctx context.Context, id uint) (domain.Subnet, error) {
	var row subnetRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	return toSubnet(row), mapErr(err)
}

// Create inserts a subnet; ports.ErrConflict if the CIDR exists.
func (r *Subnets) Create(ctx context.Context, s *domain.Subnet) error {
	row := fromSubnet(*s)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapErr(err)
	}
	s.ID = row.ID
	return nil
}

// Save updates every field of a subnet.
func (r *Subnets) Save(ctx context.Context, s *domain.Subnet) error {
	row := fromSubnet(*s)
	return mapErr(r.db.WithContext(ctx).Omit("created_at").Save(&row).Error)
}

// Delete removes a subnet.
func (r *Subnets) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&subnetRow{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// --- events ---

// Events is the event repository.
type Events struct{ db *gorm.DB }

// NewEvents returns the event repository.
func NewEvents(db *gorm.DB) *Events { return &Events{db: db} }

// Add inserts an event.
func (r *Events) Add(ctx context.Context, e *domain.Event) error {
	row := eventRow{Type: string(e.Type), DeviceID: e.DeviceID, SubnetID: e.SubnetID, Message: truncate(e.Message, 500), At: e.At}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	e.ID = row.ID
	return nil
}

// List returns the latest events, newest first.
func (r *Events) List(ctx context.Context, limit int) ([]domain.Event, error) {
	var rows []eventRow
	if err := r.db.WithContext(ctx).Order("at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Event, len(rows))
	for i, row := range rows {
		out[i] = domain.Event{ID: row.ID, Type: domain.EventType(row.Type), DeviceID: row.DeviceID, SubnetID: row.SubnetID, Message: row.Message, At: row.At}
	}
	return out, nil
}

// Prune deletes events older than before.
func (r *Events) Prune(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("at < ?", before).Delete(&eventRow{})
	return res.RowsAffected, res.Error
}
