package zigbee2mqtt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// bridgeDevice mirrors the JSON entries in <base>/bridge/devices.
//
// Z2M publishes a retained JSON array of every paired device. We only care
// about a small subset of fields — friendly_name (used as topic), ieee_address
// (stable id), software_build_id (firmware), and the definition (model,
// vendor, exposes).
type bridgeDevice struct {
	IEEE            string      `json:"ieee_address"`
	FriendlyName    string      `json:"friendly_name"`
	Type            string      `json:"type"` // EndDevice / Router / Coordinator
	Supported       bool        `json:"supported"`
	Disabled        bool        `json:"disabled"`
	PowerSource     string      `json:"power_source"`
	SoftwareBuildID string      `json:"software_build_id"`
	Definition      *definition `json:"definition"`
}

type definition struct {
	Model       string   `json:"model"`
	Vendor      string   `json:"vendor"`
	Description string   `json:"description"`
	Exposes     []expose `json:"exposes"`
}

// expose describes a feature tree. For composite types like "light" or
// "switch" the inner Features list holds the actual properties (state,
// brightness, color_xy, …). For multi-endpoint devices each feature carries
// an Endpoint string and the Property name is auto-suffixed with that
// endpoint (e.g. `state_l1`). Numeric and binary feature types carry
// unit/range and on/off values respectively.
type expose struct {
	Type     string      `json:"type"`
	Name     string      `json:"name"`
	Property string      `json:"property"`
	Endpoint string      `json:"endpoint"`
	Access   int         `json:"access"`
	Unit     string      `json:"unit,omitempty"`
	ValueMin *float64    `json:"value_min,omitempty"`
	ValueMax *float64    `json:"value_max,omitempty"`
	ValueOn  any         `json:"value_on,omitempty"`
	ValueOff any         `json:"value_off,omitempty"`
	Values   jsonStrings `json:"values"`
	Features []expose    `json:"features"`
}

// jsonStrings is a JSON array that tolerates mixed element types (string,
// number, bool) by converting each element to its string representation.
// Z2M sometimes sends numeric values inside enum-typed exposes.
type jsonStrings []string

func (j *jsonStrings) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		var s string
		if err := json.Unmarshal(r, &s); err == nil {
			out = append(out, s)
			continue
		}
		// number, bool, or other scalar — convert via interface{}
		var v any
		if err := json.Unmarshal(r, &v); err != nil {
			continue
		}
		out = append(out, fmt.Sprintf("%v", v))
	}
	*j = out
	return nil
}

// endpoint represents one bridgeable entity extracted from a device's exposes.
type endpoint struct {
	// Endpoint name (e.g. "l1", "l2"); empty for single-endpoint devices.
	// For button entities split off an `action` enum, this is "btn" (single
	// button) or "btn_<prefix>" (one of several buttons on the same device).
	// For sensor-only devices this is ""; for devices with both light and
	// sensor endpoints the sensor endpoint uses "sensor".
	Name string
	// Kind is the vDC output kind: "light" (binary), "dimmer", "colorlight",
	// "button", "sensor", or "binaryInput".
	Kind string
	// HasBrightness / HasColor reflect the supported features.
	HasBrightness bool
	HasColor      bool
	// Property names (already endpoint-suffixed where applicable).
	StateProp      string // e.g. "state" or "state_l1"
	BrightnessProp string // e.g. "brightness"
	ColorProp      string // e.g. "color"
	// Button-only fields.
	ActionProp   string // e.g. "action" — Z2M property carrying the click event
	ActionPrefix string // prefix before the action verb, "" for single-button
	// Sensor/binary-input fields (for kind == "sensor" or "binaryInput").
	// Multiple sensor inputs are packed into a single vDC device (one sensorInput
	// per numeric expose, one binaryInput per binary expose).
	SensorFeatures []sensorFeature
	BinaryFeatures []binaryFeature
}

// sensorMeta holds the vDC descriptor fields for a z2m numeric expose.
// Uses the same field layout as bridge.SensorDescriptor for easy conversion.
type sensorMeta struct {
	Type       int
	Usage      int // VdcUsageHint: 0=undefined, 1=room, 2=outdoors, 3=user
	Name       string
	Min, Max   float64
	Resolution float64
	SIUnit     string
	Symbol     string
}

// sensorFeature represents one numeric sensor input within a sensor endpoint.
type sensorFeature struct {
	Property string
	Index    int
	Meta     sensorMeta
}

// binaryInputMeta holds default descriptor fields for a z2m binary expose.
type binaryInputMeta struct {
	// Function is the dS binaryInput sensorFunction enum value.
	// 0=none, 1=presence, 5=motion, 7=smoke, 13=window_closed, 14=door_closed.
	Function int
	// Invert when true means z2m true → vDC false (rare; contact=true=closed matches dS).
	Invert bool
}

// binaryFeature represents one binary input within a sensor endpoint.
type binaryFeature struct {
	Property string
	Index    int
	Function int
	Invert   bool
}

// endpointOpts controls which optional sensor exposes are included.
type endpointOpts struct {
	IncludeBattery     bool
	IncludeLinkquality bool
}

// z2mNumericMeta maps z2m numeric property names to vDC sensor metadata.
// Properties not in this table are still included with sensorType=0 and
// units/ranges derived from the z2m expose definition.
//
// The Usage field is critical: dSS maps (sensorType, sensorUsage) to a
// concrete dS sensorInput.type. For environmental room sensors we set
// Usage=1 (room) so e.g. temperature becomes dSS type 9 instead of 255 and
// shows up in the zone status column.
var z2mNumericMeta = map[string]sensorMeta{
	"temperature":     {Type: 1, Usage: 1, Name: "temperature", Min: -40, Max: 125, Resolution: 0.1, SIUnit: "celsius", Symbol: "°C"},
	"humidity":        {Type: 2, Usage: 1, Name: "humidity", Min: 0, Max: 100, Resolution: 0.1, SIUnit: "percent", Symbol: "%"},
	"illuminance":     {Type: 3, Usage: 1, Name: "illuminance", Min: 0, Max: 150000, Resolution: 1, SIUnit: "lux", Symbol: "lx"},
	"illuminance_lux": {Type: 3, Usage: 1, Name: "illuminance", Min: 0, Max: 150000, Resolution: 1, SIUnit: "lux", Symbol: "lx"},
	"voltage":         {Type: 4, Name: "voltage", Min: 0, Max: 500, Resolution: 0.1, SIUnit: "volt", Symbol: "V"},
	"co":              {Type: 5, Usage: 1, Name: "co", Min: 0, Max: 1000, Resolution: 1, SIUnit: "ppm", Symbol: "ppm"},
	"pm10":            {Type: 8, Usage: 1, Name: "pm10", Min: 0, Max: 1000, Resolution: 1, SIUnit: "microgram_per_cubicmeter", Symbol: "µg/m³"},
	"pm25":            {Type: 9, Usage: 1, Name: "pm2.5", Min: 0, Max: 1000, Resolution: 1, SIUnit: "microgram_per_cubicmeter", Symbol: "µg/m³"},
	"power":           {Type: 14, Name: "power", Min: 0, Max: 10000, Resolution: 1, SIUnit: "watt", Symbol: "W"},
	"current":         {Type: 15, Name: "current", Min: 0, Max: 100, Resolution: 0.1, SIUnit: "ampere", Symbol: "A"},
	"energy":          {Type: 16, Name: "energy", Min: 0, Max: 1e7, Resolution: 0.1, SIUnit: "kilowatt_hour", Symbol: "kWh"},
	"pressure":        {Type: 18, Usage: 1, Name: "pressure", Min: 800, Max: 1200, Resolution: 0.1, SIUnit: "hectopascal", Symbol: "hPa"},
	"co2":             {Type: 22, Usage: 1, Name: "co2", Min: 0, Max: 5000, Resolution: 1, SIUnit: "ppm", Symbol: "ppm"},
	"battery":         {Type: 32, Name: "battery", Min: 0, Max: 100, Resolution: 1, SIUnit: "percent", Symbol: "%"},
	"linkquality":     {Type: 0, Name: "linkquality", Min: 0, Max: 255, Resolution: 1, SIUnit: "", Symbol: "lqi"},
}

// z2mBinaryMeta maps z2m binary property names to dS binaryInput metadata.
var z2mBinaryMeta = map[string]binaryInputMeta{
	"occupancy":       {Function: 1},  // presence
	"presence":        {Function: 1},  // presence
	"motion":          {Function: 5},  // motion only
	"contact":         {Function: 13}, // window closed (contact=true means closed)
	"water_leak":      {Function: 0},  // no standard dS function
	"smoke":           {Function: 7},  // smoke alarm
	"vibration":       {Function: 0},
	"tamper":          {Function: 0},
	"sos":             {Function: 6}, // panic/security
	"gas":             {Function: 0},
	"carbon_monoxide": {Function: 0},
}

// endpointsWithOpts walks the device's exposes tree and returns one endpoint
// entry per bridgeable feature group. Light, switch and button endpoints are
// always included. Numeric/binary sensor endpoints are collected and merged
// into a single "sensor" kind endpoint at the end.
//
// The opts field controls whether optional battery / link-quality numeric
// exposes are included as sensor inputs.
func (d *bridgeDevice) endpointsWithOpts(opts endpointOpts) []endpoint {
	if d.Definition == nil {
		return nil
	}
	var out []endpoint
	var sensorFeats []sensorFeature
	var binaryFeats []binaryFeature
	sensorIdx := 0
	binaryIdx := 0

	for _, ex := range d.Definition.Exposes {
		switch ex.Type {
		case "light":
			ep := endpoint{Name: ex.Endpoint, Kind: "dimmer"}
			for _, f := range ex.Features {
				switch f.Name {
				case "state":
					ep.StateProp = pickProp(f, "state")
				case "brightness":
					ep.HasBrightness = true
					ep.BrightnessProp = pickProp(f, "brightness")
				case "color_xy", "color_hs":
					ep.HasColor = true
					ep.ColorProp = pickProp(f, "color")
				}
			}
			if ep.HasColor {
				ep.Kind = "colorlight"
			} else if !ep.HasBrightness {
				ep.Kind = "light" // light without brightness ≈ binary
			}
			if ep.StateProp == "" {
				ep.StateProp = withEP("state", ep.Name)
			}
			out = append(out, ep)
		case "switch":
			ep := endpoint{Name: ex.Endpoint, Kind: "light"}
			for _, f := range ex.Features {
				if f.Name == "state" {
					ep.StateProp = pickProp(f, "state")
				}
			}
			if ep.StateProp == "" {
				ep.StateProp = withEP("state", ep.Name)
			}
			out = append(out, ep)
		case "enum":
			if ex.Name != "action" || len(ex.Values) == 0 {
				continue
			}
			prop := ex.Property
			if prop == "" {
				prop = "action"
			}
			out = append(out, expandActionButtons(prop, ex.Values)...)
		case "numeric":
			prop := ex.Property
			if prop == "" {
				continue
			}
			if prop == "battery" && !opts.IncludeBattery {
				continue
			}
			if prop == "linkquality" && !opts.IncludeLinkquality {
				continue
			}
			meta := numericMetaForExpose(ex)
			sensorFeats = append(sensorFeats, sensorFeature{
				Property: prop,
				Index:    sensorIdx,
				Meta:     meta,
			})
			sensorIdx++
		case "binary":
			prop := ex.Property
			if prop == "" {
				continue
			}
			bm := z2mBinaryMeta[prop]
			binaryFeats = append(binaryFeats, binaryFeature{
				Property: prop,
				Index:    binaryIdx,
				Function: bm.Function,
				Invert:   bm.Invert,
			})
			binaryIdx++
		}
	}

	// Merge all sensor/binary features into a single endpoint.
	if len(sensorFeats) > 0 || len(binaryFeats) > 0 {
		kind := "sensor"
		if len(sensorFeats) == 0 {
			kind = "binaryInput"
		}
		// If device also has light/switch/button endpoints, give the sensor
		// endpoint a distinct name so entity IDs don't collide.
		name := ""
		if len(out) > 0 {
			name = "sensor"
		}
		out = append(out, endpoint{
			Name:           name,
			Kind:           kind,
			SensorFeatures: sensorFeats,
			BinaryFeatures: binaryFeats,
		})
	}

	return out
}

// numericMetaForExpose returns the vDC sensor metadata for a numeric expose.
// For known properties it returns hardcoded metadata from z2mNumericMeta;
// for unknown properties it derives best-effort values from the expose itself.
func numericMetaForExpose(ex expose) sensorMeta {
	if m, ok := z2mNumericMeta[ex.Property]; ok {
		return m
	}
	// Unknown property: use whatever z2m tells us.
	m := sensorMeta{Type: 0, Name: ex.Property, Symbol: ex.Unit}
	if ex.ValueMin != nil {
		m.Min = *ex.ValueMin
	}
	if ex.ValueMax != nil {
		m.Max = *ex.ValueMax
	}
	if m.Max > m.Min {
		m.Resolution = (m.Max - m.Min) / 1000
	}
	return m
}

// expandActionButtons turns a Z2M `action` enum into one endpoint per
// distinct button. Action values that don't match a known click suffix
// (e.g. "single", "double", "hold", "..._click", "..._press_release") are
// silently skipped — they don't represent a press we can forward.
func expandActionButtons(prop string, values []string) []endpoint {
	prefixes := make(map[string]struct{})
	for _, v := range values {
		_, suffix := splitButtonAction(v)
		if suffix == "" {
			continue
		}
		prefix, _ := splitButtonAction(v)
		prefixes[prefix] = struct{}{}
	}
	if len(prefixes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(prefixes))
	for p := range prefixes {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	out := make([]endpoint, 0, len(keys))
	for _, p := range keys {
		name := "btn"
		if p != "" {
			name = "btn_" + p
		}
		out = append(out, endpoint{
			Name:         name,
			Kind:         "button",
			ActionProp:   prop,
			ActionPrefix: p,
		})
	}
	return out
}

// buttonActionSuffixes are the Z2M action verbs we recognize. Order matters:
// multi-word suffixes (e.g. "press_release") must be matched before their
// single-word prefixes ("press") to avoid mis-grouping a button id.
var buttonActionSuffixes = []string{
	"press_release", "hold_release", "long_release",
	"single", "double", "triple", "quadruple",
	"click", "press", "release", "long", "hold",
}

// splitButtonAction splits a Z2M action enum value into a (button-id prefix,
// click-verb suffix). Returns ("", "") if no known verb is present.
//
// Examples:
//
//	"single"            -> ("",           "single")
//	"arrow_left_click"  -> ("arrow_left", "click")
//	"on_press_release"  -> ("on",         "press_release")
//	"unknown_thing"     -> ("",           "")
func splitButtonAction(v string) (string, string) {
	for _, s := range buttonActionSuffixes {
		if v == s {
			return "", s
		}
		if strings.HasSuffix(v, "_"+s) {
			return strings.TrimSuffix(v, "_"+s), s
		}
	}
	return "", ""
}

// mapActionSuffix maps a Z2M click verb to a digitalSTROM-style action.
// Returns "" for verbs we don't recognize.
//
// The vocabulary mirrors vdcd's `ButtonBehaviour::clickTypeName` so the same
// strings can be parsed downstream:
//   - tip_1x .. tip_4x  ← single/multi tip
//   - hold              ← ct_hold_start (begin sustained press)
//   - release           ← ct_hold_end   (end of sustained press)
func mapActionSuffix(suffix string) string {
	switch suffix {
	case "single", "click", "press":
		return "tip"
	case "double":
		return "tip2"
	case "triple":
		return "tip3"
	case "quadruple":
		return "tip4"
	case "long", "hold":
		return "hold"
	case "release", "hold_release", "long_release", "press_release":
		return "release"
	default:
		return ""
	}
}

// pickProp returns the explicit Property name if z2m provided one, else
// composes "<base>_<endpoint>" — matching z2m's naming convention.
func pickProp(f expose, base string) string {
	if f.Property != "" {
		return f.Property
	}
	return withEP(base, f.Endpoint)
}

func withEP(base, ep string) string {
	if ep == "" {
		return base
	}
	return base + "_" + ep
}

// entityID is the vDC remote entity id for a given endpoint of this device.
// Single-endpoint devices use the IEEE alone; multi-endpoint devices append
// the endpoint name (e.g. "0x00158d0001234567:l1").
func (d *bridgeDevice) entityID(ep endpoint) string {
	if ep.Name == "" {
		return d.IEEE
	}
	return d.IEEE + ":" + ep.Name
}

// displayName returns a readable name for the given endpoint.
func (d *bridgeDevice) displayName(ep endpoint) string {
	name := d.FriendlyName
	if name == "" {
		name = d.IEEE
	}
	if ep.Kind == "button" {
		if ep.ActionPrefix == "" {
			return name + " · button"
		}
		return name + " · " + ep.ActionPrefix
	}
	if ep.Kind == "sensor" || ep.Kind == "binaryInput" {
		if ep.Name == "" {
			return name
		}
		return name + " · sensors"
	}
	if ep.Name != "" {
		name = name + " (" + ep.Name + ")"
	}
	return name
}

// stateTopic returns "<base>/<friendly_name>".
func (d *bridgeDevice) stateTopic(base string) string {
	return base + "/" + d.FriendlyName
}

// setTopic returns "<base>/<friendly_name>/set".
func (d *bridgeDevice) setTopic(base string) string {
	return base + "/" + d.FriendlyName + "/set"
}

// availabilityTopic returns "<base>/<friendly_name>/availability".
func (d *bridgeDevice) availabilityTopic(base string) string {
	return base + "/" + d.FriendlyName + "/availability"
}

// parseEntityID splits a vDC remote entity id back into IEEE + endpoint name.
// Returns ("", "") if the id is empty.
func parseEntityID(id string) (ieee, endpointName string) {
	if id == "" {
		return "", ""
	}
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}
