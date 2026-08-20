package vdcapi

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var errSetPropertyTargetRequired = errors.New("setProperty target must be a device dSUID")

// inputSettingWrite represents a write to buttonInputSettings, binaryInputSettings,
// or sensorSettings.
type inputSettingWrite struct {
	Family   string // "button", "binaryinput", or "sensor"
	Index    int
	Field    string // setting field name; "" means a recognised family but ignored sub-field
	BoolVal  bool
	IntVal   int
	FloatVal float64
}

// parseInputSettingPathJSON extracts an input setting write from JSON setProperty params.
// Handles path form {"name": "buttonInputSettings/0/setsLocalPriority", "value": true}
// and nested form {"properties": {"buttonInputSettings": {"0": {"setsLocalPriority": true}}}}.
func parseInputSettingPathJSON(params map[string]any) (inputSettingWrite, bool) {
	if name, ok := params["name"].(string); ok {
		if isw, ok := parseInputSettingPath(strings.TrimSpace(name), params["value"]); ok {
			return isw, true
		}
	}
	for _, top := range []any{params["properties"], params} {
		if top == nil {
			continue
		}
		m, ok := top.(map[string]any)
		if !ok {
			continue
		}
		for _, family := range []string{"buttonInputSettings", "binaryInputSettings", "sensorSettings"} {
			if raw, ok := m[family].(map[string]any); ok {
				for idxStr, entryRaw := range raw {
					idx, err := strconv.Atoi(idxStr)
					if err != nil {
						continue
					}
					entry, ok := entryRaw.(map[string]any)
					if !ok {
						continue
					}
					for field, val := range entry {
						if isw, ok := makeInputSettingWrite(family, idx, field, val); ok {
							return isw, true
						}
					}
				}
			}
		}
	}
	return inputSettingWrite{}, false
}

// parseInputSettingPath parses a path like "buttonInputSettings/0/setsLocalPriority".
func parseInputSettingPath(path string, rawVal any) (inputSettingWrite, bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return inputSettingWrite{}, false
	}
	family := strings.ToLower(parts[0])
	if family != "buttoninputsettings" && family != "binaryinputsettings" && family != "sensorsettings" {
		return inputSettingWrite{}, false
	}
	// Normalise family name back to canonical form
	switch family {
	case "buttoninputsettings":
		family = "button"
	case "binaryinputsettings":
		family = "binaryinput"
	case "sensorsettings":
		family = "sensor"
	}
	idx, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return inputSettingWrite{}, false
	}
	return makeInputSettingWrite(family, idx, parts[2], rawVal)
}

func makeInputSettingWrite(family string, idx int, field string, rawVal any) (inputSettingWrite, bool) {
	// Normalise family from map key form
	normalFamily := strings.ToLower(family)
	switch normalFamily {
	case "buttoninputsettings":
		normalFamily = "button"
	case "binaryinputsettings":
		normalFamily = "binaryinput"
	case "sensorsettings":
		normalFamily = "sensor"
	}
	fieldLow := strings.ToLower(strings.TrimSpace(field))
	switch {
	case normalFamily == "button" && fieldLow == "setslocalpriority":
		return inputSettingWrite{Family: "button", Index: idx, Field: "setsLocalPriority", BoolVal: asBool(rawVal)}, true
	case normalFamily == "button" && fieldLow == "callspresent":
		return inputSettingWrite{Family: "button", Index: idx, Field: "callsPresent", BoolVal: asBool(rawVal)}, true
	case normalFamily == "button" && (fieldLow == "group" || fieldLow == "mode" || fieldLow == "function"):
		n := 0
		if f, ok := asFloat(rawVal); ok {
			n = int(f)
		}
		canonical := map[string]string{"group": "buttonGroup", "mode": "buttonMode", "function": "buttonFunction"}[fieldLow]
		return inputSettingWrite{Family: "button", Index: idx, Field: canonical, IntVal: n}, true
	case normalFamily == "binaryinput" && fieldLow == "sensorfunction":
		fn := 0
		if f, ok := asFloat(rawVal); ok {
			fn = int(f)
		}
		return inputSettingWrite{Family: "binaryinput", Index: idx, Field: "sensorFunction", IntVal: fn}, true
	case normalFamily == "binaryinput" && fieldLow == "group":
		n := 0
		if f, ok := asFloat(rawVal); ok {
			n = int(f)
		}
		return inputSettingWrite{Family: "binaryinput", Index: idx, Field: "binaryInputGroup", IntVal: n}, true
	case normalFamily == "sensor":
		switch fieldLow {
		case "group", "function", "channel":
			n := 0
			if f, ok := asFloat(rawVal); ok {
				n = int(f)
			}
			return inputSettingWrite{Family: "sensor", Index: idx, Field: fieldLow, IntVal: n}, true
		case "minpushinterval":
			if f, ok := asFloat(rawVal); ok {
				return inputSettingWrite{Family: "sensor", Index: idx, Field: "minPushInterval", FloatVal: f}, true
			}
		case "changesonlyinterval":
			if f, ok := asFloat(rawVal); ok {
				return inputSettingWrite{Family: "sensor", Index: idx, Field: "changesOnlyInterval", FloatVal: f}, true
			}
		}
		// Other sensor settings sub-fields — accepted silently.
		return inputSettingWrite{Family: "sensor", Index: idx, Field: ""}, true
	case normalFamily == "button" || normalFamily == "binaryinput":
		// Other settings fields (mode, function, group, …) — accepted silently
		return inputSettingWrite{Family: normalFamily, Index: idx, Field: ""}, true
	}
	return inputSettingWrite{}, false
}

// Commander routes control commands from vDC API to external devices.
type Commander interface {
	SetLightLevel(uniqueID string, value float64) error
	// SetChannelValue writes the given value to the given channel index of the
	// addressed device. channelIndex 0 is the primary (brightness/position)
	// channel; higher indices are output-kind specific (e.g. hue/sat for
	// colorlight, tilt for movinglight).
	SetChannelValue(uniqueID string, channelIndex int, value float64) error
}

// SceneCommander is an optional Commander capability for devices that can
// recall a native remote scene/preset directly, instead of a computed
// channel-value fallback. Tried first by the scene-call fallback path in
// method_service.go; any error (including "not supported by this device")
// falls through to the existing brightness-level behavior, so implementing
// this is purely additive.
type SceneCommander interface {
	CallScene(uniqueID string, scene int) error
}

// applyColorChannelValue dispatches a channel write via the generic
// SetChannelValue interface method.
func applyColorChannelValue(commander Commander, uniqueID string, channelIndex int, value float64) error {
	return commander.SetChannelValue(uniqueID, channelIndex, value)
}

func resolveDeviceByDSUID(vdcDSUID, target string, snapshot ExternalSnapshot) (ExternalDeviceState, bool) {
	for key, d := range snapshot.Devices {
		dsuid := deviceDSUID(vdcDSUID, d, key)
		if strings.EqualFold(dsuid, target) {
			return d, true
		}
	}
	return ExternalDeviceState{}, false
}

func extractBrightnessValueFromJSON(params map[string]any) (float64, bool) {
	if p, ok := params["properties"].(map[string]any); ok {
		if v, ok := extractBrightnessFromMap(p); ok {
			return v, true
		}
	}
	if v, ok := extractBrightnessFromMap(params); ok {
		return v, true
	}
	if name, _ := params["name"].(string); strings.TrimSpace(name) != "" {
		if strings.EqualFold(strings.TrimSpace(name), "channelStates/0/value") || strings.EqualFold(strings.TrimSpace(name), "brightness") {
			if f, ok := asFloat(params["value"]); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func extractBrightnessFromMap(m map[string]any) (float64, bool) {
	if cs, ok := m["channelStates"].(map[string]any); ok {
		if c0, ok := cs["0"].(map[string]any); ok {
			if f, ok := asFloat(c0["value"]); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func validateLightControlTarget(target, vdcDSUID string) error {
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, vdcDSUID) {
		return fmt.Errorf("%w", errSetPropertyTargetRequired)
	}
	return nil
}

func isLightOutput(output string) bool {
	output = strings.TrimSpace(output)
	return output == "" ||
		strings.EqualFold(output, "light") ||
		strings.EqualFold(output, "colorlight") ||
		strings.EqualFold(output, "movinglight")
}

// sceneWrite captures a single scene property write decoded from setProperty.
type sceneWrite struct {
	SceneNum   int
	ChannelIdx int // -1 for scene-level (non-channel) fields
	Field      string
	FloatVal   float64
	BoolVal    bool
	IntVal     int
}

// channelStateWrite captures a single channelStates.{idx}.value write.
type channelStateWrite struct {
	ChannelIdx int
	Value      float64
}

// parseChannelStatesPathJSON extracts a channelStateWrite from a JSON setProperty
// params map. Handles the path form {"name": "channelStates/N/value", "value": X}
// and the nested form {"properties": {"channelStates": {"N": {"value": X}}}}.
// Returns (_, false) if the params do not encode a channelStates write.
func parseChannelStatesPathJSON(params map[string]any) (channelStateWrite, bool) {
	if name, ok := params["name"].(string); ok {
		parts := strings.Split(strings.TrimSpace(name), "/")
		if len(parts) >= 3 && strings.EqualFold(parts[0], "channelStates") {
			if idx, err := strconv.Atoi(parts[1]); err == nil && strings.EqualFold(parts[2], "value") {
				if f, ok := asFloat(params["value"]); ok {
					return channelStateWrite{ChannelIdx: idx, Value: f}, true
				}
			}
		}
	}
	for _, top := range []any{params["properties"], params} {
		m, ok := top.(map[string]any)
		if !ok {
			continue
		}
		csRaw, ok := m["channelStates"].(map[string]any)
		if !ok {
			continue
		}
		for idxStr, child := range csRaw {
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				continue
			}
			cm, ok := child.(map[string]any)
			if !ok {
				continue
			}
			if f, ok := asFloat(cm["value"]); ok {
				return channelStateWrite{ChannelIdx: idx, Value: f}, true
			}
		}
	}
	return channelStateWrite{}, false
}

// parseScenePathJSON extracts a sceneWrite from a JSON setProperty params map.
// Handles the path form {"name": "scenes/5/channels/0/value", "value": X}
// and the nested form {"properties": {"scenes": {"5": {...}}}}.
func parseScenePathJSON(params map[string]any) (sceneWrite, bool) {
	// Path form
	if name, ok := params["name"].(string); ok {
		name = strings.TrimSpace(name)
		if sw, ok := parseScenePath(name, params["value"]); ok {
			return sw, true
		}
	}
	// Nested form — look in "properties" or directly in params
	for _, top := range []any{params["properties"], params} {
		if top == nil {
			continue
		}
		m, ok := top.(map[string]any)
		if !ok {
			continue
		}
		if scenesRaw, ok := m["scenes"].(map[string]any); ok {
			if sw, ok := extractSceneWriteFromNestedMap(scenesRaw); ok {
				return sw, true
			}
		}
	}
	return sceneWrite{}, false
}

// parseScenePath parses a path like "scenes/5/channels/0/value" and the raw value.
func parseScenePath(path string, rawVal any) (sceneWrite, bool) {
	parts := strings.Split(path, "/")
	if len(parts) < 3 || !strings.EqualFold(parts[0], "scenes") {
		return sceneWrite{}, false
	}
	sceneNum, err := strconv.Atoi(parts[1])
	if err != nil {
		return sceneWrite{}, false
	}
	switch {
	case strings.EqualFold(parts[2], "channels") && len(parts) >= 5:
		chIdx, err := strconv.Atoi(parts[3])
		if err != nil {
			return sceneWrite{}, false
		}
		switch strings.ToLower(parts[4]) {
		case "value":
			if f, ok := asFloat(rawVal); ok {
				return sceneWrite{SceneNum: sceneNum, ChannelIdx: chIdx, Field: "channelValue", FloatVal: f}, true
			}
		case "dontcare":
			return sceneWrite{SceneNum: sceneNum, ChannelIdx: chIdx, Field: "channelDontCare", BoolVal: asBool(rawVal)}, true
		}
	case strings.EqualFold(parts[2], "effect"):
		if f, ok := asFloat(rawVal); ok {
			return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "effect", IntVal: int(f)}, true
		}
	case strings.EqualFold(parts[2], "dontCare"), strings.EqualFold(parts[2], "dontcare"):
		return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "dontCare", BoolVal: asBool(rawVal)}, true
	case strings.EqualFold(parts[2], "ignoreLocalPriority"):
		return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "ignoreLocalPriority", BoolVal: asBool(rawVal)}, true
	}
	return sceneWrite{}, false
}

// extractSceneWriteFromNestedMap extracts a write from {"5": {"channels": {"0": {"value": X}}}} form.
func extractSceneWriteFromNestedMap(scenesMap map[string]any) (sceneWrite, bool) {
	for numStr, sceneRaw := range scenesMap {
		sceneNum, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		sceneMap, ok := sceneRaw.(map[string]any)
		if !ok {
			continue
		}
		// channels sub-map
		if chsRaw, ok := sceneMap["channels"].(map[string]any); ok {
			for chStr, chRaw := range chsRaw {
				chIdx, err := strconv.Atoi(chStr)
				if err != nil {
					continue
				}
				chMap, ok := chRaw.(map[string]any)
				if !ok {
					continue
				}
				if v, ok := chMap["value"]; ok {
					if f, ok := asFloat(v); ok {
						return sceneWrite{SceneNum: sceneNum, ChannelIdx: chIdx, Field: "channelValue", FloatVal: f}, true
					}
				}
				if v, ok := chMap["dontCare"]; ok {
					return sceneWrite{SceneNum: sceneNum, ChannelIdx: chIdx, Field: "channelDontCare", BoolVal: asBool(v)}, true
				}
			}
		}
		if v, ok := sceneMap["effect"]; ok {
			if f, ok := asFloat(v); ok {
				return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "effect", IntVal: int(f)}, true
			}
		}
		if v, ok := sceneMap["dontCare"]; ok {
			return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "dontCare", BoolVal: asBool(v)}, true
		}
		if v, ok := sceneMap["ignoreLocalPriority"]; ok {
			return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "ignoreLocalPriority", BoolVal: asBool(v)}, true
		}
	}
	return sceneWrite{}, false
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true") || x == "1"
	}
	return false
}
