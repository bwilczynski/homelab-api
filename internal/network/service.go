package network

import (
	"context"
	"fmt"
	"regexp"
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

// ListDevices retrieves all managed network devices (list shape: no model/firmware/uptime).
func (s *Service) ListDevices(ctx context.Context) (NetworkDeviceList, error) {
	raw, err := s.unifi.GetDevices()
	if err != nil {
		return NetworkDeviceList{}, fmt.Errorf("get unifi devices: %w", err)
	}

	items := make([]NetworkDevice, 0, len(raw))
	for _, d := range raw {
		items = append(items, s.deviceToList(d))
	}
	return NetworkDeviceList{Items: items}, nil
}

// GetDevice looks up a single device by composite ID and returns its detail.
func (s *Service) GetDevice(ctx context.Context, id string) (NetworkDeviceDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok || controller != s.controller {
		return NetworkDeviceDetail{}, false, nil
	}

	raw, err := s.unifi.GetDevices()
	if err != nil {
		return NetworkDeviceDetail{}, false, fmt.Errorf("get unifi devices: %w", err)
	}

	for _, d := range raw {
		if toKebab(d.Name) == suffix {
			return s.deviceToDetail(d), true, nil
		}
	}
	return NetworkDeviceDetail{}, false, nil
}

// ListClients retrieves all connected clients (list shape: no ssid/signal/uptime).
func (s *Service) ListClients(ctx context.Context) (NetworkClientList, error) {
	raw, err := s.unifi.GetClients()
	if err != nil {
		return NetworkClientList{}, fmt.Errorf("get unifi clients: %w", err)
	}

	items := make([]NetworkClient, 0, len(raw))
	for _, sta := range raw {
		items = append(items, s.clientToList(sta))
	}
	return NetworkClientList{Items: items}, nil
}

// GetClient looks up a single client by composite ID and returns its typed detail.
func (s *Service) GetClient(ctx context.Context, id string) (NetworkClientDetail, bool, error) {
	controller, suffix, ok := parseID(id)
	if !ok || controller != s.controller {
		return NetworkClientDetail{}, false, nil
	}

	raw, err := s.unifi.GetClients()
	if err != nil {
		return NetworkClientDetail{}, false, fmt.Errorf("get unifi clients: %w", err)
	}

	for _, sta := range raw {
		if clientSuffix(sta) == suffix {
			detail, err := s.clientToDetail(sta)
			if err != nil {
				return NetworkClientDetail{}, false, err
			}
			return detail, true, nil
		}
	}
	return NetworkClientDetail{}, false, nil
}

// --- mapping helpers ---

func (s *Service) deviceToList(d adapters.UniFiDevice) NetworkDevice {
	mac := normalizeMac(d.MAC)
	dev := NetworkDevice{
		Id:     fmt.Sprintf("%s.%s", s.controller, toKebab(d.Name)),
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

func (s *Service) deviceToDetail(d adapters.UniFiDevice) NetworkDeviceDetail {
	mac := normalizeMac(d.MAC)
	det := NetworkDeviceDetail{
		Id:              fmt.Sprintf("%s.%s", s.controller, toKebab(d.Name)),
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

func (s *Service) clientToList(sta adapters.UniFiSta) NetworkClient {
	mac := normalizeMac(sta.MAC)
	client := NetworkClient{
		Id:             fmt.Sprintf("%s.%s", s.controller, clientSuffix(sta)),
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

func (s *Service) clientToDetail(sta adapters.UniFiSta) (NetworkClientDetail, error) {
	mac := normalizeMac(sta.MAC)
	id := fmt.Sprintf("%s.%s", s.controller, clientSuffix(sta))
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

// --- ID helpers ---

// toKebab converts a display name to kebab-case (lowercase, spaces and special chars → hyphens).
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func toKebab(name string) string {
	lower := strings.ToLower(name)
	kebab := nonAlphanumRe.ReplaceAllString(lower, "-")
	return strings.Trim(kebab, "-")
}

// clientSuffix returns the composite ID suffix for a client: {kebab-name}-{mac-prefix}.
func clientSuffix(sta adapters.UniFiSta) string {
	mac := normalizeMac(sta.MAC)
	prefix := strings.ReplaceAll(mac, ":", "")[:2]
	return fmt.Sprintf("%s-%s", toKebab(clientName(sta)), prefix)
}

// parseID splits a composite ID "{controller}.{suffix}" into its parts.
func parseID(id string) (controller, suffix string, ok bool) {
	dot := strings.IndexByte(id, '.')
	if dot <= 0 || dot == len(id)-1 {
		return "", "", false
	}
	return id[:dot], id[dot+1:], true
}

// --- enum mappers ---

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
		return NetworkClientConnectionTypeWired
	}
	return NetworkClientConnectionTypeWireless
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

func normalizeMac(mac string) string {
	return strings.ToLower(mac)
}
