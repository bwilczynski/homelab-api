#!/usr/bin/env bash
# Probe SYNO.Docker.Image API
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../.env"
source "$SCRIPT_DIR/dsm-auth-discover.sh"

BASE="https://${DSM_HOST}/webapi/entry.cgi"
AUTH_BASE="https://${DSM_HOST}/webapi/${AUTH_PATH}"

SID=$(curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=login&version=${AUTH_VER}&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" | jq -r '.data.sid')
echo "=== Logged in, SID=${SID:0:8}..."

echo ""
echo "=== SYNO.Docker.Image list ==="
curl -s ${INSECURE_TLS:+-k} \
  "${BASE}?api=SYNO.Docker.Image&method=list&version=1&limit=-1&offset=0&show_dsm=false&_sid=${SID}" \
  | tee "$SCRIPT_DIR/responses/docker_image_list.json" | jq .

curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=logout&version=${AUTH_VER}&_sid=${SID}" > /dev/null
echo ""
echo "=== Logged out ==="
