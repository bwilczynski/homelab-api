// Package adapters contains backend adapters for each downstream service
// (UniFi, Synology, Immich, Hue, Sonos, etc.).
// Each adapter handles authentication and credential exchange for its service.
package adapters

import (
	"context"
	"time"
)

// backendRequestTimeout is the timeout for HTTP requests to backend APIs.
const backendRequestTimeout = 30 * time.Second

// HealthChecker is implemented by adapter clients that support connectivity probes.
// Ping returns nil if the backend is reachable, or a non-nil error if it is not.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// AvailabilityChecker is consulted by services before querying a backend.
// Implementations report whether a named backend is currently reachable.
type AvailabilityChecker interface {
	Available(name string) bool
}
