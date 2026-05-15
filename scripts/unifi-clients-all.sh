#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

COOKIE_JAR=$(mktemp)
RESPONSES_DIR="$(dirname "$0")/responses"

curl -s ${INSECURE_TLS:+-k} -c "${COOKIE_JAR}" -X POST "https://${UNIFI_HOST}/api/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${UNIFI_USER}\",\"password\":\"${UNIFI_PASS}\"}" > /dev/null

echo "=== /stat/sta (active clients) ===" >&2
curl -s ${INSECURE_TLS:+-k} -b "${COOKIE_JAR}" "https://${UNIFI_HOST}/api/s/default/stat/sta" \
  | tee "${RESPONSES_DIR}/unifi-sta-raw.json" | jq 'del(.data[].mac,.data[]."_id") | {count: (.data|length), keys: (.data[0]|keys), sample: .data[0]}' 2>/dev/null || true

echo "" >&2
echo "=== /rest/user (all known clients) ===" >&2
curl -s ${INSECURE_TLS:+-k} -b "${COOKIE_JAR}" "https://${UNIFI_HOST}/api/s/default/rest/user" \
  | tee "${RESPONSES_DIR}/unifi-rest-user-raw.json" | jq 'del(.data[].mac,.data[]."_id") | {count: (.data|length), keys: (.data[0]|keys), sample: .data[0]}' 2>/dev/null || true

echo "" >&2
echo "=== /stat/alluser (all users with stats) ===" >&2
curl -s ${INSECURE_TLS:+-k} -b "${COOKIE_JAR}" "https://${UNIFI_HOST}/api/s/default/stat/alluser" \
  | tee "${RESPONSES_DIR}/unifi-alluser-raw.json" | jq 'del(.data[].mac,.data[]."_id") | {count: (.data|length), keys: (.data[0]|keys), sample: .data[0]}' 2>/dev/null || true

curl -s ${INSECURE_TLS:+-k} -b "${COOKIE_JAR}" -X POST "https://${UNIFI_HOST}/api/logout" > /dev/null
rm -f "${COOKIE_JAR}"
