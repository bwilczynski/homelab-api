#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

curl -s -k -H "X-API-KEY: ${UNIFI_API_KEY}" \
  "https://${UNIFI_HOST}/proxy/network/api/s/default/stat/health" | jq .
