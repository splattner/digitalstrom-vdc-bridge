# Protocol page

The Protocol page (`/#/protocol`) shows a live log of every vDC API protobuf frame exchanged between the bridge and the digitalSTROM Server (vDSM). It is primarily useful for debugging connection issues and understanding how the bridge communicates with digitalSTROM.

Open it at `http://<bridge>:8090/#/protocol`.

---

## What you see

### Frame table

Each row in the table is one protobuf frame. Columns:

| Column | Content |
|---|---|
| **#** | Local sequence number (monotonically increasing, resets on bridge restart) |
| **Time** | Timestamp of the frame (HH:MM:SS.mmm) |
| **Dir** | Direction: `↓ rx` (received from dSS, shown in blue) or `↑ tx` (sent to dSS, shown in green) |
| **Type** | Message type name (e.g. `helloRequest`, `announceDevice`, `callScene`) |
| **Msg ID** | The vDC API message ID, used to pair requests with responses |
| **Device** | Short form of the DSUID extracted from the frame payload, if present |
| **Name** | Device name extracted from the frame, if available |

### Request/response pairing

Frames with a message ID (most request/response pairs) are linked visually: the row has a coloured left border (the colour is deterministic from the message ID using a golden-angle spread). When you select one frame of a pair, the paired frame is highlighted with a lighter background. A small `⇄` indicator on the message ID cell confirms that the pair was found.

### Detail panel

Click any row to open the detail panel on the right side of the page. It shows:

- Full frame metadata (sequence number, time, direction, type, message ID)
- **Decoded payload** — the frame contents as a formatted JSON tree. This is the human-readable version of the protobuf message.
- **Raw hex** — the raw wire bytes, useful for low-level debugging.

---

## Common tasks

### Watch the handshake when dSS connects

When the dSS first connects to the bridge you will see a burst of frames:

1. `helloRequest` / `helloResponse` — initial handshake
2. `announceVdc` — the bridge announces itself
3. `announceDevice` (one per mapped device) — each device is registered
4. Scene table exchanges

This sequence is normal. If the dSS repeatedly connects and disconnects you will see this repeat.

### Identify why a device is not responding to a scene

1. Trigger the scene in digitalSTROM.
2. Watch the Protocol page for a `callScene` frame. Click on it to see the scene number and the DSUID of the target device.
3. If the frame arrives but the device does not respond, the issue is in the plugin (not the vDC protocol layer). Check the plugin's event log on the Plugins page.

### Find a specific device's frames

Use the **device filter** dropdown (if present) or scroll through the frame list and watch the Device column for the DSUID short form. Selecting a frame for a particular device will highlight its paired response.

### Adjust the maximum number of stored frames

By default the page stores the last 500 frames in memory. You can change this limit on the [Settings page](settings.md) under **UI Preferences → Max protocol frames**. Increasing the limit is useful for capturing long sequences; decreasing it saves browser memory.

---

## Frame types reference

Common frame types you will see:

| Type | Direction | Meaning |
|---|---|---|
| `helloRequest` | rx | dSS initiating handshake |
| `helloResponse` | tx | Bridge responding to handshake |
| `announceVdc` | tx | Bridge announcing itself to dSS |
| `announceDevice` | tx | Bridge announcing a mapped device |
| `vanishDevice` | tx | Bridge removing a device (after unmap) |
| `callScene` | rx | dSS asking a device to execute a scene |
| `setOutputChannelValue` | rx | dSS setting a direct channel value |
| `channelStateChanged` | tx | Bridge reporting a device state change |
| `getProperty` | rx | dSS reading a device property |
| `setProperty` | rx | dSS writing a device property |

---

## Note on performance

The Protocol page renders frames in real-time. In a busy system with many devices and frequent scene calls this can generate a lot of rows quickly. If the browser slows down, reduce the max frames limit in Settings or navigate away from the Protocol page until you need it.
