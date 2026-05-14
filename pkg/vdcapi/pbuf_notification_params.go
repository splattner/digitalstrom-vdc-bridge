package vdcapi

import "strings"

func (s *PbufServer) resolvePbufNotificationAudience(target string, params []pbufPropertyElement) ([]string, bool, uint64, string, bool) {
	audienceErr := "notification needs dSUID, itemSpec or zone_id/group parameters"

	targets := collectPbufParamStrings(params, "dSUID")
	if len(targets) == 0 {
		if t := strings.TrimSpace(target); t != "" {
			targets = []string{t}
		}
	}
	if len(targets) == 1 {
		t := strings.TrimSpace(targets[0])
		if t == "" {
			return nil, false, pbufResultNotFound, "addressable not found", false
		}
		if strings.EqualFold(t, "root") || strings.EqualFold(t, s.DSUID) {
			return nil, true, 0, "", true
		}
		if err := s.methodService().ensureAddressableTarget(t); err != nil {
			return nil, false, pbufResultNotFound, err.Error(), false
		}
		return []string{t}, false, 0, "", true
	}
	if len(targets) > 1 {
		resolved := make([]string, 0, len(targets))
		for _, raw := range targets {
			t := strings.TrimSpace(raw)
			if t == "" {
				continue
			}
			if strings.EqualFold(t, "root") || strings.EqualFold(t, s.DSUID) {
				return nil, true, 0, "", true
			}
			if err := s.methodService().ensureAddressableTarget(t); err == nil {
				resolved = append(resolved, t)
			}
		}
		return resolved, false, 0, "", true
	}

	if itemSpecParam, ok := findPbufParam(params, "itemSpec"); ok {
		itemSpec, _ := itemSpecParam.Value.(string)
		itemSpec = strings.TrimSpace(itemSpec)
		if itemSpec == "" {
			return nil, false, pbufResultNotFound, "missing/invalid itemSpec", false
		}
		if strings.EqualFold(itemSpec, "root") || strings.EqualFold(itemSpec, s.DSUID) {
			return nil, true, 0, "", true
		}
		if err := s.methodService().ensureAddressableTarget(itemSpec); err == nil {
			return []string{itemSpec}, false, 0, "", true
		}
		return nil, false, pbufResultNotFound, "missing/invalid itemSpec", false
	}

	_, hasZone := findPbufParam(params, "zone_id")
	_, hasGroup := findPbufParam(params, "group")
	if hasZone && hasGroup {
		return nil, true, 0, "", true
	}

	return nil, false, pbufResultMessageUnknown, audienceErr, false
}

func parsePbufNotificationParamsCallScene(params []pbufPropertyElement) ([]string, int) {
	targets := collectPbufParamStrings(params, "dSUID")
	scene, ok := firstPbufParamInt(params, "scene")
	if !ok {
		scene = 0
	}
	return targets, scene
}

func parsePbufNotificationParamsDimChannel(params []pbufPropertyElement) ([]string, int) {
	targets := collectPbufParamStrings(params, "dSUID")
	mode, ok := firstPbufParamInt(params, "mode")
	if !ok {
		mode = 0
	}
	return targets, mode
}

func parsePbufNotificationParamsSetControlValue(params []pbufPropertyElement) ([]string, string, float64, bool) {
	targets := collectPbufParamStrings(params, "dSUID")
	name := ""
	if p, ok := findPbufParam(params, "name"); ok {
		if s, ok := p.Value.(string); ok {
			name = s
		}
	}
	value, ok := firstPbufParamFloat(params, "value")
	return targets, name, value, ok
}

func parsePbufNotificationParamsSetOutputChannelValue(params []pbufPropertyElement) ([]string, float64, int, bool, bool) {
	targets := collectPbufParamStrings(params, "dSUID")
	applyNow := true
	if p, ok := findPbufParam(params, "apply_now"); ok {
		if b, ok := p.Value.(bool); ok {
			applyNow = b
		}
	}
	value, ok := firstPbufParamFloat(params, "value")
	channelIndex := 0
	if ci, ok2 := firstPbufParamInt(params, "channelIndex"); ok2 {
		channelIndex = ci
	}
	return targets, value, channelIndex, applyNow, ok
}
