#!/usr/bin/with-contenv bashio
# shellcheck shell=bash
set -e

DESCRIPTION=$(bashio::config 'description')
LISTEN_PORT=$(bashio::config 'listen_port')
VDCAPI_PORT=$(bashio::config 'vdcapi_port')

# ── Auto-configure plugins on first run ──────────────────────────────────────
# SUPERVISOR_TOKEN is rotated on every add-on restart, so we always refresh it.
# On first run we create plugins.json pre-populated with the HA plugin and the
# externaldevice plugin (using the configured listen_port).
if [ -n "${SUPERVISOR_TOKEN:-}" ]; then
    PLUGINS_FILE="/data/plugins.json"
    HA_URL="ws://supervisor/core/websocket"

    if [ ! -f "${PLUGINS_FILE}" ]; then
        bashio::log.info "First run: creating plugins.json with Home Assistant and External Device API plugins pre-configured."
        jq -n \
            --arg token "${SUPERVISOR_TOKEN}" \
            --arg url   "${HA_URL}" \
            --arg port  "${LISTEN_PORT}" \
            '[
              {"id":"homeassistant","type":"homeassistant","config":{"url":$url,"token":$token}},
              {"id":"externaldevice","type":"externaldevice","config":{"listen":$port,"nonlocal":false}}
            ]' \
            > "${PLUGINS_FILE}"
    else
        # Refresh supervisor token for any supervisor-backed HA plugin.
        MATCH=$(jq '[.[] | select(.type == "homeassistant" and .config.url == "ws://supervisor/core/websocket")] | length' "${PLUGINS_FILE}")
        if [ "${MATCH}" -gt 0 ]; then
            bashio::log.info "Refreshing Home Assistant supervisor token..."
            jq --arg token "${SUPERVISOR_TOKEN}" \
               'map(if .type == "homeassistant" and .config.url == "ws://supervisor/core/websocket" then .config.token = $token else . end)' \
               "${PLUGINS_FILE}" > /tmp/plugins.tmp && mv /tmp/plugins.tmp "${PLUGINS_FILE}"
        fi
        # Migrate: add externaldevice plugin if not yet present.
        EXT_MATCH=$(jq '[.[] | select(.type == "externaldevice")] | length' "${PLUGINS_FILE}")
        if [ "${EXT_MATCH}" -eq 0 ]; then
            bashio::log.info "Migration: adding External Device API plugin (port ${LISTEN_PORT}) to plugins.json..."
            jq --arg port "${LISTEN_PORT}" \
               '. + [{"id":"externaldevice","type":"externaldevice","config":{"listen":$port,"nonlocal":false}}]' \
               "${PLUGINS_FILE}" > /tmp/plugins.tmp && mv /tmp/plugins.tmp "${PLUGINS_FILE}"
        fi
    fi
fi

args=(
    "--datadir"     "/data"
    "--http-listen" ":8090"
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

if bashio::config.true 'use_avahi_dbus'; then
    args+=("--avahi-dbus")
fi

bashio::log.info "Starting digitalSTROM vDC Bridge on port ${LISTEN_PORT}..."
exec /usr/local/bin/vdcgo-daemon "${args[@]}"
