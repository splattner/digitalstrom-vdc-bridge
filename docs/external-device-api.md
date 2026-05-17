# External Device API reference

The External Device API lets you integrate any custom device into digitalSTROM by connecting a script or program to a TCP port and exchanging simple text or JSON messages. The protocol is identical to the [plan44 vdcd external device API](https://github.com/plan44/vdcd/blob/main/docs/plan44%20vdcd%20external%20device%20API.md) — this document is a focused reference adapted for use with the vDC Bridge.

> **Plugin setup:** Before using this API you need a running [External Device API plugin](plugins/externaldevice.md). The plugin starts the TCP server on the configured port (default `8999`).

---

## How it works

```
Your script                     vDC Bridge (External Device API plugin)
     |                                         |
     |--- TCP connect ------------------------>|
     |--- init message (JSON) ---------------->|  ← declare your device
     |<-- OK (simple) or {"status":"ok"} ------|  ← device is now registered
     |                                         |
     |<-- C0=100.000000 ----------------------|  ← channel change from dSS
     |--- C0=100.000000 --------------------->|  ← report actual output value
     |--- S0=22.5 --------------------------->|  ← push a sensor reading
     |--- B0=250 ---------------------------->|  ← simulate a button press
```

Your script keeps the connection open for as long as the device is active. If the connection drops, the device goes offline in digitalSTROM. Reconnect and re-send the `init` message to bring it back.

---

## Message format

Messages are strings delimited by a single newline character (`\n`, 0x0A). The `init` message (and the optional `initvdc` message) must always be JSON. Subsequent messages are either JSON or the **simple text** format, depending on the `protocol` field in the `init` message.

**Simple text** is easier to work with in shell scripts and on microcontrollers. It uses short key=value pairs like `C0=100` or `S0=22.5`. **JSON** is required for advanced features (actions, states, events, properties, multi-device configurations).

---

## Quick start: a simple dimmer in bash

```bash
exec 3<>/dev/tcp/127.0.0.1/8999

# Declare a dimmable light, use simple protocol for further messages
printf '{"message":"init","protocol":"simple","output":"light","name":"My Dimmer","uniqueid":"my-dimmer-001"}\n' >&3

# Read the acknowledgement (should be "OK")
read -r ack <&3
echo "ack: $ack"

# Now follow channel changes from digitalSTROM
while read -r msg <&3; do
    echo "command: $msg"
    # Parse and act on "C0=<value>" here
done
```

After sending the `init` message, go to the bridge web UI → **Discovered** and map the device. digitalSTROM can now control it.

---

## The `initvdc` message (optional)

The optional `initvdc` message can be sent **before** the `init` message to customise how the vDC (the bridge's virtual connector) appears in the dSS. In most cases you can skip this entirely.

```json
{"message":"initvdc","modelname":"My Custom vDC","name":"External Devices"}
```

| Field | Type | Description |
|---|---|---|
| `message` | string | Must be `"initvdc"` |
| `modelname` | string (opt) | Model name shown as "HW Info" in the dSS configurator |
| `modelVersion` | string (opt) | Version string for the hardware |
| `iconname` | string (opt) | Base name for the vDC icon file. Default: `"vdc_ext"` |
| `configurl` | string (opt) | URL shown in the dSS configurator context menu for the vDC |
| `alwaysVisible` | bool (opt) | If `true`, the vDC announces itself even if it has no devices yet |
| `name` | string (opt) | Default name for the vDC in digitalSTROM |

---

## The `init` message

The `init` message is the first thing your script sends after connecting. It declares all the properties of the device — its output type, sensors, buttons, etc.

```json
{
  "message": "init",
  "protocol": "simple",
  "output": "light",
  "name": "Living Room Dimmer",
  "uniqueid": "living-room-dimmer-001"
}
```

### Core fields

| Field | Type | Required | Description |
|---|---|---|---|
| `message` | string | yes | Must be `"init"` |
| `uniqueid` | string | yes | Unique identifier for this device. Use a UUID or a globally unique string. Determines the device's dSUID in digitalSTROM. |
| `protocol` | string | no | `"simple"` (default: `"json"`). Use `"simple"` for scripts that do not need JSON parsing. Only the first `init` message can set the protocol. |
| `tag` | string | no | Required when registering multiple devices on the same connection. See [Multiple devices](#multiple-devices-on-one-connection). |
| `name` | string | no | Default device name in digitalSTROM (can be changed by the user via dSS). |
| `output` | string | no | Output type — see [Output types](#output-types). |
| `group` | integer | no | Primary dS group (1 = light, 2 = shadow, 3 = heating, …). Default derived from output type. |
| `colorclass` | integer | no | Override the device colour class (1=yellow/light, 2=grey/shadow, …). |
| `subdeviceindex` | integer | no | Used for composite devices. Creates multiple virtual dS devices with the same base dSUID. |
| `modelname` | string | no | Device model name shown as "HW Info" in dSS. |
| `modelversion` | string | no | Firmware version string. |
| `vendorname` | string | no | Vendor name. |
| `hardwarename` | string | no | Hardware description (e.g. `"dimmer"`, `"relay"`). |
| `iconname` | string | no | Base name for the device icon. Default: `"ext"`. |
| `configurl` | string | no | URL shown in the dSS configurator context menu for this device. |
| `sync` | bool | no | If `true`, the device must respond to `sync` messages. |
| `move` | bool | no | If `true`, the device uses move semantics (for blinds/shades). |
| `controlvalues` | bool | no | If `true`, the bridge forwards control values (e.g. room temperature set points) to the device. |
| `scenecommands` | bool | no | If `true`, the bridge sends `scenecommand` messages for special scene calls. |
| `buttons` | array | no | Declare button inputs — see [Button object](#button-object). |
| `inputs` | array | no | Declare binary inputs — see [Input object](#input-object). |
| `sensors` | array | no | Declare sensor inputs — see [Sensor object](#sensor-object). |
| `groups` | array of integers | no | Override output group membership. |

### Output types

| Value | Description | Channels |
|---|---|---|
| `light` | Dimmable light | 0: brightness (0–100 %) |
| `colorlight` | Full colour light | 0: brightness, 1: hue, 2: saturation, 3: colour temp, 4: CIE-X, 5: CIE-Y |
| `ctlight` | Tunable white light | 0: brightness, 3: colour temperature |
| `movinglight` | Colour light with position | 0–5 as colorlight + X/Y position |
| `heatingvalve` | Heating valve | 0: valve position (0–100 %) |
| `shadow` | Blind / shade | 0: position (0–100 %), 1: angle |
| `ventilation` | Ventilation | Airflow intensity, direction, louver position |
| `fancoilunit` | Fan coil unit | Fan output + temperature set point |
| `action` | Single device (actions/events/states) | No channels; uses JSON action protocol |

For `shadow` output, an optional `kind` field selects the shade type:
- `"roller"` — simple roller blind, no tilt angle
- `"sun"` — sun blind
- `"jalousie"` — jalousie with blade angle control

For `ventilation` output, `kind` is:
- `"ventilation"` — air supply/exhaust
- `"recirculation"` — air recirculation within rooms

### Button object

Declare one or more buttons in the `buttons` array:

| Field | Type | Description |
|---|---|---|
| `id` | string (opt) | String ID for vDC API reference |
| `buttonid` | integer (opt) | Hardware button ID. Two-way buttons have two entries with the same `buttonid`. Default: `0` |
| `buttontype` | integer (opt) | `1` = single pushbutton (default), `2` = two-way rocker, `3` = 4-way, `4` = 4-way+centre, `6` = on/off switch |
| `element` | integer (opt) | `0` = single/centre, `1` = down, `2` = up, `3` = left, `4` = right |
| `group` | integer (opt) | Primary group of the button. Default: device's primary group |
| `localbutton` | bool (opt) | If `true`, this button directly controls the device's output |
| `hardwarename` | string (opt) | Description, e.g. `"up"`, `"down"` |
| `combinables` | integer (opt) | Set to a multiple of 2 to allow dSS-level combination into a two-way button |

### Input object

Declare binary sensor inputs in the `inputs` array:

| Field | Type | Description |
|---|---|---|
| `id` | string (opt) | String ID for vDC API reference |
| `inputtype` | integer (opt) | `0`=none, `1`=presence, `5`=motion, `12`=low battery, `13`=window closed, `14`=door closed, etc. Default: `0` |
| `usage` | integer (opt) | `0`=undefined, `1`=room, `2`=outdoors, `3`=user interaction |
| `group` | integer (opt) | Primary group. Default: device's group |
| `updateinterval` | double (opt) | Expected update interval in seconds |
| `alivesigninterval` | double (opt) | Timeout in seconds after which the input is considered offline |
| `hardwarename` | string (opt) | Description |

### Sensor object

Declare sensor inputs in the `sensors` array:

| Field | Type | Description |
|---|---|---|
| `id` | string (opt) | String ID for vDC API reference |
| `sensortype` | integer (opt) | `1`=temperature °C, `2`=humidity %, `3`=illumination lux, `4`=supply voltage V, `13`=wind speed m/s, `14`=power W, `16`=energy kWh, and many more. Default: `0` (undefined) |
| `usage` | integer (opt) | `0`=undefined, `1`=room, `2`=outdoors, `3`=user interaction |
| `group` | integer (opt) | Primary group. Default: device's group |
| `min` | double (opt) | Minimum sensor value. Default: `0` |
| `max` | double (opt) | Maximum sensor value. Default: `100` |
| `resolution` | double (opt) | Sensor resolution. Default: `1` |
| `updateinterval` | double (opt) | Expected update interval in seconds |
| `alivesigninterval` | double (opt) | Timeout before sensor is considered offline |
| `changesonlyinterval` | double (opt) | Minimum interval between reporting the same value again. Default: 300 s |
| `hardwarename` | string (opt) | Description |

---

## Messages from the bridge to your device

### Channel change — `channel` / `C`

The bridge sends this when digitalSTROM changes an output channel value (scene call, direct dimming, etc.).

JSON: `{"message":"channel","index":0,"value":100.0,"transition":0.5,"dimming":false}`  
Simple: `C0=100.000000`

| Field | Description |
|---|---|
| `index` | Channel index (0 = brightness for lights) |
| `id` | Channel name string (JSON only) |
| `value` | New channel value (double) |
| `transition` | Transition time in seconds (JSON only) |
| `dimming` | `true` if this is part of a continuous dim operation (JSON only) |

Your device should apply the new value to its hardware.

### Move — `move` / `MV`

Sent when `move: true` was declared in the init message. Requests continuous movement.

JSON: `{"message":"move","index":0,"direction":1}`  
Simple: `MV0=1`

`direction`: `1` = increase, `-1` = decrease, `0` = stop.

### Sync — `sync` / `SYNC`

Sent when `sync: true` was declared. The bridge needs the current output channel values (e.g. for a save-scene operation). Respond with updated channel values followed by the `synced` message.

### Control value — `control` / `CTRL`

Sent when `controlvalues: true` was declared. Carries control values like room temperature set points.

JSON: `{"message":"control","name":"TemperatureSetPoint","value":21.0}`  
Simple: `CTRL.TemperatureSetPoint=21.000000`

### Scene command — `scenecommand` / `SCMD`

Sent when `scenecommands: true` was declared. Values include `OFF`, `MIN`, `MAX`, `INC`, `DEC`, `STOP`, `CLIMATE_ENABLE`, etc.

JSON: `{"message":"scenecommand","cmd":"OFF"}`  
Simple: `SCMD=OFF`

---

## Messages from your device to the bridge

### Channel update — `channel` / `C`

Report that an output channel value has changed (e.g. the light was controlled directly, not via digitalSTROM).

JSON: `{"message":"channel","index":0,"value":80.0}`  
Simple: `C0=80.000000`

### Sensor update — `sensor` / `S`

Push a new sensor reading.

JSON: `{"message":"sensor","index":0,"value":22.5}`  
Simple: `S0=22.5`

### Button event — `button` / `B`

Report a button press/release.

JSON: `{"message":"button","index":0,"value":1}`  
Simple: `B0=1`  

`value`: `1` = pressed, `0` = released, or a duration in milliseconds to simulate press-and-release in a single message (e.g. `B0=250` = 250 ms press).

### Binary input update — `input` / `I`

Report a binary input state change.

JSON: `{"message":"input","index":0,"value":1}`  
Simple: `I0=1`

`value`: `1` = active, `0` = inactive.

### Synced — `synced` / `SYNCED`

Send this after responding to a `sync` request to signal that all channel updates have been sent.

JSON: `{"message":"synced"}`  
Simple: `SYNCED`

### Disconnect — `bye` / `BYE`

Gracefully disconnect from the bridge. The device goes offline in digitalSTROM. Closing the TCP socket has the same effect.

JSON: `{"message":"bye"}`  
Simple: `BYE`

### Log message — `log` / `L`

Send a log message to the bridge's log. Level: `7`=debug, `6`=info, `5`=notice, `4`=warning, `3`=error.

JSON: `{"message":"log","level":6,"text":"temperature updated"}`  
Simple: `L6=temperature updated`

---

## Multiple devices on one connection

Multiple devices can share a single TCP connection. Each device must have a unique `tag` in its `init` message. The tag is a short string that must not contain `=` or `:`.

Send all `init` messages as a single-line JSON array:

```json
[
  {"message":"init","tag":"A","protocol":"simple","output":"light","uniqueid":"light-001","name":"Light A"},
  {"message":"init","tag":"B","protocol":"simple","output":"light","uniqueid":"light-002","name":"Light B"}
]
```

The bridge responds `OK` once for the array. After that, all messages in either direction include the tag as a prefix (simple protocol) or a `"tag"` field (JSON protocol):

```
# Bridge → device
A:C0=100.000000
B:C0=50.000000

# Device → bridge
A:C0=100.000000
B:S0=22.5
```

---

## Examples

### Example 1 — Simple light dimmer (bash)

```bash
#!/usr/bin/env bash
# Registers a dimmable light and follows channel changes.

exec 3<>/dev/tcp/127.0.0.1/8999

printf '{"message":"init","protocol":"simple","output":"light","name":"My Light","uniqueid":"my-light-001"}\n' >&3

read -r ack <&3
[ "$ack" = "OK" ] || { echo "unexpected ack: $ack"; exit 1; }

echo "Device registered. Waiting for commands..."
while read -r msg <&3; do
    echo "Received: $msg"
    # Example: msg="C0=100.000000"
    # Parse and control your hardware here
done
```

### Example 2 — Temperature sensor (bash)

```bash
#!/usr/bin/env bash
# Registers a temperature sensor and periodically pushes readings.

exec 3<>/dev/tcp/127.0.0.1/8999

printf '{"message":"init","protocol":"simple","group":3,"uniqueid":"temp-sensor-001","sensors":[{"sensortype":1,"usage":1,"group":48,"min":0,"max":60,"resolution":0.1}]}\n' >&3

read -r ack <&3
[ "$ack" = "OK" ] || exit 1

while true; do
    # Replace with your actual temperature reading
    temp=$(cat /sys/class/thermal/thermal_zone0/temp | awk '{printf "%.1f", $1/1000}')
    printf 'S0=%s\n' "$temp" >&3
    sleep 60
done
```

### Example 3 — Button (bash)

```bash
#!/usr/bin/env bash
# Registers a light button and sends a press on demand.

exec 3<>/dev/tcp/127.0.0.1/8999

printf '{"message":"init","protocol":"simple","uniqueid":"my-button-001","buttons":[{"buttontype":1,"group":1,"element":0}]}\n' >&3

read -r ack <&3
[ "$ack" = "OK" ] || exit 1

# Simulate a 250 ms button press
printf 'B0=250\n' >&3
echo "Button press sent"
```

### Example 4 — Two devices on one connection (bash)

```bash
#!/usr/bin/env bash

exec 3<>/dev/tcp/127.0.0.1/8999

# Send both init messages as a JSON array on a single line
printf '[{"message":"init","tag":"LIGHT","protocol":"simple","output":"light","uniqueid":"multi-light","name":"Light"},{"message":"init","tag":"SENSOR","protocol":"simple","group":3,"uniqueid":"multi-sensor","sensors":[{"sensortype":1,"usage":1,"min":0,"max":50,"resolution":0.1}]}]\n' >&3

read -r ack <&3
[ "$ack" = "OK" ] || exit 1

# Push a temperature reading for the sensor device
printf 'SENSOR:S0=22.5\n' >&3

# Follow channel changes from both devices
while read -r msg <&3; do
    echo "Received: $msg"
done
```

---

## Protocol tips

- The `uniqueid` field determines the device's position in digitalSTROM. If you restart a script with the same `uniqueid`, the same dS device is reused and your scene assignments are preserved.
- Use a UUID or a stable hardware identifier (MAC address, serial number) as the `uniqueid` to ensure it never changes.
- In production, run your script under a process supervisor so it automatically reconnects if the TCP connection drops.
- If you change the `init` message (different sensors, outputs, etc.) for the same `uniqueid`, the device in digitalSTROM is updated — but large structural changes (adding a button vs. removing one) may confuse the dSS. When making breaking changes, use a new `uniqueid` and re-map the device.
- The `simple` protocol uses `C`, `S`, `B`, `I`, `L`, `BYE`, `SYNCED` as shorthand. The JSON protocol uses the full `message` field names. Both work equally well; pick whichever is easier for your programming language.

---

## Further reading

- [plan44 vdcd external device API](https://github.com/plan44/vdcd/blob/main/docs/plan44%20vdcd%20external%20device%20API.md) — the original upstream documentation with more advanced examples (single devices with actions/states/events, multi-configuration buttons)
- [External Device API plugin setup](plugins/externaldevice.md) — how to configure the TCP listener in the bridge
- [vdcd external device samples](https://github.com/plan44/vdcd/tree/main/external_devices_samples) — example code in bash, Node.js, and C from the upstream project
