package tasmota

import (
	"fmt"
	"strings"
)

// discoveryConfig is the relevant subset of the JSON Tasmota publishes to
// `<discoveryPrefix>/<MAC>/config`. Field tags use Tasmota's compact keys.
//
// Reference: https://tasmota.github.io/docs/Home-Assistant/#mqtt-discovery
type discoveryConfig struct {
	IP      string   `json:"ip"`
	DevName string   `json:"dn"`    // device name
	FNames  []any    `json:"fn"`    // friendly names (string or null) per relay
	Host    string   `json:"hn"`    // hostname
	MAC     string   `json:"mac"`   // 12-hex MAC, no separators
	Model   string   `json:"md"`    // hardware model
	OflnTxt string   `json:"ofln"`  // LWT offline payload (default "Offline")
	OnlnTxt string   `json:"onln"`  // LWT online payload (default "Online")
	State   []string `json:"state"` // POWER state strings: [off, on, toggle, hold]
	SwVer   string   `json:"sw"`    // firmware version
	Topic   string   `json:"t"`     // %topic% substitution
	FullTop string   `json:"ft"`    // full topic pattern e.g. "%prefix%/%topic%/"
	Prefix  []string `json:"tp"`    // [cmnd, stat, tele] prefixes
	RL      []int    `json:"rl"`    // per-relay function: 0=none,1=relay,2=light,3=shutter,4=mqtt
	LtSt    int      `json:"lt_st"` // light subtype: 0=none,1=PWM,2=CCT,3=RGB,4=RGBW,5=RGBCCT
	Ver     int      `json:"ver"`   // discovery payload version
}

// powerStates returns the (off, on) payload strings for this device.
// Defaults to "OFF"/"ON" when not present.
func (d *discoveryConfig) powerStates() (off, on string) {
	off, on = "OFF", "ON"
	if len(d.State) >= 1 && d.State[0] != "" {
		off = d.State[0]
	}
	if len(d.State) >= 2 && d.State[1] != "" {
		on = d.State[1]
	}
	return
}

// onlineStates returns the (online, offline) LWT payload strings.
func (d *discoveryConfig) onlineStates() (online, offline string) {
	online, offline = "Online", "Offline"
	if d.OnlnTxt != "" {
		online = d.OnlnTxt
	}
	if d.OflnTxt != "" {
		offline = d.OflnTxt
	}
	return
}

// prefix returns the prefix at index i (0=cmnd, 1=stat, 2=tele) or the default.
func (d *discoveryConfig) prefixAt(i int, def string) string {
	if i >= 0 && i < len(d.Prefix) && d.Prefix[i] != "" {
		return d.Prefix[i]
	}
	return def
}

// fullTopic resolves the configured `ft` template for the given prefix string
// (one of cmnd/stat/tele). Returns e.g. "cmnd/tasmota_AABBCC/".
func (d *discoveryConfig) fullTopic(prefix string) string {
	t := d.FullTop
	if t == "" {
		t = "%prefix%/%topic%/"
	}
	t = strings.ReplaceAll(t, "%prefix%", prefix)
	t = strings.ReplaceAll(t, "%topic%", d.Topic)
	if !strings.HasSuffix(t, "/") {
		t += "/"
	}
	return t
}

// cmndTopic returns the full command topic for the given suffix (e.g. "POWER1").
func (d *discoveryConfig) cmndTopic(suffix string) string {
	return d.fullTopic(d.prefixAt(0, "cmnd")) + suffix
}

// statTopic returns the full status topic for the given suffix.
func (d *discoveryConfig) statTopic(suffix string) string {
	return d.fullTopic(d.prefixAt(1, "stat")) + suffix
}

// teleTopic returns the full telemetry topic for the given suffix.
func (d *discoveryConfig) teleTopic(suffix string) string {
	return d.fullTopic(d.prefixAt(2, "tele")) + suffix
}

// relayCount returns how many relay/light slots this device exposes.
func (d *discoveryConfig) relayCount() int {
	n := 0
	for _, v := range d.RL {
		if v == 1 || v == 2 {
			n++
		}
	}
	return n
}

// friendlyName returns the friendly name for the (1-based) relay index, or a
// reasonable fallback derived from the device name.
func (d *discoveryConfig) friendlyName(relay int) string {
	idx := relay - 1
	if idx >= 0 && idx < len(d.FNames) {
		if s, ok := d.FNames[idx].(string); ok && s != "" {
			return s
		}
	}
	if d.DevName != "" {
		if d.relayCount() > 1 {
			return fmt.Sprintf("%s %d", d.DevName, relay)
		}
		return d.DevName
	}
	return fmt.Sprintf("Tasmota %s #%d", d.MAC, relay)
}

// powerSuffix returns the POWER topic suffix for the (1-based) relay index.
// Tasmota uses "POWER" for the only relay on a single-relay device, and
// "POWER1", "POWER2", … on multi-relay devices.
func (d *discoveryConfig) powerSuffix(relay int) string {
	if d.relayCount() <= 1 {
		return "POWER"
	}
	return fmt.Sprintf("POWER%d", relay)
}

// kindFor returns the bridge kind (light/dimmer/colorlight) for the (1-based)
// relay index — based on the `rl` slot type and the device-wide `lt_st`.
func (d *discoveryConfig) kindFor(relay int) string {
	idx := relay - 1
	if idx < 0 || idx >= len(d.RL) {
		return "light"
	}
	// For multi-relay devices the light subtype only applies when the relay is
	// flagged as a light (rl[i]==2). Pure relays (rl[i]==1) stay binary lights.
	if d.RL[idx] == 2 {
		switch d.LtSt {
		case 0, 1:
			return "dimmer"
		case 2:
			return "dimmer" // CCT — exposed as plain dimmer for now
		case 3, 4, 5:
			return "colorlight"
		}
	}
	return "light"
}

// entityID returns the bridge entity id for the (1-based) relay index.
//
// Single-relay devices use just the MAC; multi-relay devices append ":Rn" so
// each relay maps to its own bridge device.
func (d *discoveryConfig) entityID(relay int) string {
	if d.relayCount() <= 1 {
		return d.MAC
	}
	return fmt.Sprintf("%s:R%d", d.MAC, relay)
}

// parseEntityID extracts (mac, relayIndex) from an entity id; relay defaults to 1.
func parseEntityID(id string) (mac string, relay int) {
	relay = 1
	if i := strings.Index(id, ":R"); i > 0 {
		mac = id[:i]
		_, _ = fmt.Sscanf(id[i+2:], "%d", &relay)
		if relay <= 0 {
			relay = 1
		}
		return
	}
	return id, 1
}
