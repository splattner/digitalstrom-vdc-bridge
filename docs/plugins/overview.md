# Plugins overview

Plugins are the bridge's connection points to third-party systems. This page explains how to manage them from the web UI and covers concepts common to all plugin types.

For the specific configuration of each plugin type, see the individual pages:

- [MQTT Broker](mqtt.md)
- [Home Assistant](homeassistant.md)
- [Zigbee2MQTT](zigbee2mqtt.md)
- [Tasmota](tasmota.md)
- [WLED](wled.md)
- [Shelly](shelly.md)
- [External Device API](externaldevice.md)

---

## The Plugins page

Open the Plugins page at `http://<bridge>:8090/#/plugins`.

At the top you will see four KPI tiles:

| Tile | What it shows |
|---|---|
| **Plugins** | Total number of configured plugin instances |
| **Connected** | How many are currently in `connected` state (and the percentage) |
| **Devices** | Total number of bridge mappings across all plugins |
| **Issues** | Warnings and errors logged in the last hour |

Below the KPI row, each plugin has its own card.

---

## Plugin card anatomy

```
┌─────────────────────────────────────────────────────────┐
│  [Icon]  Plugin name (type)         [connected ▾]       │
│          12 discovered · 8 active   Last event: 2m ago  │
│  ─────────────────────────────────────────────────────  │
│  [Logs ▾]   [↺ Restart]  [⏸ Disable]  [✎ Edit]  [🗑]   │
└─────────────────────────────────────────────────────────┘
```

- **Icon** — colour-coded by plugin type (indigo = HA, violet = MQTT, sky = Tasmota, etc.)
- **Status badge** — see the colour table in [Core Concepts](../concepts.md#plugin-status)
- **Discovered / Active** — how many entities the plugin found and how many have active bridge mappings
- **Last event** — how long ago the plugin last logged a message
- **Logs panel** — click to expand an inline event log showing the last messages from that plugin
- **Restart** — stops and restarts the plugin without restarting the whole service
- **Disable / Enable** — suspends the plugin. A disabled plugin does not connect or discover; its mapped devices go offline in digitalSTROM.
- **Edit** — opens the configuration form
- **Delete** — removes the plugin and all its bridge mappings permanently

---

## Adding a plugin

1. Click the **+** button in the top-right corner of the Plugins page.
2. The picker shows all available plugin types with icons, descriptions, and (where applicable) connection tests. Choose your type.
3. Fill in the configuration form. Required fields are marked with an asterisk.
4. Click **Save**. The plugin starts immediately.

---

## The shared MQTT pattern

The **Tasmota** and **Zigbee2MQTT** plugins do not have their own broker credentials. They reference an **MQTT Broker** plugin instance by its ID. This means:

1. Create an **MQTT Broker** plugin first (e.g. with ID `mqtt`).
2. When adding a Tasmota or Zigbee2MQTT plugin, enter `mqtt` in the **MQTT broker plugin id** field.

This way both Tasmota and Zigbee2MQTT can share the same TCP connection to your broker. If you have two separate brokers you can create two MQTT Broker plugins with different IDs and reference each from the relevant device plugins.

---

## Plugin stats

Below the status badge, each card shows two counters:

- **Discovered** — the total number of entities the plugin has reported (including unmapped ones).
- **Active** — how many of those entities have an active bridge mapping and are currently subscribed (i.e. actively sending/receiving events).

A large gap between Discovered and Active is normal — it just means you have not mapped all entities to digitalSTROM, which is intentional.

---

## Plugin event log

Click the **Logs** button on a plugin card to expand an inline log. Each entry shows:

- Timestamp
- Log level (`info`, `warn`, `error`, `debug`)
- A short message and optional structured fields

The log is a rolling buffer of the last ~500 events. For persistent logs, use your container or system's logging infrastructure.
