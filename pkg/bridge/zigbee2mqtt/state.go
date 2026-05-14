package zigbee2mqtt

import (
	"encoding/json"
	"strings"
)

// stateMessage is a generic decoder for "<base>/<friendly_name>" payloads.
//
// We unmarshal into a map so multi-endpoint devices (state_l1, brightness_l2)
// work without a hand-written struct per device.
type stateMessage map[string]any

// stringField returns the string value at key, or "" if missing/wrong type.
func (m stateMessage) stringField(key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

// numberField returns the float64 value at key, or 0/false if missing.
func (m stateMessage) numberField(key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// colorField returns the color sub-object (or nil) at key.
func (m stateMessage) colorField(key string) map[string]any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	if c, ok := v.(map[string]any); ok {
		return c
	}
	return nil
}

// briToVDC converts z2m's 0..255 brightness to a 0..100 vDC channel value.
// If the device is off, returns 0 regardless.
func briToVDC(bri float64, on bool) float64 {
	if !on {
		return 0
	}
	v := bri * 100.0 / 255.0
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// vdcToBri converts a 0..100 vDC channel value to z2m's 1..254 range.
func vdcToBri(pct float64) int {
	if pct <= 0 {
		return 1
	}
	v := int(pct*254.0/100.0 + 0.5)
	if v < 1 {
		v = 1
	}
	if v > 254 {
		v = 254
	}
	return v
}

// stateOn reports whether a "state" string (e.g. "ON", "OFF", "TOGGLE") is on.
func stateOn(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "ON")
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
