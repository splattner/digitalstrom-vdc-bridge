# External Device API plugin

The External Device API plugin lets you integrate any custom device into digitalSTROM using a simple TCP protocol. Your device can be a shell script, a Python program, a Node.js script, a microcontroller, or anything else that can open a TCP socket.

> **See also:** [External Device API reference](../external-device-api.md) for the full protocol specification.

---

## How it works

The plugin starts a TCP server (default port `8999`). Your script connects to that port, sends an `init` message describing the device (name, output type, sensors, buttons, etc.), and then exchanges messages to control the device and report state changes.

From digitalSTROM's point of view the device is just another light, sensor, or button. You handle the actual hardware side in your script.

---

## Setup walkthrough

### Step 1 — Decide on a port

By default the External Device API plugin listens on TCP port `8999`. If you are running in Docker, map that port in your container:

```yaml
ports:
  - "8999:8999"
```

If you need a different port (for example because something else is already using `8999`), note the port number for the plugin configuration.

### Step 2 — Add the plugin

1. Open the bridge web UI and go to the **Plugins page**.
2. Click **+** and choose **External Device API**.
3. Fill in the form:

| Field | What to enter |
|---|---|
| **Plugin ID** | e.g. `externaldevice` |
| **Listen address** | Port number (e.g. `8999`) or `host:port` (e.g. `0.0.0.0:8999`). Leave blank to use the default `8999`. |
| **Accept non-local connections** | Disabled by default — the server only accepts connections from the same machine. Enable this if your script runs on a different host. |

4. Click **Save**. The plugin status should change to **connected**, indicating the server is listening.

### Step 3 — Write or run a device script

Connect your script to the bridge's TCP port. The bridge expects a JSON `init` message on the first line. Here is the simplest possible example — a dimmable light registered with the `simple` text protocol:

```json
{"message":"init","protocol":"simple","output":"light","name":"My Light","uniqueid":"my-light-001"}
```

The bridge responds with `OK`. Your script then waits for channel change messages like `C0=100.000000` (brightness set to 100 %) and can send state updates back (e.g. `C0=0` to report the light turned off).

A ready-to-run shell script example ships with the project:

```bash
# Register a simple light dimmer and follow its channel changes
./examples/external_light_simple.sh 127.0.0.1 8999 "My Light" "my-light-001" follow
```

For more complete examples in different languages, see the [External Device API reference](../external-device-api.md#examples).

### Step 4 — Map the device in the web UI

Once your script has sent the `init` message, the device appears in the **Discovered page** with the kind you declared in the init message. Click **Map** to add it to digitalSTROM.

After mapping, the bridge announces the device to the dSS. From this point commands from digitalSTROM scenes will flow to your script as channel change messages, and any state changes your script reports will be forwarded to digitalSTROM.

### Step 5 — Keep the script running

The TCP connection must stay open for the device to remain active. If the connection drops (for any reason), the device goes offline in digitalSTROM. Your script should reconnect automatically and re-send the `init` message. A process supervisor (systemd, Docker restart policy, runit) is recommended for production use.

---

## Multiple devices on one connection

You can register multiple devices over a single TCP connection by sending a JSON array of `init` messages. Each device must have a unique `tag` field so the protocol can route messages to the right device:

```json
[
  {"message":"init","tag":"A","protocol":"simple","output":"light","uniqueid":"light-001","name":"Light A"},
  {"message":"init","tag":"B","protocol":"simple","output":"light","uniqueid":"light-002","name":"Light B"}
]
```

Subsequent messages must be prefixed with the tag:

```
A:C0=100.000000
B:C0=50.000000
```

---

## Configuration reference

| Field | Type | Default | Description |
|---|---|---|---|
| `listen` | string | `8999` | TCP port or `host:port` to listen on |
| `nonlocal` | bool | `false` | Accept connections from non-loopback addresses |

---

## Troubleshooting

**Script connects but no device appears in Discovered**  
- Make sure the `init` message is a valid single-line JSON and ends with a newline (`\n`).
- Check the `uniqueid` field is present — it is required.
- Look at the External Device API plugin's event log for parse errors.

**Device appears in Discovered but goes offline after a moment**  
- Your script's TCP connection probably dropped. The device is only online while the connection is open.
- Add reconnect logic to your script.

**Port already in use**  
- Change the `listen` field to a different port and restart the plugin. Update the Docker port mapping if needed.
