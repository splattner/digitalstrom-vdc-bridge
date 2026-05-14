package vdcapi

import (
	"fmt"
	"strings"
)

func findPbufParam(params []pbufPropertyElement, name string) (pbufPropertyElement, bool) {
	for i := range params {
		if strings.EqualFold(strings.TrimSpace(params[i].Name), name) {
			return params[i], true
		}
	}
	return pbufPropertyElement{}, false
}

func findPbufParams(params []pbufPropertyElement, name string) []pbufPropertyElement {
	result := make([]pbufPropertyElement, 0, 2)
	for i := range params {
		if strings.EqualFold(strings.TrimSpace(params[i].Name), name) {
			result = append(result, params[i])
		}
	}
	return result
}

func collectPbufParamStrings(params []pbufPropertyElement, name string) []string {
	items := findPbufParams(params, name)
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for i := range items {
		if s, ok := items[i].Value.(string); ok {
			if strings.TrimSpace(s) != "" {
				result = append(result, s)
			}
		}
	}
	return result
}

func firstPbufParamInt(params []pbufPropertyElement, name string) (int, bool) {
	p, ok := findPbufParam(params, name)
	if !ok {
		return 0, false
	}
	v, ok := asFloat(p.Value)
	if !ok {
		return 0, false
	}
	return int(v), true
}

func firstPbufParamFloat(params []pbufPropertyElement, name string) (float64, bool) {
	p, ok := findPbufParam(params, name)
	if !ok {
		return 0, false
	}
	v, ok := asFloat(p.Value)
	if !ok {
		return 0, false
	}
	return v, true
}

func pbufBoolParam(params []pbufPropertyElement, name string, dflt bool) (bool, error) {
	p, ok := findPbufParam(params, name)
	if !ok {
		return dflt, nil
	}
	b, ok, err := pbufBoolLike(p.Value)
	if err != nil {
		return false, err
	}
	if !ok {
		return dflt, nil
	}
	return b, nil
}

func pbufBoolLike(v any) (bool, bool, error) {
	switch x := v.(type) {
	case bool:
		return x, true, nil
	case float64:
		return x != 0, true, nil
	case int:
		return x != 0, true, nil
	case uint64:
		return x != 0, true, nil
	case nil:
		return false, false, nil
	default:
		return false, false, fmt.Errorf("must be boolean-like")
	}
}

type pbufScanDevicesParams struct {
	Incremental bool
	Exhaustive  bool
	Reenumerate bool
	ClearConfig bool
}

type pbufPairParams struct {
	Timeout               int
	DisableProximityCheck bool
	EstablishDefined      bool
	Establish             bool
}

func parsePbufScanDevicesParams(params []pbufPropertyElement) (pbufScanDevicesParams, error) {
	incremental, err := pbufBoolParam(params, "incremental", true)
	if err != nil {
		return pbufScanDevicesParams{}, fmt.Errorf("invalid incremental: %w", err)
	}
	exhaustive, err := pbufBoolParam(params, "exhaustive", false)
	if err != nil {
		return pbufScanDevicesParams{}, fmt.Errorf("invalid exhaustive: %w", err)
	}
	reenumerate, err := pbufBoolParam(params, "reenumerate", false)
	if err != nil {
		return pbufScanDevicesParams{}, fmt.Errorf("invalid reenumerate: %w", err)
	}
	clearConfig, err := pbufBoolParam(params, "clearconfig", false)
	if err != nil {
		return pbufScanDevicesParams{}, fmt.Errorf("invalid clearconfig: %w", err)
	}
	return pbufScanDevicesParams{
		Incremental: incremental,
		Exhaustive:  exhaustive,
		Reenumerate: reenumerate,
		ClearConfig: clearConfig,
	}, nil
}

func parsePbufPairParams(params []pbufPropertyElement) (pbufPairParams, error) {
	timeout, ok := firstPbufParamInt(params, "timeout")
	if !ok {
		timeout = 30
	}
	disableProximityCheck, err := pbufBoolParam(params, "disableProximityCheck", false)
	if err != nil {
		return pbufPairParams{}, fmt.Errorf("invalid disableProximityCheck: %w", err)
	}
	establishDefined := false
	establish := false
	if p, ok := findPbufParam(params, "establish"); ok {
		b, ok, err := pbufBoolLike(p.Value)
		if err != nil {
			return pbufPairParams{}, fmt.Errorf("invalid establish: %w", err)
		}
		if ok {
			establishDefined = true
			establish = b
		}
	}
	return pbufPairParams{
		Timeout:               timeout,
		DisableProximityCheck: disableProximityCheck,
		EstablishDefined:      establishDefined,
		Establish:             establish,
	}, nil
}
