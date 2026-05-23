package network

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// SSIDsBackend is the narrow interface for SSID operations.
type SSIDsBackend interface {
	GetWlanConf() ([]adapters.UniFiWlanConf, error)
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetClients() ([]adapters.UniFiSta, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}

// ListSSIDs returns all enabled SSIDs across all controllers.
func (s *Service) ListSSIDs(_ context.Context) (SsidList, error) {
	var items []Ssid

	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		controller, backend := cb.controller, cb.unifi
		wlans, err := backend.GetWlanConf()
		if err != nil {
			return SsidList{}, fmt.Errorf("get wlan conf from %s: %w", controller, err)
		}
		networks, err := backend.GetNetworkConf()
		if err != nil {
			return SsidList{}, fmt.Errorf("get network conf from %s: %w", controller, err)
		}
		clients, err := backend.GetClients()
		if err != nil {
			return SsidList{}, fmt.Errorf("get clients from %s: %w", controller, err)
		}

		networkByID := indexNetworksByID(networks)
		clientsBySSID := buildClientsBySSID(clients)

		for _, w := range wlans {
			if !w.Enabled {
				continue
			}
			id := fmt.Sprintf("%s.%s", controller, toKebab(w.Name))
			vlanID := 1 // default native VLAN
			if n, ok := networkByID[w.NetworkConfID]; ok {
				vlanID = extractVlanID(n.Vlan)
			}
			items = append(items, Ssid{
				Id:         id,
				Name:       w.Name,
				Uri:        fmt.Sprintf("/network/ssids/%s", id),
				Bands:      mapBands(w.WlanBands),
				VlanId:     vlanID,
				NumClients: len(clientsBySSID[w.Name]),
			})
		}
	}

	if items == nil {
		items = []Ssid{}
	}
	return SsidList{Items: items}, nil
}

// GetSSID returns the detail for a single SSID identified by its composite ID.
func (s *Service) GetSSID(_ context.Context, id string) (SsidDetail, bool, error) {
	controller, name, ok := parseID(id)
	if !ok {
		return SsidDetail{}, false, nil
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return SsidDetail{}, false, nil
	}

	wlans, err := backend.GetWlanConf()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get wlan conf: %w", err)
	}
	networks, err := backend.GetNetworkConf()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get network conf: %w", err)
	}
	clients, err := backend.GetClients()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get clients: %w", err)
	}
	devices, err := backend.GetDevices()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get devices: %w", err)
	}

	// Find the WLAN (including disabled ones — direct lookup by kebab name)
	var wlan *adapters.UniFiWlanConf
	for i := range wlans {
		if toKebab(wlans[i].Name) == name {
			wlan = &wlans[i]
			break
		}
	}
	if wlan == nil {
		return SsidDetail{}, false, nil
	}

	networkByID := indexNetworksByID(networks)
	vlanID := 1 // default native VLAN
	if n, ok := networkByID[wlan.NetworkConfID]; ok {
		vlanID = extractVlanID(n.Vlan)
	}

	clientsBySSID := buildClientsBySSID(clients)
	ssidClients := clientsBySSID[wlan.Name]

	clientRefs := make([]NetworkClientRef, 0, len(ssidClients))
	for _, sta := range ssidClients {
		clientRefs = append(clientRefs, clientRef(controller, sta))
	}

	deviceByMAC := indexDevicesByMAC(devices)
	broadcastingAPs := collectBroadcastingAPs(controller, deviceByMAC)

	return SsidDetail{
		Id:               id,
		Name:             wlan.Name,
		Uri:              fmt.Sprintf("/network/ssids/%s", id),
		Bands:            mapBands(wlan.WlanBands),
		VlanId:           vlanID,
		NumClients:       len(ssidClients),
		Clients:          clientRefs,
		BroadcastingAps:  broadcastingAPs,
		SecurityProtocol: mapSecurityProtocol(wlan),
	}, true, nil
}

// --- helpers ---

func indexNetworksByID(networks []adapters.UniFiNetworkConf) map[string]adapters.UniFiNetworkConf {
	m := make(map[string]adapters.UniFiNetworkConf, len(networks))
	for _, n := range networks {
		m[n.ID] = n
	}
	return m
}

func indexDevicesByMAC(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	m := make(map[string]adapters.UniFiDevice, len(devices))
	for _, d := range devices {
		m[normalizeMac(d.MAC)] = d
	}
	return m
}

// buildClientsBySSID groups non-wired clients by their ESSID.
func buildClientsBySSID(clients []adapters.UniFiSta) map[string][]adapters.UniFiSta {
	m := make(map[string][]adapters.UniFiSta)
	for _, c := range clients {
		if c.IsWired || c.ESSID == nil {
			continue
		}
		m[*c.ESSID] = append(m[*c.ESSID], c)
	}
	return m
}

// collectBroadcastingAPs returns all connected APs (type "uap", state 1).
// All connected APs broadcast all enabled SSIDs site-wide in a standard UniFi setup.
func collectBroadcastingAPs(controller string, deviceByMAC map[string]adapters.UniFiDevice) []NetworkDeviceRef {
	var refs []NetworkDeviceRef
	for _, d := range deviceByMAC {
		if d.Type == "uap" && d.State == 1 {
			refs = append(refs, deviceRef(controller, d))
		}
	}
	slices.SortFunc(refs, func(a, b NetworkDeviceRef) int {
		return cmp.Compare(a.Id, b.Id)
	})
	return refs
}

func mapBands(bands []string) []WifiBand {
	result := make([]WifiBand, 0, len(bands))
	for _, b := range bands {
		switch b {
		case "2g":
			result = append(result, Band2g)
		case "5g":
			result = append(result, Band5g)
		case "6g":
			result = append(result, Band6g)
		}
	}
	return result
}

func mapSecurityProtocol(w *adapters.UniFiWlanConf) WifiSecurityProtocol {
	if w.Security == "open" {
		return Open
	}
	switch w.WpaMode {
	case "wpa2":
		if w.Wpa3Transition {
			return Wpa2Wpa3
		}
		return Wpa2
	case "wpa3":
		return Wpa3
	default:
		return Wpa2
	}
}
