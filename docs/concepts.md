# Core Concepts

Understanding a few key ideas will make working with the bridge much easier.

---

## digitalSTROM, vDSM, and vDC

**digitalSTROM** is a smart home platform built around a bus system and a central controller called the **dSS** (digitalSTROM Server).

A **vDSM** (virtual digitalSTROM Meter) is a software component inside the dSS that manages virtual (non-bus) devices. When the bridge connects to the dSS, it registers itself with the vDSM.

A **vDC** (Virtual Device Connector) is the bridge's role in this picture: it translates between the dSS's vDC API protocol (protobuf over TCP) and the third-party systems that hold the actual devices.

```
[dSS / vDSM]  ←— vDC API (TCP port 8999) —→  [vDC Bridge]  ←—→  [HA / MQTT / WLED / …]
```

You do not need to understand the protocol in depth. The bridge handles it. The important thing to know is that the dSS connects *to* the bridge (not the other way around), so the bridge must be reachable from the dSS on the configured TCP port.

---

## Plugins

A **plugin** is a connection to one third-party system. Each plugin:

- Connects to its source system (e.g. opens a WebSocket to Home Assistant).
- Discovers the devices or entities available in that system.
- Forwards channel changes (brightness, colour, etc.) back and forth once a device is mapped.

You can have **multiple plugin instances** of the same type — for example two separate Home Assistant connections pointing at different HA instances, or two Zigbee2MQTT plugins pointing at different Z2M instances on different MQTT brokers.

Plugins are created and configured from the [Plugins page](ui/plugins.md) in the web UI.

### Plugin status

The badge on each plugin card tells you the current connection state:

| Badge | Colour | Meaning |
|---|---|---|
| `connected` | Green | Plugin is running and connected to the source system |
| `connecting` / `reconnecting` | Amber | Trying to establish or re-establish the connection |
| `idle` | Grey | Plugin is loaded but not yet fully started (e.g. External Device API not yet listening) |
| `disabled` | Grey | Plugin has been manually disabled |
| `error` / `auth_failed` | Red | Connection failed; check the plugin's event log |

### Shared MQTT connection

The **Tasmota** and **Zigbee2MQTT** plugins do not manage their own MQTT connections. Instead they reference a separate **MQTT Broker** plugin by its instance ID. This lets multiple plugins share a single broker connection without duplicating credentials. You must create the MQTT Broker plugin first, then reference its ID in the Tasmota or Z2M config.

---

## Discovered entities

When a plugin connects to its source system it reports a list of **discovered entities** — the devices or data points it found. Examples:

- A Home Assistant light entity (`light.living_room`)
- A Zigbee2MQTT device (`0x00158d0001a2b3c4`)
- A WLED controller found via mDNS
- An external device script that connected to the External Device API

Discovered entities are visible on the [Discovered page](ui/discovered.md). They are *not yet* in digitalSTROM — they are just candidates waiting to be mapped.

Each entity has a **kind** that tells the bridge what type of digitalSTROM device to create:

| Kind | digitalSTROM device type |
|---|---|
| `colorlight` | Full-colour light (brightness, hue, saturation, colour temperature) |
| `dimmer` | Dimmable light (brightness only) |
| `light` | Simple on/off light |
| `sensor` | Sensor input (temperature, humidity, etc.) |
| `binary` | Binary sensor (motion, door contact, etc.) |
| `button` | Push button or switch |

---

## Bridge mappings

A **bridge mapping** (or just "mapping") connects a discovered entity to a digitalSTROM device. When you click **Map** on the Discovered page you create a mapping. The bridge then:

1. Announces the device to the dSS via the vDC API.
2. Starts forwarding commands from digitalSTROM (scene calls, direct channel changes) to the plugin.
3. Starts forwarding state updates from the plugin (e.g. the light was turned on directly from a physical switch) back to digitalSTROM.

Mapped devices appear on the [Devices page](ui/devices.md). They also appear in the dSS configurator where you can assign them to rooms and include them in scenes.

You can remove a mapping at any time from the Devices page. The device disappears from digitalSTROM but the entity remains in the Discovered list so you can re-map it later.

---

## Persistent storage

The bridge stores all its state in the `--datadir` directory (default `/data` in the Docker image, `./` when running locally). This directory contains:

| File | Contents |
|---|---|
| `plugins.json` | Plugin configurations (type, ID, settings) |
| `bridges.json` | Bridge mappings (which entity → which dS device) |
| `status.json` | Runtime state (device channel values, scene tables, last-known states) |
| `config.json` | Global service settings (vDC identity, description) |

**Backups:** Copy the `datadir` directory to back up your entire configuration. Restoring it to a fresh install will recreate all plugins, mappings, and device states.

**Upgrades:** The data format is versioned. Upgrades never delete existing data.

---

## DNS-SD advertisement

When DNS-SD (mDNS / Avahi) is enabled, the bridge advertises itself on the local network so the dSS can discover it automatically without you having to enter its IP address manually. This works out of the box on bare-metal Linux installs. In Docker or the HA add-on, automatic discovery may require host networking or Avahi D-Bus access — see [Home Assistant Add-on](ha-addon.md) for the add-on specifics.

You can disable DNS-SD with the `--nodiscovery` flag (or the `no_discovery` option in the HA add-on) if you prefer to enter the address manually.
