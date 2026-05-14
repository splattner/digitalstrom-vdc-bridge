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
				if a := strings.TrimSpace(action); a != "" {
					state["action"] = a
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
