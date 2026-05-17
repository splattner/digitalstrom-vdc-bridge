# Tasmota plugin

The Tasmota plugin discovers Tasmota-flashed devices via MQTT discovery and bridges their relays and lights as vDC devices.

> **Prerequisites:** A running MQTT broker and one or more Tasmota devices with MQTT discovery enabled. You must configure an [MQTT Broker plugin](mqtt.md) first.

---

## Setup walkthrough

### Step 1 — Enable MQTT discovery on your Tasmota devices

Tasmota's MQTT discovery (called `SetOption19`) must be enabled on every device you want to bridge.

Connect to the Tasmota device's web UI and open the console (**Console** tab), then type:

```
SetOption19 1
```

Press Enter. The device will publish its discovery message immediately and on every future startup.

> **Note:** `SetOption19 1` changes the MQTT topic scheme of the device. If you are also using Tasmota devices from Home Assistant via its own Tasmota integration, check that both integrations use compatible topic settings.

You also need to ensure the device is connected to the same MQTT broker you configured in the bridge.

### Step 2 — Create an MQTT Broker plugin (if you have not already)

See [MQTT Broker plugin](mqtt.md). Give it an ID you will remember, for example `mqtt`.

### Step 3 — Add the Tasmota plugin

1. Open the bridge web UI and go to the **Plugins page**.
2. Click **+** and choose **Tasmota (MQTT discovery)**.
3. Fill in the form:

| Field | What to enter |
|---|---|
| **Plugin ID** | A short unique name, e.g. `tasmota` |
| **MQTT broker plugin id** | The ID of your MQTT Broker plugin, e.g. `mqtt` |
| **Discovery topic prefix** | Leave as `tasmota/discovery` unless you customised it in Tasmota |

4. Click **Save**.

The plugin subscribes to the discovery prefix and populates the Discovered page as devices publish their discovery messages. Devices that were already on the broker before the plugin started will also appear — they re-publish their discovery message after a broker reconnect.

### Step 4 — Map devices

Open the **Discovered page**, look for the Tasmota plugin badge, and click **Map** next to the device you want to add to digitalSTROM.

---

## Supported device types

| Tasmota device | Bridged as |
|---|---|
| Single relay | `light` (simple on/off) |
| Dimmer (PWM) | `dimmer` |
| RGB / RGBW / RGBCW strip | `colorlight` |
| Colour temperature light | `dimmer` (brightness) |

---

## Configuration reference

| Field | Type | Default | Description |
|---|---|---|---|
| `broker` | string | — | ID of the MQTT Broker plugin instance. **Required.** |
| `discoveryPrefix` | string | `tasmota/discovery` | The discovery root topic. Must match the Tasmota `SetOption19` prefix. |

---

## Troubleshooting

**Devices do not appear**  
- Open the Tasmota console and run `SetOption19 1` again — this re-triggers the discovery publish.
- Make sure the discovery prefix in the plugin config matches what Tasmota is publishing to. You can verify by subscribing to `tasmota/discovery/#` with an MQTT client like MQTT Explorer.
- Check that the MQTT Broker plugin is connected (green badge).

**Device appears but does not control the hardware**  
- Verify the Tasmota device is still connected to the broker (check its "MQTT Connected" status in the Tasmota web UI).
- Look at the plugin's event log for errors.
