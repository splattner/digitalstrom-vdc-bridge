package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/websocket"
)

// haArea is one entry from `config/area_registry/list`.
type haArea struct {
	AreaID string `json:"area_id"`
	Name   string `json:"name"`
}

// haDevice is one entry from `config/device_registry/list`. Only the fields
// we surface to the UI are decoded.
//
// `identifiers` is decoded leniently: HA serialises each identifier as a
// JSON array (originally a Python tuple) whose elements can in principle be
// any JSON value. We accept anything and stringify it so a single odd
// device can't fail the whole batch decode.
type haDevice struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	NameByUser   string     `json:"name_by_user"`
	AreaID       string     `json:"area_id"`
	Manufacturer string     `json:"manufacturer"`
	Model        string     `json:"model"`
	SWVersion    string     `json:"sw_version"`
	HWVersion    string     `json:"hw_version"`
	Identifiers  [][]string `json:"-"`
	RawIDs       [][]any    `json:"identifiers"`
	ViaDeviceID  string     `json:"via_device_id"`
}

// normaliseIdentifiers converts the loosely-typed RawIDs into the [][]string
// shape the rest of the package expects.
func (d *haDevice) normaliseIdentifiers() {
	if len(d.RawIDs) == 0 {
		return
	}
	out := make([][]string, 0, len(d.RawIDs))
	for _, tuple := range d.RawIDs {
		row := make([]string, 0, len(tuple))
		for _, v := range tuple {
			row = append(row, fmt.Sprintf("%v", v))
		}
		out = append(out, row)
	}
	d.Identifiers = out
	d.RawIDs = nil
}

// haEntityReg is one entry from `config/entity_registry/list`.
type haEntityReg struct {
	EntityID string `json:"entity_id"`
	DeviceID string `json:"device_id"`
	AreaID   string `json:"area_id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// haRegistries holds the resolved HA registry data, keyed for fast lookup.
type haRegistries struct {
	Areas    map[string]haArea      // area_id → area
	Devices  map[string]haDevice    // device_id → device
	Entities map[string]haEntityReg // entity_id → entity registry entry
}

// fetchRegistries loads area, device, entity registries from the connected HA
// instance. Best-effort: if any one call fails it returns what it has plus an
// error (the caller may still publish partial data). Decode errors are
// reported via onWarn so silent registry corruption surfaces in the UI log.
func (c *wsClient) fetchRegistries(ctx context.Context, conn *websocket.Conn) (haRegistries, error) {
	regs := haRegistries{
		Areas:    map[string]haArea{},
		Devices:  map[string]haDevice{},
		Entities: map[string]haEntityReg{},
	}
	warn := func(code, msg string, fields map[string]any) {
		if c.onWarn != nil {
			c.onWarn(code, msg, fields)
		}
	}

	// Areas
	if raw, err := c.command(ctx, conn, map[string]any{"type": "config/area_registry/list"}); err == nil {
		var areas []haArea
		if err := decodeListResult(raw, &areas); err != nil {
			warn("ha_registry_decode", "area_registry decode failed",
				map[string]any{"error": err.Error(), "raw": truncate(string(raw), 240)})
		} else {
			for _, a := range areas {
				regs.Areas[a.AreaID] = a
			}
		}
	} else {
		return regs, fmt.Errorf("area_registry: %w", err)
	}

	// Devices
	if raw, err := c.command(ctx, conn, map[string]any{"type": "config/device_registry/list"}); err == nil {
		var devs []haDevice
		if err := decodeListResult(raw, &devs); err != nil {
			warn("ha_registry_decode", "device_registry decode failed",
				map[string]any{"error": err.Error(), "raw": truncate(string(raw), 240)})
		} else {
			for _, d := range devs {
				d.normaliseIdentifiers()
				regs.Devices[d.ID] = d
			}
		}
	} else {
		return regs, fmt.Errorf("device_registry: %w", err)
	}

	// Entities
	if raw, err := c.command(ctx, conn, map[string]any{"type": "config/entity_registry/list"}); err == nil {
		var ents []haEntityReg
		if err := decodeListResult(raw, &ents); err != nil {
			warn("ha_registry_decode", "entity_registry decode failed",
				map[string]any{"error": err.Error(), "raw": truncate(string(raw), 240)})
		} else {
			for _, e := range ents {
				regs.Entities[e.EntityID] = e
			}
		}
	} else {
		return regs, fmt.Errorf("entity_registry: %w", err)
	}

	return regs, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// command sends a typed command to HA and waits for the matching `result`.
// The returned bytes are the full result envelope (see decodeListResult).
func (c *wsClient) command(ctx context.Context, conn *websocket.Conn, msg map[string]any) (json.RawMessage, error) {
	id := c.nextMsgID()
	ch := c.registerPending(id)
	defer c.cancelPending(id)
	msg["id"] = id
	if err := c.send(ctx, conn, msg); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case raw := <-ch:
		return raw, nil
	}
}

// decodeListResult decodes the `result` field of a generic command envelope
// into the given destination (typically a slice).
func decodeListResult(raw json.RawMessage, dst any) error {
	var env struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Error   any             `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if !env.Success {
		return fmt.Errorf("command failed: %v", env.Error)
	}
	if len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, dst)
}

// deviceMeta returns the manufacturer, model, and sw_version for the HA device
// associated with entityID. Any field absent from the registry is returned as "".
func (r haRegistries) deviceMeta(entityID string) (manufacturer, model, swVersion string) {
	ent, ok := r.Entities[entityID]
	if !ok {
		return
	}
	dev, ok := r.Devices[ent.DeviceID]
	if !ok {
		return
	}
	manufacturer = dev.Manufacturer
	model = dev.Model
	swVersion = dev.SWVersion
	return
}

// resolveExtras returns plugin-specific attributes for an entity (device name,
// area name, manufacturer, model, integration). Fields that resolve to empty are omitted.
func (r haRegistries) resolveExtras(entityID string) map[string]any {
	out := map[string]any{}
	ent, ok := r.Entities[entityID]
	areaID := ""
	if ok {
		areaID = ent.AreaID
		if ent.Platform != "" {
			out["integration"] = ent.Platform
		}
		if dev, ok := r.Devices[ent.DeviceID]; ok {
			devName := dev.NameByUser
			if devName == "" {
				devName = dev.Name
			}
			if devName != "" {
				out["device"] = devName
			}
			if dev.Manufacturer != "" {
				out["manufacturer"] = dev.Manufacturer
			}
			if dev.Model != "" {
				out["model"] = dev.Model
			}
			if areaID == "" {
				// Entities inherit their device's area when none is set explicitly.
				areaID = dev.AreaID
			}
		}
	}
	if areaID != "" {
		if a, ok := r.Areas[areaID]; ok && a.Name != "" {
			out["area"] = a.Name
		}
	}
	return out
}

// isZigbee2MQTTDevice reports whether the given device looks like it was
// created by the Zigbee2MQTT MQTT-discovery integration. Detection is by:
//
//  1. an identifier of the form ["mqtt", "zigbee2mqtt_..."] (covers both
//     bridge devices "zigbee2mqtt_bridge_<ieee>" and child devices
//     "zigbee2mqtt_0x<ieee>"), OR
//  2. manufacturer = "Zigbee2MQTT" (the Z2M bridge device itself), OR
//  3. via_device_id resolves to a device matching one of the above.
//
// The `origin` field from MQTT discovery payloads is recorded internally by
// HA but is not exposed via the WS registry APIs, so we cannot rely on it.
func (r haRegistries) isZigbee2MQTTDevice(deviceID string) bool {
	dev, ok := r.Devices[deviceID]
	if !ok {
		return false
	}
	if deviceMatchesZ2M(dev) {
		return true
	}
	// Walk via_device_id once (Z2M child → bridge) to catch devices whose own
	// identifiers happen to be customised but that are still parented to the
	// Z2M bridge.
	if dev.ViaDeviceID != "" {
		if parent, ok := r.Devices[dev.ViaDeviceID]; ok {
			if deviceMatchesZ2M(parent) {
				return true
			}
		}
	}
	return false
}

func deviceMatchesZ2M(dev haDevice) bool {
	if strings.EqualFold(dev.Manufacturer, "Zigbee2MQTT") {
		return true
	}
	for _, id := range dev.Identifiers {
		if len(id) >= 2 && id[0] == "mqtt" && strings.HasPrefix(id[1], "zigbee2mqtt_") {
			return true
		}
	}
	return false
}

// shouldIgnore reports whether an entity_id should be filtered out based on
// the configured filter rules: by integration platform, by Zigbee2MQTT
// origin, or by an explicit list of entity_id prefixes.
//
// Entities for which no registry entry exists (rare; happens for some legacy
// entities) cannot be filtered by integration and pass through.
func (r haRegistries) shouldIgnore(entityID string, f entityFilter) bool {
	for _, prefix := range f.ignoreEntityPrefixes {
		if strings.HasPrefix(entityID, prefix) {
			return true
		}
	}
	ent, ok := r.Entities[entityID]
	if !ok {
		return false
	}
	if _, blocked := f.ignoreIntegrations[ent.Platform]; blocked {
		return true
	}
	if f.ignoreZigbee2MQTT && r.isZigbee2MQTTDevice(ent.DeviceID) {
		return true
	}
	return false
}

// entityFilter holds the resolved filter configuration.
type entityFilter struct {
	ignoreIntegrations   map[string]struct{} // platform names (mqtt, hue, zha, ...)
	ignoreZigbee2MQTT    bool
	ignoreEntityPrefixes []string // entity_id prefixes (e.g. "sensor.battery_")
}

// parseEntityFilter extracts the filter config from a plugin config map.
// All keys are optional; missing/empty fields disable that filter.
//
// Recognised keys:
//   - "ignore_integrations": []string of HA integration/platform names.
//   - "ignore_zigbee2mqtt": bool — true to skip entities backed by Z2M devices.
//   - "ignore_entity_prefixes": []string of entity_id prefixes.
func parseEntityFilter(cfg map[string]any) entityFilter {
	f := entityFilter{ignoreIntegrations: map[string]struct{}{}}
	for _, p := range stringList(cfg["ignore_integrations"]) {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			f.ignoreIntegrations[p] = struct{}{}
		}
	}
	if v, ok := cfg["ignore_zigbee2mqtt"].(bool); ok {
		f.ignoreZigbee2MQTT = v
	}
	for _, p := range stringList(cfg["ignore_entity_prefixes"]) {
		p = strings.TrimSpace(p)
		if p != "" {
			f.ignoreEntityPrefixes = append(f.ignoreEntityPrefixes, p)
		}
	}
	return f
}

// stringList coerces a config value to a string slice. Accepts []string,
// []any, or a comma-separated string. Anything else returns nil.
func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return strings.Split(t, ",")
	}
	return nil
}
