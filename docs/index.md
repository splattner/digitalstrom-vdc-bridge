# digitalSTROM vDC Bridge — Documentation

The digitalSTROM vDC Bridge connects third-party smart home devices to the [digitalSTROM](https://www.digitalstrom.com/) ecosystem. It acts as a Virtual Device Connector (vDC) — a software bridge that makes devices from other platforms appear as native digitalSTROM devices inside your dSS (digitalSTROM Server).

Once a device is bridged you can use it inside digitalSTROM exactly like any first-party device: assign it to rooms, include it in scenes, automate it with floor plans, and control it from the digitalSTROM app.

---

## What can I do with it?

| Source | What gets bridged |
|---|---|
| **Home Assistant** | Lights (dimmable, colour, tunable white), switches, and sensors |
| **Zigbee2MQTT** | Zigbee lights, switches, sensors, and groups |
| **Tasmota** | Relays, dimmable and colour lights discovered via MQTT |
| **WLED** | LED controllers discovered automatically via mDNS |
| **External Device API** | Any device you script yourself — a shell script, Python program, or any language that can open a TCP socket |

---

## How it works (in one paragraph)

The bridge runs a background service that speaks the digitalSTROM vDC API (a protobuf protocol over TCP). Plugins connect to third-party systems (Home Assistant via WebSocket, Zigbee2MQTT via MQTT, etc.) and report the devices they find. You then open the web UI, browse the discovered devices, and map the ones you want to digitalSTROM. Mapped devices are announced to the digitalSTROM server and behave like native dS devices from that point on.

---

## Documentation map

| Topic | Description |
|---|---|
| [Getting Started](getting-started.md) | Install, connect to digitalSTROM, open the web UI |
| [Core Concepts](concepts.md) | Plugins, discovered entities, bridge mappings, persistent storage |
| **Plugins** | |
| [Plugin overview](plugins/overview.md) | Status indicators, enable/disable, shared MQTT pattern |
| [MQTT Broker](plugins/mqtt.md) | Shared broker connection used by Tasmota and Zigbee2MQTT |
| [Home Assistant](plugins/homeassistant.md) | Bridge HA entities via WebSocket |
| [Zigbee2MQTT](plugins/zigbee2mqtt.md) | Bridge Zigbee devices via Z2M |
| [Tasmota](plugins/tasmota.md) | Bridge Tasmota devices via MQTT discovery |
| [WLED](plugins/wled.md) | Bridge WLED LED controllers via mDNS |
| [External Device API](plugins/externaldevice.md) | Write your own device script |
| **Web UI pages** | |
| [Plugins page](ui/plugins.md) | Manage plugin instances |
| [Discovered page](ui/discovered.md) | Browse and map discovered devices |
| [Devices page](ui/devices.md) | View and manage mapped digitalSTROM devices |
| [Protocol page](ui/protocol.md) | Live vDC API frame inspector |
| [Settings page](ui/settings.md) | Identity, runtime info, UI preferences |
| [Home Assistant Add-on](ha-addon.md) | HA add-on specific setup and options |
| [External Device API reference](external-device-api.md) | Full protocol reference for the TCP device API |
