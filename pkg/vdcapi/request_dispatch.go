package vdcapi

import (
	"fmt"
	"strings"
)

// processRequest dispatches an inbound vDC API request and returns a response
// and a boolean indicating whether the connection should be closed after
// sending the response.
func (s *Server) processRequest(req request, sess *session) (*response, bool) {
	// Notifications are not method calls and do not require an active session.
	if req.Notification != "" {
		return s.handleJSONNotification(req.Notification, req.Params, sess), false
	}

	switch strings.ToLower(req.Method) {
	case "hello":
		return s.handleHello(req.Params, sess), false
	case "bye":
		return nil, true
	}

	// All other methods require an active session.
	if !sess.active {
		return &response{
			ID:       req.ID,
			Error:    401,
			ErrorMsg: "no vDC session - cannot call method",
		}, false
	}

	switch strings.ToLower(req.Method) {
	case "getproperty":
		r := s.handleGetProperty(req.Params)
		r.ID = req.ID
		return r, false
	case "setproperty":
		r := s.handleSetProperty(req.Params)
		r.ID = req.ID
		return r, false
	case "genericrequest":
		r := s.handleGenericRequest(req.Params, sess)
		r.ID = req.ID
		return r, false
	default:
		return &response{
			ID:       req.ID,
			Error:    404,
			ErrorMsg: fmt.Sprintf("unknown method '%s'", req.Method),
		}, false
	}
}

// handleHello processes the hello handshake, validates the API version and
// dSUID, and establishes the session.
func (s *Server) handleHello(params map[string]any, sess *session) *response {
	if params == nil {
		params = map[string]any{}
	}
	versionRaw, _ := asFloat(params["api_version"])
	version := int(versionRaw)
	if version < APIVersionMin || version > APIVersionMax {
		return &response{
			Error:    505,
			ErrorMsg: fmt.Sprintf("Incompatible vDC API version - found %d, expected %d..%d", version, APIVersionMin, APIVersionMax),
		}
	}
	vdsmDSUID := strings.TrimSpace(stringFromAny(params["dSUID"]))
	if vdsmDSUID == "" {
		return &response{Error: 505, ErrorMsg: "missing dSUID"}
	}
	sess.active = true
	sess.vdsmDSUID = vdsmDSUID
	sess.apiVersion = version

	snapshot := ExternalSnapshot{}
	if s.State != nil {
		snapshot = s.State.Snapshot()
	}
	root, _, _ := buildPropertyTree(s.DSUID, s.Description, snapshot, s.Scenes, s.Config)
	return &response{Result: root}
}

// handleGetProperty processes a getProperty method call. It resolves the
// target specified by params["dSUID"] and applies an optional query filter.
func (s *Server) handleGetProperty(params map[string]any) *response {
	if params == nil {
		params = map[string]any{}
	}
	target := strings.TrimSpace(stringFromAny(params["dSUID"]))
	full, err := s.methodService().resolveGetPropertyTarget(target)
	if err != nil {
		return &response{Error: 404, ErrorMsg: err.Error()}
	}
	if query, ok := params["query"].(map[string]any); ok && len(query) > 0 {
		full = applyObjectQuery(full, query)
	}
	return &response{Result: full}
}

// handleSetProperty processes a setProperty method call.
func (s *Server) handleSetProperty(params map[string]any) *response {
	if params == nil {
		params = map[string]any{}
	}
	if err := s.methodService().setPropertyFromJSON(params); err != nil {
		code := setPropertyJSONStatusCode(err)
		return &response{Error: code, ErrorMsg: err.Error()}
	}
	return &response{}
}

// handleGenericRequest processes a genericRequest method call by forwarding to
// the notification dispatcher with the merged parameter set.
func (s *Server) handleGenericRequest(params map[string]any, sess *session) *response {
	if params == nil {
		params = map[string]any{}
	}
	name := strings.TrimSpace(stringFromAny(params["methodname"]))
	if strings.EqualFold(name, "genericRequest") {
		return &response{Error: 415, ErrorMsg: "recursive call of genericRequest"}
	}
	// Merge top-level params with inner params["params"] map.
	merged := make(map[string]any, len(params))
	for k, v := range params {
		merged[k] = v
	}
	if inner, ok := params["params"].(map[string]any); ok {
		for k, v := range inner {
			merged[k] = v
		}
	}
	return s.handleJSONNotification(name, merged, sess)
}

// handleJSONNotification dispatches a vDC API notification or
// genericRequest-style method call by name.
func (s *Server) handleJSONNotification(name string, params map[string]any, sess *session) *response {
	if params == nil {
		params = map[string]any{}
	}
	switch {
	case strings.EqualFold(name, "ping"):
		// ping always succeeds; no audience required.
		return &response{}

	case strings.EqualFold(name, "callScene"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		sceneNum := 0
		if v, ok := asFloat(params["scene"]); ok {
			sceneNum = int(v)
		}
		s.methodService().handleCallSceneNotification(targets, all, sceneNum)
		return &response{}

	case strings.EqualFold(name, "saveScene"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		sceneNum := 0
		if v, ok := asFloat(params["scene"]); ok {
			sceneNum = int(v)
		}
		s.methodService().handleSaveSceneNotification(targets, all, sceneNum)
		return &response{}

	case strings.EqualFold(name, "undoScene"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		sceneNum := 0
		if v, ok := asFloat(params["scene"]); ok {
			sceneNum = int(v)
		}
		s.methodService().handleSaveSceneNotification(targets, all, sceneNum)
		return &response{}

	case strings.EqualFold(name, "setLocalPriority"):
		_, _, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		return &response{}

	case strings.EqualFold(name, "callMinScene"):
		_, _, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		return &response{}

	case strings.EqualFold(name, "identify"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		if all {
			if err := s.methodService().ensureAddressableTarget("root"); err != nil {
				return &response{Error: 404, ErrorMsg: err.Error()}
			}
		} else {
			for _, t := range targets {
				if err := s.methodService().ensureAddressableTarget(t); err != nil {
					return &response{Error: 404, ErrorMsg: err.Error()}
				}
			}
		}
		if _, err := validateJSONIdentifyParams(params); err != nil {
			return &response{Error: 415, ErrorMsg: err.Error()}
		}
		return &response{}

	case strings.EqualFold(name, "setOutputChannelValue"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		value, hasValue := asFloat(params["value"])
		channelIndex := 0
		if ci, ok := asFloat(params["channelIndex"]); ok {
			channelIndex = int(ci)
		}
		applyNow := true
		s.methodService().handleSetOutputChannelValueNotification(targets, all, channelIndex, value, applyNow, hasValue)
		return &response{}

	case strings.EqualFold(name, "dimChannel"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		mode := 0
		if v, ok := asFloat(params["mode"]); ok {
			mode = int(v)
		}
		s.methodService().handleDimChannelNotification(targets, all, mode)
		return &response{}

	case strings.EqualFold(name, "setControlValue"):
		targets, all, apiErr := s.resolveJSONNotificationAudience(params)
		if apiErr != nil {
			return apiErr
		}
		ctrlName := stringFromAny(params["name"])
		value, hasValue := asFloat(params["value"])
		s.methodService().handleSetControlValueNotification(targets, all, ctrlName, value, hasValue)
		return &response{}

	case strings.EqualFold(name, "loglevel"):
		if _, err := validateJSONLogLevelParams(params); err != nil {
			return &response{Error: 405, ErrorMsg: err.Error()}
		}
		return &response{}

	case strings.EqualFold(name, "logleveloffset"):
		if err := validateJSONLogLevelOffsetParams(params); err != nil {
			return &response{Error: 405, ErrorMsg: err.Error()}
		}
		return &response{}

	case strings.EqualFold(name, "logoptions"):
		if err := validateJSONLogOptionsParams(params); err != nil {
			return &response{Error: 415, ErrorMsg: err.Error()}
		}
		return &response{}

	case strings.EqualFold(name, "scanDevices"):
		return &response{Result: map[string]any{"status": "started"}}

	case strings.EqualFold(name, "pair"):
		timeout := 30
		if v, ok := asFloat(params["timeout"]); ok {
			timeout = int(v)
		}
		if err := s.methodService().ensurePairTimeout(timeout); err != nil {
			return &response{Error: 404, ErrorMsg: err.Error()}
		}
		return &response{}

	case strings.EqualFold(name, "remove"):
		removeTarget := strings.TrimSpace(stringFromAny(params["dSUID"]))
		if err := s.methodService().removeDeviceByDSUID(removeTarget); err != nil {
			return &response{Error: 404, ErrorMsg: err.Error()}
		}
		return &response{}

	default:
		return &response{Error: 404, ErrorMsg: fmt.Sprintf("unknown notification '%s'", name)}
	}
}

// resolveJSONNotificationAudience extracts the target set from JSON notification
// params. It accepts dSUID (string or []any), itemSpec, or zone_id+group.
// Returns (targets, all, errorResponse). errorResponse is non-nil on failure.
func (s *Server) resolveJSONNotificationAudience(params map[string]any) ([]string, bool, *response) {
	const audienceErr = "notification needs dSUID, itemSpec or zone_id/group parameters"

	// Extract dSUID targets (may be a single string or an array).
	var rawTargets []string
	switch v := params["dSUID"].(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			rawTargets = []string{v}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				rawTargets = append(rawTargets, s)
			}
		}
	}

	if len(rawTargets) == 1 {
		t := strings.TrimSpace(rawTargets[0])
		if strings.EqualFold(t, "root") || strings.EqualFold(t, s.DSUID) {
			return nil, true, nil
		}
		if err := s.methodService().ensureAddressableTarget(t); err != nil {
			return nil, false, &response{Error: 404, ErrorMsg: err.Error()}
		}
		return []string{t}, false, nil
	}

	if len(rawTargets) > 1 {
		resolved := make([]string, 0, len(rawTargets))
		for _, raw := range rawTargets {
			t := strings.TrimSpace(raw)
			if t == "" {
				continue
			}
			if strings.EqualFold(t, "root") || strings.EqualFold(t, s.DSUID) {
				return nil, true, nil
			}
			if err := s.methodService().ensureAddressableTarget(t); err == nil {
				resolved = append(resolved, t)
			}
			// Unknown DSUIDs in a multi-target list are silently ignored.
		}
		return resolved, false, nil
	}

	// Try itemSpec.
	if itemSpec, ok := params["itemSpec"].(string); ok {
		itemSpec = strings.TrimSpace(itemSpec)
		if itemSpec == "" {
			return nil, false, &response{Error: 404, ErrorMsg: "missing/invalid itemSpec"}
		}
		if strings.EqualFold(itemSpec, "root") || strings.EqualFold(itemSpec, s.DSUID) {
			return nil, true, nil
		}
		if err := s.methodService().ensureAddressableTarget(itemSpec); err == nil {
			return []string{itemSpec}, false, nil
		}
		return nil, false, &response{Error: 404, ErrorMsg: "missing/invalid itemSpec"}
	}

	// Try zone_id + group (broadcast).
	_, hasZone := params["zone_id"]
	_, hasGroup := params["group"]
	if hasZone && hasGroup {
		return nil, true, nil
	}

	return nil, false, &response{Error: 400, ErrorMsg: audienceErr}
}
