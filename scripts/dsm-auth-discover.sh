#!/usr/bin/env bash
# Discovers the SYNO.API.Auth endpoint path and maximum supported version.
# Requires DSM_HOST to be set. Respects INSECURE_TLS (set to any non-empty value to skip TLS verification).
# Exports AUTH_PATH and AUTH_VER.
AUTH_INFO=$(curl -s ${INSECURE_TLS:+-k} "https://${DSM_HOST}/webapi/query.cgi?api=SYNO.API.Info&version=1&method=query&query=SYNO.API.Auth")
AUTH_PATH=$(echo "$AUTH_INFO" | jq -r '.data["SYNO.API.Auth"].path')
AUTH_VER=$(echo "$AUTH_INFO" | jq -r '.data["SYNO.API.Auth"].maxVersion')
