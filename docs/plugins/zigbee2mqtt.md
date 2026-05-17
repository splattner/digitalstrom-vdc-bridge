# Zigbee2MQTT plugin

The Zigbee2MQTT plugin connects to a Zigbee2MQTT instance via MQTT and discovers Zigbee devices (lights, switches, sensors, buttons, and groups) as vDC devices.

> **Prerequisites:** A running MQTT broker and a Zigbee2MQTT instance. You must configure an [MQTT Broker plugin](mqtt.md) first and note its plugin ID.

---

## Setup walkthrough

### Step 1 — Verify Zigbee2MQTT is running and publishing

In Zigbee2MQTT, make sure MQTT is configured and devices are appearing in the Z2M dashboard. The plugin reads the device list from `<base_topic>/bridge/devices` and subscribes to individual device topics.

Note the **base topic** configured in Zigbee2MQTT (`mqtt.base_topic` in `configuration.yaml`). The default is `zigbee2mqtt`.

### Step 2 — Create an MQTT Broker plugin (if you have not already)

See [MQTT Broker plugin](mqtt.md). Give it an ID you will remember, for example `mqtt`.

### Step 3 — Add the Zigbee2MQTT plugin

1. Open the bridge web UI and go to the **Plugins page**.
2. Click **+** and choose **Zigbee2MQTT**.
3. Fill in the form:

| Field | What to enter |
|---|---|
| **Plugin ID** | A short unique name, e.g. `zigbee2mqtt` |
| **MQTT broker plugin id** | The ID of your MQTT Broker plugin, e.g. `mqtt` |
| **Base topic** | The Z2M base topic, e.g. `zigbee2mqtt` |

4. Click **Save**.

The plugin subscribes to Z2M, reads the device list, and starts populating the Discovered page.

### Step 4 — Optional: include battery and link-quality sensors

By default, battery level and link quality readings are not exposed as separate sensor inputs (they would create a lot of extra clutter in digitalSTROM). You can enable them individually:

- **Include battery sensors** — adds a battery-level sensor input to every battery-powered device.
- **Include link-quality sensors** — adds a link-quality (signal strength) sensor input to every device.

### Step 5 — Map devices

Open the **Discovered page** and look for entities with the Zigbee2MQTT plugin badge. Click **Map** next to any device to add it to digitalSTROM.

---

## Supported device kinds

| Zigbee device type | Bridged as |
|---|---|
| Light with colour + colour temp | `colorlight` |
| Light with colour temp only | `dimmer` |
| Light with brightness only | `dimmer` |
| Light on/off only | `light` |
| Switch / relay | `light` |
| Binary sensor (contact, motion, etc.) | `binary` |
| Numeric sensor (temperature, humidity, etc.) | `sensor` |
| Button / remote | `button` |
| Z2M group | `colorlight` / `dimmer` / `light` (depends on members) |

---

## Z2M groups

Zigbee2MQTT groups appear in the Discovered list alongside individual devices. A group can be mapped to digitalSTROM exactly like a single device — the bridge sends commands to the group topic and Z2M distributes them to all group members.

---

## Configuration reference

| Field | Type | Default | Description |
|---|---|---|---|
| `broker` | string | — | ID of the MQTT Broker plugin instance. **Required.** |
| `baseTopic` | string | `zigbee2mqtt` | Z2M `mqtt.base_topic` |
| `includeBattery` | bool | `false` | Expose battery level as a sensor input |
| `includeLinkquality` | bool | `false` | Expose link quality as a sensor input |

---

## Troubleshooting

**No devices appear**  
- Check the base topic — it must match `mqtt.base_topic` in Z2M's `configuration.yaml`.
- Make sure Zigbee2MQTT is publishing to the broker (check `<base_topic>/bridge/state`).
- Verify the MQTT Broker plugin is connected (green badge) and that Z2M uses the same broker.

**Devices appear but do not respond to commands**  
- Check that Zigbee2MQTT has `permit_join` disabled and devices are paired.
- Look at the plugin's event log for MQTT publish errors.

**Duplicate devices with the HA plugin**  
Enable **Ignore Zigbee2MQTT devices** in the Home Assistant plugin config. This tells the HA plugin to skip entities managed by Z2M.
