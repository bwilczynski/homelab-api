#!/usr/bin/env bash
# Captures raw UniFi OS API responses (via /proxy/network/) for struct verification.
# Requires UNIFI_HOST and UNIFI_API_KEY to be set (or present in .env).
set -euo pipefail

ENV_FILE="$(dirname "$0")/../.env"
[[ -f "$ENV_FILE" ]] && source "$ENV_FILE"

HOST="${UNIFI_HOST:-}"
API_KEY="${UNIFI_API_KEY:-}"

if [[ -z "$HOST" || -z "$API_KEY" ]]; then
  echo "ERROR: UNIFI_HOST and UNIFI_API_KEY are required" >&2
  exit 1
fi

OUT="$(dirname "$0")/responses"
mkdir -p "$OUT"

get() {
  local label="$1"
  local path="$2"
  local file="$3"
  echo "Fetching $label ..."
  curl -sk -H "X-API-KEY: ${API_KEY}" "https://${HOST}${path}" > "${OUT}/${file}"
  echo "  -> saved to scripts/responses/${file}"
  echo "  -> keys: $(jq -c 'if type == "array" then {array_length: length, first_keys: (.[0] | keys)} else keys end' "${OUT}/${file}" 2>/dev/null || echo "(not JSON)")"
}

get "devices"           "/proxy/network/api/s/default/stat/device"   "unifi-devices-os-raw.json"
get "active clients v1" "/proxy/network/api/s/default/stat/sta"       "unifi-sta-os-raw.json"
get "health"            "/proxy/network/api/s/default/stat/health"    "unifi-health-os-raw.json"
get "wlanconf"          "/proxy/network/api/s/default/rest/wlanconf"  "unifi-wlanconf-os-raw.json"
get "networkconf"       "/proxy/network/api/s/default/rest/networkconf" "unifi-networkconf-os-raw.json"
get "active clients v2" "/proxy/network/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false" "unifi-v2-active-os-raw.json"
get "offline clients v2" "/proxy/network/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=720" "unifi-v2-history-os-raw.json"

echo ""
echo "Done. Raw responses saved to scripts/responses/"
