package network

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// DevicesBackend is the narrow interface for device operations.
type DevicesBackend interface {
	GetDevices() ([]adapters.UniFiDevice, error)
}

// ListDevices retrieves all managed network devices from all backends.
func (s *Service) ListDevices(ctx context.Context) (NetworkDeviceList, error) {
	var items []NetworkDevice
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		raw, err := cb.unifi.GetDevices()
		if err != nil {
			return NetworkDeviceList{}, fmt.Errorf("get unifi devices from %s: %w", cb.controller, err)
		}
		for _, d := range raw {
			items = append(items, deviceToList(cb.controller, d))
		}
	}
	if items == nil {
		items = []NetworkDevice{}
	}
	return NetworkDeviceList{Items: items}, nil
}

// GetDevice looks up a single device by composite ID and returns its detail.
func (s *Service) GetDevice(ctx context.Context, id string) (NetworkDeviceDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok {
		return NetworkDeviceDetail{}, false, nil
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkDeviceDetail{}, false, nil
	}

	raw, err := backend.GetDevices()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}

	for _, d := range raw {
		if toKebab(d.Name) == suffix {
			return deviceToDetail(controller, d), true, nil
		}
	}
	return NetworkDeviceDetail{}, false, nil
}

func deviceToList(controller string, d adapters.UniFiDevice) NetworkDevice {
	mac := normalizeMac(d.MAC)
	dev := NetworkDevice{
		Id:     fmt.Sprintf("%s.%s", controller, toKebab(d.Name)),
		Name:   d.Name,
		Mac:    mac,
		Ip:     d.IP,
		Type:   mapDeviceType(d.Type),
		Status: mapDeviceStatus(d.State),
	}
	if d.Type == "uap" {
		total := d.UserNumSta + d.GuestNumSta
		dev.NumClients = &total
	}
	return dev
}

func deviceToDetail(controller string, d adapters.UniFiDevice) NetworkDeviceDetail {
	mac := normalizeMac(d.MAC)
	det := NetworkDeviceDetail{
		Id:              fmt.Sprintf("%s.%s", controller, toKebab(d.Name)),
		Name:            d.Name,
		Mac:             mac,
		Ip:              d.IP,
		Type:            mapDeviceType(d.Type),
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
	}
	if d.Type == "uap" {
		total := d.UserNumSta + d.GuestNumSta
		det.NumClients = &total
	}
	return det
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
