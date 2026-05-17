# Home Assistant plugin

The Home Assistant plugin connects to a Home Assistant instance via its WebSocket API and discovers lights, switches, and sensors as vDC devices.

> **Prerequisites:** A running Home Assistant instance reachable from the bridge host, and a Long-Lived Access Token.

---

## Setup walkthrough

### Step 1 — Create a Long-Lived Access Token in Home Assistant

1. In Home Assistant, click your **user icon** in the bottom-left corner → **Profile**.
2. Scroll to the bottom of the page to **Long-Lived Access Tokens**.
3. Click **Create Token**, give it a name (e.g. `vdc-bridge`), and copy the token. You will only see it once.

### Step 2 — Add the plugin

1. Open the bridge web UI and go to the **Plugins page**.
2. Click **+** and choose **Home Assistant**.
3. Fill in the form:

| Field | What to enter |
|---|---|
| **Plugin ID** | A short unique name, e.g. `homeassistant` |
| **WebSocket URL** | `ws://homeassistant.local:8123/api/websocket` — replace the hostname if needed |
| **Long-lived access token** | Paste the token you created in Step 1 |

4. Click **Save**.

The plugin connects and starts discovering entities. After a moment the status badge should say **connected** and the discovered count will start growing.

### Step 3 — Filter unwanted entities (optional)

By default the plugin discovers *all* light, switch, and sensor entities in Home Assistant. That can be a lot. Two filters help you narrow it down:

**Ignore integrations:**  
After saving the plugin once, re-open the edit form. The **Ignore integrations** field is now populated with all the integrations currently present in your HA instance. Tick any integrations whose entities you do not want to bridge (for example, skip all `zha` entities if you are also using the Zigbee2MQTT plugin for those devices).

**Ignore Zigbee2MQTT devices:**  
If you are running both a Home Assistant plugin and a Zigbee2MQTT plugin, HA will likely expose Z2M devices via the MQTT integration. Enable **Ignore Zigbee2MQTT devices** to deduplicate — the HA plugin will skip those entities and the Z2M plugin will handle them directly.

**Ignore entity prefixes:**  
Enter a comma-separated list of entity ID prefixes to skip. For example: `sensor.battery_, binary_sensor.linkquality_` hides all battery and link-quality entities.

### Step 4 — Map devices

Open the **Discovered page** and you will see all the entities the HA plugin found. Each one has a kind badge (`colorlight`, `dimmer`, `light`, `sensor`, `binary`, `button`). Click **Map** next to any entity to add it to digitalSTROM.

---

## Supported entity kinds

| HA entity type | Bridged as | digitalSTROM output |
|---|---|---|
| `light` with colour + colour temp | `colorlight` | Full colour light (brightness, hue, saturation, colour temp) |
| `light` with colour temp only | `dimmer` | Tunable white (brightness) |
| `light` with brightness only | `dimmer` | Dimmable light |
| `light` on/off only | `light` | Simple switch |
| `switch` | `light` | Simple switch (yellow/light group) |
| `binary_sensor` | `binary` | Binary sensor input |
| `sensor` (numeric) | `sensor` | Sensor input |

---

## Configuration reference

| Field | Type | Default | Description |
|---|---|---|---|
| `url` | string | — | WebSocket URL of the HA instance. **Required.** |
| `token` | password | — | Long-Lived Access Token. **Required.** |
| `ignore_integrations` | multiselect | — | Integration names whose entities are skipped |
| `ignore_zigbee2mqtt` | bool | `false` | Skip entities backed by Zigbee2MQTT |
| `ignore_entity_prefixes` | string | — | Comma-separated entity ID prefixes to ignore |

---

## Troubleshooting

**Status shows `auth_failed`**  
The token is wrong or has been deleted. Generate a new token in HA and update the plugin config.

**Status stays `connecting`**  
- Make sure the WebSocket URL is reachable from the bridge host. Try `ws://` not `wss://` for local connections.
- If Home Assistant runs behind a reverse proxy with TLS, use `wss://` and the correct hostname.

**No entities appear**  
- Check the ignore rules — you may have accidentally filtered everything out.
- Some entities are excluded by HA itself if they are unavailable or disabled in HA.
