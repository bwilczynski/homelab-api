#!/usr/bin/env bash
# Capture Synology Hyper Backup task logs.
# Usage: source .env && bash scripts/dsm-backup-logs.sh <host> <user> <pass> <task_id>
set -euo pipefail

DSM_HOST="${1:-${DSM_HOST:-}}"
DSM_USER="${2:-${DSM_USER:-ai-agent}}"
DSM_PASS="${3:-${DSM_PASS:-}}"
TASK_ID="${4:-3}"

if [[ -z "$DSM_HOST" || -z "$DSM_PASS" ]]; then
  echo "Usage: $0 <host> <user> <pass> [task_id]" >&2
  exit 1
fi

# Login
SID=$(curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=login&version=6&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" \
  | jq -r '.data.sid')

echo "SID: $SID" >&2

# List backup logs for given task
echo "--- SYNO.SDS.Backup.Client.Common.Log list task_id=${TASK_ID} ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.SDS.Backup.Client.Common.Log&method=list&version=1&task_id=${TASK_ID}&offset=0&limit=100&_sid=${SID}" \
  | tee "scripts/responses/dsm-backup-logs-task${TASK_ID}-raw.json" | jq .

# Logout
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6&_sid=${SID}" > /dev/null
echo "Logged out" >&2
