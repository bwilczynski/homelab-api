package network

import "github.com/bwilczynski/homelab-api/internal/adapters"

// WANsBackend is the narrow interface for WAN operations.
type WANsBackend interface {
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}
