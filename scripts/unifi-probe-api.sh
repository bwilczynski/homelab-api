#!/usr/bin/env bash
# Probes UniFi OS API paths to determine which endpoints are reachable.
# Run: UNIFI_HOST=192.168.1.1 UNIFI_API_KEY=<key> bash scripts/unifi-probe-api.sh
set -euo pipefail

HOST="${UNIFI_HOST:-}"
API_KEY="${UNIFI_API_KEY:-}"

if [[ -z "$HOST" || -z "$API_KEY" ]]; then
  # Try loading from .env if vars not already set
  ENV_FILE="$(dirname "$0")/../.env"
  if [[ -f "$ENV_FILE" ]]; then
    source "$ENV_FILE"
    HOST="${UNIFI_HOST:-$HOST}"
    API_KEY="${UNIFI_API_KEY:-$API_KEY}"
  fi
fi

if [[ -z "$HOST" ]]; then
  echo "ERROR: UNIFI_HOST is required (set it or add to .env)" >&2
  exit 1
fi
if [[ -z "$API_KEY" ]]; then
  echo "ERROR: UNIFI_API_KEY is required (set it or add to .env)" >&2
  exit 1
fi

CURL="curl -sk -w '\n%{http_code}'"
AUTH="-H 'X-API-KEY: ${API_KEY}'"

probe() {
  local label="$1"
  local path="$2"
  local extra="${3:-}"
  echo ""
  echo "=== $label ==="
  echo "    GET https://${HOST}${path}"
  result=$(curl -sk -w "\n__HTTP_CODE__%{http_code}" $extra \
    -H "X-API-KEY: ${API_KEY}" \
    "https://${HOST}${path}" 2>&1) || true
  http_code=$(echo "$result" | grep -o '__HTTP_CODE__[0-9]*' | sed 's/__HTTP_CODE__//')
  body=$(echo "$result" | sed 's/__HTTP_CODE__[0-9]*$//')
  echo "    HTTP $http_code"
  # Print a short summary: first 200 chars of body
  echo "    Body preview: ${body:0:300}" | tr -d '\n'
  echo ""
  # If it's JSON, show top-level keys
  if echo "$body" | jq -e . >/dev/null 2>&1; then
    echo "    JSON keys: $(echo "$body" | jq -c 'keys? // (if type=="array" then ["array(\(length))"] else [type] end)' 2>/dev/null || true)"
  fi
}

echo "============================================================"
echo "UniFi API Probe"
echo "Host: $HOST"
echo "============================================================"

# ── 1. Basic reachability ────────────────────────────────────────
echo ""
echo "--- REACHABILITY ---"
probe "Root" "/"
probe "Ping (UniFi OS status)" "/status"

# ── 2. New UniFi API (UniFi OS 3+/4+) ───────────────────────────
echo ""
echo "--- NEW UNIFI API (/api/) ---"
probe "Devices (new API)" "/api/devices"
probe "Sites (new API)" "/api/sites"
probe "Clients (new API)" "/api/clients"

# ── 3. Legacy Network app via proxy path (UDM/CloudGW) ──────────
echo ""
echo "--- LEGACY NETWORK APP VIA /proxy/network/ ---"
probe "Devices (proxy)" "/proxy/network/api/s/default/stat/device"
probe "Active clients (proxy)" "/proxy/network/api/s/default/stat/sta"
probe "Health (proxy)" "/proxy/network/api/s/default/stat/health"
probe "WLAN conf (proxy)" "/proxy/network/api/s/default/rest/wlanconf"
probe "Network conf (proxy)" "/proxy/network/api/s/default/rest/networkconf"
probe "Active clients v2 (proxy)" "/proxy/network/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false"
probe "Offline clients v2 (proxy)" "/proxy/network/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=720"

# ── 4. Legacy paths (standalone controller or old UDM) ──────────
echo ""
echo "--- LEGACY PATHS (direct, no proxy prefix) ---"
probe "Devices (legacy)" "/api/s/default/stat/device"
probe "Active clients (legacy)" "/api/s/default/stat/sta"
probe "Health (legacy)" "/api/s/default/stat/health"
probe "WLAN conf (legacy)" "/api/s/default/rest/wlanconf"
probe "Network conf (legacy)" "/api/s/default/rest/networkconf"
probe "Active clients v2 (legacy)" "/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false"

# ── 5. New UniFi API site-specific ──────────────────────────────
echo ""
echo "--- NEW UNIFI API SITE-SPECIFIC ---"
probe "Devices (new API, site)" "/api/site/default/devices"
probe "Clients (new API, site)" "/api/site/default/clients"
probe "Active clients (new API, site)" "/api/site/default/clients/active"

echo ""
echo "============================================================"
echo "Done. Share the full output above to determine which paths"
echo "are reachable and what response format to expect."
echo "============================================================"
