package network

import (
	"context"
	"fmt"
	"strconv"

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

	devices, err := backend.GetDevices()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}

	clients, err := backend.GetClients()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}

	macToDevice := buildMacToDevice(devices)
	swPortToDevice := buildSwPortToDevice(devices)
	swPortToClient := buildSwPortToClient(clients)
	apMacToClients := buildApMacToClients(clients)

	for _, d := range devices {
		if toKebab(d.Name) == suffix {
			detail, err := buildDeviceDetail(controller, d, macToDevice, swPortToDevice, swPortToClient, apMacToClients)
			if err != nil {
				return NetworkDeviceDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkDeviceDetail{}, false, nil
}

func deviceToList(controller string, d adapters.UniFiDevice) NetworkDevice {
	mac := normalizeMac(d.MAC)
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	return NetworkDevice{
		Id:     id,
		Uri:    fmt.Sprintf("/network/devices/%s", id),
		Name:   d.Name,
		Mac:    mac,
		Ip:     d.IP,
		Type:   mapDeviceType(d.Type),
		Status: mapDeviceStatus(d.State),
	}
}

func buildDeviceDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	apMacToClients map[string][]adapters.UniFiSta,
) (NetworkDeviceDetail, error) {
	switch d.Type {
	case "usw":
		return buildSwitchDetail(controller, d, macToDevice, swPortToDevice, swPortToClient)
	case "uap":
		return buildAPDetail(controller, d, macToDevice, apMacToClients)
	case "ugw", "udm", "udm-pro":
		return buildGatewayDetail(controller, d)
	default:
		return buildUnknownDetail(controller, d, macToDevice)
	}
}

func buildGatewayDetail(controller string, d adapters.UniFiDevice) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	var det NetworkDeviceDetail
	err := det.FromGatewayDetail(GatewayDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            Gateway,
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
	})
	return det, err
}

func buildUnknownDetail(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	var det NetworkDeviceDetail
	err := det.FromUnknownDeviceDetail(UnknownDeviceDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            Unknown,
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
		Uplink:          uplink,
	})
	return det, err
}

func buildSwitchDetail(
	controller string,
	d adapters.UniFiDevice,
	macToDevice map[string]adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	ports := buildSwitchPorts(controller, d, swPortToDevice, swPortToClient)

	var det NetworkDeviceDetail
	err := det.FromSwitchDetail(SwitchDetail{
		Id:              id,
		Uri:             fmt.Sprintf("/network/devices/%s", id),
		Name:            d.Name,
		Mac:             normalizeMac(d.MAC),
		Ip:              d.IP,
		Type:            Switch,
		Status:          mapDeviceStatus(d.State),
		Model:           d.Model,
		FirmwareVersion: d.Version,
		Uptime:          d.Uptime,
		Traffic:         deviceTraffic(d),
		Uplink:          uplink,
		Ports:           ports,
	})
	return det, err
}

func buildSwitchPorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
) []SwitchPort {
	ports := make([]SwitchPort, 0, len(d.PortTable))
	switchMAC := normalizeMac(d.MAC)
	for _, p := range d.PortTable {
		port := SwitchPort{
			Number:  p.PortIdx,
			State:   mapPortState(p.Up),
			PoeMode: mapPoeMode(p.PoeMode),
			Traffic: NetworkTraffic{
				RxBytesTotal:  p.RxBytes,
				TxBytesTotal:  p.TxBytes,
				RxBytesPerSec: int64(p.RxBytesR),
				TxBytesPerSec: int64(p.TxBytesR),
			},
		}
		if p.Up && p.Speed > 0 {
			ls := mapLinkSpeed(p.Speed)
			if ls != "" {
				port.LinkSpeed = &ls
			}
		}
		if p.PortPoe && p.PoePower != "" {
			watts, err := strconv.ParseFloat(p.PoePower, 64)
			if err == nil {
				w := Watts(watts)
				port.PoePowerWatts = &w
			}
		}
		port.ConnectedTo = resolvePortConnectedTo(controller, switchMAC, p.PortIdx, swPortToDevice, swPortToClient)
		ports = append(ports, port)
	}
	return ports
}

func resolvePortConnectedTo(
	controller string,
	switchMAC string,
	portIdx int,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
) *NetworkConnectionRef {
	key := fmt.Sprintf("%s:%d", switchMAC, portIdx)
	if dev, ok := swPortToDevice[key]; ok {
		ref := deviceRef(controller, dev)
		var conn NetworkConnectionRef
		if err := conn.FromNetworkDeviceRef(ref); err == nil {
			return &conn
		}
	}
	if sta, ok := swPortToClient[key]; ok {
		ref := clientRef(controller, sta)
		var conn NetworkConnectionRef
		if err := conn.FromNetworkClientRef(ref); err == nil {
			return &conn
		}
	}
	return nil
}

func buildAPDetail(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice, apMacToClients map[string][]adapters.UniFiSta) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	apMAC := normalizeMac(d.MAC)
	stas := apMacToClients[apMAC]

	connectedClients := make([]AccessPointClient, 0, len(stas))
	for _, sta := range stas {
		ref := clientRef(controller, sta)
		apc := AccessPointClient{Client: ref}
		if sta.ESSID != nil {
			apc.Ssid = *sta.ESSID
		}
		if sta.Signal != nil {
			apc.SignalStrength = *sta.Signal
		}
		connectedClients = append(connectedClients, apc)
	}

	var det NetworkDeviceDetail
	err := det.FromAccessPointDetail(AccessPointDetail{
		Id:               id,
		Uri:              fmt.Sprintf("/network/devices/%s", id),
		Name:             d.Name,
		Mac:              normalizeMac(d.MAC),
		Ip:               d.IP,
		Type:             AccessPointDetailTypeAccessPoint,
		Status:           mapDeviceStatus(d.State),
		Model:            d.Model,
		FirmwareVersion:  d.Version,
		Uptime:           d.Uptime,
		Traffic:          deviceTraffic(d),
		Uplink:           uplink,
		NumClients:       len(connectedClients),
		ConnectedClients: connectedClients,
	})
	return det, err
}

// --- index helpers ---

func buildMacToDevice(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	m := make(map[string]adapters.UniFiDevice, len(devices))
	for _, d := range devices {
		m[normalizeMac(d.MAC)] = d
	}
	return m
}

func buildSwPortToDevice(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	m := make(map[string]adapters.UniFiDevice)
	for _, d := range devices {
		if d.Uplink == nil || d.Uplink.UplinkMAC == "" || d.Uplink.UplinkRemotePort == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", normalizeMac(d.Uplink.UplinkMAC), *d.Uplink.UplinkRemotePort)
		m[key] = d
	}
	return m
}

func buildSwPortToClient(clients []adapters.UniFiSta) map[string]adapters.UniFiSta {
	m := make(map[string]adapters.UniFiSta)
	for _, c := range clients {
		if !c.IsWired || c.SwMAC == "" || c.SwPort == 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", normalizeMac(c.SwMAC), c.SwPort)
		m[key] = c
	}
	return m
}

func buildApMacToClients(clients []adapters.UniFiSta) map[string][]adapters.UniFiSta {
	m := make(map[string][]adapters.UniFiSta)
	for _, c := range clients {
		if c.IsWired || c.ApMAC == "" {
			continue
		}
		mac := normalizeMac(c.ApMAC)
		m[mac] = append(m[mac], c)
	}
	return m
}

// --- ref helpers ---

func deviceRef(controller string, d adapters.UniFiDevice) NetworkDeviceRef {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	return NetworkDeviceRef{
		Kind: NetworkDeviceRefKindDevice,
		Id:   id,
		Uri:  fmt.Sprintf("/network/devices/%s", id),
		Name: d.Name,
	}
}

func clientRef(controller string, sta adapters.UniFiSta) NetworkClientRef {
	id := fmt.Sprintf("%s.%s", controller, clientSuffix(sta))
	return NetworkClientRef{
		Kind: NetworkClientRefKindClient,
		Id:   id,
		Uri:  fmt.Sprintf("/network/clients/%s", id),
		Name: clientName(sta),
	}
}

// --- traffic helpers ---

func deviceTraffic(d adapters.UniFiDevice) NetworkTraffic {
	switch d.Type {
	case "ugw", "udm", "udm-pro":
		if d.Wan1 != nil {
			return NetworkTraffic{
				RxBytesTotal:  d.Wan1.RxBytes,
				TxBytesTotal:  d.Wan1.TxBytes,
				RxBytesPerSec: int64(d.Wan1.RxBytesR),
				TxBytesPerSec: int64(d.Wan1.TxBytesR),
			}
		}
		return NetworkTraffic{}
	default:
		rxR, txR := 0.0, 0.0
		if d.Uplink != nil {
			rxR = d.Uplink.RxBytesR
			txR = d.Uplink.TxBytesR
		}
		return NetworkTraffic{
			RxBytesTotal:  d.RxBytes,
			TxBytesTotal:  d.TxBytes,
			RxBytesPerSec: int64(rxR),
			TxBytesPerSec: int64(txR),
		}
	}
}

// --- uplink helpers ---

func deviceUplink(controller string, d adapters.UniFiDevice, macToDevice map[string]adapters.UniFiDevice) *NetworkConnection {
	if d.Uplink == nil || d.Uplink.UplinkMAC == "" {
		return nil
	}
	upstream, ok := macToDevice[normalizeMac(d.Uplink.UplinkMAC)]
	if !ok {
		return nil
	}
	ref := deviceRef(controller, upstream)
	conn := &NetworkConnection{Device: ref}
	if d.Uplink.UplinkRemotePort != nil {
		port := *d.Uplink.UplinkRemotePort
		conn.Port = &port
	}
	if d.Uplink.Speed > 0 {
		ls := mapLinkSpeed(d.Uplink.Speed)
		if ls != "" {
			conn.LinkSpeed = &ls
		}
	}
	return conn
}

func mapLinkSpeed(mbps int) NetworkLinkSpeed {
	switch mbps {
	case 10:
		return "e"
	case 100:
		return "fe"
	case 1000:
		return "gbe1"
	case 2500:
		return "gbe2_5"
	case 5000:
		return "gbe5"
	case 10000:
		return "gbe10"
	default:
		return ""
	}
}

func mapPortState(up bool) NetworkPortState {
	if up {
		return "up"
	}
	return "down"
}

func mapPoeMode(mode string) SwitchPortPoeMode {
	switch mode {
	case "auto":
		return Auto
	case "passive24v":
		return Passive24v
	case "passthrough":
		return Passthrough
	default:
		return Off
	}
}

func mapDeviceType(t string) NetworkDeviceType {
	switch t {
	case "uap":
		return NetworkDeviceTypeAccessPoint
	case "usw":
		return NetworkDeviceTypeSwitch
	case "ugw", "udm", "udm-pro":
		return NetworkDeviceTypeGateway
	default:
		return NetworkDeviceTypeUnknown
	}
}

func mapDeviceStatus(state int) NetworkDeviceStatus {
	if state == 1 {
		return NetworkDeviceStatusConnected
	}
	return NetworkDeviceStatusDisconnected
}
