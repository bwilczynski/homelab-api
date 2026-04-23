#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

COOKIE_JAR=$(mktemp)

curl -s ${INSECURE_TLS:+-k} -c "${COOKIE_JAR}" -X POST "https://${UNIFI_HOST}/api/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${UNIFI_USER}\",\"password\":\"${UNIFI_PASS}\"}" > /dev/null

curl -s ${INSECURE_TLS:+-k} -b "${COOKIE_JAR}" "https://${UNIFI_HOST}/api/s/default/stat/sta" | jq .

curl -s ${INSECURE_TLS:+-k} -b "${COOKIE_JAR}" -X POST "https://${UNIFI_HOST}/api/logout" > /dev/null
rm -f "${COOKIE_JAR}"
