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

// Event is one line of the history shown in the UI. Data holds the values
// the UI puts into its own (translated) sentence: name, mac, ip, old_ip,
// cidr, count, error. Message is the same in English, for logs and for
// clients that don't translate.
type Event struct {
	ID       uint
	Type     EventType
	DeviceID *uint
	SubnetID *uint
	Message  string
	Data     map[string]string
	At       time.Time
}

// Sighting is one MAC answering on a subnet during a scan.
type Sighting struct {
	MAC string
	IP  string
	// IPs are all the addresses the MAC answered for in the scan (a host
	// with secondary IPs, a node holding load balancer VIPs), lowest first.
	// IP is IPs[0] once the scan has grouped them.
	IPs []string
	// Hostname is optional: some sources (a DHCP lease) bring the name with
	// the MAC.
	Hostname       string
	HostnameSource string
}
