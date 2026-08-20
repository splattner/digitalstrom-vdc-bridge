# Shelly plugin

The Shelly plugin discovers Shelly **Gen2+** devices (Plus, Pro, Gen3, Gen4) on your local network via mDNS and bridges their relays and dimmers into digitalSTROM. Each device is controlled directly over its own local RPC API — no cloud account, no MQTT broker, and no per-device setup are required.

> **Prerequisites:** One or more Shelly Gen2+ devices on the same network as the bridge, with authentication disabled (the default). No additional configuration is required.

---

## Setup walkthrough

### Step 1 — Verify your Shelly devices are reachable

Open each device's web UI (usually at its IP address in a browser) and confirm it shows firmware **Gen2** or newer. Gen1 devices (an older, entirely different protocol) are not supported by this plugin.

### Step 2 — Add the Shelly plugin

1. Open the bridge web UI and go to the **Plugins page**.
2. Click **+** and choose **Shelly**.
3. The Shelly plugin has no required configuration fields. Give the plugin an ID (e.g. `shelly`) and click **Save**.

The plugin starts an mDNS listener and will discover Shelly devices on the local network automatically. Discovery typically takes a few seconds to a minute after the plugin starts.

### Step 3 — Map devices

Open the **Discovered page** and look for entries with the Shelly plugin badge. A device with more than one bridgeable entity (e.g. a relay plus its power metering, or a relay plus its inputs) shows each as a separate row, named `<device> · <detail>`. Click **Map** on each entity you want to bring into digitalSTROM.

---

## What gets bridged

Each physical Shelly device can expose more than one bridgeable entity. Currently bridged component types:

| Shelly component | digitalSTROM output | Notes |
|---|---|---|
| `switch:N` (relay) | Non-dimmable light (on/off) | The common case — most Shelly Plus/Pro devices |
| `light:N` (dimmer) | Dimmer (0–100% brightness) | Devices with a dimming output |
| Power metering + temperature (from a metered `switch:N` or a standalone `pm1:N`) | Sensor | Voltage, power, current, energy, and/or temperature, whichever fields the device actually reports |
| `input:N` configured as a plain switch/contact | Binary input | Folded into the same sensor entity as any power metering on the device — one bridge mapping per device covers all of its sensors and inputs |
| `input:N` configured as a button | Button | Reports `tip` / `tip2` / `tip3` / `hold` from single/double/triple/long presses |

All power-metering and switch-type-input fields on one physical device share a **single** sensor/binary entity — a relay with a power meter and two wired contacts still only needs two bridge mappings total (the relay, and everything else), not four. A button-configured input is the one exception and always gets its own entity, since button presses are discrete events rather than a channel or sensor value.

Covers (`cover:N`) are not bridged yet — that needs a positional-output kind digitalSTROM doesn't have a builder for in this bridge today.

---

## mDNS requirements

The mDNS-based discovery requires that:

- The bridge host and the Shelly devices are on the **same network segment** (same subnet / VLAN). mDNS traffic does not cross router boundaries without additional multicast routing.
- If you are running the bridge in Docker, use **host networking** (`--network host`) or ensure the Docker host forwards mDNS to the container. Bridge networking (the default) will block mDNS discovery.

A device that has been configured with a friendly name in the Shelly app advertises **two** mDNS records at the same address (one under its device id, one under the friendly name) — the plugin resolves both to the canonical device id via its RPC API and shows it once, so this is expected and not a bug if you notice it in the mDNS traffic.

---

## Input type (switch vs. button)

Whether an `input:N` becomes part of the binary-input sensor entity or gets its own button entity depends on how it's configured on the device itself (its **Input Type** setting — "Switch" vs. "Button" — in the Shelly device's own web UI or app), read once when the plugin first discovers the device.

If you change an input's type on the device after it has already been discovered, the plugin won't pick up the change until it's restarted (the **Restart** button on the Plugins page is enough — no need to delete and re-add the plugin).

---

## Authentication

Devices with authentication enabled (`auth_en: true` in the device's settings) are **not currently supported**. Such a device is skipped with a warning in the plugin's event log rather than failing silently — disable authentication on the device, or wait for a future version, to bridge it.

---

## Configuration reference

The Shelly plugin has no configuration fields. Just set the plugin ID and save.

---

## Troubleshooting

**No Shelly devices appear**
- Make sure the bridge and the Shelly devices are on the same network segment.
- If running in Docker with bridge networking, switch to host networking.
- Confirm the device is Gen2 or newer — Gen1 devices are silently skipped (check the plugin's event log for a `shelly_skip_gen1` entry).
- Check the plugin's event log for mDNS or RPC errors.

**A device is skipped with an "authentication is enabled" warning**
- Disable authentication on the device (its own web UI, under **Settings → Device**) to bridge it.

**Device appears but does not respond**
- Verify the device is still reachable at its IP address.
- Check that no firewall is blocking HTTP/WebSocket traffic from the bridge host to the device's port 80.
