// Package config reads Vartija's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Router is a MikroTik router given in VARTIJA_MIKROTIK_ROUTERS.
type Router struct {
	Name    string
	Address string
}

// Config is every setting, with defaults applied.
type Config struct {
	Listen       string
	DatabaseURL  string
	ScanInterval time.Duration
	// EventRetention is how long the history is kept.
	EventRetention time.Duration

	HtpasswdFile   string
	AdminUser      string
	AdminPassword  string
	SessionSecret  string
	SessionTTL     time.Duration
	SecureCookie   bool
	TrustedProxies []string
	BaseURL        string

	// ARP sweep: interfaces allowed (empty = any with an address on the
	// subnet).
	Interfaces []string

	DNSServer   string
	DNSSuffixes []string
	MDNS        bool
	NetBIOS     bool

	MikrotikRouters  []Router
	MikrotikUser     string
	MikrotikPassword string

	SlackToken   string
	SlackChannel string
	SlackWebhook string
	// Language of the Slack messages: en or pt.
	Language string
}

// Load reads the environment.
func Load() (Config, error) {
	c := Config{
		Listen:           env("VARTIJA_LISTEN", ":8090"),
		DatabaseURL:      os.Getenv("VARTIJA_DATABASE_URL"),
		HtpasswdFile:     os.Getenv("VARTIJA_HTPASSWD_FILE"),
		AdminUser:        env("VARTIJA_ADMIN_USER", "admin"),
		AdminPassword:    os.Getenv("VARTIJA_ADMIN_PASSWORD"),
		SessionSecret:    os.Getenv("VARTIJA_SESSION_SECRET"),
		BaseURL:          os.Getenv("VARTIJA_BASE_URL"),
		Interfaces:       list(os.Getenv("VARTIJA_INTERFACES")),
		DNSServer:        os.Getenv("VARTIJA_DNS_SERVER"),
		DNSSuffixes:      list(os.Getenv("VARTIJA_DNS_STRIP_SUFFIXES")),
		TrustedProxies:   list(os.Getenv("VARTIJA_TRUSTED_PROXIES")),
		MikrotikUser:     os.Getenv("VARTIJA_MIKROTIK_USER"),
		MikrotikPassword: os.Getenv("VARTIJA_MIKROTIK_PASSWORD"),
		SlackToken:       os.Getenv("VARTIJA_SLACK_BOT_TOKEN"),
		SlackChannel:     os.Getenv("VARTIJA_SLACK_CHANNEL"),
		SlackWebhook:     os.Getenv("VARTIJA_SLACK_WEBHOOK_URL"),
		Language:         strings.ToLower(env("VARTIJA_LANGUAGE", "en")),
	}
	var errs []error
	var err error
	if c.ScanInterval, err = duration("VARTIJA_SCAN_INTERVAL", 5*time.Minute); err != nil {
		errs = append(errs, err)
	} else if c.ScanInterval < time.Minute {
		errs = append(errs, errors.New("VARTIJA_SCAN_INTERVAL: at least 1m"))
	}
	if c.EventRetention, err = duration("VARTIJA_EVENT_RETENTION", 90*24*time.Hour); err != nil {
		errs = append(errs, err)
	}
	if c.SessionTTL, err = duration("VARTIJA_SESSION_TTL", 12*time.Hour); err != nil {
		errs = append(errs, err)
	}
	if c.SecureCookie, err = boolean("VARTIJA_SECURE_COOKIE", true); err != nil {
		errs = append(errs, err)
	}
	if c.MDNS, err = boolean("VARTIJA_RESOLVE_MDNS", true); err != nil {
		errs = append(errs, err)
	}
	if c.NetBIOS, err = boolean("VARTIJA_RESOLVE_NETBIOS", true); err != nil {
		errs = append(errs, err)
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("VARTIJA_DATABASE_URL is required"))
	}
	if len(c.SessionSecret) < 32 {
		errs = append(errs, errors.New("VARTIJA_SESSION_SECRET is required, at least 32 characters"))
	}
	if c.HtpasswdFile == "" && c.AdminPassword == "" {
		errs = append(errs, errors.New("set VARTIJA_HTPASSWD_FILE or VARTIJA_ADMIN_PASSWORD"))
	}
	if c.DNSServer != "" {
		if _, _, err := net.SplitHostPort(c.DNSServer); err != nil {
			c.DNSServer = net.JoinHostPort(c.DNSServer, "53")
		}
	}
	routers, err := parseRouters(os.Getenv("VARTIJA_MIKROTIK_ROUTERS"))
	if err != nil {
		errs = append(errs, err)
	}
	c.MikrotikRouters = routers
	if len(routers) > 0 && (c.MikrotikUser == "" || c.MikrotikPassword == "") {
		errs = append(errs, errors.New("VARTIJA_MIKROTIK_USER and VARTIJA_MIKROTIK_PASSWORD are required with VARTIJA_MIKROTIK_ROUTERS"))
	}
	if c.Language != "en" && c.Language != "pt" {
		errs = append(errs, fmt.Errorf("VARTIJA_LANGUAGE: %q is not supported (en, pt)", c.Language))
	}
	if c.SlackChannel != "" && c.SlackToken == "" {
		errs = append(errs, errors.New("VARTIJA_SLACK_CHANNEL needs VARTIJA_SLACK_BOT_TOKEN"))
	}
	return c, errors.Join(errs...)
}

// SlackEnabled reports whether alerts go to Slack.
func (c Config) SlackEnabled() bool {
	return c.SlackWebhook != "" || (c.SlackToken != "" && c.SlackChannel != "")
}

// parseRouters reads "name=host[:port],name2=host2". The port defaults to
// 8729 (api-ssl).
func parseRouters(s string) ([]Router, error) {
	var out []Router
	for _, item := range list(s) {
		name, addr, ok := strings.Cut(item, "=")
		if !ok {
			name, addr = item, item
		}
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "8729")
		}
		if name == "" || addr == "" {
			return nil, fmt.Errorf("VARTIJA_MIKROTIK_ROUTERS: bad entry %q", item)
		}
		out = append(out, Router{Name: name, Address: addr})
	}
	return out, nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func list(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func duration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func boolean(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}
