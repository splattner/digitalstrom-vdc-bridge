# Getting Started

This guide walks you through installing the digitalSTROM vDC Bridge, connecting it to your digitalSTROM system, and adding your first device.

---

## Prerequisites

- A running **digitalSTROM Server (dSS)** on your network (any recent firmware)
- The dSS configurator accessible in a browser
- One of: Docker, Docker Compose, Home Assistant OS, or a Linux machine with Go 1.23+ for building from source

---

## Installation options

Choose the method that fits your setup:

- [Docker (quick start)](#docker-quick-start)
- [Docker Compose (recommended for self-hosters)](#docker-compose)
- [Home Assistant Add-on](#home-assistant-add-on)
- [Build from source](#build-from-source)

---

## Docker quick start

The fastest way to get the bridge running is a single Docker command.

**1. Pull and run the container:**

```bash
docker run -d \
  --name digitalstrom-vdc-bridge \
  --restart unless-stopped \
  -p 8090:8090 \
  -p 8999:8999 \
  -v ./vdc-data:/data \
  ghcr.io/splattner/digitalstrom-vdc-bridge:latest
```

| Port | Purpose |
|---|---|
| `8090` | Web UI |
| `8999` | vDC API (the port your dSS connects to) |

The `-v ./vdc-data:/data` flag stores the bridge's configuration and state in a `vdc-data` folder in your current directory. This means your settings survive container restarts and upgrades.

**2. Open the web UI:**

Navigate to `http://<docker-host>:8090` in your browser. You should see the Plugins page.

**3. Continue with [Connect to digitalSTROM](#connect-to-digitalstrom) below.**

---

## Docker Compose

A Compose file is the recommended approach if you are already running other services with Docker Compose.

**1. Create a `docker-compose.yml`:**

```yaml
services:
  vdc-bridge:
    image: ghcr.io/splattner/digitalstrom-vdc-bridge:latest
    restart: unless-stopped
    ports:
      - "8090:8090"   # Web UI
      - "8999:8999"   # vDC API
    volumes:
      - ./vdc-data:/data
```

**2. Start the service:**

```bash
docker compose up -d
```

**3. Open `http://<host>:8090` and continue with [Connect to digitalSTROM](#connect-to-digitalstrom).**

> **Tip:** If you are running the bridge alongside a Zigbee2MQTT or Home Assistant container, you can put them all in the same Compose file so they share a Docker network. Use the service name (e.g. `mqtt`) as the broker hostname in the MQTT plugin config.

---

## Home Assistant Add-on

If you run Home Assistant OS (or Home Assistant Supervised), the easiest path is the built-in add-on.

**1. Add the repository to the add-on store:**

In Home Assistant go to **Settings → Add-ons → Add-on Store** and click the three-dot menu in the top-right corner → **Repositories**. Add:

```
https://github.com/splattner/digitalstrom-vdc-bridge
```

Alternatively, click the badge below to open the dialog pre-filled:

[![Add repository](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fsplattner%2Fdigitalstrom-vdc-bridge)

**2. Install the add-on:**

Scroll down in the add-on store until you see **digitalSTROM vDC Bridge**. Click it and then click **Install**.

**3. Review options (optional):**

Open the **Configuration** tab. The defaults work for most setups. See [Home Assistant Add-on](ha-addon.md) for a full description of all options.

**4. Start the add-on:**

Click **Start** on the **Info** tab. After a few seconds the status light should turn green.

**5. Open the web UI:**

Click **Open Web UI** on the Info tab, or enable **Show in sidebar** so the UI is always one click away.

**6. Continue with [Connect to digitalSTROM](#connect-to-digitalstrom).**

> **Note:** A Home Assistant plugin is automatically created the first time the add-on starts, pre-configured with your HA instance's WebSocket URL and a placeholder token that you will need to fill in.

---

## Build from source

**Prerequisites:** Go 1.23+, Node.js 20+

```bash
# 1. Clone the repository
git clone https://github.com/splattner/digitalstrom-vdc-bridge.git
cd digitalstrom-vdc-bridge

# 2. Build the web UI (outputs into pkg/httpapi/webdist, gets embedded in the binary)
make web

# 3. Build the Go binary
make build
```

The resulting binary is `./vdcgo-daemon`.

**Run it:**

```bash
./vdcgo-daemon \
  --datadir ./data \
  --http-listen :8090
```

The bridge listens for the vDC API on port `8999` by default (configured via the External Device API plugin in the UI).

Open `http://localhost:8090` for the web UI.

---

## Connect to digitalSTROM

The bridge announces itself on the local network via **mDNS (DNS-SD)**. The digitalSTROM Server discovers it automatically — there is nothing to add manually in the dSS configurator.

**1. Make sure the bridge and the dSS are on the same network segment** so that mDNS traffic can reach the dSS. Multicast traffic is typically not forwarded across VLANs or routed subnets.

**2. Wait a few seconds.** The dSS polls for mDNS announcements periodically. Once it sees the bridge it opens a connection on its own.

**3. Confirm the connection** on the [Settings page](ui/settings.md) of the bridge web UI — the vDSM connection status will turn green.

> **If the bridge is not discovered:**
> - Check that mDNS/DNS-SD is working on your network. Tools like `avahi-browse -a` on Linux or **Discovery** on macOS can confirm the service is visible.
> - When running in Docker, use `--network host` (or the equivalent in Compose) so the container's mDNS announcements reach the host network. Bridge-mode networking isolates multicast.
> - The HA add-on handles this automatically.

---

## Add your first plugin and device

Now that the bridge is connected to digitalSTROM, you can add plugins to discover devices.

**Step 1 — Open the Plugins page** at `http://<bridge>:8090/#/plugins`.

**Step 2 — Add a plugin.** Click the **+** button in the top-right corner. A modal shows all available plugin types with descriptions. Choose the one that matches your setup (for example, **Home Assistant**).

**Step 3 — Fill in the configuration** form. Each plugin type has its own fields (URL, credentials, etc.). See the individual plugin pages for details:

- [Home Assistant](plugins/homeassistant.md)
- [MQTT Broker](plugins/mqtt.md) + [Zigbee2MQTT](plugins/zigbee2mqtt.md)
- [MQTT Broker](plugins/mqtt.md) + [Tasmota](plugins/tasmota.md)
- [WLED](plugins/wled.md)
- [External Device API](plugins/externaldevice.md)

**Step 4 — Save.** The plugin starts and discovers devices. After a moment the status badge on the plugin card should say **connected**.

**Step 5 — Open the Discovered page** at `http://<bridge>:8090/#/discovered`. You will see a list of all the entities the plugin found (lights, sensors, buttons, etc.).

**Step 6 — Map a device.** Click **Map** next to the entity you want to add to digitalSTROM. A dialog lets you set the device name and type. Confirm and the device is announced to the dSS.

**Step 7 — Check digitalSTROM.** Open the dSS configurator. The new device will appear in the default room for external devices. Drag it to the room where it belongs and include it in scenes as you would any native dS device.

---

## Next steps

- Add more plugins and map more devices — see the [Plugins overview](plugins/overview.md).
- Understand how devices, mappings, and state persistence work — see [Core Concepts](concepts.md).
- Explore the web UI in depth — see the [UI section](ui/plugins.md).
