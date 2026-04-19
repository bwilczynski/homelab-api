package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// UniFiBackend defines the adapter interface for UniFi network operations.
type UniFiBackend interface {
	GetDevices() ([]adapters.UniFiDevice, error)
	GetClients() ([]adapters.UniFiSta, error)
}

// Service implements network domain business logic.
type Service struct {
	controller string
	unifi      UniFiBackend
}

// NewService creates a new network service.
func NewService(controller string, unifi UniFiBackend) *Service {
	return &Service{controller: controller, unifi: unifi}
}

// ListDevices retrieves all managed network devices.
func (s *Service) ListDevices(ctx context.Context) (NetworkDeviceList, error) {
	raw, err := s.unifi.GetDevices()
	if err != nil {
		return NetworkDeviceList{}, fmt.Errorf("get unifi devices: %w", err)
	}

	items := make([]NetworkDevice, 0, len(raw))
	for _, d := range raw {
		mac := normalizeMac(d.MAC)
		dev := NetworkDevice{
			Id:              fmt.Sprintf("%s.%s", s.controller, strings.ReplaceAll(mac, ":", "")),
			Name:            d.Name,
			Mac:             mac,
			Ip:              d.IP,
			Type:            mapDeviceType(d.Type),
			Model:           d.Model,
			FirmwareVersion: d.Version,
			Status:          mapDeviceStatus(d.State),
			Uptime:          d.Uptime,
		}
		total := d.UserNumSta + d.GuestNumSta
		if total > 0 {
			dev.NumClients = &total
		}
		items = append(items, dev)
	}

	return NetworkDeviceList{Items: items}, nil
}

// ListClients retrieves all currently connected network clients.
func (s *Service) ListClients(ctx context.Context) (NetworkClientList, error) {
	raw, err := s.unifi.GetClients()
	if err != nil {
		return NetworkClientList{}, fmt.Errorf("get unifi clients: %w", err)
	}

	items := make([]NetworkClient, 0, len(raw))
	for _, sta := range raw {
		mac := normalizeMac(sta.MAC)
		client := NetworkClient{
			Id:             fmt.Sprintf("%s.%s", s.controller, strings.ReplaceAll(mac, ":", "")),
			Name:           clientName(sta),
			Mac:            mac,
			ConnectionType: mapConnectionType(sta.IsWired),
			Uptime:         sta.Uptime,
		}
		if sta.IP != "" {
			ip := sta.IP
			client.Ip = &ip
		}
		if !sta.IsWired && sta.ESSID != nil {
			client.Ssid = sta.ESSID
			client.SignalStrength = sta.Signal
		}
		items = append(items, client)
	}

	return NetworkClientList{Items: items}, nil
}

func mapDeviceType(t string) NetworkDeviceType {
	switch t {
	case "uap":
		return AccessPoint
	case "usw":
		return Switch
	case "ugw", "udm", "udm-pro":
		return Gateway
	default:
		return Unknown
	}
}

func mapDeviceStatus(state int) NetworkDeviceStatus {
	if state == 1 {
		return Connected
	}
	return Disconnected
}

func mapConnectionType(isWired bool) NetworkClientConnectionType {
	if isWired {
		return Wired
	}
	return Wireless
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

// normalizeMac ensures the MAC address is lowercase.
func normalizeMac(mac string) string {
	return strings.ToLower(mac)
}
