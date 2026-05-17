package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// TopologyBackend is the narrow interface for topology operations.
// It is a subset of UniFiBackend, so all existing backends satisfy it.
type TopologyBackend interface {
	GetDevices() ([]adapters.UniFiDevice, error)
	GetClients() ([]adapters.UniFiSta, error)
	GetOfflineClients(historyDays int) ([]adapters.UniFiClientV2, error)
}

// GetTopology builds the network topology graph.
// Pass 1 always: device nodes + device-to-device uplink edges.
// Passes 2+3 when includeClients=true: online client nodes (V1 stas) and
// offline client nodes (V2 history) with their wired/wireless edges.
func (s *Service) GetTopology(ctx context.Context, includeClients bool) (NetworkTopology, error) {
	var nodes []TopologyNode
	var edges []TopologyEdge

	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}

		devices, err := cb.unifi.GetDevices()
		if err != nil {
			return NetworkTopology{}, fmt.Errorf("get unifi devices from %s: %w", cb.controller, err)
		}
		macToDevice := buildMacToDevice(devices)

		// Pass 1: device nodes + device-device uplink edges.
		for _, d := range devices {
			node, err := buildDeviceNode(cb.controller, d)
			if err != nil {
				return NetworkTopology{}, fmt.Errorf("build device node: %w", err)
			}
			nodes = append(nodes, node)

			if d.Uplink != nil && d.Uplink.UplinkMAC != "" {
				if upstream, ok := macToDevice[normalizeMac(d.Uplink.UplinkMAC)]; ok {
					edge, err := buildDeviceUplinkEdge(cb.controller, d, upstream)
					if err != nil {
						return NetworkTopology{}, fmt.Errorf("build uplink edge: %w", err)
					}
					edges = append(edges, edge)
				}
			}
		}

		if !includeClients {
			continue
		}

		stas, err := cb.unifi.GetClients()
		if err != nil {
			return NetworkTopology{}, fmt.Errorf("get unifi clients from %s: %w", cb.controller, err)
		}

		offline, err := cb.unifi.GetOfflineClients(s.historyDays)
		if err != nil {
			return NetworkTopology{}, fmt.Errorf("get offline clients from %s: %w", cb.controller, err)
		}

		// Pass 2: online client nodes + edges.
		for _, sta := range stas {
			node, err := buildOnlineClientNode(cb.controller, sta)
			if err != nil {
				return NetworkTopology{}, fmt.Errorf("build client node: %w", err)
			}
			nodes = append(nodes, node)
			if edge := buildOnlineClientEdge(cb.controller, sta, macToDevice); edge != nil {
				edges = append(edges, *edge)
			}
		}

		// Pass 3: offline client nodes + edges.
		for _, c := range offline {
			node, err := buildOfflineClientNode(cb.controller, c)
			if err != nil {
				return NetworkTopology{}, fmt.Errorf("build offline client node: %w", err)
			}
			nodes = append(nodes, node)
			if edge := buildOfflineClientEdge(cb.controller, c, macToDevice); edge != nil {
				edges = append(edges, *edge)
			}
		}
	}

	if nodes == nil {
		nodes = []TopologyNode{}
	}
	if edges == nil {
		edges = []TopologyEdge{}
	}
	return NetworkTopology{Nodes: nodes, Edges: edges}, nil
}

// buildDeviceNode converts a UniFiDevice to a TopologyDeviceNode wrapped in TopologyNode.
// numClients is populated only for access points (type=uap).
func buildDeviceNode(controller string, d adapters.UniFiDevice) (TopologyNode, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	devNode := TopologyDeviceNode{
		Kind:   TopologyDeviceNodeKindDevice,
		Id:     id,
		Uri:    fmt.Sprintf("/network/devices/%s", id),
		Name:   d.Name,
		Type:   mapDeviceType(d.Type),
		Status: mapDeviceStatus(d.State),
	}
	if d.Type == "uap" {
		n := d.UserNumSta + d.GuestNumSta
		devNode.NumClients = &n
	}
	var node TopologyNode
	err := node.FromTopologyDeviceNode(devNode)
	return node, err
}

// buildDeviceUplinkEdge builds a wired edge from d to its upstream device.
// Port is omitted when UplinkRemotePort is nil.
func buildDeviceUplinkEdge(controller string, d adapters.UniFiDevice, upstream adapters.UniFiDevice) (TopologyEdge, error) {
	srcRef := deviceRef(controller, d)
	var source NetworkConnectionRef
	if err := source.FromNetworkDeviceRef(srcRef); err != nil {
		return TopologyEdge{}, err
	}
	tgtRef := deviceRef(controller, upstream)

	wired := TopologyWiredEdge{
		Kind:   TopologyWiredEdgeKindWired,
		Source: source,
		Target: tgtRef,
	}
	if d.Uplink.UplinkRemotePort != nil {
		port := *d.Uplink.UplinkRemotePort
		wired.Port = &port
	}
	if d.Uplink.Speed > 0 {
		if ls := mapLinkSpeed(d.Uplink.Speed); ls != "" {
			wired.LinkSpeed = &ls
		}
	}

	var edge TopologyEdge
	err := edge.FromTopologyWiredEdge(wired)
	return edge, err
}

// buildOnlineClientNode wraps a V1 sta as a TopologyClientNode (status=online).
func buildOnlineClientNode(controller string, sta adapters.UniFiSta) (TopologyNode, error) {
	ref := clientRef(controller, sta)
	clientNode := TopologyClientNode{
		Kind:           TopologyClientNodeKindClient,
		Id:             ref.Id,
		Uri:            ref.Uri,
		Name:           ref.Name,
		ConnectionType: mapConnectionType(sta.IsWired),
		Status:         Online,
	}
	var node TopologyNode
	err := node.FromTopologyClientNode(clientNode)
	return node, err
}

// buildOnlineClientEdge builds a wired or wireless edge for an online V1 sta.
// Returns nil when the upstream device cannot be resolved.
func buildOnlineClientEdge(controller string, sta adapters.UniFiSta, macToDevice map[string]adapters.UniFiDevice) *TopologyEdge {
	ref := clientRef(controller, sta)
	var source NetworkConnectionRef
	if err := source.FromNetworkClientRef(ref); err != nil {
		return nil
	}

	if sta.IsWired {
		if sta.SwMAC == "" {
			return nil
		}
		upstream, ok := macToDevice[normalizeMac(sta.SwMAC)]
		if !ok {
			return nil
		}
		wired := TopologyWiredEdge{
			Kind:   TopologyWiredEdgeKindWired,
			Source: source,
			Target: deviceRef(controller, upstream),
		}
		if sta.SwPort > 0 {
			port := sta.SwPort
			wired.Port = &port
		}
		if sta.WiredRateMbps > 0 {
			if ls := mapLinkSpeed(sta.WiredRateMbps); ls != "" {
				wired.LinkSpeed = &ls
			}
		}
		var edge TopologyEdge
		if err := edge.FromTopologyWiredEdge(wired); err != nil {
			return nil
		}
		return &edge
	}

	// wireless
	if sta.ApMAC == "" || sta.ESSID == nil {
		return nil
	}
	upstream, ok := macToDevice[normalizeMac(sta.ApMAC)]
	if !ok {
		return nil
	}
	wireless := TopologyWirelessEdge{
		Kind:   TopologyWirelessEdgeKindWireless,
		Source: ref,
		Target: deviceRef(controller, upstream),
		Ssid:   *sta.ESSID,
	}
	if sta.Signal != nil {
		wireless.SignalStrength = sta.Signal
	}
	var edge TopologyEdge
	if err := edge.FromTopologyWirelessEdge(wireless); err != nil {
		return nil
	}
	return &edge
}

// buildOfflineClientNode wraps a V2 history client as a TopologyClientNode (status=offline).
func buildOfflineClientNode(controller string, c adapters.UniFiClientV2) (TopologyNode, error) {
	ref := clientRefV2(controller, c)
	clientNode := TopologyClientNode{
		Kind:           TopologyClientNodeKindClient,
		Id:             ref.Id,
		Uri:            ref.Uri,
		Name:           ref.Name,
		ConnectionType: mapConnectionType(c.IsWired),
		Status:         Offline,
	}
	var node TopologyNode
	err := node.FromTopologyClientNode(clientNode)
	return node, err
}

// buildOfflineClientEdge builds a wired or wireless edge for an offline V2 client.
// Port and signalStrength are always omitted (no live measurement for offline clients).
// Returns nil when LastUplinkMAC is absent or does not resolve to a known device.
func buildOfflineClientEdge(controller string, c adapters.UniFiClientV2, macToDevice map[string]adapters.UniFiDevice) *TopologyEdge {
	if c.LastUplinkMAC == "" {
		return nil
	}
	upstream, ok := macToDevice[normalizeMac(c.LastUplinkMAC)]
	if !ok {
		return nil
	}
	ref := clientRefV2(controller, c)
	tgtRef := deviceRef(controller, upstream)

	if c.IsWired {
		var source NetworkConnectionRef
		if err := source.FromNetworkClientRef(ref); err != nil {
			return nil
		}
		wired := TopologyWiredEdge{
			Kind:   TopologyWiredEdgeKindWired,
			Source: source,
			Target: tgtRef,
			// Port omitted: offline client has no live connection.
		}
		var edge TopologyEdge
		if err := edge.FromTopologyWiredEdge(wired); err != nil {
			return nil
		}
		return &edge
	}

	if c.ESSID == nil {
		return nil
	}
	wireless := TopologyWirelessEdge{
		Kind:   TopologyWirelessEdgeKindWireless,
		Source: ref,
		Target: tgtRef,
		Ssid:   *c.ESSID,
		// SignalStrength omitted: no live measurement for offline clients.
	}
	var edge TopologyEdge
	if err := edge.FromTopologyWirelessEdge(wireless); err != nil {
		return nil
	}
	return &edge
}

// clientRefV2 builds a NetworkClientRef from a V2 client using the same ID scheme
// as clientToListV2: "{controller}.{kebab-name}-{mac-prefix-2chars}".
func clientRefV2(controller string, c adapters.UniFiClientV2) NetworkClientRef {
	name := clientNameV2(c)
	mac := normalizeMac(c.MAC)
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	id := fmt.Sprintf("%s.%s-%s", controller, toKebab(name), prefix)
	return NetworkClientRef{
		Kind: NetworkClientRefKindClient,
		Id:   id,
		Uri:  fmt.Sprintf("/network/clients/%s", id),
		Name: name,
	}
}
