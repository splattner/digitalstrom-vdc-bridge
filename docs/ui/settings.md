# Settings page

The Settings page (`/#/settings`) shows identity information, runtime status, and UI preferences. It also provides a danger zone for resetting state.

Open it at `http://<bridge>:8090/#/settings`.

---

## Sections

### Identity

This section shows how the bridge presents itself to the digitalSTROM system.

| Field | Description |
|---|---|
| **Description** | The human-readable name shown in the dSS configurator for this vDC instance |
| **vDC DSUID** | The permanent identifier of the bridge's virtual device connector. This is generated once and never changes. |
| **vDC Host DSUID** | The identifier of the vDC host (the bridge process). |

The description can be changed by editing the `config.json` in the data directory or via the `--description` flag when starting the bridge. It cannot currently be changed from this UI page.

### Runtime

Shows live information about the running bridge process.

| Field | Description |
|---|---|
| **Version** | The bridge software version |
| **vDSM connection** | Whether the bridge is currently connected to a digitalSTROM Server |
| **Connected DSS** | IP address and port of the connected dSS, if any |
| **Uptime** | How long the bridge has been running since the last restart |
| **Plugins** | Total / connected count |
| **Mapped devices** | Total number of active bridge mappings |

Use the **Refresh** button to reload the current values.

### UI Preferences

These settings are stored in your browser and apply only to your local session.

| Setting | Options | Description |
|---|---|---|
| **Theme** | Light / Dark / System | Colour scheme for the web UI |
| **Time format** | 12h / 24h | Time display format used throughout the UI |
| **Max protocol frames** | Number | Maximum number of frames stored on the Protocol page (default 500). Increase for longer captures; decrease to save browser memory. |

Click **Save preferences** to apply changes. Click **Reset to defaults** to clear all UI preferences.

### Danger zone

> **Warning:** The actions in this section are irreversible.

**Purge all state**  
Deletes `status.json` — the file that stores device channel values, scene tables, and last-known device states. Bridge mappings and plugin configurations are *not* deleted. After purging, devices will re-announce themselves to the dSS and their states will be rebuilt from the next plugin update.

Use this if device states in digitalSTROM have drifted out of sync and a fresh start is needed.

---

## Common tasks

### Check if the dSS is connected

Open the Settings page and look at the **vDSM connection** row in the Runtime section. If it shows a green indicator and the dSS address, the bridge is connected. If it is grey or red, the dSS has not connected yet — check that the dSS is configured to point at the bridge address and port.

### Copy the vDC DSUID

The vDC DSUID may be requested by digitalSTROM support or when debugging a pairing issue. Click the **copy** icon next to it.

### Switch to dark mode

Open Settings → UI Preferences → Theme and select **Dark** (or **System** to follow your OS preference). Click **Save preferences**.
