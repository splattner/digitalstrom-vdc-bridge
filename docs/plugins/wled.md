# WLED plugin

The WLED plugin discovers WLED LED controllers on your local network via mDNS and bridges them as colour lights in digitalSTROM.

> **Prerequisites:** One or more WLED devices on the same network as the bridge. No additional configuration is required.

---

## Setup walkthrough

### Step 1 — Verify your WLED devices are reachable

Open the WLED web UI on each device (usually at its IP address in a browser). Confirm each device is connected to your Wi-Fi and responsive.

### Step 2 — Add the WLED plugin

1. Open the bridge web UI and go to the **Plugins page**.
2. Click **+** and choose **WLED**.
3. The WLED plugin has no required configuration fields. Give the plugin an ID (e.g. `wled`) and click **Save**.

The plugin starts an mDNS listener and will discover WLED devices on the local network automatically. Discovery typically takes a few seconds to a minute after the plugin starts.

### Step 3 — Map devices

Open the **Discovered page** and look for entries with the WLED plugin badge. Click **Map** to add a WLED controller to digitalSTROM as a colour light.

---

## What gets bridged

Each WLED controller is exposed as a single `colorlight` device with:

- Brightness
- Hue and saturation (colour)
- Colour temperature (on controllers that support white/CCT channels)

WLED segments are not exposed individually — the bridge controls the controller as a whole.

---

## mDNS requirements

The mDNS-based discovery requires that:

- The bridge host and the WLED devices are on the **same network segment** (same subnet / VLAN). mDNS traffic does not cross router boundaries without additional multicast routing.
- If you are running the bridge in Docker, use **host networking** (`--network host`) or ensure the Docker host forwards mDNS to the container. Bridge networking (the default) will block mDNS discovery.

If mDNS discovery is not feasible in your setup, a future version may add manual IP configuration.

---

## Configuration reference

The WLED plugin has no configuration fields. Just set the plugin ID and save.

---

## Troubleshooting

**No WLED devices appear**  
- Make sure the bridge and the WLED devices are on the same network segment.
- If running in Docker with bridge networking, switch to host networking.
- Try restarting the WLED device — it will re-announce itself via mDNS.
- Check the plugin's event log for mDNS errors.

**Device appears but does not respond**  
- Verify the WLED device is still reachable at its IP address.
- Check that no firewall is blocking HTTP traffic from the bridge host to the WLED device.
