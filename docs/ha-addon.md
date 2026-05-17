# Home Assistant Add-on

The digitalSTROM vDC Bridge is available as a native Home Assistant add-on for users running Home Assistant OS or Home Assistant Supervised. The add-on is the easiest way to use the bridge alongside Home Assistant.

---

## Installation

### Step 1 — Add the repository

In Home Assistant, go to **Settings → Add-ons → Add-on Store**. Click the three-dot menu in the top-right corner and choose **Repositories**. Add:

```
https://github.com/splattner/digitalstrom-vdc-bridge
```

Or use the quick-add link:

[![Add repository](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fsplattner%2Fdigitalstrom-vdc-bridge)

### Step 2 — Install the add-on

Scroll down in the add-on store to find **digitalSTROM vDC Bridge** and click **Install**.

### Step 3 — Configure the add-on

Open the **Configuration** tab and review the options (see [Options reference](#options-reference) below). The defaults work for most setups.

### Step 4 — Start the add-on

Click **Start** on the **Info** tab. Check the **Log** tab to confirm the add-on started successfully.

### Step 5 — Open the web UI

Click **Open Web UI** or enable **Show in sidebar** to access the bridge UI from the HA sidebar.

### Step 6 — Connect digitalSTROM

Configure your dSS to point at the HA host's IP address on port `8340` (the vDC API port). See [Connect to digitalSTROM](getting-started.md#connect-to-digitalstrom) in the Getting Started guide.

---

## First run: automatic plugin setup

On the very first start, if no `plugins.json` exists yet, the add-on automatically creates one pre-populated with two plugins:

1. **Home Assistant plugin** — pre-configured with the internal HA Supervisor WebSocket URL (`ws://supervisor/core/websocket`) and the current Supervisor token. You do not need to create a Long-Lived Access Token separately; the add-on handles authentication via the Supervisor API.

2. **External Device API plugin** — pre-configured to listen on the port specified in the `listen_port` option.

This means that on first start you can immediately open the Discovered page and start mapping HA entities to digitalSTROM — no manual plugin setup required.

### Automatic token refresh

The Supervisor token is rotated on every add-on restart. The add-on automatically refreshes the token in `plugins.json` on every start so the HA plugin always has a valid credential.

### Migration: adding the External Device API plugin to existing installs

If you already have a `plugins.json` from an earlier version without the External Device API plugin, the add-on will add the External Device API plugin automatically on the next restart.

---

## Options reference

| Option | Type | Default | Description |
|---|---|---|---|
| `description` | string | `digitalSTROM vDC Bridge` | Name advertised via DNS-SD and shown in the dSS configurator |
| `listen_port` | port | `8999` | TCP port for the External Device API — the port external scripts connect to. Also used to pre-configure the External Device API plugin. |
| `vdcapi_port` | port | `8340` | Internal port for the vDC API connection from the dSS. Point your dSS at this port. |
| `non_local` | bool | `true` | Accept vDC API connections from any host. Required when the dSS is not running on the same machine as the add-on (which is the normal case). |
| `no_discovery` | bool | `false` | Disable DNS-SD advertisement. Enable this if you prefer to enter the bridge address manually in the dSS. |
| `no_auto` | bool | `false` | Publish the vDC with the `noauto` flag. This prevents the vDSM from automatically including new devices in default scenes. |
| `use_avahi_dbus` | bool | `true` | Use Avahi via D-Bus for DNS-SD advertisement. Required in the HA add-on environment. Disable only if Avahi is not available on the host. |

---

## Port mapping

The add-on exposes two ports:

| Port | Exposed to network | Description |
|---|---|---|
| `8340/tcp` | Yes (configurable) | vDC API — the dSS connects here |
| `8999/tcp` | Optional (null by default) | External Device API — only expose this if you want external scripts running outside the HA host to connect |

The web UI port (`8090`) is served via HA ingress and is not directly exposed.

> **Note on port 8999:** By default this port is not mapped to the host network (`null` in the add-on config). If you only run the External Device API scripts inside the same HA host (e.g. in another add-on or in the HA Scripts integration), the port does not need to be exposed. If you want to connect from another machine, set the port mapping in the add-on's Network configuration tab.

---

## Ingress

The web UI is available via HA ingress, which means it is embedded directly in the HA sidebar without any extra port exposure. Enable **Show in sidebar** in the add-on's Info tab for one-click access.

If you prefer direct browser access, the UI is also available at `http://<ha-host>:8090` from within your local network (as long as port `8090` is routable — ingress proxies it through HA's own HTTPS endpoint).

---

## Troubleshooting

**Add-on fails to start**  
Check the Log tab. Common reasons:
- Port conflict: `8340` or `8999` already in use on the host. Change the port in the options.
- Permission error: the `/data` volume is not writable. This usually resolves itself on reinstall.

**HA plugin shows `auth_failed` after a restart**  
This should not happen because the token is refreshed automatically. If it does occur, restart the add-on again — the token refresh runs at startup.

**dSS cannot connect to the bridge**  
- Verify the `vdcapi_port` option matches what you entered in the dSS configurator.
- Make sure `non_local` is enabled (it defaults to `true` in the add-on).
- Check that no network-level firewall is blocking port `8340` from the dSS host.

**DNS-SD discovery not working**  
- Make sure `use_avahi_dbus` is enabled. The add-on uses D-Bus to talk to the host's Avahi daemon for mDNS advertisement.
- The HA host must have Avahi installed (it typically does on HA OS).
- If the dSS still cannot find the bridge via DNS-SD, use manual IP+port entry in the dSS configurator and set `no_discovery: true` to avoid misleading logs.
