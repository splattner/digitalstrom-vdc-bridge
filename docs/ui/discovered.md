# Discovered page

The Discovered page (`/#/discovered`) shows all the entities that the bridge's plugins have found in connected third-party systems. This is where you map entities to digitalSTROM devices.

Open it at `http://<bridge>:8090/#/discovered`.

---

## What you see

### Toolbar

At the top of the page there is a search box and a row of filter chips:

- **Search** — filters by entity name or ID (partial match, case-insensitive)
- **Kind chips** — show only entities of a specific kind: `colorlight`, `dimmer`, `light`, `sensor`, `binary`, `button`
- **Plugin chips** — show only entities from a specific plugin
- **Mapped / Unmapped** — show only entities that have or have not been mapped to a digitalSTROM device

All filters combine. For example: select the `sensor` kind chip and the `zigbee2mqtt` plugin chip to see only Zigbee sensors.

### Entity table

Each row in the table represents one discovered entity. Columns:

| Column | Content |
|---|---|
| **Icon** | Visual icon matched to the entity kind |
| **Name** | Entity name as reported by the plugin |
| **Remote ID** | The entity's identifier in the source system (HA entity ID, Z2M address, etc.) |
| **Plugin** | Which plugin owns this entity, with its connection status badge |
| **Kind** | The device kind that will be used when mapping (`colorlight`, `dimmer`, etc.) |
| **Mapped** | A badge showing whether the entity has an active bridge mapping |
| **Action** | **Map** (if unmapped) or **Unmap** (if already mapped) button |

### Sibling grouping

Some devices consist of multiple endpoints that the bridge discovers as separate entities (for example, a Zigbee remote with several buttons, or a Z2M device with both a light and a sensor). The bridge groups these **siblings** under the first entity using a collapsible chevron. Click the row with the chevron to expand or collapse the group.

---

## Common tasks

### Map an entity to digitalSTROM

1. Find the entity in the table. Use the search box or filter chips to narrow it down.
2. Click **Map** in the action column.
3. A dialog appears with pre-filled fields:
   - **Name** — the display name the device will have in digitalSTROM (you can change this)
   - **Kind** — the device type (usually pre-selected from the entity's kind)
4. Click **Create**. The device is immediately announced to the dSS and appears on the [Devices page](devices.md).

### Unmap a device

Click **Unmap** in the action column. The device is removed from digitalSTROM but the entity remains in the Discovered list so you can re-map it at any time.

Alternatively, go to the [Devices page](devices.md) and use the unmap action there.

### Sort the table

Click any column header to sort by that column. Click again to reverse the sort order. The arrow indicator shows the current sort direction.

---

## Kind badges explained

| Badge | Colour | digitalSTROM output |
|---|---|---|
| `colorlight` | Fuchsia | Full-colour light (brightness, hue, saturation, colour temp) |
| `dimmer` | Amber | Dimmable light (brightness) |
| `light` | Yellow | On/off light |
| `sensor` | Sky blue | Sensor input (temperature, humidity, power, etc.) |
| `binary` | Slate | Binary sensor (motion, door contact, etc.) |
| `button` | Violet | Push button / switch input |

---

## Plugin status badge on each row

The plugin badge next to the plugin name uses the same colour coding as the Plugins page. If a plugin disconnects after you have mapped a device, the mapped device goes offline in digitalSTROM — you will see the plugin badge turn amber or red on this page as a warning.
