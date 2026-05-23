# SSID Broadcasting APs Design

**Date:** 2026-05-23
**Impl commit:** `28e53cd` — "fix: derive broadcastingAps from vap_table instead of client inference"

## Goal

Replace the client-inference approach to `broadcastingAps` on `SsidDetail` with an authoritative
source from the UniFi AP device data — so that every connected AP broadcasting an SSID appears in
the list regardless of whether it currently has any clients connected.

---

## Problem Statement

The initial implementation derived `broadcastingAps` from active client records: it collected the
`ap_mac` field from each wireless client connected to the SSID, then resolved those MACs to device
refs. A fallback returned all connected UAPs when no clients were present.

This is wrong for two reasons:

1. **An AP with zero clients appears to not be broadcasting.** In a lightly-loaded network, an AP
   may broadcast an SSID continuously without any clients connected at query time.
2. **The fallback is not semantically correct.** Falling back to "all connected APs" conflates
   "this AP broadcasts this SSID" with "this AP is online" — it would include APs that are not
   configured to broadcast the SSID.

The UniFi web UI shows both APs broadcasting both SSIDs regardless of client count. The correct
data source is the AP device's `vap_table`.

---

## Research: UniFi Data Model

### `wlanconf` entry

Each SSID has a `wlanconf` entry with a unique `_id`:

```json
{
  "_id": "5e1ccd6caf427c0011f58d7c",
  "name": "hamster-iot",
  "ap_group_mode": "all",
  "ap_group_ids": ["615359caa6635245fda89e2d"],
  "enabled": true
}
```

`ap_group_mode` can be `"all"`, `"named"` (specific APs), or `"group"`. Rather than resolving
`ap_group_ids` through a separate `/api/s/default/rest/apgroups` endpoint, we use a simpler
authoritative source: `vap_table`.

### AP device `vap_table`

Each UAP device has a `vap_table` array listing every virtual AP it is currently broadcasting,
one row per radio per SSID. The `id` field is the `wlanconf._id`:

```json
{
  "name": "UAP-01",
  "type": "uap",
  "state": 1,
  "vap_table": [
    { "id": "5e1ccd6caf427c0011f58d7c", "up": true, "radio": "na" },
    { "id": "5e163cedaf427c0011f23b66", "up": true, "radio": "na" },
    { "id": "5e1ccd6caf427c0011f58d7c", "up": true, "radio": "ng" },
    { "id": "5e163cedaf427c0011f23b66", "up": true, "radio": "ng" }
  ]
}
```

Each SSID appears twice (once per radio band: `na` = 5 GHz, `ng` = 2.4 GHz). The `up` field
reflects whether the VAP is actively broadcasting.

### Why `vap_table` is the right source

- It is authoritative: populated by the controller from actual AP state, not inferred from clients.
- It handles all three UI targeting modes (All / Specific / Groups) correctly — the controller
  resolves group membership before writing `vap_table`, so the API consumer does not need to.
- It reflects per-radio state: an AP can broadcast an SSID on 5 GHz but not 2.4 GHz (`up: false`
  on one entry). Checking `up == true` on any entry is the right condition.
- Disconnected APs (`device.state != 1`) have stale `vap_table` entries; filtering by `state == 1`
  first excludes them naturally.

---

## Design

### New adapter type

Add `UniFiVap` to `internal/adapters/unifi.go` and a `VapTable` field on `UniFiDevice`:

```go
type UniFiVap struct {
    ID  string `json:"id"` // wlanconf _id
    Up  bool   `json:"up"`
}

// in UniFiDevice:
VapTable []UniFiVap `json:"vap_table"`
```

Only `id` and `up` are needed. `radio` and `essid` are not captured (YAGNI).

### `collectBroadcastingAPs` logic

```go
func collectBroadcastingAPs(controller string, wlanID string, deviceByMAC map[string]adapters.UniFiDevice) []NetworkDeviceRef {
    var refs []NetworkDeviceRef
    for _, d := range deviceByMAC {
        if d.Type != "uap" || d.State != 1 {
            continue
        }
        for _, vap := range d.VapTable {
            if vap.ID == wlanID && vap.Up {
                refs = append(refs, deviceRef(controller, d))
                break // one ref per AP regardless of radio count
            }
        }
    }
    slices.SortFunc(refs, func(a, b NetworkDeviceRef) int {
        return cmp.Compare(a.Id, b.Id)
    })
    return refs
}
```

`wlanID` is `UniFiWlanConf.ID` (the `_id` field), passed from `GetSSID` after the wlan lookup.
The `break` avoids adding the same AP twice (one entry per radio band in `vap_table`).
Results are sorted by device ID for deterministic ordering.

### Dropped: client-based inference and fallback

The previous primary path (collect AP MACs from `sta.ap_mac`) and the fallback (all connected UAPs)
are both removed. `collectBroadcastingAPs` no longer takes a `clients` parameter.

---

## Fixture changes

`internal/network/testdata/unifi-devices.json` — add `vap_table` to both UAP entries using the
sanitized wlanconf IDs from `unifi-wlanconf.json`:

```json
"vap_table": [
  { "id": "aabbccddee0011223344aa01", "up": true, "radio": "na" },
  { "id": "aabbccddee0011223344aa02", "up": true, "radio": "na" },
  { "id": "aabbccddee0011223344aa01", "up": true, "radio": "ng" },
  { "id": "aabbccddee0011223344aa02", "up": true, "radio": "ng" }
]
```

UAP-02 remains `state: 0` (disconnected) in the fixture — the test asserts 1 broadcasting AP, not
2, which validates that disconnected APs are excluded.

---

## What was not designed (YAGNI)

- **AP group resolution** via `/api/s/default/rest/apgroups` — unnecessary since `vap_table`
  already reflects the resolved state.
- **Per-radio breakdown** in the response — `broadcastingAps` is a flat list of devices; exposing
  which bands each AP is using on a given SSID is out of scope for this endpoint.
- **`wlangroup_id_na` / `wlangroup_id_ng`** on AP devices — these reference UniFi "WLAN Groups"
  (a legacy grouping feature), not AP Groups. Not used.
