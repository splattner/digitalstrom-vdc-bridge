package vdcapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/splattner/vdcgo/pkg/logging"
)

func (s *PbufServer) handlePbufGetProperty(msgID uint32, hasID bool, target string, query []pbufPropertyElement) [][]byte {
	queryNames := make([]string, 0, len(query))
	for _, q := range query {
		queryNames = append(queryNames, q.Name)
	}
	logging.Info("pbuf_get_property", logging.Fields{"target": target, "query_count": len(query), "query": queryNames})
	full, err := s.methodService().resolveGetPropertyTarget(target)
	if err != nil {
		if errors.Is(err, errAddressableNotFound) {
			logging.Warn("pbuf_get_property_not_found", logging.Fields{"target": target})
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotFound, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultServiceUnavailable, err.Error())}
	}
	logging.Debug("pbuf_get_property_resolved", logging.Fields{"target": target, "active": full["active"]})
	props := buildPbufPropertyResult(query, full)
	respNames := make([]string, 0, len(props))
	for i := range props {
		respNames = append(respNames, props[i].Name)
	}
	resp := buildPbufGetPropertyResponse(msgID, hasID, props)
	logging.Info("pbuf_get_property_response", logging.Fields{"target": target, "props_count": len(props), "props": respNames, "response_bytes": len(resp)})
	return [][]byte{resp}
}

func (s *PbufServer) handlePbufSetProperty(msgID uint32, hasID bool, target string, props []pbufPropertyElement) [][]byte {
	propNames := make([]string, 0, len(props))
	for _, p := range props {
		propNames = append(propNames, p.Name)
	}
	logging.Info("pbuf_set_property", logging.Fields{"target": target, "props": propNames})
	if err := s.methodService().setPropertyFromPbuf(target, props); err != nil {
		return [][]byte{buildPbufGenericResponse(msgID, hasID, setPropertyPbufStatusCode(err), err.Error())}
	}
	return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
}

func (s *PbufServer) handlePbufGenericRequest(msgID uint32, hasID bool, target, method string, params []pbufPropertyElement) [][]byte {
	method = strings.TrimSpace(method)
	if strings.EqualFold(method, "genericRequest") {
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultInvalidValueType, "recursive call of genericRequest")}
	}

	switch {
	case strings.EqualFold(method, "getProperty"):
		paramTarget := target
		if dsuidParam, ok := findPbufParam(params, "dSUID"); ok {
			if v, ok := dsuidParam.Value.(string); ok && strings.TrimSpace(v) != "" {
				paramTarget = v
			}
		}
		query := []pbufPropertyElement{}
		if queryParam, ok := findPbufParam(params, "query"); ok {
			query = queryParam.Elements
		}
		return s.handlePbufGetProperty(msgID, hasID, paramTarget, query)

	case strings.EqualFold(method, "setProperty"):
		paramTarget := target
		if dsuidParam, ok := findPbufParam(params, "dSUID"); ok {
			if v, ok := dsuidParam.Value.(string); ok && strings.TrimSpace(v) != "" {
				paramTarget = v
			}
		}
		var propList []pbufPropertyElement
		if propParam, ok := findPbufParam(params, "properties"); ok {
			if len(propParam.Elements) > 0 {
				propList = append(propList, propParam.Elements...)
			} else {
				propList = append(propList, propParam)
			}
		}
		if len(propList) == 0 {
			for i := range params {
				if strings.EqualFold(params[i].Name, "dSUID") || strings.EqualFold(params[i].Name, "preload") {
					continue
				}
				propList = append(propList, params[i])
			}
		}
		return s.handlePbufSetProperty(msgID, hasID, paramTarget, propList)

	case strings.EqualFold(method, "scanDevices"):
		if _, err := parsePbufScanDevicesParams(params); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultInvalidValueType, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "pair"):
		pairParams, err := parsePbufPairParams(params)
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultInvalidValueType, err.Error())}
		}
		if err := s.methodService().ensurePairTimeout(pairParams.Timeout); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotFound, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "remove"):
		removeTarget := strings.TrimSpace(target)
		if dsuidParam, ok := findPbufParam(params, "dSUID"); ok {
			if v, ok := dsuidParam.Value.(string); ok && strings.TrimSpace(v) != "" {
				removeTarget = strings.TrimSpace(v)
			}
		}
		if err := s.methodService().ensureRemovableTarget(removeTarget); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotFound, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "loglevel"):
		if _, err := validatePbufLogLevelParams(params); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "logleveloffset"):
		if err := validatePbufLogLevelOffsetParams(params); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "logoptions"):
		if err := validatePbufLogOptionsParams(params); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "callScene"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		_, scene := parsePbufNotificationParamsCallScene(params)
		s.methodService().handleCallSceneNotification(targets, all, scene)
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "dimChannel"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		_, mode := parsePbufNotificationParamsDimChannel(params)
		s.methodService().handleDimChannelNotification(targets, all, mode)
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "setControlValue"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		_, name, value, hasValue := parsePbufNotificationParamsSetControlValue(params)
		s.methodService().handleSetControlValueNotification(targets, all, name, value, hasValue)
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "setOutputChannelValue"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		_, value, channelIndex, applyNow, hasValue := parsePbufNotificationParamsSetOutputChannelValue(params)
		s.methodService().handleSetOutputChannelValueNotification(targets, all, channelIndex, value, applyNow, hasValue)
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "identify"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		if all {
			if err := s.methodService().ensureAddressableTarget("root"); err != nil {
				return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotFound, err.Error())}
			}
		} else {
			for _, t := range targets {
				if err := s.methodService().ensureAddressableTarget(t); err != nil {
					return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotFound, err.Error())}
				}
			}
		}
		if _, err := validatePbufIdentifyParams(params); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultInvalidValueType, err.Error())}
		}
		frames := [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
		if all {
			frames = append(frames, buildPbufIdentify(s.DSUID))
		} else {
			for _, t := range targets {
				frames = append(frames, buildPbufIdentify(t))
			}
		}
		return frames
	case strings.EqualFold(method, "saveScene"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		_, sceneNum := parsePbufNotificationParamsCallScene(params)
		s.methodService().handleSaveSceneNotification(targets, all, sceneNum)
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "undoScene"):
		if _, _, code, desc, ok := s.resolvePbufNotificationAudience(target, params); !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "setLocalPriority"):
		if _, _, code, desc, ok := s.resolvePbufNotificationAudience(target, params); !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "callSceneMin"):
		if _, _, code, desc, ok := s.resolvePbufNotificationAudience(target, params); !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
	case strings.EqualFold(method, "ping"):
		targets, all, code, desc, ok := s.resolvePbufNotificationAudience(target, params)
		if !ok {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, code, desc)}
		}
		pongDSUID := s.DSUID
		if !all {
			if len(targets) == 0 {
				return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
			}
			pongDSUID = targets[0]
		}
		if pongResolved, ok := s.methodService().resolvePongTarget(pongDSUID); ok {
			pongDSUID = pongResolved
		} else {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, ""), buildPbufPong(pongDSUID)}
	default:
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotImplemented, fmt.Sprintf("unknown notification '%s'", method))}
	}
}
