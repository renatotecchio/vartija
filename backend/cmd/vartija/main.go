// Command vartija watches subnets for new MAC addresses.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/adapters/arp"
	"github.com/renatotecchio/vartija/backend/internal/adapters/hostname"
	"github.com/renatotecchio/vartija/backend/internal/adapters/httpapi"
	"github.com/renatotecchio/vartija/backend/internal/adapters/mikrotik"
	"github.com/renatotecchio/vartija/backend/internal/adapters/postgres"
	"github.com/renatotecchio/vartija/backend/internal/adapters/slack"
	"github.com/renatotecchio/vartija/backend/internal/app"
	"github.com/renatotecchio/vartija/backend/internal/config"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		hashPassword()
		return
	}
	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// hashPassword prints an htpasswd line: vartija hash-password <user>
// (password on stdin), for when htpasswd isn't at hand.
func hashPassword() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: echo -n 'password' | vartija hash-password <user>")
		os.Exit(2)
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1024))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read password:", err)
		os.Exit(1)
	}
	h, err := app.HashPassword(strings.TrimRight(string(raw), "\r\n"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s:%s\n", os.Args[2], h)
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log.Info("starting vartija", "version", version, "listen", cfg.Listen, "interval", cfg.ScanInterval.String())

	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	devices := postgres.NewDevices(db)
	subnets := postgres.NewSubnets(db)
	events := postgres.NewEvents(db)
	clock := systemClock{}

	users, err := loadUsers(cfg)
	if err != nil {
		return err
	}
	auth, err := app.NewAuthService(users, []byte(cfg.SessionSecret), cfg.SessionTTL, time.Now)
	if err != nil {
		return err
	}

	// Discoverers, in order of preference: a real ARP sweep where this host
	// has an interface, the routers' ARP tables elsewhere.
	discoverers := []ports.Discoverer{arp.New(cfg.Interfaces)}
	resolvers := hostname.Chain{}
	if len(cfg.MikrotikRouters) > 0 {
		routers := make([]mikrotik.Router, len(cfg.MikrotikRouters))
		for i, r := range cfg.MikrotikRouters {
			routers[i] = mikrotik.Router{Name: r.Name, Address: r.Address}
		}
		mt := mikrotik.New(routers, cfg.MikrotikUser, cfg.MikrotikPassword, log)
		discoverers = append(discoverers, mt)
		resolvers = append(resolvers, mt)
	}
	resolvers = append(resolvers, hostname.DNS{Server: cfg.DNSServer, Suffixes: cfg.DNSSuffixes})
	if cfg.MDNS {
		resolvers = append(resolvers, hostname.MDNS{})
	}
	if cfg.NetBIOS {
		resolvers = append(resolvers, hostname.NetBIOS{})
	}

	var notifier ports.Notifier
	if cfg.SlackEnabled() {
		n, err := slack.New(cfg.SlackToken, cfg.SlackChannel, cfg.SlackWebhook, cfg.BaseURL)
		if err != nil {
			return err
		}
		notifier = n
	} else {
		log.Warn("Slack not configured: new devices are only shown in the UI")
	}

	scan := app.NewScanService(devices, subnets, events, discoverers, resolvers, notifier, clock, log)
	handler, err := httpapi.New(httpapi.Deps{
		Auth:           auth,
		Devices:        app.NewDeviceService(devices, events, clock),
		Subnets:        app.NewSubnetService(subnets, clock),
		Scan:           scan,
		Events:         events,
		Ready:          func(ctx context.Context) error { return postgres.Ping(ctx, db) },
		Log:            log,
		SecureCookie:   cfg.SecureCookie,
		TrustedProxies: cfg.TrustedProxies,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go app.NewScheduler(scan, events, cfg.ScanInterval, cfg.EventRetention, clock, log).Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func loadUsers(cfg config.Config) (map[string][]byte, error) {
	users := map[string][]byte{}
	if cfg.HtpasswdFile != "" {
		f, err := os.Open(cfg.HtpasswdFile)
		if err != nil {
			return nil, fmt.Errorf("htpasswd: %w", err)
		}
		defer f.Close()
		if users, err = app.ParseHtpasswd(f); err != nil {
			return nil, err
		}
	}
	if cfg.AdminPassword != "" {
		h, err := app.HashPassword(cfg.AdminPassword)
		if err != nil {
			return nil, fmt.Errorf("VARTIJA_ADMIN_PASSWORD: %w", err)
		}
		users[cfg.AdminUser] = h
	}
	return users, nil
}
