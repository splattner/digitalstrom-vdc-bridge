package vdcapi

import (
	"fmt"
	"strings"
)

func validateJSONLogLevelParams(params map[string]any) (int, error) {
	v, ok := params["value"]
	if !ok {
		return 0, fmt.Errorf("missing value")
	}
	level, ok := asFloat(v)
	if !ok {
		return 0, fmt.Errorf("value must be numeric")
	}
	intLevel := int(level)
	if intLevel < 0 || intLevel > 8 {
		return 0, fmt.Errorf("invalid log level %d", intLevel)
	}
	return intLevel, nil
}

func validateJSONLogLevelOffsetParams(params map[string]any) error {
	if _, err := validateJSONLogLevelParams(params); err != nil {
		return err
	}
	if topic := strings.TrimSpace(stringFromAny(params["topic"])); topic != "" {
		return fmt.Errorf("unknown logging topic '%s'", topic)
	}
	return nil
}

func validateJSONLogOptionsParams(params map[string]any) error {
	for _, key := range []string{"deltas", "symbols", "colors"} {
		if _, _, err := boolFromAny(params[key]); err != nil && params[key] != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
	}
	return nil
}

func validateJSONIdentifyParams(params map[string]any) (float64, error) {
	if params == nil {
		return 0, nil
	}
	raw, ok := params["duration"]
	if !ok || raw == nil {
		return 0, nil
	}
	duration, ok := asFloat(raw)
	if !ok {
		return 0, fmt.Errorf("invalid duration: must be numeric")
	}
	return duration, nil
}

func validatePbufLogLevelParams(params []pbufPropertyElement) (int, error) {
	value, ok := firstPbufParamInt(params, "value")
	if !ok {
		return 0, fmt.Errorf("missing value")
	}
	if value < 0 || value > 8 {
		return 0, fmt.Errorf("invalid log level %d", value)
	}
	return value, nil
}

func validatePbufLogLevelOffsetParams(params []pbufPropertyElement) error {
	if _, err := validatePbufLogLevelParams(params); err != nil {
		return err
	}
	if topicParam, ok := findPbufParam(params, "topic"); ok {
		if topic, ok := topicParam.Value.(string); ok && strings.TrimSpace(topic) != "" {
			return fmt.Errorf("unknown logging topic '%s'", topic)
		}
	}
	return nil
}

func validatePbufLogOptionsParams(params []pbufPropertyElement) error {
	for _, key := range []string{"deltas", "symbols", "colors"} {
		if p, ok := findPbufParam(params, key); ok {
			if _, _, err := pbufBoolLike(p.Value); err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
		}
	}
	return nil
}

func validatePbufIdentifyParams(params []pbufPropertyElement) (float64, error) {
	p, ok := findPbufParam(params, "duration")
	if !ok {
		return 0, nil
	}
	duration, ok := asFloat(p.Value)
	if !ok {
		return 0, fmt.Errorf("invalid duration: must be numeric")
	}
	return duration, nil
}
