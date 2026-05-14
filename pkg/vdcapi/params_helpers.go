package vdcapi

import "fmt"

// stringFromAny converts any value to its string representation.
// Returns "" for nil.
func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// boolFromAny converts any value to a bool.
// Returns (value, present, error). present is false when the value is nil.
// error is non-nil when the value is present but cannot be converted to a bool.
func boolFromAny(v any) (bool, bool, error) {
	if v == nil {
		return false, false, nil
	}
	switch b := v.(type) {
	case bool:
		return b, true, nil
	case string:
		switch b {
		case "true", "1", "yes":
			return true, true, nil
		case "false", "0", "no", "":
			return false, true, nil
		}
		return false, false, fmt.Errorf("cannot convert %q to bool", b)
	case float64:
		return b != 0, true, nil
	default:
		return false, false, fmt.Errorf("cannot convert %T to bool", v)
	}
}
