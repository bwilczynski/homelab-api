# Network Topology Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `GET /network/topology` — a graph of device nodes + optional client nodes with wired/wireless edges, backed by the UniFi adapter.

**Architecture:** Three assembly passes per backend: (1) device nodes + device-uplink edges, (2) online client nodes from V1 stas, (3) offline client nodes from V2 history. Online wired edges carry switch port + link speed; offline edges omit them. No new adapter methods needed.

**Tech Stack:** Go, oapi-codegen strict server, chi router, UniFi adapter (existing).

---

### Task 1: Regenerate server stubs

The `spec/` submodule has already been updated (`git submodule update --remote spec` was run). Run `make generate` to produce the new types and add `GetNetworkTopology` to `StrictServerInterface`.

**Files:**
- Modify: `internal/network/api.gen.go` (auto-generated, read-only after this step)

- [ ] **Step 1: Run code generation**

```bash
make generate
```

Expected: no errors, `internal/network/api.gen.go` updated.

- [ ] **Step 2: Verify new types and interface method exist**

```bash
grep -n "GetNetworkTopology\|NetworkTopology\|TopologyDeviceNode\|TopologyClientNode\|TopologyWiredEdge\|TopologyWirelessEdge\|TopologyNode\|TopologyEdge" internal/network/api.gen.go | head -40
```

Expected: all six types present, `GetNetworkTopology` in `StrictServerInterface`.

- [ ] **Step 3: Note the Kind field types for wired/wireless edges**

```bash
grep -A5 "type TopologyWiredEdge\|type TopologyWirelessEdge" internal/network/api.gen.go
```

If `Kind` is a named string type (e.g. `TopologyWiredEdgeKind`), note the constant name — you will use it in Task 3. If it is `string`, the string literals `"wired"` and `"wireless"` used in the plan are correct as-is.

- [ ] **Step 4: Note the GetNetworkTopologyParams type**

```bash
grep -A5 "GetNetworkTopologyParams\|IncludeClients" internal/network/api.gen.go
```

Confirm the query param field name (expected: `IncludeClients *bool`).

- [ ] **Step 5: Commit the regenerated file**

```bash
git add internal/network/api.gen.go
git commit -m "chore: regenerate network stubs — add GetNetworkTopology"
```

---

### Task 2: Add stub handler to restore compilation

`GetNetworkTopology` is now in `StrictServerInterface` but `ServerHandler` does not implement it. Add a stub so the project compiles.

**Files:**
- Modify: `internal/network/handler.go`

- [ ] **Step 1: Add stub handler method**

Append to `internal/network/handler.go` (before the final closing brace, or at the end of the file):

```go
// GetNetworkTopology implements StrictServerInterface.
// Full implementation wired in Task 5.
func (h *ServerHandler) GetNetworkTopology(ctx context.Context, request GetNetworkTopologyRequestObject) (GetNetworkTopologyResponseObject, error) {
	detail := "not implemented"
	return GetNetworkTopology500ApplicationProblemPlusJSONResponse{
		InternalServerErrorApplicationProblemPlusJSONResponse{
			Type:   apierrors.URNInternalServerError,
			Title:  apierrors.TitleInternalServerError,
			Status: 500,
			Detail: &detail,
		},
	}, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
make build
```

Expected: `bin/server` produced with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/network/handler.go
git commit -m "feat: stub GetNetworkTopology handler"
```

---

### Task 3: TDD — topology service (devices only)

Create `internal/network/topology_service.go` with the `TopologyBackend` interface and a `GetTopology` method covering Pass 1 (device nodes + device-device uplink edges).

**Files:**
- Create: `internal/network/topology_service.go`
- Modify: `internal/network/service_test.go`

**Fixtures used (from `testdata/`):**

| Fixture | Contents summary |
|---------|-----------------|
| `unifi-devices.json` | 5 devices: USG 3P (gateway, no uplink), US 8 60W (switch, uplink→USG via .01, speed=1000, remote_port=nil), Switch Flex Mini (switch, uplink→US8 via .02, speed=1000, remote_port=6), UAP-01 (AP, uplink→US8 via .02, speed=1000, remote_port=5, user_num_sta=7), UAP-02 (AP, offline state=0, no uplink) |

Expected for devices-only: 5 nodes, 3 edges (US8→USG, FlexMini→US8, UAP01→US8). USG has no outgoing edge. UAP-01 numClients=7. Non-AP nodes have no numClients field.

- [ ] **Step 1: Write the failing tests**

Add the following to `internal/network/service_test.go` (after the existing tests):

```go
// --- topology tests ---

// testNode and testEdge are thin structs for inspecting union topology types via JSON round-trip.
type testNode struct {
	Kind           string `json:"kind"`
	Id             string `json:"id"`
	Type           string `json:"type,omitempty"`
	Status         string `json:"status,omitempty"`
	NumClients     *int   `json:"numClients,omitempty"`
	ConnectionType string `json:"connectionType,omitempty"`
}

type testEdge struct {
	Kind   string `json:"kind"`
	Source struct {
		Kind string `json:"kind"`
		Id   string `json:"id"`
	} `json:"source"`
	Target struct {
		Id string `json:"id"`
	} `json:"target"`
	Port           *int    `json:"port,omitempty"`
	LinkSpeed      *string `json:"linkSpeed,omitempty"`
	Ssid           *string `json:"ssid,omitempty"`
	SignalStrength  *int    `json:"signalStrength,omitempty"`
}

func parseTopologyNode(t *testing.T, n TopologyNode) testNode {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal node: %v", err)
	}
	var tn testNode
	if err := json.Unmarshal(b, &tn); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	return tn
}

func parseTopologyEdge(t *testing.T, e TopologyEdge) testEdge {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal edge: %v", err)
	}
	var te testEdge
	if err := json.Unmarshal(b, &te); err != nil {
		t.Fatalf("unmarshal edge: %v", err)
	}
	return te
}

func TestGetTopology_DevicesOnly(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	svc := NewService(map[string]UniFiBackend{"unifi": &mockUniFi{devices: devices}}, 30)

	topo, err := svc.GetTopology(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 device nodes, 0 client nodes
	if len(topo.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(topo.Nodes))
	}

	// 3 device-device edges; UAP-02 (offline, no uplink) contributes none
	if len(topo.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(topo.Edges))
	}

	// all nodes are kind=device
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Kind != "device" {
			t.Errorf("expected kind=device, got %s (id=%s)", pn.Kind, pn.Id)
		}
	}

	// gateway node: type=gateway, no outgoing edge
	gatewayCount := 0
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Type == "gateway" {
			gatewayCount++
		}
	}
	if gatewayCount != 1 {
		t.Errorf("expected 1 gateway node, got %d", gatewayCount)
	}
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.usg-3p" {
			t.Error("gateway should not be an edge source")
		}
	}

	// AP node numClients: UAP-01 has user_num_sta=7; UAP-02 has 0
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Id == "unifi.uap-01" {
			if pn.NumClients == nil || *pn.NumClients != 7 {
				t.Errorf("UAP-01: expected numClients=7, got %v", pn.NumClients)
			}
		}
		if pn.Id == "unifi.uap-02" {
			if pn.NumClients == nil || *pn.NumClients != 0 {
				t.Errorf("UAP-02: expected numClients=0, got %v", pn.NumClients)
			}
		}
		// switches and gateway must not have numClients
		if pn.Type == "switch" || pn.Type == "gateway" {
			if pn.NumClients != nil {
				t.Errorf("%s (%s): expected no numClients, got %d", pn.Id, pn.Type, *pn.NumClients)
			}
		}
	}

	// edge UAP-01 → US 8 60W: wired, port=5, linkSpeed=gbe1
	var uap01Edge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.uap-01" {
			uap01Edge = &pe
			break
		}
	}
	if uap01Edge == nil {
		t.Fatal("expected edge with source=unifi.uap-01")
	}
	if uap01Edge.Target.Id != "unifi.us-8-60w" {
		t.Errorf("UAP-01 edge target: expected unifi.us-8-60w, got %s", uap01Edge.Target.Id)
	}
	if uap01Edge.Port == nil || *uap01Edge.Port != 5 {
		t.Errorf("UAP-01 edge port: expected 5, got %v", uap01Edge.Port)
	}
	if uap01Edge.LinkSpeed == nil || *uap01Edge.LinkSpeed != "gbe1" {
		t.Errorf("UAP-01 edge linkSpeed: expected gbe1, got %v", uap01Edge.LinkSpeed)
	}

	// edge Switch Flex Mini → US 8 60W: wired, port=6, linkSpeed=gbe1
	var flexEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.switch-flex-mini" {
			flexEdge = &pe
			break
		}
	}
	if flexEdge == nil {
		t.Fatal("expected edge with source=unifi.switch-flex-mini")
	}
	if flexEdge.Port == nil || *flexEdge.Port != 6 {
		t.Errorf("Flex Mini edge port: expected 6, got %v", flexEdge.Port)
	}

	// edge US 8 60W → USG: wired, port=nil (uplink_remote_port is null in fixture), linkSpeed=gbe1
	var us8Edge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Source.Id == "unifi.us-8-60w" {
			us8Edge = &pe
			break
		}
	}
	if us8Edge == nil {
		t.Fatal("expected edge with source=unifi.us-8-60w")
	}
	if us8Edge.Target.Id != "unifi.usg-3p" {
		t.Errorf("US8 edge target: expected unifi.usg-3p, got %s", us8Edge.Target.Id)
	}
	if us8Edge.Port != nil {
		t.Errorf("US8 edge port: expected nil (no uplink_remote_port in fixture), got %d", *us8Edge.Port)
	}
	if us8Edge.LinkSpeed == nil || *us8Edge.LinkSpeed != "gbe1" {
		t.Errorf("US8 edge linkSpeed: expected gbe1, got %v", us8Edge.LinkSpeed)
	}
}
```

- [ ] **Step 2: Run the tests — expect failure**

```bash
go test ./internal/network/ -run TestGetTopology_DevicesOnly -v
```

Expected: compile error (`GetTopology` undefined) or test failure.

- [ ] **Step 3: Create topology_service.go**

Create `internal/network/topology_service.go`:

```go
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

		// Pass 2+3: online client nodes + edges.
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
		Kind:   Device,
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
// Port is omitted when UplinkRemotePort is nil (e.g. USG which uplinks to WAN).
func buildDeviceUplinkEdge(controller string, d adapters.UniFiDevice, upstream adapters.UniFiDevice) (TopologyEdge, error) {
	srcRef := deviceRef(controller, d)
	var source NetworkConnectionRef
	if err := source.FromNetworkDeviceRef(srcRef); err != nil {
		return TopologyEdge{}, err
	}
	tgtRef := deviceRef(controller, upstream)

	wired := TopologyWiredEdge{
		Kind:   "wired",
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
		Kind:           Client,
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
			Kind:   "wired",
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
		Kind:   "wireless",
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
		Kind:           Client,
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
			Kind:   "wired",
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
		Kind:   "wireless",
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
		Kind: Client,
		Id:   id,
		Uri:  fmt.Sprintf("/network/clients/%s", id),
		Name: name,
	}
}
```

> **Note on Kind fields:** `TopologyWiredEdge.Kind` and `TopologyWirelessEdge.Kind` use string literals `"wired"` / `"wireless"`. In Go, an untyped string constant can be assigned to any named string type, so this compiles regardless of whether `Kind` is `string` or a generated named type. If the compiler rejects it, check the generated constant name in `api.gen.go` and use it instead.

- [ ] **Step 4: Run the test — expect pass**

```bash
go test ./internal/network/ -run TestGetTopology_DevicesOnly -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/topology_service.go internal/network/service_test.go
git commit -m "feat: GetTopology — device nodes and uplink edges"
```

---

### Task 4: TDD — extend topology with client nodes and edges

Add `TestGetTopology_WithClients` and implement the client passes in `GetTopology` (already written in Task 3 — verify the test passes, fix if needed).

**Files:**
- Modify: `internal/network/service_test.go`

**Fixture summary for this test:**

| Fixture | Key data |
|---------|----------|
| `unifi-clients.json` | 5 online V1 stas: MacBook Pro (wireless→UAP-01 .04, ssid=homelab, signal=-69), nas-1 (wired→US8 .02, port=3, rate=1000Mbps→gbe1), Nintendo Switch (wireless→UAP-01, ssid=homelab-iot, signal=-49), iPhone (wireless→UAP-01, ssid=homelab, signal=-66), ec:b5:fa (wired→UAP-01 .04, port=6, rate=100Mbps→fe) |
| `unifi-v2-history.json` | 2 offline V2 clients: Kindle Paperwhite (wireless, last_uplink=UAP-01 .04, ssid=homelab), host-02 aa (wired, last_uplink=FlexMini .03, no port) |

Expected counts: 5 device + 5 online client + 2 offline client = **12 nodes**; 3 device-device + 5 online-client + 2 offline-client = **10 edges**.

Client IDs (derived from `{controller}.{toKebab(name)}-{mac[0:2]}`):
- nas-1: `unifi.nas-1-68` (mac=68:d7:…)
- MacBook Pro: `unifi.macbook-pro-3c` (mac=3c:22:…)
- Kindle Paperwhite: `unifi.kindle-paperwhite-e0` (mac=e0:f7:…)
- host-02 aa: `unifi.host-02-aa-aa` (mac=aa:bb:…)

- [ ] **Step 1: Add the failing test**

Append to `internal/network/service_test.go`:

```go
func TestGetTopology_WithClients(t *testing.T) {
	devices := loadFixture[[]adapters.UniFiDevice](t, "testdata/unifi-devices.json")
	clients := loadFixture[[]adapters.UniFiSta](t, "testdata/unifi-clients.json")
	history := loadFixture[[]adapters.UniFiClientV2](t, "testdata/unifi-v2-history.json")

	mock := &mockUniFi{devices: devices, clients: clients, offlineClients: history}
	svc := NewService(map[string]UniFiBackend{"unifi": mock}, 30)

	topo, err := svc.GetTopology(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 device + 5 online client + 2 offline client = 12 nodes
	if len(topo.Nodes) != 12 {
		t.Fatalf("expected 12 nodes, got %d", len(topo.Nodes))
	}

	// 3 device-device + 5 online-client + 2 offline-client = 10 edges
	if len(topo.Edges) != 10 {
		t.Fatalf("expected 10 edges, got %d", len(topo.Edges))
	}

	// Verify node kinds: 5 device + 7 client
	deviceNodes, clientNodes := 0, 0
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		switch pn.Kind {
		case "device":
			deviceNodes++
		case "client":
			clientNodes++
		default:
			t.Errorf("unexpected node kind: %s", pn.Kind)
		}
	}
	if deviceNodes != 5 {
		t.Errorf("expected 5 device nodes, got %d", deviceNodes)
	}
	if clientNodes != 7 {
		t.Errorf("expected 7 client nodes (5 online + 2 offline), got %d", clientNodes)
	}

	// Online wired edge: nas-1 → US 8 60W, port=3, linkSpeed=gbe1
	var nasEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wired" && pe.Source.Kind == "client" && pe.Source.Id == "unifi.nas-1-68" {
			nasEdge = &pe
			break
		}
	}
	if nasEdge == nil {
		t.Fatal("expected wired edge for nas-1")
	}
	if nasEdge.Target.Id != "unifi.us-8-60w" {
		t.Errorf("nas-1 edge target: expected unifi.us-8-60w, got %s", nasEdge.Target.Id)
	}
	if nasEdge.Port == nil || *nasEdge.Port != 3 {
		t.Errorf("nas-1 edge port: expected 3, got %v", nasEdge.Port)
	}
	if nasEdge.LinkSpeed == nil || *nasEdge.LinkSpeed != "gbe1" {
		t.Errorf("nas-1 edge linkSpeed: expected gbe1, got %v", nasEdge.LinkSpeed)
	}

	// Online wireless edge: MacBook Pro → UAP-01, ssid=homelab, signalStrength=-69
	var macbookEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wireless" && pe.Source.Id == "unifi.macbook-pro-3c" {
			macbookEdge = &pe
			break
		}
	}
	if macbookEdge == nil {
		t.Fatal("expected wireless edge for MacBook Pro")
	}
	if macbookEdge.Target.Id != "unifi.uap-01" {
		t.Errorf("MacBook Pro edge target: expected unifi.uap-01, got %s", macbookEdge.Target.Id)
	}
	if macbookEdge.Ssid == nil || *macbookEdge.Ssid != "homelab" {
		t.Errorf("MacBook Pro edge ssid: expected homelab, got %v", macbookEdge.Ssid)
	}
	if macbookEdge.SignalStrength == nil || *macbookEdge.SignalStrength != -69 {
		t.Errorf("MacBook Pro edge signalStrength: expected -69, got %v", macbookEdge.SignalStrength)
	}

	// Offline wireless edge: Kindle Paperwhite → UAP-01, ssid=homelab, no signalStrength
	var kindleEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wireless" && pe.Source.Id == "unifi.kindle-paperwhite-e0" {
			kindleEdge = &pe
			break
		}
	}
	if kindleEdge == nil {
		t.Fatal("expected wireless edge for Kindle Paperwhite")
	}
	if kindleEdge.Target.Id != "unifi.uap-01" {
		t.Errorf("Kindle edge target: expected unifi.uap-01, got %s", kindleEdge.Target.Id)
	}
	if kindleEdge.Ssid == nil || *kindleEdge.Ssid != "homelab" {
		t.Errorf("Kindle edge ssid: expected homelab, got %v", kindleEdge.Ssid)
	}
	if kindleEdge.SignalStrength != nil {
		t.Errorf("Kindle (offline): expected no signalStrength, got %d", *kindleEdge.SignalStrength)
	}

	// Offline wired edge: host-02 aa → Switch Flex Mini, no port
	var hostEdge *testEdge
	for _, e := range topo.Edges {
		pe := parseTopologyEdge(t, e)
		if pe.Kind == "wired" && pe.Source.Id == "unifi.host-02-aa-aa" {
			hostEdge = &pe
			break
		}
	}
	if hostEdge == nil {
		t.Fatal("expected wired edge for host-02 aa")
	}
	if hostEdge.Target.Id != "unifi.switch-flex-mini" {
		t.Errorf("host-02 edge target: expected unifi.switch-flex-mini, got %s", hostEdge.Target.Id)
	}
	if hostEdge.Port != nil {
		t.Errorf("host-02 (offline): expected no port, got %d", *hostEdge.Port)
	}

	// Offline client nodes have status=offline; online client nodes have status=online
	for _, n := range topo.Nodes {
		pn := parseTopologyNode(t, n)
		if pn.Kind != "client" {
			continue
		}
		switch pn.Id {
		case "unifi.kindle-paperwhite-e0", "unifi.host-02-aa-aa":
			if pn.Status != "offline" {
				t.Errorf("%s: expected status=offline, got %s", pn.Id, pn.Status)
			}
		default:
			if pn.Status != "online" {
				t.Errorf("%s: expected status=online, got %s", pn.Id, pn.Status)
			}
		}
	}
}
```

- [ ] **Step 2: Run the test — expect pass (implementation already written in Task 3)**

```bash
go test ./internal/network/ -run TestGetTopology -v
```

Expected: both `TestGetTopology_DevicesOnly` and `TestGetTopology_WithClients` PASS. If any test fails, debug and fix `topology_service.go` before proceeding.

- [ ] **Step 3: Commit**

```bash
git add internal/network/service_test.go
git commit -m "test: topology with client nodes and edges"
```

---

### Task 5: Wire the real handler and final verification

Replace the stub handler from Task 2 with the real `GetTopology` call. Verify no custom response wrapper is needed (topology is a plain struct — nested union slices encode correctly via each element's `MarshalJSON`).

**Files:**
- Modify: `internal/network/handler.go`

- [ ] **Step 1: Replace stub with real handler**

Replace the `GetNetworkTopology` method in `internal/network/handler.go`:

```go
// GetNetworkTopology implements StrictServerInterface.
func (h *ServerHandler) GetNetworkTopology(ctx context.Context, request GetNetworkTopologyRequestObject) (GetNetworkTopologyResponseObject, error) {
	includeClients := request.Params.IncludeClients != nil && *request.Params.IncludeClients
	result, err := h.svc.GetTopology(ctx, includeClients)
	if err != nil {
		detail := err.Error()
		return GetNetworkTopology500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return GetNetworkTopology200JSONResponse(result), nil
}
```

- [ ] **Step 2: Run all tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 3: Build**

```bash
make build
```

Expected: `bin/server` produced with no errors.

- [ ] **Step 4: Lint**

```bash
make lint
```

Expected: no issues.

- [ ] **Step 5: Commit**

```bash
git add internal/network/handler.go
git commit -m "feat: wire GetNetworkTopology handler"
```

---

## Self-review

**Spec coverage:**
- `GET /network/topology` → `getNetworkTopology` ✓ (Task 5)
- `includeClients` query param → `request.Params.IncludeClients` ✓ (Task 5)
- Device nodes with type/status/numClients(APs only) ✓ (Task 3, `buildDeviceNode`)
- Client nodes with connectionType/status ✓ (Task 3, `buildOnlineClientNode`, `buildOfflineClientNode`)
- Device-device wired edges with port+linkSpeed ✓ (Task 3, `buildDeviceUplinkEdge`)
- Online wired client edges with port+linkSpeed ✓ (Task 3, `buildOnlineClientEdge`)
- Online wireless client edges with ssid+signalStrength ✓ (Task 3, `buildOnlineClientEdge`)
- Offline wired edges without port ✓ (Task 3, `buildOfflineClientEdge`)
- Offline wireless edges with ssid, without signalStrength ✓ (Task 3, `buildOfflineClientEdge`)
- 500 on backend error ✓ (Task 5)
- Gateway identified by type=gateway, no outgoing edge ✓ (by construction — USG has no uplink)
- 401, 429, 500 response types → generated stubs handle auth/rate-limit middleware ✓

**No placeholders:** All code is complete. Kind field note is a compile-time check, not a TBD.

**Type consistency:** `clientRefV2`, `buildOnlineClientNode`, `buildOfflineClientNode`, `buildOnlineClientEdge`, `buildOfflineClientEdge` all use `NetworkClientRef`, `NetworkConnectionRef`, `TopologyClientNode`, `TopologyWiredEdge`, `TopologyWirelessEdge`, `TopologyEdge`, `TopologyNode` — consistent with generated types. `deviceRef` reused from `devices_service.go`. `clientRef`/`clientNameV2`/`mapConnectionType`/`mapLinkSpeed`/`mapDeviceType`/`mapDeviceStatus`/`normalizeMac`/`toKebab` reused from existing service files.
