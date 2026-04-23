#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

SID=$(curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=login&version=6&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" | jq -r '.data.sid')

curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Storage.CGI.Storage&method=load_info&version=1&_sid=${SID}" | jq .

curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6&_sid=${SID}" > /dev/null
