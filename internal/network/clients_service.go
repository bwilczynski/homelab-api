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
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	id := fmt.Sprintf("%s.%s-%s", controller, toKebab(name), prefix)

	var ip string
	if c.Status == "online" {
		ip = c.IP
	} else {
		ip = c.LastIP
	}

	client := NetworkClient{
		Id:             id,
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

	raw, err := backend.GetClients()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}

	for _, sta := range raw {
		if clientSuffix(sta) == suffix {
			detail, err := clientToDetail(controller, sta)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkClientDetail{}, false, nil
}

func clientToList(controller string, sta adapters.UniFiSta) NetworkClient {
	mac := normalizeMac(sta.MAC)
	client := NetworkClient{
		Id:             fmt.Sprintf("%s.%s", controller, clientSuffix(sta)),
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

func clientToDetail(controller string, sta adapters.UniFiSta) (NetworkClientDetail, error) {
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
		switchName := sta.LastUplinkName
		switchPort := sta.SwPort
		uptime := sta.Uptime
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Online,
			SwitchName:     &switchName,
			SwitchPort:     &switchPort,
			Uptime:         &uptime,
		})
		if err != nil {
			return NetworkClientDetail{}, fmt.Errorf("build wired client detail: %w", err)
		}
	} else {
		ssid := ""
		if sta.ESSID != nil {
			ssid = *sta.ESSID
		}
		signal := 0
		if sta.Signal != nil {
			signal = *sta.Signal
		}
		uptime := sta.Uptime
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: Wireless,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Status:         Online,
			Ssid:           &ssid,
			SignalStrength:  &signal,
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
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	return fmt.Sprintf("%s-%s", toKebab(clientName(sta)), prefix)
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
