# Plugins page

The Plugins page (`/#/plugins`) is the starting point for all plugin management. Open it at `http://<bridge>:8090/#/plugins`.

---

## What you see

### KPI tiles

Four summary tiles appear at the top of the page:

| Tile | Meaning |
|---|---|
| **Plugins** | Total number of configured plugin instances |
| **Connected** | Number (and percentage) of plugins currently in the `connected` state |
| **Devices** | Total number of bridge mappings across all plugins |
| **Issues** | Count of warnings and errors logged in the last hour |

### Plugin cards

Each configured plugin has a card showing:

- **Icon tile** — colour-coded by plugin type
- **Plugin name** and type label
- **Status badge** — current connection state (see colours below)
- **Stats row** — *N discovered · M active* and *Last event: X ago*
- **Action buttons** — Logs, Restart, Disable/Enable, Edit, Delete

**Status badge colours:**

| Badge text | Colour | Meaning |
|---|---|---|
| `connected` | Green | Running and connected |
| `connecting` / `reconnecting` | Amber | Trying to connect |
| `starting` | Amber | Initialising |
| `idle` | Grey | Loaded but not yet active |
| `disabled` | Grey | Manually disabled |
| `error` / `auth_failed` | Red | Connection failed |

---

## Common tasks

### Add a new plugin

1. Click the **+** button in the top-right corner.
2. In the picker, read the description for each plugin type and select the one you want.
3. Fill in the configuration form. Required fields are marked with an asterisk (\*).
4. Click **Save**. The plugin starts immediately.

### Edit a plugin's configuration

1. Click the **Edit** button (pencil icon) on the plugin card.
2. Change the fields you need to update.
3. Click **Save**. The plugin restarts automatically with the new configuration.

> **Note:** Changing the plugin ID is not possible after creation. To rename a plugin, delete it and re-create it. Any bridge mappings referencing the old ID will be removed.

### Restart a plugin

Click the **Restart** button (circular arrow) on the plugin card. This stops and re-starts the plugin without affecting the rest of the bridge. Useful when a plugin gets stuck in a reconnect loop.

### Disable a plugin temporarily

Click the **Disable** button (pause icon). The plugin disconnects from its source system and its mapped devices go offline in digitalSTROM. Click **Enable** to bring it back. Use this when, for example, you want to take a HA instance offline for maintenance without losing your mappings.

### View a plugin's event log

Click the **Logs** button to expand an inline log panel at the bottom of the card. Each entry shows the timestamp, log level, and message. The panel displays the last ~500 events from the in-memory ring buffer.

Log levels:
- `info` — normal operation messages
- `warn` — something unexpected but not fatal
- `error` — a failure that may affect functionality
- `debug` — verbose detail (usually hidden unless you need to diagnose a problem)

### Delete a plugin

Click the **Delete** button (trash icon). A confirmation dialog appears. Confirm to remove the plugin and **all its bridge mappings**. The corresponding devices disappear from digitalSTROM.

> This action cannot be undone. You will need to re-create the plugin and re-map any devices.

---

## Auto-refresh

The Plugins page polls the bridge every 5 seconds and updates status badges, stats, and last-event times automatically. You do not need to refresh the browser manually.
