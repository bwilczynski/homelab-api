package network

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// WANsBackend is the narrow interface for WAN operations.
type WANsBackend interface {
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}

// ListWANs returns all WAN interfaces from all backends.
func (s *Service) ListWANs(ctx context.Context) (WanList, error) {
	var items []Wan
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		networks, err := cb.unifi.GetNetworkConf()
		if err != nil {
			return WanList{}, fmt.Errorf("get network conf from %s: %w", cb.controller, err)
		}
		devices, err := cb.unifi.GetDevices()
		if err != nil {
			return WanList{}, fmt.Errorf("get devices from %s: %w", cb.controller, err)
		}
		gateway := findGateway(devices)
		for _, n := range networks {
			if n.Purpose != "wan" {
				continue
			}
			iface := resolveWanIface(gateway, n.WanNetworkGroup)
			items = append(items, buildWan(cb.controller, n, iface, gateway))
		}
	}
	if items == nil {
		items = []Wan{}
	}
	return WanList{Items: items}, nil
}

// GetWAN looks up a single WAN interface by composite ID.
func (s *Service) GetWAN(ctx context.Context, id string) (WanDetail, bool, error) {
	controller, name, ok := parseID(id)
	if !ok {
		return WanDetail{}, false, nil
	}
	backend, err := s.findBackend(controller)
	if err != nil {
		return WanDetail{}, false, nil
	}
	networks, err := backend.GetNetworkConf()
	if err != nil {
		return WanDetail{}, false, fmt.Errorf("get network conf: %w", err)
	}
	devices, err := backend.GetDevices()
	if err != nil {
		return WanDetail{}, false, fmt.Errorf("get devices: %w", err)
	}
	gateway := findGateway(devices)
	for _, n := range networks {
		if n.Purpose != "wan" {
			continue
		}
		if toKebab(n.Name) == name {
			iface := resolveWanIface(gateway, n.WanNetworkGroup)
			return buildWanDetail(controller, n, iface, gateway), true, nil
		}
	}
	return WanDetail{}, false, nil
}

// findGateway returns the first gateway-type device from the device list, or nil.
func findGateway(devices []adapters.UniFiDevice) *adapters.UniFiDevice {
	for i := range devices {
		switch devices[i].Type {
		case "ugw", "udm", "udm-pro":
			return &devices[i]
		}
	}
	return nil
}

// resolveWanIface returns the WAN interface for a given networkgroup ("WAN" or "WAN2").
func resolveWanIface(gw *adapters.UniFiDevice, networkGroup string) *adapters.UniFiWanIface {
	if gw == nil {
		return nil
	}
	switch networkGroup {
	case "WAN":
		return gw.Wan1
	case "WAN2":
		return gw.Wan2
	}
	return nil
}

func buildWan(controller string, n adapters.UniFiNetworkConf, iface *adapters.UniFiWanIface, gw *adapters.UniFiDevice) Wan {
	id := fmt.Sprintf("%s.%s", controller, toKebab(n.Name))
	ip, status, uptime := wanLiveFields(iface, gw)
	return Wan{
		Id:        id,
		Uri:       fmt.Sprintf("/network/wans/%s", id),
		Name:      n.Name,
		IpAddress: ip,
		Status:    status,
		Uptime:    uptime,
	}
}

func buildWanDetail(controller string, n adapters.UniFiNetworkConf, iface *adapters.UniFiWanIface, gw *adapters.UniFiDevice) WanDetail {
	id := fmt.Sprintf("%s.%s", controller, toKebab(n.Name))
	ip, status, uptime := wanLiveFields(iface, gw)
	dns := wanDNSServers(iface, n)
	return WanDetail{
		Id:         id,
		Uri:        fmt.Sprintf("/network/wans/%s", id),
		Name:       n.Name,
		IpAddress:  ip,
		Status:     status,
		Uptime:     uptime,
		DnsServers: dns,
	}
}

// wanLiveFields extracts IP, status, and uptime from the live WAN interface and gateway.
// Uptime is the gateway device uptime — wan1 does not expose a per-interface uptime in the API.
func wanLiveFields(iface *adapters.UniFiWanIface, gw *adapters.UniFiDevice) (ip string, status WanStatus, uptime int) {
	if iface != nil {
		ip = iface.IP
		if iface.Up {
			status = WanStatusConnected
		} else {
			status = WanStatusDisconnected
		}
	} else {
		status = WanStatusDisconnected
	}
	if gw != nil {
		uptime = gw.Uptime
	}
	return
}

// wanDNSServers returns DNS servers for a WAN.
// Prefers configured wan_dns1/wan_dns2 from networkconf; falls back to the live
// interface DNS. The live value (wan1.dns) often reflects the gateway's local
// resolver (127.0.0.1) rather than the upstream servers the user configured.
func wanDNSServers(iface *adapters.UniFiWanIface, n adapters.UniFiNetworkConf) []string {
	if configured := collectDNSServers(n.WanDNS1, n.WanDNS2); len(configured) > 0 {
		return configured
	}
	if iface != nil {
		return collectDNSServers(iface.DNS...)
	}
	return []string{}
}
