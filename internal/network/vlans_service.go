package network

import (
	"context"
	"fmt"
	"net"
	"slices"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// VLANsBackend is the narrow interface for VLAN operations.
type VLANsBackend interface {
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
}

// ListVLANs returns all LAN networks from all backends as a flat list.
func (s *Service) ListVLANs(ctx context.Context) (VlanList, error) {
	var items []Vlan
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		networks, err := cb.unifi.GetNetworkConf()
		if err != nil {
			return VlanList{}, fmt.Errorf("get network conf from %s: %w", cb.controller, err)
		}
		for _, n := range networks {
			if !isLanNetwork(n) {
				continue
			}
			items = append(items, networkToVlan(cb.controller, n))
		}
	}
	if items == nil {
		items = []Vlan{}
	}
	return VlanList{Items: items}, nil
}

// GetVLAN looks up a single VLAN by composite ID.
func (s *Service) GetVLAN(ctx context.Context, id string) (VlanDetail, bool, error) {
	controller, name, ok := parseID(id)
	if !ok {
		return VlanDetail{}, false, nil
	}
	backend, err := s.findBackend(controller)
	if err != nil {
		return VlanDetail{}, false, nil
	}
	networks, err := backend.GetNetworkConf()
	if err != nil {
		return VlanDetail{}, false, fmt.Errorf("get network conf: %w", err)
	}
	for _, n := range networks {
		if !isLanNetwork(n) {
			continue
		}
		if toKebab(n.Name) == name {
			return buildVlanDetail(controller, n), true, nil
		}
	}
	return VlanDetail{}, false, nil
}

// isLanNetwork returns true for LAN-type network entries (excludes WAN).
func isLanNetwork(n adapters.UniFiNetworkConf) bool {
	return n.Purpose == "corporate" || n.Purpose == "guest"
}

func networkToVlan(controller string, n adapters.UniFiNetworkConf) Vlan {
	id := fmt.Sprintf("%s.%s", controller, toKebab(n.Name))
	subnet, _, _ := parseIPSubnet(n.IPSubnet)
	return Vlan{
		Id:     id,
		Uri:    fmt.Sprintf("/network/vlans/%s", id),
		Name:   n.Name,
		VlanId: extractVlanID(n.Vlan),
		Subnet: subnet,
	}
}

func buildVlanDetail(controller string, n adapters.UniFiNetworkConf) VlanDetail {
	id := fmt.Sprintf("%s.%s", controller, toKebab(n.Name))
	subnet, gatewayIP, broadcastIP := parseIPSubnet(n.IPSubnet)
	dhcpMode := mapDhcpMode(n.DhcpdEnabled, n.DHCPRelayEnabled)

	detail := VlanDetail{
		Id:          id,
		Uri:         fmt.Sprintf("/network/vlans/%s", id),
		Name:        n.Name,
		VlanId:      extractVlanID(n.Vlan),
		Subnet:      subnet,
		GatewayIp:   gatewayIP,
		BroadcastIp: broadcastIP,
		DhcpMode:    dhcpMode,
		DnsServers:  collectDNSServers(n.DhcpdDNS1, n.DhcpdDNS2),
	}
	if dhcpMode == DhcpModeServer {
		r := DhcpRange{Start: n.DhcpdStart, End: n.DhcpdStop}
		detail.DhcpRange = &r
	}
	if dhcpMode == DhcpModeRelay {
		detail.RelayServer = &n.DhcpdStart
	}
	return detail
}

// --- helpers shared across service files ---

// parseIPSubnet splits a UniFi ip_subnet ("192.168.1.1/24") into
// subnet CIDR ("192.168.1.0/24"), gateway IP ("192.168.1.1"),
// and broadcast IP ("192.168.1.255").
func parseIPSubnet(ipSubnet string) (subnet, gatewayIP, broadcastIP string) {
	if ipSubnet == "" {
		return "", "", ""
	}
	ip, ipnet, err := net.ParseCIDR(ipSubnet)
	if err != nil {
		return ipSubnet, "", ""
	}
	gatewayIP = ip.String()
	subnet = ipnet.String()
	network := ipnet.IP.To4()
	mask := ipnet.Mask
	broadcast := make(net.IP, 4)
	for i := range 4 {
		broadcast[i] = network[i] | ^mask[i]
	}
	broadcastIP = broadcast.String()
	return
}

// extractVlanID returns the integer VLAN tag from a UniFi vlan field.
// Returns 1 (native/untagged VLAN) when the value is absent, null, or empty string.
func extractVlanID(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 1
}

func mapDhcpMode(dhcpdEnabled, relayEnabled bool) DhcpMode {
	switch {
	case dhcpdEnabled:
		return DhcpModeServer
	case relayEnabled:
		return DhcpModeRelay
	default:
		return DhcpModeDisabled
	}
}

func collectDNSServers(dns ...string) []string {
	result := slices.DeleteFunc(slices.Clone(dns), func(s string) bool { return s == "" })
	if result == nil {
		result = []string{}
	}
	return result
}
