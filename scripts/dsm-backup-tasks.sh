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

# Discover auth endpoint and max version
AUTH_INFO=$(curl -sk "https://${DSM_HOST}/webapi/query.cgi?api=SYNO.API.Info&version=1&method=query&query=SYNO.API.Auth")
AUTH_PATH=$(echo "$AUTH_INFO" | jq -r '.data["SYNO.API.Auth"].path')
AUTH_VER=$(echo "$AUTH_INFO" | jq -r '.data["SYNO.API.Auth"].maxVersion')
echo "Auth path: $AUTH_PATH, max version: $AUTH_VER" >&2

# Login
SID=$(curl -sk "https://${DSM_HOST}/webapi/${AUTH_PATH}?api=SYNO.API.Auth&method=login&version=${AUTH_VER}&account=${DSM_USER}&passwd=${DSM_PASS}&format=sid" \
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

FIRST_TASK_ID=$(jq -r '.data.task_list[0].task_id' scripts/responses/dsm-backup-tasks-raw.json)
echo "--- SYNO.Backup.Task get (task_id=${FIRST_TASK_ID}) ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Task&method=get&version=1&task_id=${FIRST_TASK_ID}&additional=%5B%22repository%22%2C%22schedule%22%5D&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-task-get-raw.json | jq . >&2

echo "--- SYNO.Backup.Task status (task_id=${FIRST_TASK_ID}) ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Task&method=status&version=1&task_id=${FIRST_TASK_ID}&blOnline=false&additional=%5B%22last_bkp_time%22%2C%22next_bkp_time%22%2C%22last_bkp_result%22%2C%22is_modified%22%2C%22last_bkp_progress%22%2C%22last_bkp_success_version%22%5D&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-task-status-raw.json | jq . >&2

echo "--- SYNO.Backup.Target get (task_id=${FIRST_TASK_ID}) ---" >&2
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.Backup.Target&method=get&version=1&task_id=${FIRST_TASK_ID}&additional=%5B%22is_online%22%2C%22used_size%22%2C%22check_task_key%22%2C%22check_auth%22%2C%22account_meta%22%5D&_sid=${SID}" \
  | tee scripts/responses/dsm-backup-target-get-raw.json | jq . >&2

# Logout
curl -sk "https://${DSM_HOST}/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6&_sid=${SID}" > /dev/null
echo "Logged out" >&2
