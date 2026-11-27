package domain

import "time"

// EventType is what happened to a device or a subnet.
type EventType string

const (
	EventNewDevice    EventType = "new_device"
	EventIPChanged    EventType = "ip_changed"
	EventAcknowledged EventType = "acknowledged"
	EventBaseline     EventType = "baseline"
	EventScanFailed   EventType = "scan_failed"
)

// Event is one line of the history shown in the UI.
type Event struct {
	ID       uint
	Type     EventType
	DeviceID *uint
	SubnetID *uint
	Message  string
	At       time.Time
}

// Sighting is one MAC answering on a subnet during a scan.
type Sighting struct {
	MAC string
	IP  string
	// Hostname is optional: some sources (a DHCP lease) bring the name with
	// the MAC.
	Hostname       string
	HostnameSource string
}
