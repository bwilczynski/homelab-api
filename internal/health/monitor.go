// Package health provides a background health monitor for backend adapters.
// It periodically pings each registered backend and tracks availability,
// logging transitions when a backend goes offline or comes back online.
package health

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// Monitor periodically pings registered backends and tracks their availability.
// It implements adapters.AvailabilityChecker.
type Monitor struct {
	mu        sync.RWMutex
	backends  map[string]adapters.HealthChecker
	available map[string]bool
	logger    *slog.Logger
	interval  time.Duration
}

// NewMonitor creates a monitor for the given backends. Each backend is assumed
// available at startup; the first probe runs immediately when Start is called.
func NewMonitor(backends map[string]adapters.HealthChecker, interval time.Duration, logger *slog.Logger) *Monitor {
	avail := make(map[string]bool, len(backends))
	for name := range backends {
		avail[name] = true
	}
	return &Monitor{
		backends:  backends,
		available: avail,
		logger:    logger,
		interval:  interval,
	}
}

// Available reports whether the named backend is currently reachable.
// It implements adapters.AvailabilityChecker.
func (m *Monitor) Available(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.available[name]
}

// Start runs the health monitor until ctx is cancelled.
// It probes all backends immediately, then on every interval tick.
func (m *Monitor) Start(ctx context.Context) {
	m.probe(ctx)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.probe(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Monitor) probe(ctx context.Context) {
	for name, hc := range m.backends {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := hc.Ping(probeCtx)
		cancel()

		m.mu.Lock()
		was := m.available[name]
		now := err == nil
		m.available[name] = now
		m.mu.Unlock()

		switch {
		case was && !now:
			m.logger.Warn("backend went offline", "backend", name, "err", err)
		case !was && now:
			m.logger.Info("backend came back online", "backend", name)
		}
	}
}
