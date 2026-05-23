# Network SSID / VLAN / WAN Design

**Date:** 2026-05-22
**Spec commit:** `4c71e7d` (spec submodule) — adds SSID, VLAN, WAN paths and schemas

## Goal

Implement six new endpoints for the network domain — `listSsids`, `getSsid`, `listVlans`, `getVlan`,
`listWans`, `getWan` — backed by the UniFi `wlanconf` and `networkconf` REST APIs.

---

## Architecture

Three new `*_service.go` files follow the existing narrow-backend-interface pattern already used by
`devices_service.go` and `clients_service.go`. Each file defines its own `XxxBackend` interface
that is composed into `UniFiBackend` in `service.go`. Two new adapter methods (`GetWlanConf`,
`GetNetworkConf`) are added to the UniFi client. Handlers delegate to the service and return
RFC 9457 problem+json errors — identical to existing network handlers.

No new API routes or middleware are needed. The generated `StrictServerInterface` in
`api.gen.go` gains six new method slots after `make generate`.

---

## Section 1: UniFi API Endpoints

Two new UniFi REST calls are needed:

| Endpoint | Response field |
|---|---|
| `GET /api/s/default/rest/wlanconf` | `data: []WlanConf` |
| `GET /api/s/default/rest/networkconf` | `data: []NetworkConf` |

Both use the existing cookie-based session auth pattern already implemented in `UniFiClient`.

---

## Section 2: UniFi Data Model

### `networkconf` entry

A single `networkconf` entry covers both LAN networks and WAN interfaces. The `purpose` field
distinguishes them:

- `purpose: "corporate"` or `purpose: "guest"` → LAN network → maps to a **VLAN**
- `purpose: "wan"` → WAN interface → maps to a **WAN**

Key fields and their quirks:

```
_id             string    unique ID — used as foreign key from wlanconf
name            string    human-readable name (e.g. "LAN-IOT", "Internet 1")
purpose         string    "corporate" | "guest" | "wan"

// LAN fields
vlan            mixed     int | "" | null — JSON polymorphic; int when tagged, "" or null when untagged
vlan_enabled    bool
ip_subnet       string    GATEWAY IP + prefix, not network address (e.g. "192.168.1.1/24")
dhcpd_enabled   bool
dhcp_relay_enabled bool
dhcpd_start     string    first DHCP address
dhcpd_stop      string    last DHCP address
dhcpd_dns_1     string    may be empty string
dhcpd_dns_2     string    may be empty string

// WAN fields
wan_networkgroup string   "WAN" | "WAN2" — links to gateway device.wan1 / device.wan2
wan_dns1        string    configured DNS (fallback when live DNS unavailable)
wan_dns2        string
```

**`ip_subnet` quirk:** UniFi stores the gateway IP with prefix length, not the network address.
`net.ParseCIDR("192.168.1.1/24")` correctly separates the host IP from the network mask, yielding:
- `subnet`: `"192.168.1.0/24"` (network address)
- `gatewayIp`: `"192.168.1.1"`
- `broadcastIp`: computed by ORing the host bits

**`vlan` type quirk:** The field is an integer when a VLAN tag is configured, an empty string `""`
when the network is untagged, and `null` for WAN entries. Go unmarshals JSON numbers into
`interface{}` as `float64`, so the extractor type-asserts to `float64` and converts to `int`.
Any non-numeric value defaults to VLAN ID 1 (native VLAN).

### `wlanconf` entry

```
_id              string    unique ID — matched against vap_table[].id on AP devices
name             string    SSID name as broadcast (e.g. "hamster-iot")
networkconf_id   string    FK → networkconf._id; used to look up VLAN ID
security         string    "open" | "wpapsk" | ...
wpa_mode         string    "wpa2" | "wpa3"
wpa3_transition  bool      true = WPA2/WPA3 mixed mode
wlan_bands       []string  ["2g", "5g"] — radio bands this SSID is configured on
enabled          bool
ap_group_mode    string    "all" | "named" | "group" — which APs broadcast this SSID
ap_group_ids     []string  AP group IDs (resolved via vap_table, not via apgroups API)
```

### Gateway device WAN interfaces

The USG/UDM gateway device exposes live WAN state via `wan1` and `wan2` sub-objects. Linkage from
networkconf: `wan_networkgroup == "WAN"` → `device.wan1`; `"WAN2"` → `device.wan2`.

```
wan1.ip     string    current public IP address
wan1.up     bool      link state
wan1.dns    []string  live DNS servers pushed by ISP (may differ from wan_dns1/wan_dns2)
```

**Uptime:** UniFi does not expose per-WAN-interface uptime. `device.uptime` (gateway device uptime)
is used as the best available approximation.

**DNS priority:** Live `wan1.dns` is preferred; falls back to `wan_dns1`/`wan_dns2` from networkconf
when the interface object is absent or its DNS list is empty.

### AP device `vap_table`

Each UAP device has a `vap_table` listing every virtual AP it is actively broadcasting — one row
per radio per SSID:

```
vap_table[].id    string   wlanconf._id of the SSID
vap_table[].up    bool     whether this VAP is actively broadcasting
vap_table[].radio string   "na" (5 GHz) | "ng" (2.4 GHz)
```

A dual-band AP broadcasting 2 SSIDs produces 4 entries. `vap_table` reflects resolved AP group
membership — querying the separate `apgroups` endpoint is unnecessary.

---

## Section 3: API Contract

### VLANs

`GET /network/vlans` → `VlanList { items: Vlan[] }`

`GET /network/vlans/{vlanId}` → `VlanDetail`

```
Vlan (list item):
  id            "{controller}.{toKebab(name)}"
  uri           "/network/vlans/{id}"
  name          string
  vlanId        int          from networkconf.vlan; default 1 if untagged/missing
  subnet        string       network address with prefix ("192.168.1.0/24")
  gatewayIp     string       host IP from ip_subnet ("192.168.1.1")
  broadcastIp   string       broadcast address ("192.168.1.255")
  dhcpMode      server|relay|disabled

VlanDetail (extends Vlan):
  dnsServers    string[]     from dhcpd_dns_1, dhcpd_dns_2 — empty strings filtered out
  dhcpRange     {start, end} present only when dhcpMode == server
  relayServer   string       present only when dhcpMode == relay (dhcpd_start holds the relay target)
```

**dhcpMode mapping** (priority order matters — server wins over relay if both set):
- `dhcpd_enabled == true` → `server`
- `dhcp_relay_enabled == true` → `relay`
- else → `disabled`

**Filtering:** Only `purpose == "corporate"` or `purpose == "guest"` entries.

### WANs

`GET /network/wans` → `WanList { items: Wan[] }`

`GET /network/wans/{wanId}` → `WanDetail`

```
Wan (list item):
  id            "{controller}.{toKebab(name)}"
  uri           "/network/wans/{id}"
  name          string
  ipAddress     string       from gateway device.wan1.ip (empty string if offline)
  status        connected|disconnected
  uptime        int          gateway device.uptime (seconds)

WanDetail (extends Wan):
  dnsServers    string[]     live from device.wan1.dns; fallback to wan_dns1/wan_dns2
```

**Gateway detection:** device type in `{"ugw", "udm", "udm-pro"}`.

**Status:** `wan1.up == true` → `connected`; `wan1.up == false` or interface absent → `disconnected`.

**Filtering:** Only `purpose == "wan"` entries.

### SSIDs

`GET /network/ssids` → `SsidList { items: Ssid[] }`

`GET /network/ssids/{ssidId}` → `SsidDetail`

```
Ssid (list item):
  id            "{controller}.{toKebab(name)}"
  uri           "/network/ssids/{id}"
  name          string
  vlanId        int          via networkconf_id → networkconf.vlan; default 1 if missing
  bands         WifiBand[]   mapped from wlan_bands: "2g"→band2g, "5g"→band5g, "6g"→band6g
  numClients    int          count of active (non-wired) UniFiSta with matching essid

SsidDetail (extends Ssid):
  securityProtocol  WifiSecurityProtocol
  clients           NetworkClientRef[]
  broadcastingAps   NetworkDeviceRef[]   sorted by device ID
```

**Filtering for list:** `enabled == true` only. `GetSSID` looks up by name regardless of enabled
state (direct lookup, not a filtered list).

**numClients:** Counted from `GetClients()` (`/api/s/default/rest/sta`), filtering to
`!IsWired && ESSID != nil`. Active clients only — offline clients are not included.

**broadcastingAps:** For each connected UAP (`type == "uap"`, `state == 1`), check if its
`vap_table` contains an entry where `id == wlanconf._id && up == true`. Break after first match
per AP (avoids per-radio duplicates). See separate spec
`2026-05-23-ssid-broadcasting-aps-design.md` for full rationale.

**Security protocol mapping:**

| `security` | `wpa_mode` | `wpa3_transition` | Result |
|---|---|---|---|
| `"open"` | any | any | `open` |
| other | `"wpa2"` | `true` | `wpa2Wpa3` |
| other | `"wpa2"` | `false` | `wpa2` |
| other | `"wpa3"` | any | `wpa3` |
| other | other | any | `wpa2` (default) |

---

## Section 4: Composite ID Format

All three resources use the same pattern: `{controller}.{toKebab(name)}`.

`toKebab` lowercases the string and replaces spaces and underscores with hyphens. Examples:
- `"LAN-IOT"` → `"lan-iot"` → id `"unifi.lan-iot"`
- `"Internet 1"` → `"internet-1"` → id `"unifi.internet-1"`
- `"hamster-iot"` → `"hamster-iot"` → id `"unifi.hamster-iot"`

`GetXxx` handlers parse the composite ID with `parseID(id)` to split controller and name, then
compare `toKebab(entry.Name) == name` for lookup.

---

## Section 5: Implementation Structure

```
internal/adapters/unifi.go       Add UniFiWlanConf, UniFiNetworkConf, UniFiVap structs;
                                  add VapTable field to UniFiDevice;
                                  add GetWlanConf(), GetNetworkConf() methods

internal/network/api.gen.go      Regenerated — new operations, enums, response types

internal/network/service.go      Extend UniFiBackend to embed SSIDsBackend, VLANsBackend, WANsBackend

internal/network/vlans_service.go   VLANsBackend interface, ListVLANs, GetVLAN, helpers
internal/network/wans_service.go    WANsBackend interface, ListWANs, GetWAN, helpers
internal/network/ssids_service.go   SSIDsBackend interface, ListSSIDs, GetSSID, helpers

internal/network/handler.go      6 new handler methods (ListVlans, GetVlan, ListWans, GetWan,
                                  ListSsids, GetSsid) — no custom union wrappers needed

internal/network/service_test.go    Extend mockUniFi; add VLAN, WAN, SSID tests
internal/network/testdata/unifi-wlanconf.json    New fixture
internal/network/testdata/unifi-networkconf.json New fixture
```

---

## Section 6: Key Design Decisions

**Shared helpers across service files.** `parseIPSubnet`, `extractVlanID`, `mapDhcpMode`,
`collectDNSServers` are defined in `vlans_service.go` and called from `ssids_service.go`.
These are not promoted to `service.go` to keep the service file focused on orchestration;
the VLAN file owns the DHCP/IP parsing logic.

**`vap_table` over `ap_group_ids` for broadcastingAps.** The `ap_group_ids` field on wlanconf
requires a separate `/api/s/default/rest/apgroups` call to resolve AP membership. `vap_table` on
each AP device already contains the resolved broadcast state and handles all three targeting modes
(All / Specific / Groups) without an extra round-trip.

**No per-interface WAN uptime.** UniFi does not expose per-WAN-interface uptime in `wan1`/`wan2`.
Gateway device `uptime` is used instead. This is documented in a comment on `wanLiveFields`.

**DNS source priority for WANs.** Live `wan1.dns` is preferred because it reflects what the ISP
actually pushed (e.g. `127.0.0.1` for a local resolver) rather than the static config. Fallback
to `wan_dns1`/`wan_dns2` from networkconf when the live list is empty.

**`ip_subnet` holds gateway IP, not network address.** This is a UniFi quirk — the field stores
`"192.168.1.1/24"` not `"192.168.1.0/24"`. `net.ParseCIDR` is used to separate host IP from
network, then broadcast is computed by filling host bits.
