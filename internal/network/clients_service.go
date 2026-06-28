package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// ClientsBackend is the narrow interface for client operations.
type ClientsBackend interface {
	GetClients() ([]adapters.UniFiSta, error)
	GetActiveClients() ([]adapters.UniFiClientV2, error)
	GetOfflineClients(historyDays int) ([]adapters.UniFiClientV2, error)
	GetAllClients(historyDays int) ([]adapters.UniFiClientV2, error)
}

// ListClients retrieves clients from all backends. status filters by "online", "offline", or "" for all.
func (s *Service) ListClients(ctx context.Context, status string) (NetworkClientList, error) {
	var items []NetworkClient
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		var raw []adapters.UniFiClientV2
		var err error
		switch status {
		case "online":
			raw, err = cb.unifi.GetActiveClients()
		case "offline":
			raw, err = cb.unifi.GetOfflineClients(s.historyDays)
		default:
			raw, err = cb.unifi.GetAllClients(s.historyDays)
		}
		if err != nil {
			return NetworkClientList{}, fmt.Errorf("get unifi clients from %s: %w", cb.controller, err)
		}
		for _, c := range raw {
			items = append(items, clientToListV2(cb.controller, c))
		}
	}
	if items == nil {
		items = []NetworkClient{}
	}
	return NetworkClientList{Items: items}, nil
}

func clientNameV2(c adapters.UniFiClientV2) string {
	if c.Name != nil && *c.Name != "" {
		return *c.Name
	}
	if c.Hostname != nil && *c.Hostname != "" {
		return *c.Hostname
	}
	return c.MAC
}

func clientToListV2(controller string, c adapters.UniFiClientV2) NetworkClient {
	name := clientNameV2(c)
	mac := normalizeMac(c.MAC)
	prefix := macHexPrefix(mac, c.ID)
	id := fmt.Sprintf("%s.%s-%s", controller, toKebab(name), prefix)

	var ip string
	if c.Status == "online" {
		ip = c.IP
	} else {
		ip = c.LastIP
	}

	client := NetworkClient{
		Id:             id,
		Uri:            fmt.Sprintf("/network/clients/%s", id),
		Name:           name,
		Mac:            mac,
		ConnectionType: mapConnectionType(c.IsWired),
		Status:         NetworkClientStatus(c.Status),
	}
	if ip != "" {
		client.Ip = &ip
	}
	return client
}

// GetClient looks up a single client by composite ID and returns its typed detail.
func (s *Service) GetClient(ctx context.Context, id string) (NetworkClientDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok {
		return NetworkClientDetail{}, false, nil
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkClientDetail{}, false, nil
	}

	// Fetch devices for cross-reference (device refs in connectedTo).
	devices, err := backend.GetDevices()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}
	macToDevice := buildMacToDevice(devices)

	raw, err := backend.GetClients()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}

	for _, sta := range raw {
		if clientSuffix(sta) == suffix {
			detail, err := clientToDetail(controller, sta, macToDevice)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}

	// Not found in active clients — check offline history.
	offline, err := backend.GetOfflineClients(s.historyDays)
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi offline clients: %w", err)
	}

	for _, c := range offline {
		name := clientNameV2(c)
		mac := normalizeMac(c.MAC)
		prefix := macHexPrefix(mac, c.ID)
		if fmt.Sprintf("%s-%s", toKebab(name), prefix) == suffix {
			detail, err := clientToDetailV2(controller, c, macToDevice)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkClientDetail{}, false, nil
}

func clientToDetailV2(controller string, c adapters.UniFiClientV2, macToDevice map[string]adapters.UniFiDevice) (NetworkClientDetail, error) {
	name := clientNameV2(c)
	mac := normalizeMac(c.MAC)
	prefix := macHexPrefix(mac, c.ID)
	id := fmt.Sprintf("%s.%s-%s", controller, toKebab(name), prefix)

	var ip *string
	if c.LastIP != "" {
		v := c.LastIP
		ip = &v
	}

	var detail NetworkClientDetail
	if c.IsWired {
		conn := NetworkConnection{}
		if dev, ok := macToDevice[normalizeMac(c.LastUplinkMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: Wired,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Offline,
			ConnectedTo:    conn,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build offline wired client detail: %w", err)
		}
	} else {
		conn := WirelessConnection{}
		if dev, ok := macToDevice[normalizeMac(c.LastUplinkMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		if c.ESSID != nil {
			conn.Ssid = *c.ESSID
		}
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: WirelessNetworkClientDetailConnectionTypeWireless,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Offline,
			ConnectedTo:    conn,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build offline wireless client detail: %w", err)
		}
	}
	return detail, nil
}

func clientToList(controller string, sta adapters.UniFiSta) NetworkClient {
	mac := normalizeMac(sta.MAC)
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	client := NetworkClient{
		Id:             id,
		Uri:            fmt.Sprintf("/network/clients/%s", id),
		Name:           clientName(sta),
		Mac:            mac,
		ConnectionType: mapConnectionType(sta.IsWired),
		Status:         Online,
	}
	if sta.IP != "" {
		ip := sta.IP
		client.Ip = &ip
	}
	return client
}

func clientToDetail(controller string, sta adapters.UniFiSta, macToDevice map[string]adapters.UniFiDevice) (NetworkClientDetail, error) {
	mac := normalizeMac(sta.MAC)
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	name := clientName(sta)
	var ip *string
	if sta.IP != "" {
		v := sta.IP
		ip = &v
	}

	var detail NetworkClientDetail
	if sta.IsWired {
		conn := NetworkConnection{}
		if dev, ok := macToDevice[normalizeMac(sta.SwMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		if sta.SwPort > 0 {
			port := sta.SwPort
			conn.Port = &port
		}
		if sta.WiredRateMbps > 0 {
			ls := mapLinkSpeed(sta.WiredRateMbps)
			if ls != "" {
				conn.LinkSpeed = &ls
			}
		}
		uptime := sta.Uptime
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: Wired,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Online,
			ConnectedTo:    conn,
			Uptime:         &uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wired client detail: %w", err)
		}
	} else {
		conn := WirelessConnection{}
		if dev, ok := macToDevice[normalizeMac(sta.ApMAC)]; ok {
			conn.Device = deviceRef(controller, dev)
		}
		if sta.ESSID != nil {
			conn.Ssid = *sta.ESSID
		}
		conn.SignalStrength = sta.Signal
		uptime := sta.Uptime
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: WirelessNetworkClientDetailConnectionTypeWireless,
			Id:             id,
			Uri:            fmt.Sprintf("/network/clients/%s", id),
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Online,
			ConnectedTo:    conn,
			Uptime:         &uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wireless client detail: %w", err)
		}
	}
	return detail, nil
}

// clientSuffix returns the composite ID suffix for a client: {kebab-name}-{mac-prefix}.
func clientSuffix(sta adapters.UniFiSta) string {
	mac := normalizeMac(sta.MAC)
	prefix := macHexPrefix(mac, "")
	return fmt.Sprintf("%s-%s", toKebab(clientName(sta)), prefix)
}

// macHexPrefix returns the first two hex characters from a normalised MAC address
// (colons stripped). When the MAC is absent, it falls back to the first two
// characters of fallbackID. Returns "xx" if both are empty.
func macHexPrefix(mac, fallbackID string) string {
	hex := strings.ReplaceAll(mac, ":", "")
	if len(hex) >= 2 {
		return hex[:2]
	}
	id := strings.ReplaceAll(fallbackID, "-", "")
	if len(id) >= 2 {
		return id[:2]
	}
	return "xx"
}

func clientName(sta adapters.UniFiSta) string {
	if sta.Name != nil && *sta.Name != "" {
		return *sta.Name
	}
	if sta.Hostname != nil && *sta.Hostname != "" {
		return *sta.Hostname
	}
	return sta.MAC
}

func mapConnectionType(isWired bool) NetworkClientConnectionType {
	if isWired {
		return NetworkClientConnectionTypeWired
	}
	return NetworkClientConnectionTypeWireless
}
