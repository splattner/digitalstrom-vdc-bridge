#!/usr/bin/with-contenv bashio
# shellcheck shell=bash
set -e

DESCRIPTION=$(bashio::config 'description')
LISTEN_PORT=$(bashio::config 'listen_port')
VDCAPI_PORT=$(bashio::config 'vdcapi_port')

# ── Auto-configure the Home Assistant plugin ─────────────────────────────────
# SUPERVISOR_TOKEN is rotated on every add-on restart, so we always update it.
# On first run we also create plugins.json pre-populated with the HA plugin.
if [ -n "${SUPERVISOR_TOKEN:-}" ]; then
    PLUGINS_FILE="/data/plugins.json"
    HA_URL="ws://supervisor/core/websocket"

    if [ ! -f "${PLUGINS_FILE}" ]; then
        bashio::log.info "First run: creating plugins.json with Home Assistant plugin pre-configured."
        jq -n \
            --arg token "${SUPERVISOR_TOKEN}" \
            --arg url   "${HA_URL}" \
            '[{"id":"homeassistant","type":"homeassistant","config":{"url":$url,"token":$token}}]' \
            > "${PLUGINS_FILE}"
    else
        # If a supervisor-backed HA plugin already exists, refresh its token.
        MATCH=$(jq '[.[] | select(.type == "homeassistant" and .config.url == "ws://supervisor/core/websocket")] | length' "${PLUGINS_FILE}")
        if [ "${MATCH}" -gt 0 ]; then
            bashio::log.info "Refreshing Home Assistant supervisor token..."
            jq --arg token "${SUPERVISOR_TOKEN}" \
               'map(if .type == "homeassistant" and .config.url == "ws://supervisor/core/websocket" then .config.token = $token else . end)' \
               "${PLUGINS_FILE}" > /tmp/plugins.tmp && mv /tmp/plugins.tmp "${PLUGINS_FILE}"
        fi
    fi
fi

args=(
    "--datadir"     "/data"
    "--http-listen" ":8090"
    "--listen"      "${LISTEN_PORT}"
    "--description" "${DESCRIPTION}"
    "--vdcapi-port" "${VDCAPI_PORT}"
)

if bashio::config.true 'non_local'; then
    args+=("--non-local")
fi

if bashio::config.true 'no_discovery'; then
    args+=("--nodiscovery")
fi

if bashio::config.true 'no_auto'; then
    args+=("--noauto")
fi

bashio::log.info "Starting digitalSTROM vDC Bridge on port ${LISTEN_PORT}..."
exec /usr/local/bin/vdcgo-daemon "${args[@]}"
