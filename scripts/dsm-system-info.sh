#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

source "$(dirname "$0")/dsm-auth-discover.sh"

SID=$(curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/${AUTH_PATH}?api=SYNO.API.Auth&method=login&version=${AUTH_VER}&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" | jq -r '.data.sid')

curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Core.System&method=info&version=1&_sid=${SID}" | jq .

curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/${AUTH_PATH}?api=SYNO.API.Auth&method=logout&version=${AUTH_VER}&_sid=${SID}" > /dev/null
