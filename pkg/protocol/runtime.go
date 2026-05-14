package protocol

import "fmt"

func ValidateDeviceMessageJSON(msg string, obj map[string]any) error {
	switch msg {
	case "channel":
		if !hasAny(obj, "index", "id", "type") {
			return fmt.Errorf("missing 'id', 'index' or 'type'")
		}
		if !hasKey(obj, "value") {
			return fmt.Errorf("missing 'value'")
		}
	case "channel_progress", "channel_config":
		if !hasAny(obj, "index", "id", "type") {
			return fmt.Errorf("missing 'id', 'index' or 'type'")
		}
		if !hasKey(obj, "value") && msg != "channel_config" {
			return fmt.Errorf("missing 'value'")
		}
	case "button", "input", "sensor":
		if !hasAny(obj, "index", "id") {
			return fmt.Errorf("missing 'id' or 'index'")
		}
		if !hasKey(obj, "value") {
			return fmt.Errorf("missing 'value'")
		}
	case "sync", "synced", "bye":
		return nil
	case "active":
		if hasKey(obj, "value") {
			if !isBoolLike(obj["value"]) {
				return fmt.Errorf("'value' must be boolean-like")
			}
		}
	case "opstate":
		if lvl, ok := obj["level"]; ok && !isNumber(lvl) {
			return fmt.Errorf("'level' must be numeric")
		}
		if txt, ok := obj["text"]; ok {
			if _, ok := txt.(string); !ok {
				return fmt.Errorf("'text' must be string")
			}
		}
	case "confirmAction":
		if !hasString(obj, "action") {
			return fmt.Errorf("confirmAction must identify 'action'")
		}
	case "updateProperty":
		if !hasString(obj, "property") {
			return fmt.Errorf("updateProperty must identify 'property'")
		}
		if !hasKey(obj, "value") && !hasKey(obj, "push") {
			return fmt.Errorf("updateProperty needs 'value' and/or 'push'")
		}
	case "pushNotification":
		if !hasKey(obj, "statechange") && !hasKey(obj, "events") {
			return fmt.Errorf("pushNotification needs 'statechange' and/or 'events'")
		}
		if sc, ok := obj["statechange"]; ok {
			if _, ok := sc.(map[string]any); !ok {
				return fmt.Errorf("'statechange' must be an object")
			}
		}
		if ev, ok := obj["events"]; ok {
			if _, ok := ev.([]any); !ok {
				return fmt.Errorf("'events' must be an array")
			}
		}
	case "dynamicAction":
		if !hasKey(obj, "changes") {
			return fmt.Errorf("dynamicAction needs 'changes'")
		}
		if _, ok := obj["changes"].(map[string]any); !ok {
			return fmt.Errorf("'changes' must be an object")
		}
	default:
		return fmt.Errorf("unknown message %q", msg)
	}
	return nil
}

func hasKey(obj map[string]any, key string) bool {
	_, ok := obj[key]
	return ok
}

func hasAny(obj map[string]any, keys ...string) bool {
	for _, k := range keys {
		if hasKey(obj, k) {
			return true
		}
	}
	return false
}

func hasString(obj map[string]any, key string) bool {
	v, ok := obj[key]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && s != ""
}

func isNumber(v any) bool {
	_, ok := v.(float64)
	return ok
}

func isBoolLike(v any) bool {
	if _, ok := v.(bool); ok {
		return true
	}
	return isNumber(v)
}
