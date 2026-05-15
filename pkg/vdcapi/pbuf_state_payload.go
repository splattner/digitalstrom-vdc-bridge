package vdcapi

import (
	"strconv"
	"strings"
)

func changedStatePayload(d ExternalDeviceState) map[string]any {
	channelStates := map[string]any{"0": map[string]any{"value": channelValue(d, 0)}}
	if len(d.Channels) > 0 {
		for idx, value := range d.Channels {
			channelStates[strconv.Itoa(idx)] = map[string]any{"value": value}
		}
	}
	changed := map[string]any{
		"active":        d.Active,
		"channelStates": channelStates,
		"deviceevents":  buildDeviceEvents(d),
	}
	if bs := indexedFloatState(d.Buttons); len(bs) > 0 {
		for idx, action := range d.ButtonActions {
			if state, ok := bs[strconv.Itoa(idx)].(map[string]any); ok {
				if ct, ok := dsClickType(strings.TrimSpace(action)); ok {
					state["clickType"] = ct
				}
			}
		}
		changed["buttonInputStates"] = bs
	}
	if is := indexedFloatState(d.Inputs); len(is) > 0 {
		changed["binaryInputStates"] = is
	}
	if ss := indexedFloatState(d.Sensors); len(ss) > 0 {
		changed["sensorStates"] = ss
	}
	return changed
}

func indexedFloatState(values map[int]float64) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for idx, value := range values {
		out[strconv.Itoa(idx)] = map[string]any{"value": value}
	}
	return out
}

// dsClickType maps a vdcd-style click action name to the DsClickType integer
// expected by dSS's proxy device (proxydevice.cpp "clickType" property).
// Returns (value, true) when a valid mapping exists; (0, false) for unknown/empty actions.
func dsClickType(action string) (int, bool) {
	switch action {
	case "tip", "tip_1x":
		return 0, true // ct_tip_1x
	case "tip2", "tip_2x":
		return 1, true // ct_tip_2x
	case "tip3", "tip_3x":
		return 2, true // ct_tip_3x
	case "tip4", "tip_4x":
		return 3, true // ct_tip_4x
	case "hold":
		return 4, true // ct_hold_start
	case "release":
		return 6, true // ct_hold_end
	default:
		return 0, false
	}
}
