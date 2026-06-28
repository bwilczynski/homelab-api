#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

RESPONSES_DIR="$(dirname "$0")/responses"

echo "=== /proxy/network/stat/sta (active clients v1) ===" >&2
curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/stat/sta" \
  | tee "${RESPONSES_DIR}/unifi-sta-raw.json" \
  | jq '{count: (.data|length), keys: (.data[0]|keys)}' 2>/dev/null || true

echo "" >&2
echo "=== /proxy/network/v2/clients/active ===" >&2
curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/clients/active?includeTrafficUsage=false&includeUnifiDevices=false" \
  | tee "${RESPONSES_DIR}/unifi-v2-active-raw.json" \
  | jq '{count: length, keys: (.[0]|keys)}' 2>/dev/null || true

echo "" >&2
echo "=== /proxy/network/v2/clients/history ===" >&2
curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/v2/api/site/default/clients/history?onlyNonBlocked=true&withinHours=720" \
  | tee "${RESPONSES_DIR}/unifi-v2-history-raw.json" \
  | jq '{count: length, keys: (.[0]|keys)}' 2>/dev/null || true
