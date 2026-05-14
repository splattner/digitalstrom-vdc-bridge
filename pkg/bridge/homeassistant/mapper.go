package homeassistant

import (
	"strconv"
	"strings"
)

// classifyEntity returns the bridge kind (light, dimmer, colorlight, sensor,
// binary, "") for an HA entity, or "" if it isn't supported in v1.
//
// Rules (v1, lights + sensors only):
//   - light.* with `supported_color_modes` containing any of {hs, rgb, rgbw, rgbww, xy} → "colorlight"
//   - light.* with `brightness` in supported_color_modes (or attributes) → "dimmer"
//   - light.* otherwise → "light" (relay/on-off)
//   - sensor.* with a known numeric device_class → "sensor"
//   - everything else → "" (filtered out of discoverable list)
func classifyEntity(e haEntity) string {
	domain, _, ok := splitEntityID(e.EntityID)
	if !ok {
		return ""
	}
	switch domain {
	case "light":
		return classifyLight(e)
	case "sensor":
		if isNumericSensor(e) {
			return "sensor"
		}
		return ""
	default:
		return ""
	}
}

func classifyLight(e haEntity) string {
	modes := stringSliceAttr(e.Attributes, "supported_color_modes")
	for _, m := range modes {
		switch strings.ToLower(m) {
		case "hs", "rgb", "rgbw", "rgbww", "xy":
			return "colorlight"
		}
	}
	for _, m := range modes {
		if strings.EqualFold(m, "brightness") || strings.EqualFold(m, "color_temp") {
			return "dimmer"
		}
	}
	// Some integrations don't report supported_color_modes but expose brightness.
	if _, hasBrightness := e.Attributes["brightness"]; hasBrightness {
		return "dimmer"
	}
	return "light"
}

// supportedSensorClasses are the HA sensor device_class values exposed as vDC sensors.
var supportedSensorClasses = map[string]struct{}{
	"temperature": {},
	"humidity":    {},
	"illuminance": {},
	"power":       {},
	"energy":      {},
	"voltage":     {},
	"current":     {},
	"battery":     {},
	"pressure":    {},
	"co2":         {},
	"pm25":        {},
	"pm10":        {},
}

// vdcSensorMeta describes a sensor's vDC type, range and unit.
type vdcSensorMeta struct {
	Type       int
	Name       string
	Min, Max   float64
	Resolution float64
	SIUnit     string
	Symbol     string
}

// sensorMetaFor returns vDC sensor metadata for the given HA entity, derived
// from its device_class. Returns (meta, true) if the class is mapped.
// VdcSensorType values come from p44vdc/vdc_common/dsdefs.h (sensorType_*).
func sensorMetaFor(e haEntity) (vdcSensorMeta, bool) {
	dc, _ := e.Attributes["device_class"].(string)
	switch strings.ToLower(strings.TrimSpace(dc)) {
	case "temperature":
		return vdcSensorMeta{Type: 1, Name: "temperature", Min: -40, Max: 125, Resolution: 0.1, SIUnit: "celsius", Symbol: "°C"}, true
	case "humidity":
		return vdcSensorMeta{Type: 2, Name: "humidity", Min: 0, Max: 100, Resolution: 0.1, SIUnit: "percent", Symbol: "%"}, true
	case "illuminance":
		return vdcSensorMeta{Type: 3, Name: "illuminance", Min: 0, Max: 150000, Resolution: 1, SIUnit: "lux", Symbol: "lx"}, true
	case "voltage":
		return vdcSensorMeta{Type: 4, Name: "voltage", Min: 0, Max: 500, Resolution: 0.1, SIUnit: "volt", Symbol: "V"}, true
	case "co":
		return vdcSensorMeta{Type: 5, Name: "co", Min: 0, Max: 1000, Resolution: 1, SIUnit: "ppm", Symbol: "ppm"}, true
	case "pm10":
		return vdcSensorMeta{Type: 8, Name: "pm10", Min: 0, Max: 1000, Resolution: 1, SIUnit: "microgram_per_cubicmeter", Symbol: "µg/m³"}, true
	case "pm25":
		return vdcSensorMeta{Type: 9, Name: "pm2.5", Min: 0, Max: 1000, Resolution: 1, SIUnit: "microgram_per_cubicmeter", Symbol: "µg/m³"}, true
	case "power":
		return vdcSensorMeta{Type: 14, Name: "power", Min: 0, Max: 10000, Resolution: 1, SIUnit: "watt", Symbol: "W"}, true
	case "current":
		return vdcSensorMeta{Type: 15, Name: "current", Min: 0, Max: 100, Resolution: 0.1, SIUnit: "ampere", Symbol: "A"}, true
	case "energy":
		return vdcSensorMeta{Type: 16, Name: "energy", Min: 0, Max: 1e7, Resolution: 0.1, SIUnit: "kilowatt_hour", Symbol: "kWh"}, true
	case "pressure":
		return vdcSensorMeta{Type: 18, Name: "pressure", Min: 800, Max: 1200, Resolution: 0.1, SIUnit: "hectopascal", Symbol: "hPa"}, true
	case "co2":
		return vdcSensorMeta{Type: 22, Name: "co2", Min: 0, Max: 5000, Resolution: 1, SIUnit: "ppm", Symbol: "ppm"}, true
	case "battery":
		// Use sensorType_percent (32) for battery percentage.
		return vdcSensorMeta{Type: 32, Name: "battery", Min: 0, Max: 100, Resolution: 1, SIUnit: "percent", Symbol: "%"}, true
	}
	return vdcSensorMeta{}, false
}

func isNumericSensor(e haEntity) bool {
	dc, _ := e.Attributes["device_class"].(string)
	if dc == "" {
		return false
	}
	_, ok := supportedSensorClasses[strings.ToLower(dc)]
	return ok
}

// channelValueFromState converts an HA entity state to a vDC channel value
// (brightness 0..100). Returns (value, true) if applicable.
func brightnessFromState(e haEntity) (float64, bool) {
	if e.State == "off" {
		return 0, true
	}
	if e.State != "on" {
		return 0, false
	}
	if v, ok := e.Attributes["brightness"]; ok {
		if f, ok := numericValue(v); ok {
			// HA brightness is 0..255; vDC dimmer is 0..100.
			return clamp(f/255.0*100.0, 0, 100), true
		}
	}
	// On/off light: on = 100.
	return 100, true
}

// sensorValueFromState extracts a numeric sensor value from the entity state.
func sensorValueFromState(e haEntity) (float64, bool) {
	return numericValue(e.State)
}

// hueSatFromState extracts hue (0..360) and saturation (0..100) from an HA
// light entity's `hs_color` attribute. Returns (hue, sat, true) if present.
func hueSatFromState(e haEntity) (float64, float64, bool) {
	if e.State != "on" {
		return 0, 0, false
	}
	raw, ok := e.Attributes["hs_color"].([]any)
	if !ok || len(raw) < 2 {
		return 0, 0, false
	}
	h, hOK := numericValue(raw[0])
	s, sOK := numericValue(raw[1])
	if !hOK || !sOK {
		return 0, 0, false
	}
	return clamp(h, 0, 360), clamp(s, 0, 100), true
}

// colorTempMiredFromState extracts the color temperature in mired from an HA
// light entity's `color_temp` attribute (HA reports mired directly).
func colorTempMiredFromState(e haEntity) (float64, bool) {
	if e.State != "on" {
		return 0, false
	}
	if v, ok := e.Attributes["color_temp"]; ok {
		if f, ok := numericValue(v); ok {
			return f, true
		}
	}
	return 0, false
}

func splitEntityID(id string) (domain, name string, ok bool) {
	i := strings.IndexByte(id, '.')
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

func stringSliceAttr(attrs map[string]any, key string) []string {
	if attrs == nil {
		return nil
	}
	v, ok := attrs[key]
	if !ok {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" || s == "unknown" || s == "unavailable" {
			return 0, false
		}
		// Avoid pulling in strconv just here? We'll use it.
		f, ok := parseFloat(s)
		return f, ok
	}
	return 0, false
}

func parseFloat(s string) (float64, bool) {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v, true
	}
	return 0, false
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// friendlyName returns the entity's friendly_name or falls back to entity_id.
func friendlyName(e haEntity) string {
	if name, ok := e.Attributes["friendly_name"].(string); ok && name != "" {
		return name
	}
	return e.EntityID
}
