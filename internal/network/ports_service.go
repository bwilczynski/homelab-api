package network

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// PortsBackend is the narrow interface for port listing.
type PortsBackend interface {
	GetDevices(ctx context.Context) ([]adapters.UniFiDevice, error)
	GetClients(ctx context.Context) ([]adapters.UniFiSta, error)
	GetNetworkConf(ctx context.Context) ([]adapters.UniFiNetworkConf, error)
}

// ListPorts returns all switch ports across all backends, filtered by params.
func (s *Service) ListPorts(ctx context.Context, params ListNetworkPortsParams) (NetworkPortList, error) {
	var items []NetworkPort
	for _, entry := range s.backends {
		if s.monitor != nil && !s.monitor.Available(entry.Name) {
			continue
		}
		devices, err := entry.Backend.GetDevices(ctx)
		if err != nil {
			s.logger.Warn("skipping backend on list ports error", "controller", entry.Name, "err", err)
			continue
		}
		clients, err := entry.Backend.GetClients(ctx)
		if err != nil {
			s.logger.Warn("skipping backend on list ports error", "controller", entry.Name, "err", err)
			continue
		}
		confs, err := entry.Backend.GetNetworkConf(ctx)
		if err != nil {
			s.logger.Warn("skipping backend on list ports error", "controller", entry.Name, "err", err)
			continue
		}
		swPortToDevice := buildSwPortToDevice(devices)
		swPortToClient := buildSwPortToClient(clients)
		for _, d := range devices {
			if d.Type != "usw" {
				continue
			}
			switchID := fmt.Sprintf("%s.%s", entry.Name, toKebab(d.Name))
			sw := NetworkDeviceRef{
				Kind: NetworkDeviceRefKindDevice,
				Id:   switchID,
				Uri:  fmt.Sprintf("/network/devices/%s", switchID),
				Name: d.Name,
			}
			ports := buildDevicePorts(entry.Name, d, swPortToDevice, swPortToClient, confs)
			for _, p := range ports {
				np := toNetworkPort(p, sw)
				if matchesFilter(np, params) {
					items = append(items, np)
				}
			}
		}
	}
	if items == nil {
		items = []NetworkPort{}
	}
	return NetworkPortList{Items: items}, nil
}

func toNetworkPort(port DevicePort, sw NetworkDeviceRef) NetworkPort {
	return NetworkPort{
		Number:           port.Number,
		Label:            port.Label,
		State:            port.State,
		LinkSpeed:        port.LinkSpeed,
		LinkUptime:       port.LinkUptime,
		SfpModulePresent: port.SfpModulePresent,
		PoeMode:          port.PoeMode,
		PoePowerWatts:    port.PoePowerWatts,
		LagMembership:    port.LagMembership,
		VlanConfig:       port.VlanConfig,
		Traffic:          port.Traffic,
		ConnectedTo:      port.ConnectedTo,
		Device:           sw,
	}
}

func matchesFilter(port NetworkPort, params ListNetworkPortsParams) bool {
	if params.DeviceId != nil && port.Device.Id != *params.DeviceId {
		return false
	}
	if params.State != nil && port.State != *params.State {
		return false
	}
	if params.Mode != nil {
		if port.VlanConfig == nil || port.VlanConfig.Mode != *params.Mode {
			return false
		}
	}
	if params.VlanId != nil {
		if !matchesVlanID(port.VlanConfig, *params.VlanId) {
			return false
		}
	}
	return true
}

// matchesVlanID reports whether the given VLAN ID is carried by the port.
// Implements the three-way spec rule: native match OR tagged-custom item match OR trunk-all (scope=all).
// Returns false when cfg is nil (disabled / unresolvable ports).
func matchesVlanID(cfg *DevicePortVlanConfig, vlanID int) bool {
	if cfg == nil {
		return false
	}
	if cfg.NativeVlan.VlanId == vlanID {
		return true
	}
	if cfg.TaggedVlans == nil {
		return false
	}
	if cfg.TaggedVlans.Scope == All {
		return true
	}
	if cfg.TaggedVlans.Items != nil {
		for _, v := range *cfg.TaggedVlans.Items {
			if v.VlanId == vlanID {
				return true
			}
		}
	}
	return false
}
