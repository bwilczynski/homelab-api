#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

COOKIE_JAR=$(mktemp)

curl -sk -c "${COOKIE_JAR}" -X POST "https://${UNIFI_HOST}/api/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${UNIFI_USER}\",\"password\":\"${UNIFI_PASS}\"}" > /dev/null

curl -sk -b "${COOKIE_JAR}" "https://${UNIFI_HOST}/api/s/default/stat/device" | jq .

curl -sk -b "${COOKIE_JAR}" -X POST "https://${UNIFI_HOST}/api/logout" > /dev/null
rm -f "${COOKIE_JAR}"
