package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// Scheduler runs a full scan every interval, and prunes old events daily.
type Scheduler struct {
	scan      *ScanService
	events    ports.EventRepository
	interval  time.Duration
	retention time.Duration
	clock     ports.Clock
	log       *slog.Logger
}

// NewScheduler wires the periodic jobs.
func NewScheduler(scan *ScanService, events ports.EventRepository, interval, retention time.Duration, clock ports.Clock, log *slog.Logger) *Scheduler {
	return &Scheduler{scan: scan, events: events, interval: interval, retention: retention, clock: clock, log: log}
}

// Run blocks until ctx is done. The first scan starts right away.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	lastPrune := time.Time{}
	for {
		if err := s.scan.ScanAll(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("scheduled scan", "error", err)
		}
		if s.clock.Now().Sub(lastPrune) > 24*time.Hour {
			lastPrune = s.clock.Now()
			if n, err := s.events.Prune(ctx, lastPrune.Add(-s.retention)); err != nil {
				s.log.Error("prune events", "error", err)
			} else if n > 0 {
				s.log.Info("pruned old events", "count", n)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
