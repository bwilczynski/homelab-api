package network

import "github.com/bwilczynski/homelab-api/internal/adapters"

// SSIDsBackend is the narrow interface for SSID operations.
type SSIDsBackend interface {
	GetWlanConf() ([]adapters.UniFiWlanConf, error)
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetClients() ([]adapters.UniFiSta, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}
