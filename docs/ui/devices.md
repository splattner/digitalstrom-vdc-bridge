# Devices page

The Devices page (`/#/devices`) shows all devices that have been mapped to digitalSTROM. These are the devices that the dSS knows about and can control via scenes and automations.

Open it at `http://<bridge>:8090/#/devices`.

---

## What you see

### Toolbar

- **Search** — filter by device name or DSUID (partial match)
- **Group colour dots** — filter by digitalSTROM primary group (yellow = light, grey = shadow, blue = climate, etc.)
- **Kind chips** — filter by device kind

### Device table

Each row represents one mapped device. The table uses an accordion layout: click a row to expand it and see full device details.

**Collapsed row shows:**

| Column | Content |
|---|---|
| **Icon** | Kind-specific icon with group colour tint |
| **Name** | Device name as it appears in digitalSTROM |
| **Group dot** | Small coloured circle indicating the dS primary group |
| **Channel pills** | Compact inline view of the current output channel values |
| **Plugin badge** | Which plugin manages this device |
| **Actions** | Copy DSUID, Unmap |

**Expanded row adds:**

- Full channel table with progress bars
- Sensor readings (value, unit, last updated)
- Button input states
- Raw DSUID and remote entity ID

---

## Channel pills

For lights and dimmers, the collapsed row shows small progress-bar pills for each output channel:

```
brightness ████████░░ 80 %
hue        ███░░░░░░░ 30°
saturation ██████░░░░ 60 %
```

The pill label is the channel name (brightness, hue, saturation, colour temperature, position, angle, etc.). The bar fills from left to right based on the channel's min/max range. The number shows the exact value with its unit.

Channel values are the last values known to the bridge. They update whenever digitalSTROM sends a command or the source plugin reports a state change.

---

## Sensor rows

Devices with sensor inputs (temperature, humidity, power, etc.) show a sensor table in the expanded row:

| Sensor name | Value | Unit | Type |
|---|---|---|---|
| Temperature | 21.5 | °C | indoor |
| Humidity | 55 | % | indoor |

The value is the last one reported by the plugin. If the plugin has not yet sent a value, the field shows `—`.

---

## Button rows

Devices with button inputs show a button table in the expanded row. The last button state (pressed/released) and the timestamp are shown.

---

## digitalSTROM group colour coding

The small dot on each row represents the device's **primary group** in digitalSTROM. The group determines which scene calls the device responds to by default:

| Colour | Group | Typical devices |
|---|---|---|
| Yellow | Light | Lights, dimmers |
| Grey | Shadow / blind | Roller blinds, shutters |
| Blue | Heating / climate | Thermostats, valves |
| Cyan | Audio | Speakers |
| Magenta | Video | Displays |
| Red | Security | Alarms |
| Green | Access | Door locks |
| Black | Joker / variable | Multi-purpose |

---

## Common tasks

### Find a device

Use the search box to filter by name. You can also click a group colour dot to show only devices in that group, or use the kind chips.

### Copy the DSUID

Click the **copy** icon next to the DSUID in the row. The DSUID is the device's permanent identifier in digitalSTROM. You may need it when configuring scenes or checking the dSS configurator.

### Unmap a device

Click the **Unmap** button (chain-link icon) on the device row. A confirmation dialog appears. Confirm to remove the bridge mapping — the device disappears from digitalSTROM.

The source entity remains in the Discovered list and can be re-mapped at any time.

### Rename a device

Device names are managed in digitalSTROM (via the dSS configurator or app). Changing the name in digitalSTROM does not affect the bridge. If you want to change the name on the bridge side, unmap and re-map the device with the new name.

---

## Auto-refresh

The Devices page polls the bridge every 5 seconds and updates channel values, sensor readings, and button states automatically.
