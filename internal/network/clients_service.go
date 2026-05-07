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
}

// ListClients retrieves all connected clients from all backends.
func (s *Service) ListClients(ctx context.Context) (NetworkClientList, error) {
	var items []NetworkClient
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		raw, err := cb.unifi.GetClients()
		if err != nil {
			return NetworkClientList{}, fmt.Errorf("get unifi clients from %s: %w", cb.controller, err)
		}
		for _, sta := range raw {
			items = append(items, clientToList(cb.controller, sta))
		}
	}
	if items == nil {
		items = []NetworkClient{}
	}
	return NetworkClientList{Items: items}, nil
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
		err := detail.FromWiredNetworkClientDetail(WiredNetworkClientDetail{
			ConnectionType: WiredNetworkClientDetailConnectionTypeWired,
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			SwitchName:     sta.LastUplinkName,
			SwitchPort:     sta.SwPort,
			Uptime:         sta.Uptime,
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
		err := detail.FromWirelessNetworkClientDetail(WirelessNetworkClientDetail{
			ConnectionType: Wireless, // WirelessNetworkClientDetailConnectionType constant
			Id:             id,
			Name:           name,
			Mac:            mac,
			Ip:             ip,
			Ssid:           ssid,
			SignalStrength:  signal,
			Uptime:         sta.Uptime,
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
