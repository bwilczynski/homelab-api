#!/usr/bin/env bash
# Capture Synology Hyper Backup task list.
# Usage: source .env && bash scripts/dsm-backup-tasks.sh <host> <user> <pass>
set -euo pipefail

DSM_HOST="${1:-${DSM_HOST:-}}"
DSM_USER="${2:-${DSM_USER:-ai-agent}}"
DSM_PASS="${3:-${DSM_PASS:-}}"

if [[ -z "$DSM_HOST" || -z "$DSM_PASS" ]]; then
  echo "Usage: $0 <host> <user> <pass>" >&2
  exit 1
fi

# Login
SID=$(curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=login&version=6&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" \
  | jq -r '.data.sid')

echo "SID: $SID" >&2

# List backup tasks (SYNO.Backup.Task)
echo "--- SYNO.Backup.Task list ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Task&method=list&version=1&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-tasks-raw.json | jq .

# List scheduled tasks (SYNO.Core.TaskScheduler)
echo "--- SYNO.Core.TaskScheduler list ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Core.TaskScheduler&method=list&version=2&offset=0&limit=50&_sid=${SID}" \
  | tee scripts/responses/dsm-task-scheduler-raw.json | jq .

# Logout
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6&_sid=${SID}" > /dev/null
echo "Logged out" >&2
