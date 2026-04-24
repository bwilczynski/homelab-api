#!/usr/bin/env bash
# Explore Synology DSM Docker container APIs
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../.env"

source "$SCRIPT_DIR/dsm-auth-discover.sh"

BASE="https://${DSM_HOST}/webapi/entry.cgi"
AUTH_BASE="https://${DSM_HOST}/webapi/${AUTH_PATH}"

# Login
SID=$(curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=login&version=${AUTH_VER}&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" | jq -r '.data.sid')
echo "=== Logged in, SID=${SID:0:8}..."

echo ""
echo "=== SYNO.Docker.Container list ==="
curl -s ${INSECURE_TLS:+-k} "${BASE}?api=SYNO.Docker.Container&method=list&version=1&limit=0&offset=0&_sid=${SID}" | jq .

echo ""
echo "=== SYNO.Docker.Container.Resource get ==="
curl -s ${INSECURE_TLS:+-k} "${BASE}?api=SYNO.Docker.Container.Resource&method=get&version=1&_sid=${SID}" | jq .

# Get detail for first container
FIRST_NAME=$(curl -s ${INSECURE_TLS:+-k} "${BASE}?api=SYNO.Docker.Container&method=list&version=1&limit=1&offset=0&_sid=${SID}" | jq -r '.data.containers[0].name')
if [ -n "$FIRST_NAME" ] && [ "$FIRST_NAME" != "null" ]; then
  echo ""
  echo "=== SYNO.Docker.Container get (name=${FIRST_NAME}) ==="
  curl -s ${INSECURE_TLS:+-k} "${BASE}?api=SYNO.Docker.Container&method=get&version=1&name=${FIRST_NAME}&_sid=${SID}" | jq .
fi

# Logout
curl -s ${INSECURE_TLS:+-k} "${AUTH_BASE}?api=SYNO.API.Auth&method=logout&version=${AUTH_VER}&_sid=${SID}" > /dev/null
echo ""
echo "=== Logged out ==="
