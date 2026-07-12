package network

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// DevicesBackend is the narrow interface for device operations.
type DevicesBackend interface {
	GetDevices(ctx context.Context) ([]adapters.UniFiDevice, error)
}

// ListDevices retrieves all managed network devices from all backends.
func (s *Service) ListDevices(ctx context.Context) (NetworkDeviceList, error) {
	var items []NetworkDevice
	for _, entry := range s.backends {
		if s.monitor != nil && !s.monitor.Available(entry.Name) {
			continue
		}
		raw, err := entry.Backend.GetDevices(ctx)
		if err != nil {
			// Network list methods have no device filter — always skip and warn.
			s.logger.Warn("skipping backend on list devices error", "controller", entry.Name, "err", err)
			continue
		}
		for _, d := range raw {
			items = append(items, deviceToList(entry.Name, d))
		}
	}
	if items == nil {
		items = []NetworkDevice{}
	}
	return NetworkDeviceList{Items: items}, nil
}

// GetDevice looks up a single device by composite ID and returns its detail.
func (s *Service) GetDevice(ctx context.Context, id string) (NetworkDeviceDetail, error) {
	controller, suffix, err := parseID(id)
	if err != nil {
		return NetworkDeviceDetail{}, err
	}

	backend, err := s.findBackend(controller)
	if err != nil {
		return NetworkDeviceDetail{}, err
	}

	devices, err := backend.GetDevices(ctx)
	if err != nil {
		return NetworkDeviceDetail{}, fmt.Errorf("get unifi devices: %w", err)
	}

	clients, err := backend.GetClients(ctx)
	if err != nil {
		return NetworkDeviceDetail{}, fmt.Errorf("get unifi clients: %w", err)
	}

	confs, err := backend.GetNetworkConf(ctx)
	if err != nil {
		return NetworkDeviceDetail{}, fmt.Errorf("get unifi network conf: %w", err)
	}

	macToDevice := buildMacToDevice(devices)
	swPortToDevice := buildSwPortToDevice(devices)
	swPortToClient := buildSwPortToClient(clients)
	apMacToClients := buildApMacToClients(clients)

	for _, d := range devices {
		if toKebab(d.Name) == suffix {
			detail, err := buildDeviceDetail(controller, d, macToDevice, swPortToDevice, swPortToClient, apMacToClients, confs)
			if err != nil {
				return NetworkDeviceDetail{}, err
			}
			return detail, nil
		}
	}
	return NetworkDeviceDetail{}, fmt.Errorf("Network device not found: %s: %w", id, apierrors.ErrNotFound)
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
	confs []adapters.UniFiNetworkConf,
) (NetworkDeviceDetail, error) {
	switch d.Type {
	case "usw":
		return buildSwitchDetail(controller, d, macToDevice, swPortToDevice, swPortToClient, confs)
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
	confs []adapters.UniFiNetworkConf,
) (NetworkDeviceDetail, error) {
	id := fmt.Sprintf("%s.%s", controller, toKebab(d.Name))
	uplink := deviceUplink(controller, d, macToDevice)
	ports := buildDevicePorts(controller, d, swPortToDevice, swPortToClient, confs)

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

func buildDevicePorts(
	controller string,
	d adapters.UniFiDevice,
	swPortToDevice map[string]adapters.UniFiDevice,
	swPortToClient map[string]adapters.UniFiSta,
	confs []adapters.UniFiNetworkConf,
) []DevicePort {
	// Build per-ID conf lookup and find the default (untagged) network ID.
	confByID := make(map[string]adapters.UniFiNetworkConf, len(confs))
	for _, c := range confs {
		confByID[c.ID] = c
	}
	defaultNetID := findDefaultNetID(confs)

	// Pass 1: find which port indices are LAG masters (have members pointing to them).
	masterSet := make(map[int]bool)
	for _, p := range d.PortTable {
		if v, ok := p.AggregatedBy.(float64); ok {
			masterSet[int(v)] = true
		}
	}

	switchMAC := normalizeMac(d.MAC)
	ports := make([]DevicePort, 0, len(d.PortTable))
	for _, p := range d.PortTable {
		port := DevicePort{
			Number:           p.PortIdx,
			State:            mapPortState(p.Up),
			Label:            buildPortLabel(p),
			SfpModulePresent: buildSfpModulePresent(p),
			LinkUptime:       buildLinkUptime(p),
			LagMembership:    buildLagMembership(p, masterSet),
			VlanConfig:       buildVlanConfig(p, confByID, defaultNetID, controller),
			Traffic: NetworkTraffic{
				RxBytesTotal:  p.RxBytes,
				TxBytesTotal:  p.TxBytes,
				RxBytesPerSec: int64(p.RxBytesR),
				TxBytesPerSec: int64(p.TxBytesR),
			},
		}
		if p.PortPoe {
			pm := mapPoeMode(p.PoeMode)
			port.PoeMode = &pm
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

// confToVlanRef converts a UniFiNetworkConf to a NetworkVlanRef using
// the same composite-ID convention as the VLANs service.
func confToVlanRef(conf adapters.UniFiNetworkConf, controller string) NetworkVlanRef {
	id := fmt.Sprintf("%s.%s", controller, toKebab(conf.Name))
	return NetworkVlanRef{
		Id:     id,
		Uri:    fmt.Sprintf("/network/vlans/%s", id),
		Name:   conf.Name,
		VlanId: extractVlanID(conf.Vlan),
	}
}

// buildVlanConfig maps UniFi port VLAN fields to the API DevicePortVlanConfig.
// Returns nil when the port is disabled or the native VLAN cannot be resolved.
func buildVlanConfig(
	p adapters.UniFiPortEntry,
	confByID map[string]adapters.UniFiNetworkConf,
	defaultNetID string,
	controller string,
) *DevicePortVlanConfig {
	switch p.Forward {
	case "disable":
		return nil
	case "native":
		// access mode: single untagged VLAN, no tagged traffic
		nativeConf, ok := resolveNativeConf(p.NativeNetworkConfID, defaultNetID, confByID)
		if !ok {
			return nil
		}
		return &DevicePortVlanConfig{
			Mode:       Access,
			NativeVlan: confToVlanRef(nativeConf, controller),
		}
	case "all", "customize":
		nativeConf, ok := resolveNativeConf(p.NativeNetworkConfID, defaultNetID, confByID)
		if !ok {
			return nil
		}
		cfg := &DevicePortVlanConfig{
			Mode:       Trunk,
			NativeVlan: confToVlanRef(nativeConf, controller),
		}
		if p.Forward == "customize" && len(p.ExcludedNetworkConfIDs) > 0 {
			// trunk-custom: all corporate VLANs minus excluded and minus native
			excludedSet := make(map[string]bool, len(p.ExcludedNetworkConfIDs))
			for _, id := range p.ExcludedNetworkConfIDs {
				excludedSet[id] = true
			}
			var items []NetworkVlanRef
			for _, conf := range confByID {
				if conf.Purpose != "corporate" {
					continue
				}
				if excludedSet[conf.ID] || conf.ID == nativeConf.ID {
					continue
				}
				items = append(items, confToVlanRef(conf, controller))
			}
			slices.SortFunc(items, func(a, b NetworkVlanRef) int {
				return a.VlanId - b.VlanId
			})
			cfg.TaggedVlans = &struct {
				Items *[]NetworkVlanRef                          `json:"items,omitempty"`
				Scope DevicePortVlanConfigTaggedVlansScope `json:"scope"`
			}{Scope: Custom, Items: &items}
		} else {
			// trunk-all
			cfg.TaggedVlans = &struct {
				Items *[]NetworkVlanRef                          `json:"items,omitempty"`
				Scope DevicePortVlanConfigTaggedVlansScope `json:"scope"`
			}{Scope: All}
		}
		return cfg
	default:
		return nil
	}
}

// resolveNativeConf returns the network conf for the port's native VLAN.
// Falls back to defaultNetID when NativeNetworkConfID is nil or empty.
func resolveNativeConf(
	nativeID *string,
	defaultNetID string,
	confByID map[string]adapters.UniFiNetworkConf,
) (adapters.UniFiNetworkConf, bool) {
	id := defaultNetID
	if nativeID != nil && *nativeID != "" {
		id = *nativeID
	}
	if id == "" {
		return adapters.UniFiNetworkConf{}, false
	}
	conf, ok := confByID[id]
	return conf, ok
}

func buildPortLabel(p adapters.UniFiPortEntry) *string {
	if p.Name == "Port "+strconv.Itoa(p.PortIdx) {
		return nil
	}
	return &p.Name
}

func buildSfpModulePresent(p adapters.UniFiPortEntry) *bool {
	return p.SfpFound
}

func buildLinkUptime(p adapters.UniFiPortEntry) *Seconds {
	if !p.Up || p.Uptime == nil {
		return nil
	}
	s := Seconds(*p.Uptime)
	return &s
}

// buildLagMembership derives LAG role from AggregatedBy and a pre-computed masterSet.
// masterSet is keyed by port_idx of every port that has members pointing to it.
func buildLagMembership(p adapters.UniFiPortEntry, masterSet map[int]bool) *DevicePortLagMembership {
	switch v := p.AggregatedBy.(type) {
	case float64:
		masterIdx := int(v)
		return &DevicePortLagMembership{Id: masterIdx, Role: Member}
	case bool:
		if !v && masterSet[p.PortIdx] {
			return &DevicePortLagMembership{Id: p.PortIdx, Role: Master}
		}
	}
	return nil
}

// findDefaultNetID returns the _id of the default (untagged, corporate) network conf.
// Returns empty string when none is found.
func findDefaultNetID(confs []adapters.UniFiNetworkConf) string {
	for _, c := range confs {
		if c.Purpose == "corporate" && !c.VlanEnabled {
			return c.ID
		}
	}
	return ""
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

// buildSwPortToDevice maps (switch MAC, switch port) → the device on that port.
// It covers both directions of every uplink relationship:
//   - downstream: (upstream MAC, upstream remote port) → this device
//   - uplink: (this device MAC, this device's local uplink port) → upstream device
//
// Without the uplink direction, a switch's own port facing the gateway/parent
// switch would resolve to nil ConnectedTo (see issue #38).
func buildSwPortToDevice(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	byMAC := make(map[string]adapters.UniFiDevice, len(devices))
	for _, d := range devices {
		byMAC[normalizeMac(d.MAC)] = d
	}
	m := make(map[string]adapters.UniFiDevice)
	for _, d := range devices {
		if d.Uplink == nil || d.Uplink.UplinkMAC == "" {
			continue
		}
		upstreamMAC := normalizeMac(d.Uplink.UplinkMAC)
		if d.Uplink.UplinkRemotePort != nil {
			key := fmt.Sprintf("%s:%d", upstreamMAC, *d.Uplink.UplinkRemotePort)
			m[key] = d
		}
		if d.Uplink.PortIdx != nil {
			upstream, ok := byMAC[upstreamMAC]
			if !ok {
				continue
			}
			key := fmt.Sprintf("%s:%d", normalizeMac(d.MAC), *d.Uplink.PortIdx)
			m[key] = upstream
		}
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

func mapPoeMode(mode string) DevicePortPoeMode {
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
