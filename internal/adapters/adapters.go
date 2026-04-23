// Package adapters contains backend adapters for each downstream service
// (UniFi, Synology, Immich, Hue, Sonos, etc.).
// Each adapter handles authentication and credential exchange for its service.
package adapters

// HealthChecker is implemented by adapter clients that support connectivity probes.
// Ping returns nil if the backend is reachable, or a non-nil error if it is not.
type HealthChecker interface {
	Ping() error
}

// AvailabilityChecker is consulted by services before querying a backend.
// Implementations report whether a named backend is currently reachable.
type AvailabilityChecker interface {
	Available(name string) bool
}
