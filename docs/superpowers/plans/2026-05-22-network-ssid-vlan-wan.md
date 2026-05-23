# Network SSID / VLAN / WAN Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `listSsids`, `getSsid`, `listVlans`, `getVlan`, `listWans`, `getWan` endpoints for the network domain, backed by UniFi `wlanconf` and `networkconf` APIs.

**Architecture:** Three new `*_service.go` files follow the existing narrow-backend-interface pattern. Two new adapter methods (`GetWlanConf`, `GetNetworkConf`) are added to `UniFiClient`. Handlers delegate to the service and return RFC 9457 error responses — identical to existing network handlers.

**Tech Stack:** Go, chi, oapi-codegen v2, UniFi Controller REST API (v1 classic).

---

## File Map

| Action | File |
|--------|------|
| Modify | `internal/adapters/unifi.go` — add `UniFiWlanConf`, `UniFiNetworkConf`, expand `UniFiWanIface`, add `Wan2` to `UniFiDevice`, add `GetWlanConf()`/`GetNetworkConf()` |
| Regenerate | `internal/network/api.gen.go` — run `make generate` |
| Modify | `internal/network/service.go` — extend `UniFiBackend` with three new narrow interfaces |
| Create | `internal/network/vlans_service.go` — `VLANsBackend`, `ListVLANs`, `GetVLAN` |
| Create | `internal/network/wans_service.go` — `WANsBackend`, `ListWANs`, `GetWAN` |
| Create | `internal/network/ssids_service.go` — `SSIDsBackend`, `ListSSIDs`, `GetSSID` |
| Modify | `internal/network/handler.go` — add 6 handler methods |
| Modify | `internal/network/service_test.go` — extend `mockUniFi`, add tests |
| Create | `internal/network/testdata/unifi-wlanconf.json` |
| Create | `internal/network/testdata/unifi-networkconf.json` |

---

## Task 1: Regenerate API stubs

**Files:** Regenerate `internal/network/api.gen.go`

- [ ] **Step 1: Run make generate**

```bash
make generate
```

Expected: exits 0, `internal/network/api.gen.go` updated.

- [ ] **Step 2: Verify new operations are present**

```bash
grep -E "ListSsids|GetSsid|ListVlans|GetVlan|ListWans|GetWan" internal/network/api.gen.go
```

Expected: 12 lines — one `StrictServerInterface` method + one `strictHandler` dispatch per operation.

- [ ] **Step 3: Note generated constant names for use in later tasks**

```bash
grep -E "WifiBand|WifiSecurity|DhcpMode|WanStatus" internal/network/api.gen.go | grep "^// Defines\|^\t[A-Z]" | head -40
```

Record the exact constant identifiers — they will be referenced in service code.

- [ ] **Step 4: Commit**

```bash
git add internal/network/api.gen.go
git commit -m "chore: regenerate network stubs for SSID/VLAN/WAN endpoints"
```

---

## Task 2: Extend adapter types

**Files:** Modify `internal/adapters/unifi.go`

- [ ] **Step 1: Add `UniFiWlanConf` struct after the existing `UniFiWanIface` block (~line 154)**

```go
// UniFiWlanConf represents a WiFi network configuration from the UniFi Controller.
type UniFiWlanConf struct {
	ID            string   `json:"_id"`
	Name          string   `json:"name"`
	NetworkConfID string   `json:"networkconf_id"`
	Security      string   `json:"security"`
	WpaMode       string   `json:"wpa_mode"`
	Wpa3Support   bool     `json:"wpa3_support"`
	Wpa3Transition bool    `json:"wpa3_transition"`
	WlanBands     []string `json:"wlan_bands"`
	Enabled       bool     `json:"enabled"`
}

// UniFiNetworkConf represents a network/VLAN configuration from the UniFi Controller.
// The Vlan field is interface{} because the JSON value is an integer for tagged VLANs,
// an empty string for the default untagged network, and null for WAN entries.
type UniFiNetworkConf struct {
	ID               string      `json:"_id"`
	Name             string      `json:"name"`
	Purpose          string      `json:"purpose"` // "corporate", "guest", "wan"
	NetworkGroup     string      `json:"networkgroup"`
	Vlan             interface{} `json:"vlan"`
	VlanEnabled      bool        `json:"vlan_enabled"`
	IPSubnet         string      `json:"ip_subnet"` // gateway IP + prefix, e.g. "192.168.1.1/24"
	DhcpdEnabled     bool        `json:"dhcpd_enabled"`
	DHCPRelayEnabled bool        `json:"dhcp_relay_enabled"`
	DhcpdStart       string      `json:"dhcpd_start"`
	DhcpdStop        string      `json:"dhcpd_stop"`
	DhcpdDNS1        string      `json:"dhcpd_dns_1"`
	DhcpdDNS2        string      `json:"dhcpd_dns_2"`
	WanNetworkGroup  string      `json:"wan_networkgroup"` // "WAN" → wan1, "WAN2" → wan2
	WanDNS1          string      `json:"wan_dns1"`
	WanDNS2          string      `json:"wan_dns2"`
}
```

- [ ] **Step 2: Expand `UniFiWanIface` with live-state fields**

Replace the existing `UniFiWanIface` struct:

```go
type UniFiWanIface struct {
	Name     string   `json:"name"`
	IP       string   `json:"ip"`
	Up       bool     `json:"up"`
	DNS      []string `json:"dns"`
	TxBytes  int64    `json:"tx_bytes"`
	RxBytes  int64    `json:"rx_bytes"`
	TxBytesR float64  `json:"tx_bytes-r"`
	RxBytesR float64  `json:"rx_bytes-r"`
}
```

- [ ] **Step 3: Add `Wan2` to `UniFiDevice`**

In the `UniFiDevice` struct, after the `Wan1` field add:

```go
	Wan2        *UniFiWanIface   `json:"wan2"`
```

- [ ] **Step 4: Add `GetWlanConf` and `GetNetworkConf` methods at the end of the file**

```go
// --- WLAN / Network config types ---

// GetWlanConf retrieves all WiFi network configurations from the UniFi Controller.
func (c *UniFiClient) GetWlanConf() ([]UniFiWlanConf, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	var result unifiResponse[[]UniFiWlanConf]
	if err := c.get("/api/s/default/rest/wlanconf", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetNetworkConf retrieves all network configurations (VLANs + WAN) from the UniFi Controller.
func (c *UniFiClient) GetNetworkConf() ([]UniFiNetworkConf, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	var result unifiResponse[[]UniFiNetworkConf]
	if err := c.get("/api/s/default/rest/networkconf", &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
```

- [ ] **Step 5: Build to verify no compile errors**

```bash
make build
```

Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/unifi.go
git commit -m "feat: add UniFiWlanConf, UniFiNetworkConf adapter types and GetWlanConf/GetNetworkConf methods"
```

---

## Task 3: Update `UniFiBackend` interface and `mockUniFi`

**Files:** Modify `internal/network/service.go`, `internal/network/service_test.go`

- [ ] **Step 1: Add three narrow backend interfaces to `service.go`**

The `UniFiBackend` interface is defined as a composite of narrow interfaces. Add three new narrow interfaces (one per resource domain) in `service.go`, then include them in `UniFiBackend`.

In `service.go`, replace:

```go
// UniFiBackend is the combined interface satisfied by the UniFi adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type UniFiBackend interface {
	DevicesBackend
	ClientsBackend
	TopologyBackend
}
```

with:

```go
// UniFiBackend is the combined interface satisfied by the UniFi adapter.
// Narrow per-resource interfaces are defined in each *_service.go file.
type UniFiBackend interface {
	DevicesBackend
	ClientsBackend
	TopologyBackend
	SSIDsBackend
	VLANsBackend
	WANsBackend
}
```

- [ ] **Step 2: Add `wlanConf` and `networkConf` fields to `mockUniFi` in `service_test.go`**

Replace the `mockUniFi` struct:

```go
type mockUniFi struct {
	devices        []adapters.UniFiDevice
	clients        []adapters.UniFiSta
	activeClients  []adapters.UniFiClientV2
	offlineClients []adapters.UniFiClientV2
	wlanConf       []adapters.UniFiWlanConf
	networkConf    []adapters.UniFiNetworkConf
	err            error
}
```

- [ ] **Step 3: Add the two new interface methods to `mockUniFi`**

After the existing mock methods in `service_test.go`, add:

```go
func (m *mockUniFi) GetWlanConf() ([]adapters.UniFiWlanConf, error) {
	return m.wlanConf, m.err
}

func (m *mockUniFi) GetNetworkConf() ([]adapters.UniFiNetworkConf, error) {
	return m.networkConf, m.err
}
```

- [ ] **Step 4: Build to verify the interface is satisfied**

```bash
make build
```

Expected: compile error about `SSIDsBackend`, `VLANsBackend`, `WANsBackend` not defined — that is expected since those files don't exist yet. The `mockUniFi` will not yet satisfy `UniFiBackend`.

**Note:** The build will not pass until Tasks 5–7 are complete. That's fine — we're setting up the skeleton. Proceed to Task 4.

---

## Task 4: Create test fixtures

**Files:** Create `internal/network/testdata/unifi-wlanconf.json`, `internal/network/testdata/unifi-networkconf.json`

The fixtures are sanitized from the real captured responses (see `scripts/responses/unifi-wlanconf-raw.json` and `scripts/responses/unifi-networkconf-raw.json`). Sensitive values (IDs, passphrases) are replaced.

The wlanconf fixture uses IDs that cross-reference networkconf fixture IDs — `networkconf_id` in wlanconf must match `_id` in networkconf. Use the sanitized IDs below consistently.

- [ ] **Step 1: Write `internal/network/testdata/unifi-wlanconf.json`**

```json
{
  "data": [
    {
      "_id": "aabbccddee0011223344aa01",
      "name": "hamster-iot",
      "networkconf_id": "aabbccddee0011223344bb10",
      "security": "wpapsk",
      "wpa_mode": "wpa2",
      "wpa3_support": false,
      "wpa3_transition": false,
      "wlan_bands": ["2g", "5g"],
      "enabled": true
    },
    {
      "_id": "aabbccddee0011223344aa02",
      "name": "hamster",
      "networkconf_id": "aabbccddee0011223344bb11",
      "security": "wpapsk",
      "wpa_mode": "wpa2",
      "wpa3_support": false,
      "wpa3_transition": false,
      "wlan_bands": ["2g", "5g"],
      "enabled": true
    }
  ]
}
```

- [ ] **Step 2: Write `internal/network/testdata/unifi-networkconf.json`**

```json
{
  "data": [
    {
      "_id": "aabbccddee0011223344bb20",
      "name": "LAN-MGMT",
      "purpose": "corporate",
      "networkgroup": "LAN",
      "vlan": "",
      "vlan_enabled": false,
      "ip_subnet": "192.168.1.1/24",
      "dhcpd_enabled": true,
      "dhcp_relay_enabled": false,
      "dhcpd_start": "192.168.1.6",
      "dhcpd_stop": "192.168.1.49",
      "dhcpd_dns_1": "192.168.1.1",
      "dhcpd_dns_2": ""
    },
    {
      "_id": "aabbccddee0011223344bb10",
      "name": "LAN-IOT",
      "purpose": "corporate",
      "networkgroup": "LAN",
      "vlan": 20,
      "vlan_enabled": true,
      "ip_subnet": "192.168.20.1/24",
      "dhcpd_enabled": true,
      "dhcp_relay_enabled": false,
      "dhcpd_start": "192.168.20.6",
      "dhcpd_stop": "192.168.20.254",
      "dhcpd_dns_1": "192.168.20.1",
      "dhcpd_dns_2": ""
    },
    {
      "_id": "aabbccddee0011223344bb11",
      "name": "LAN-INT",
      "purpose": "corporate",
      "networkgroup": "LAN",
      "vlan": 10,
      "vlan_enabled": true,
      "ip_subnet": "192.168.10.1/24",
      "dhcpd_enabled": true,
      "dhcp_relay_enabled": false,
      "dhcpd_start": "192.168.10.6",
      "dhcpd_stop": "192.168.10.254",
      "dhcpd_dns_1": "192.168.100.5",
      "dhcpd_dns_2": "192.168.10.1"
    },
    {
      "_id": "aabbccddee0011223344bb12",
      "name": "LAN-SRV",
      "purpose": "corporate",
      "networkgroup": "LAN",
      "vlan": 100,
      "vlan_enabled": true,
      "ip_subnet": "192.168.100.1/24",
      "dhcpd_enabled": true,
      "dhcp_relay_enabled": false,
      "dhcpd_start": "192.168.100.200",
      "dhcpd_stop": "192.168.100.254",
      "dhcpd_dns_1": null,
      "dhcpd_dns_2": null
    },
    {
      "_id": "aabbccddee0011223344bb30",
      "name": "Internet 1",
      "purpose": "wan",
      "wan_networkgroup": "WAN",
      "wan_dns1": "8.8.8.8",
      "wan_dns2": "8.8.4.4"
    }
  ]
}
```

Note: the `_id` values for LAN-IOT (`bb10`) and LAN-INT (`bb11`) match the `networkconf_id` values in the wlanconf fixture.

---

## Task 5: Implement VLANs service

**Files:** Create `internal/network/vlans_service.go`, add tests to `internal/network/service_test.go`

- [ ] **Step 1: Write failing tests — add to `service_test.go`**

```go
// --- VLAN list tests ---

func TestListVLANs(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	result, err := svc.ListVLANs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 LAN networks; WAN entry is excluded
	if len(result.Items) != 4 {
		t.Fatalf("expected 4 VLANs, got %d", len(result.Items))
	}

	byID := make(map[string]Vlan)
	for _, v := range result.Items {
		byID[v.Id] = v
	}

	mgmt, ok := byID["unifi.lan-mgmt"]
	if !ok {
		t.Fatal("expected unifi.lan-mgmt")
	}
	if mgmt.VlanId != 1 {
		t.Errorf("LAN-MGMT: expected vlanId 1 (untagged), got %d", mgmt.VlanId)
	}
	if mgmt.Subnet != "192.168.1.0/24" {
		t.Errorf("LAN-MGMT: expected subnet 192.168.1.0/24, got %s", mgmt.Subnet)
	}
	if mgmt.Uri != "/network/vlans/unifi.lan-mgmt" {
		t.Errorf("LAN-MGMT: expected uri /network/vlans/unifi.lan-mgmt, got %s", mgmt.Uri)
	}

	iot, ok := byID["unifi.lan-iot"]
	if !ok {
		t.Fatal("expected unifi.lan-iot")
	}
	if iot.VlanId != 20 {
		t.Errorf("LAN-IOT: expected vlanId 20, got %d", iot.VlanId)
	}
	if iot.Subnet != "192.168.20.0/24" {
		t.Errorf("LAN-IOT: expected subnet 192.168.20.0/24, got %s", iot.Subnet)
	}

	srv, ok := byID["unifi.lan-srv"]
	if !ok {
		t.Fatal("expected unifi.lan-srv")
	}
	if srv.VlanId != 100 {
		t.Errorf("LAN-SRV: expected vlanId 100, got %d", srv.VlanId)
	}
}

func TestListVLANsEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: []adapters.UniFiNetworkConf{}}}, 30)
	result, err := svc.ListVLANs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 VLANs, got %d", len(result.Items))
	}
}

// --- VLAN detail tests ---

func TestGetVLAN_ServerDHCP(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-iot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-IOT to be found")
	}
	if detail.Id != "unifi.lan-iot" {
		t.Errorf("expected id unifi.lan-iot, got %s", detail.Id)
	}
	if detail.VlanId != 20 {
		t.Errorf("expected vlanId 20, got %d", detail.VlanId)
	}
	if detail.Subnet != "192.168.20.0/24" {
		t.Errorf("expected subnet 192.168.20.0/24, got %s", detail.Subnet)
	}
	if detail.GatewayIp != "192.168.20.1" {
		t.Errorf("expected gatewayIp 192.168.20.1, got %s", detail.GatewayIp)
	}
	if detail.BroadcastIp != "192.168.20.255" {
		t.Errorf("expected broadcastIp 192.168.20.255, got %s", detail.BroadcastIp)
	}
	if detail.DhcpMode != "server" {
		t.Errorf("expected dhcpMode server, got %s", detail.DhcpMode)
	}
	if detail.DhcpRange == nil {
		t.Fatal("expected dhcpRange to be set")
	}
	if detail.DhcpRange.Start != "192.168.20.6" {
		t.Errorf("expected dhcpRange.start 192.168.20.6, got %s", detail.DhcpRange.Start)
	}
	if detail.DhcpRange.End != "192.168.20.254" {
		t.Errorf("expected dhcpRange.end 192.168.20.254, got %s", detail.DhcpRange.End)
	}
	if len(detail.DnsServers) != 1 || detail.DnsServers[0] != "192.168.20.1" {
		t.Errorf("expected dnsServers [192.168.20.1], got %v", detail.DnsServers)
	}
}

func TestGetVLAN_MultipleDNS(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-int")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-INT to be found")
	}
	if len(detail.DnsServers) != 2 {
		t.Fatalf("expected 2 DNS servers, got %d", len(detail.DnsServers))
	}
	if detail.DnsServers[0] != "192.168.100.5" || detail.DnsServers[1] != "192.168.10.1" {
		t.Errorf("unexpected DNS servers: %v", detail.DnsServers)
	}
}

func TestGetVLAN_NullDNS(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-srv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-SRV to be found")
	}
	if len(detail.DnsServers) != 0 {
		t.Errorf("expected empty dnsServers for LAN-SRV, got %v", detail.DnsServers)
	}
}

func TestGetVLAN_UntaggedVLAN1(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	detail, found, err := svc.GetVLAN(context.Background(), "unifi.lan-mgmt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected LAN-MGMT to be found")
	}
	if detail.VlanId != 1 {
		t.Errorf("expected vlanId 1 for untagged, got %d", detail.VlanId)
	}
	if detail.GatewayIp != "192.168.1.1" {
		t.Errorf("expected gatewayIp 192.168.1.1, got %s", detail.GatewayIp)
	}
	if detail.BroadcastIp != "192.168.1.255" {
		t.Errorf("expected broadcastIp 192.168.1.255, got %s", detail.BroadcastIp)
	}
}

func TestGetVLANNotFound(t *testing.T) {
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{networkConf: networks}}, 30)

	_, found, err := svc.GetVLAN(context.Background(), "unifi.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/network/ -run "TestListVLANs|TestGetVLAN" -v 2>&1 | head -20
```

Expected: compilation error — `ListVLANs`, `GetVLAN`, `SSIDsBackend`, `VLANsBackend`, `WANsBackend` not defined.

- [ ] **Step 3: Create `internal/network/vlans_service.go`**

```go
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
	if dhcpMode == "server" {
		r := DhcpRange{Start: n.DhcpdStart, End: n.DhcpdStop}
		detail.DhcpRange = &r
	}
	if dhcpMode == "relay" {
		detail.RelayServer = &n.DhcpdStart // DhcpdStart holds relay IP when relay mode
	}
	return detail
}

// --- helpers ---

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
	for i := 0; i < 4; i++ {
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
	if relayEnabled {
		return "relay"
	}
	if dhcpdEnabled {
		return "server"
	}
	return "disabled"
}

func collectDNSServers(dns ...string) []string {
	result := slices.DeleteFunc(slices.Clone(dns), func(s string) bool { return s == "" })
	if result == nil {
		result = []string{}
	}
	return result
}
```

**Note on relay server field:** In the relay DHCP case, the relay IP comes from the actual `dhcp_relay_server` or similar field in the real data. The fixture does not have a relay entry (no such VLAN in the real setup), so the relay branch is a precaution. If the real UniFi data surfaces the relay IP in a different field, update accordingly.

- [ ] **Step 4: Also create stubs for `SSIDsBackend` and `WANsBackend` so the build compiles**

Create `internal/network/ssids_service.go` with just the interface stub:

```go
package network

import "github.com/bwilczynski/homelab-api/internal/adapters"

// SSIDsBackend is the narrow interface for SSID operations.
type SSIDsBackend interface {
	GetWlanConf() ([]adapters.UniFiWlanConf, error)
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetClients() ([]adapters.UniFiSta, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}
```

Create `internal/network/wans_service.go` with just the interface stub:

```go
package network

import "github.com/bwilczynski/homelab-api/internal/adapters"

// WANsBackend is the narrow interface for WAN operations.
type WANsBackend interface {
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}
```

- [ ] **Step 5: Run VLAN tests — should pass**

```bash
go test ./internal/network/ -run "TestListVLANs|TestGetVLAN" -v
```

Expected: all 7 VLAN tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/network/vlans_service.go internal/network/ssids_service.go internal/network/wans_service.go internal/network/service_test.go internal/network/testdata/unifi-wlanconf.json internal/network/testdata/unifi-networkconf.json
git commit -m "feat: implement ListVLANs and GetVLAN"
```

---

## Task 6: Implement WANs service

**Files:** Fill in `internal/network/wans_service.go`, add tests to `service_test.go`

The WAN service reads `networkconf` entries with `purpose == "wan"` for names and config, then cross-references the gateway device's `wan1`/`wan2` for live state (IP, up/down, DNS). The `wan_networkgroup` field ("WAN" → `wan1`, "WAN2" → `wan2`) performs the linkage.

- [ ] **Step 1: Write failing WAN tests — add to `service_test.go`**

```go
// --- WAN list tests ---

func TestListWANs(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, networkConf: networks}}, 30)

	result, err := svc.ListWANs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 WAN, got %d", len(result.Items))
	}

	wan := result.Items[0]
	if wan.Id != "unifi.internet-1" {
		t.Errorf("expected id unifi.internet-1, got %s", wan.Id)
	}
	if wan.Uri != "/network/wans/unifi.internet-1" {
		t.Errorf("expected uri /network/wans/unifi.internet-1, got %s", wan.Uri)
	}
	if wan.Name != "Internet 1" {
		t.Errorf("expected name Internet 1, got %s", wan.Name)
	}
	if wan.IpAddress != "203.0.113.42" {
		t.Errorf("expected ipAddress 203.0.113.42, got %s", wan.IpAddress)
	}
	if wan.Status != "connected" {
		t.Errorf("expected status connected, got %s", wan.Status)
	}
	if wan.Uptime <= 0 {
		t.Errorf("expected positive uptime, got %d", wan.Uptime)
	}
}

func TestListWANsEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		devices:     []adapters.UniFiDevice{},
		networkConf: []adapters.UniFiNetworkConf{},
	}}, 30)
	result, err := svc.ListWANs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 WANs, got %d", len(result.Items))
	}
}

// --- WAN detail tests ---

func TestGetWAN(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, networkConf: networks}}, 30)

	detail, found, err := svc.GetWAN(context.Background(), "unifi.internet-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected internet-1 to be found")
	}
	if detail.Id != "unifi.internet-1" {
		t.Errorf("expected id unifi.internet-1, got %s", detail.Id)
	}
	if detail.IpAddress != "203.0.113.42" {
		t.Errorf("expected ipAddress 203.0.113.42, got %s", detail.IpAddress)
	}
	if detail.Status != "connected" {
		t.Errorf("expected status connected, got %s", detail.Status)
	}
	if len(detail.DnsServers) == 0 {
		t.Error("expected at least one DNS server")
	}
}

func TestGetWANNotFound(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices, networkConf: networks}}, 30)

	_, found, err := svc.GetWAN(context.Background(), "unifi.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
}
```

**Important:** The WAN test uses the existing `unifi-devices.json` fixture. The gateway device in that fixture currently has only traffic fields in `wan1`. You must update the fixture to add `ip`, `up`, and `dns` to the `wan1` object. Find the gateway entry (type `"ugw"`) and add these fields to its `wan1`:

```json
"wan1": {
  "ip": "203.0.113.42",
  "up": true,
  "dns": ["8.8.8.8"],
  "tx_bytes-r": 79558.09798451557,
  "rx_bytes-r": 134932.2634235025,
  "rx_bytes": 3913232707026,
  "tx_bytes": 306168680348
}
```

Edit `internal/network/testdata/unifi-devices.json` — find the gateway object and update its `wan1` field.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/network/ -run "TestListWANs|TestGetWAN" -v 2>&1 | head -20
```

Expected: compilation error — `ListWANs`/`GetWAN` not defined.

- [ ] **Step 3: Implement `internal/network/wans_service.go`**

```go
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

// resolveWanIface returns the WAN interface struct for a given networkgroup ("WAN" or "WAN2").
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
// IP and up/down come from the interface; uptime is the gateway device uptime (best
// available approximation — wan1 does not expose a per-interface uptime in the API).
func wanLiveFields(iface *adapters.UniFiWanIface, gw *adapters.UniFiDevice) (ip string, status WanStatus, uptime int) {
	if iface != nil {
		ip = iface.IP
		if iface.Up {
			status = "connected"
		} else {
			status = "disconnected"
		}
	} else {
		status = "disconnected"
	}
	if gw != nil {
		uptime = gw.Uptime
	}
	return
}

// wanDNSServers returns DNS servers for a WAN.
// Prefers the live DNS list from the interface; falls back to the configured dns1/dns2.
func wanDNSServers(iface *adapters.UniFiWanIface, n adapters.UniFiNetworkConf) []string {
	if iface != nil && len(iface.DNS) > 0 {
		return iface.DNS
	}
	return collectDNSServers(n.WanDNS1, n.WanDNS2)
}
```

- [ ] **Step 4: Run WAN tests**

```bash
go test ./internal/network/ -run "TestListWANs|TestGetWAN" -v
```

Expected: all 4 WAN tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/wans_service.go internal/network/service_test.go internal/network/testdata/unifi-devices.json
git commit -m "feat: implement ListWANs and GetWAN"
```

---

## Task 7: Implement SSIDs service

**Files:** Fill in `internal/network/ssids_service.go`, add tests to `service_test.go`

SSIDs require data from four sources: `GetWlanConf` (SSID config), `GetNetworkConf` (VLAN ID lookup via `networkconf_id`), `GetClients` (numClients count per SSID), `GetDevices` (broadcastingAps list for detail endpoint).

- [ ] **Step 1: Write failing SSID tests — add to `service_test.go`**

```go
// --- SSID list tests ---

func TestListSSIDs(t *testing.T) {
	wlans := loadFixture[[]adapters.UniFiWlanConf](t, "testdata/unifi-wlanconf.json")
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	// Two clients on "hamster-iot", one on "hamster"
	clients := []adapters.UniFiSta{
		{MAC: "aa:bb:cc:dd:ee:01", IsWired: false, ESSID: ptr("hamster-iot")},
		{MAC: "aa:bb:cc:dd:ee:02", IsWired: false, ESSID: ptr("hamster-iot")},
		{MAC: "aa:bb:cc:dd:ee:03", IsWired: false, ESSID: ptr("hamster")},
	}
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		wlanConf: wlans, networkConf: networks, clients: clients,
	}}, 30)

	result, err := svc.ListSSIDs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 SSIDs, got %d", len(result.Items))
	}

	byID := make(map[string]Ssid)
	for _, s := range result.Items {
		byID[s.Id] = s
	}

	iot, ok := byID["unifi.hamster-iot"]
	if !ok {
		t.Fatal("expected unifi.hamster-iot")
	}
	if iot.VlanId != 20 {
		t.Errorf("hamster-iot: expected vlanId 20 (from LAN-IOT), got %d", iot.VlanId)
	}
	if iot.NumClients != 2 {
		t.Errorf("hamster-iot: expected 2 clients, got %d", iot.NumClients)
	}
	if iot.Uri != "/network/ssids/unifi.hamster-iot" {
		t.Errorf("hamster-iot: unexpected uri %s", iot.Uri)
	}
	if len(iot.Bands) != 2 {
		t.Fatalf("hamster-iot: expected 2 bands, got %d", len(iot.Bands))
	}
	if iot.Bands[0] != "band2g" || iot.Bands[1] != "band5g" {
		t.Errorf("hamster-iot: expected [band2g band5g], got %v", iot.Bands)
	}

	home, ok := byID["unifi.hamster"]
	if !ok {
		t.Fatal("expected unifi.hamster")
	}
	if home.VlanId != 10 {
		t.Errorf("hamster: expected vlanId 10 (from LAN-INT), got %d", home.VlanId)
	}
	if home.NumClients != 1 {
		t.Errorf("hamster: expected 1 client, got %d", home.NumClients)
	}
}

func TestListSSIDsEmpty(t *testing.T) {
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		wlanConf: []adapters.UniFiWlanConf{}, networkConf: []adapters.UniFiNetworkConf{}, clients: []adapters.UniFiSta{},
	}}, 30)
	result, err := svc.ListSSIDs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 SSIDs, got %d", len(result.Items))
	}
}

// --- SSID detail tests ---

func TestGetSSID(t *testing.T) {
	wlans := loadFixture[[]adapters.UniFiWlanConf](t, "testdata/unifi-wlanconf.json")
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := []adapters.UniFiSta{
		{MAC: "aa:bb:cc:dd:ee:01", Name: ptr("Device A"), IsWired: false, ESSID: ptr("hamster-iot"), ApMAC: "bb:bb:bb:bb:bb:03"},
		{MAC: "aa:bb:cc:dd:ee:02", Name: ptr("Device B"), IsWired: false, ESSID: ptr("hamster-iot"), ApMAC: "bb:bb:bb:bb:bb:03"},
	}
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		wlanConf: wlans, networkConf: networks, devices: devices, clients: clients,
	}}, 30)

	detail, found, err := svc.GetSSID(context.Background(), "unifi.hamster-iot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected hamster-iot to be found")
	}
	if detail.Id != "unifi.hamster-iot" {
		t.Errorf("expected id unifi.hamster-iot, got %s", detail.Id)
	}
	if detail.SecurityProtocol != "wpa2" {
		t.Errorf("expected securityProtocol wpa2, got %s", detail.SecurityProtocol)
	}
	if detail.VlanId != 20 {
		t.Errorf("expected vlanId 20, got %d", detail.VlanId)
	}
	if detail.NumClients != 2 {
		t.Errorf("expected 2 clients, got %d", detail.NumClients)
	}
	if len(detail.Clients) != 2 {
		t.Fatalf("expected 2 client refs, got %d", len(detail.Clients))
	}
	// broadcastingAps should contain connected APs from device fixture
	if len(detail.BroadcastingAps) == 0 {
		t.Error("expected at least one broadcasting AP")
	}
	for _, ap := range detail.BroadcastingAps {
		if ap.Kind != "device" {
			t.Errorf("broadcastingAp kind: expected device, got %s", ap.Kind)
		}
	}
}

func TestGetSSIDNotFound(t *testing.T) {
	wlans := loadFixture[[]adapters.UniFiWlanConf](t, "testdata/unifi-wlanconf.json")
	networks := loadFixture[[]adapters.UniFiNetworkConf](t, "testdata/unifi-networkconf.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{
		wlanConf: wlans, networkConf: networks, clients: []adapters.UniFiSta{}, devices: []adapters.UniFiDevice{},
	}}, 30)

	_, found, err := svc.GetSSID(context.Background(), "unifi.nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
}
```

Add this generic helper at the bottom of `service_test.go` (needed by the SSID tests). The generic form works for any type — `ptr("foo")`, `ptr(42)`, `ptr(true)` — so no type-specific variants are needed:

```go
func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 2: Run tests to confirm failure**

```bash
go test ./internal/network/ -run "TestListSSIDs|TestGetSSID" -v 2>&1 | head -20
```

Expected: compilation error — `ListSSIDs`/`GetSSID` not defined.

- [ ] **Step 3: Implement `internal/network/ssids_service.go`**

```go
package network

import (
	"context"
	"fmt"

	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// SSIDsBackend is the narrow interface for SSID operations.
type SSIDsBackend interface {
	GetWlanConf() ([]adapters.UniFiWlanConf, error)
	GetNetworkConf() ([]adapters.UniFiNetworkConf, error)
	GetClients() ([]adapters.UniFiSta, error)
	GetDevices() ([]adapters.UniFiDevice, error)
}

// ListSSIDs returns all enabled SSIDs from all backends.
func (s *Service) ListSSIDs(ctx context.Context) (SsidList, error) {
	var items []Ssid
	for _, cb := range s.backends {
		if s.monitor != nil && !s.monitor.Available(cb.controller) {
			continue
		}
		wlans, err := cb.unifi.GetWlanConf()
		if err != nil {
			return SsidList{}, fmt.Errorf("get wlan conf from %s: %w", cb.controller, err)
		}
		networks, err := cb.unifi.GetNetworkConf()
		if err != nil {
			return SsidList{}, fmt.Errorf("get network conf from %s: %w", cb.controller, err)
		}
		clients, err := cb.unifi.GetClients()
		if err != nil {
			return SsidList{}, fmt.Errorf("get clients from %s: %w", cb.controller, err)
		}
		networkByID := buildNetworkByID(networks)
		clientsBySSID := buildClientsBySSID(clients)
		for _, w := range wlans {
			if !w.Enabled {
				continue
			}
			items = append(items, wlanToSSID(cb.controller, w, networkByID, clientsBySSID))
		}
	}
	if items == nil {
		items = []Ssid{}
	}
	return SsidList{Items: items}, nil
}

// GetSSID looks up a single SSID by composite ID and returns its detail.
func (s *Service) GetSSID(ctx context.Context, id string) (SsidDetail, bool, error) {
	controller, name, ok := parseID(id)
	if !ok {
		return SsidDetail{}, false, nil
	}
	backend, err := s.findBackend(controller)
	if err != nil {
		return SsidDetail{}, false, nil
	}
	wlans, err := backend.GetWlanConf()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get wlan conf: %w", err)
	}
	networks, err := backend.GetNetworkConf()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get network conf: %w", err)
	}
	clients, err := backend.GetClients()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get clients: %w", err)
	}
	devices, err := backend.GetDevices()
	if err != nil {
		return SsidDetail{}, false, fmt.Errorf("get devices: %w", err)
	}
	networkByID := buildNetworkByID(networks)
	clientsBySSID := buildClientsBySSID(clients)
	apMacToAP := buildAPMacToDevice(devices)
	for _, w := range wlans {
		if toKebab(w.Name) == name {
			return buildSSIDDetail(controller, w, networkByID, clientsBySSID, apMacToAP, devices), true, nil
		}
	}
	return SsidDetail{}, false, nil
}

func wlanToSSID(controller string, w adapters.UniFiWlanConf, networkByID map[string]adapters.UniFiNetworkConf, clientsBySSID map[string][]adapters.UniFiSta) Ssid {
	id := fmt.Sprintf("%s.%s", controller, toKebab(w.Name))
	return Ssid{
		Id:         id,
		Uri:        fmt.Sprintf("/network/ssids/%s", id),
		Name:       w.Name,
		VlanId:     lookupVlanID(w.NetworkConfID, networkByID),
		Bands:      mapWifiBands(w.WlanBands),
		NumClients: len(clientsBySSID[w.Name]),
	}
}

func buildSSIDDetail(
	controller string,
	w adapters.UniFiWlanConf,
	networkByID map[string]adapters.UniFiNetworkConf,
	clientsBySSID map[string][]adapters.UniFiSta,
	apMacToAP map[string]adapters.UniFiDevice,
	allDevices []adapters.UniFiDevice,
) SsidDetail {
	id := fmt.Sprintf("%s.%s", controller, toKebab(w.Name))
	ssidClients := clientsBySSID[w.Name]

	clientRefs := make([]NetworkClientRef, 0, len(ssidClients))
	for _, sta := range ssidClients {
		clientRefs = append(clientRefs, clientRef(controller, sta))
	}

	broadcastingAPs := collectBroadcastingAPs(controller, w.Name, ssidClients, apMacToAP, allDevices)

	return SsidDetail{
		Id:               id,
		Uri:              fmt.Sprintf("/network/ssids/%s", id),
		Name:             w.Name,
		VlanId:           lookupVlanID(w.NetworkConfID, networkByID),
		Bands:            mapWifiBands(w.WlanBands),
		NumClients:       len(ssidClients),
		SecurityProtocol: mapWifiSecurity(w.Security, w.WpaMode, w.Wpa3Transition),
		Clients:          clientRefs,
		BroadcastingAps:  broadcastingAPs,
	}
}

// collectBroadcastingAPs returns device refs for all APs currently broadcasting this SSID.
// Primary source: APs seen in the active clients for this SSID (via ApMAC field).
// Fallback: all connected APs from the device list (all connected APs broadcast all enabled SSIDs).
func collectBroadcastingAPs(
	controller string,
	ssidName string,
	ssidClients []adapters.UniFiSta,
	apMacToAP map[string]adapters.UniFiDevice,
	allDevices []adapters.UniFiDevice,
) []NetworkDeviceRef {
	seen := make(map[string]bool)
	var refs []NetworkDeviceRef

	// collect APs referenced by connected clients
	for _, sta := range ssidClients {
		mac := normalizeMac(sta.ApMAC)
		if mac == "" || seen[mac] {
			continue
		}
		if ap, ok := apMacToAP[mac]; ok {
			seen[mac] = true
			refs = append(refs, deviceRef(controller, ap))
		}
	}

	// fall back to all connected APs
	if len(refs) == 0 {
		for _, d := range allDevices {
			if d.Type != "uap" || d.State != 1 {
				continue
			}
			mac := normalizeMac(d.MAC)
			if seen[mac] {
				continue
			}
			seen[mac] = true
			refs = append(refs, deviceRef(controller, d))
		}
	}

	if refs == nil {
		refs = []NetworkDeviceRef{}
	}
	return refs
}

// --- index helpers ---

func buildNetworkByID(networks []adapters.UniFiNetworkConf) map[string]adapters.UniFiNetworkConf {
	m := make(map[string]adapters.UniFiNetworkConf, len(networks))
	for _, n := range networks {
		m[n.ID] = n
	}
	return m
}

func buildClientsBySSID(clients []adapters.UniFiSta) map[string][]adapters.UniFiSta {
	m := make(map[string][]adapters.UniFiSta)
	for _, c := range clients {
		if c.IsWired || c.ESSID == nil {
			continue
		}
		m[*c.ESSID] = append(m[*c.ESSID], c)
	}
	return m
}

func buildAPMacToDevice(devices []adapters.UniFiDevice) map[string]adapters.UniFiDevice {
	m := make(map[string]adapters.UniFiDevice)
	for _, d := range devices {
		if d.Type == "uap" {
			m[normalizeMac(d.MAC)] = d
		}
	}
	return m
}

// lookupVlanID returns the VLAN tag for a SSID's networkconf entry.
// Returns 1 (native VLAN) when the entry is missing or untagged.
func lookupVlanID(networkConfID string, networkByID map[string]adapters.UniFiNetworkConf) int {
	if n, ok := networkByID[networkConfID]; ok {
		return extractVlanID(n.Vlan)
	}
	return 1
}

// --- mapping helpers ---

func mapWifiBands(bands []string) []WifiBand {
	result := make([]WifiBand, 0, len(bands))
	for _, b := range bands {
		switch b {
		case "2g":
			result = append(result, "band2g")
		case "5g":
			result = append(result, "band5g")
		case "6g":
			result = append(result, "band6g")
		}
	}
	return result
}

func mapWifiSecurity(security, wpaMode string, wpa3Transition bool) WifiSecurityProtocol {
	if security == "open" {
		return "open"
	}
	switch wpaMode {
	case "wpa2":
		if wpa3Transition {
			return "wpa2Wpa3"
		}
		return "wpa2"
	case "wpa3":
		return "wpa3"
	default:
		return "wpa2"
	}
}
```

- [ ] **Step 4: Run SSID tests**

```bash
go test ./internal/network/ -run "TestListSSIDs|TestGetSSID" -v
```

Expected: all 4 SSID tests PASS.

- [ ] **Step 5: Run full test suite to catch regressions**

```bash
make test
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/network/ssids_service.go internal/network/service_test.go
git commit -m "feat: implement ListSSIDs and GetSSID"
```

---

## Task 8: Add handler methods

**Files:** Modify `internal/network/handler.go`

The six new handler methods follow the exact same pattern as the existing five. None of the new response types use `anyOf` + discriminator, so no custom union wrappers are needed.

- [ ] **Step 1: Add the six handler methods to `handler.go`**

Append after the existing `GetNetworkTopology` method:

```go
// ListVLANs implements StrictServerInterface.
func (h *ServerHandler) ListVLANs(ctx context.Context, request ListVlansRequestObject) (ListVlansResponseObject, error) {
	result, err := h.svc.ListVLANs(ctx)
	if err != nil {
		detail := err.Error()
		return ListVlans500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListVlans200JSONResponse(result), nil
}

// GetVlan implements StrictServerInterface.
func (h *ServerHandler) GetVlan(ctx context.Context, request GetVlanRequestObject) (GetVlanResponseObject, error) {
	detail, found, err := h.svc.GetVLAN(ctx, request.VlanId)
	if err != nil {
		msg := err.Error()
		return GetVlan500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "VLAN not found: " + request.VlanId
		return GetVlan404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetVlan200JSONResponse(detail), nil
}

// ListWans implements StrictServerInterface.
func (h *ServerHandler) ListWans(ctx context.Context, request ListWansRequestObject) (ListWansResponseObject, error) {
	result, err := h.svc.ListWANs(ctx)
	if err != nil {
		detail := err.Error()
		return ListWans500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListWans200JSONResponse(result), nil
}

// GetWan implements StrictServerInterface.
func (h *ServerHandler) GetWan(ctx context.Context, request GetWanRequestObject) (GetWanResponseObject, error) {
	detail, found, err := h.svc.GetWAN(ctx, request.WanId)
	if err != nil {
		msg := err.Error()
		return GetWan500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "WAN not found: " + request.WanId
		return GetWan404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetWan200JSONResponse(detail), nil
}

// ListSsids implements StrictServerInterface.
func (h *ServerHandler) ListSsids(ctx context.Context, request ListSsidsRequestObject) (ListSsidsResponseObject, error) {
	result, err := h.svc.ListSSIDs(ctx)
	if err != nil {
		detail := err.Error()
		return ListSsids500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListSsids200JSONResponse(result), nil
}

// GetSsid implements StrictServerInterface.
func (h *ServerHandler) GetSsid(ctx context.Context, request GetSsidRequestObject) (GetSsidResponseObject, error) {
	detail, found, err := h.svc.GetSSID(ctx, request.SsidId)
	if err != nil {
		msg := err.Error()
		return GetSsid500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "SSID not found: " + request.SsidId
		return GetSsid404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetSsid200JSONResponse(detail), nil
}
```

**Note:** The exact method and type names above (`ListVlans`, `GetVlan`, `ListWans`, `GetWan`, `ListSsids`, `GetSsid`, and corresponding request/response types) must match what `make generate` produced. Verify against `api.gen.go` before writing — method names follow the `operationId` from the spec (e.g., `listVlans` → `ListVlans`).

- [ ] **Step 2: Build to verify the handler satisfies `StrictServerInterface`**

```bash
make build
```

Expected: exits 0. If any method name is wrong, the compiler will report an interface satisfaction error pointing to the exact mismatch.

- [ ] **Step 3: Run full test suite**

```bash
make test
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/network/handler.go
git commit -m "feat: add SSID, VLAN, WAN handler methods"
```

---

## Task 9: Final verification

- [ ] **Step 1: Run full build + test**

```bash
make build && make test
```

Expected: clean build, all tests pass.

- [ ] **Step 2: Run lint**

```bash
make lint
```

Expected: no errors.

- [ ] **Step 3: Run go mod tidy and check for drift**

```bash
make tidy && git diff go.sum
```

Expected: no changes (no new dependencies added).

- [ ] **Step 4: Commit capture scripts**

```bash
git add scripts/unifi-wlanconf.sh scripts/unifi-networkconf.sh
git commit -m "chore: add UniFi WLAN and network conf capture scripts"
```
