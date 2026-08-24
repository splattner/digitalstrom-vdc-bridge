package vdcapi

import (
	"fmt"
	"net"
	"strings"

	"github.com/splattner/vdcgo/pkg/logging"
)

func (s *PbufServer) processPbufMessage(payload []byte, sess *session) ([][]byte, bool) {
	return s.processPbufMessageForConn(payload, sess, nil)
}

// pbufTypeName returns a human-readable name for a pbuf message type constant.
func pbufTypeName(t int) string {
	switch t {
	case pbufTypeGenericResponse:
		return "generic_response"
	case pbufTypeVdsmRequestHello:
		return "vdsm_request_hello"
	case pbufTypeVdcResponseHello:
		return "vdc_response_hello"
	case pbufTypeVdsmRequestGetProp:
		return "vdsm_request_get_property"
	case pbufTypeVdcResponseGetProp:
		return "vdc_response_get_property"
	case pbufTypeVdsmRequestSetProp:
		return "vdsm_request_set_property"
	case pbufTypeVdsmSendPing:
		return "vdsm_send_ping"
	case pbufTypeVdcSendPong:
		return "vdc_send_pong"
	case pbufTypeVdcSendAnnounceDevice:
		return "vdc_send_announce_device"
	case pbufTypeVdsmSendRemove:
		return "vdsm_send_remove"
	case pbufTypeVdcSendVanish:
		return "vdc_send_vanish"
	case pbufTypeVdcSendPushNotification:
		return "vdc_send_push_notification"
	case pbufTypeVdsmSendBye:
		return "vdsm_send_bye"
	case pbufTypeVdsmNotifyCallScene:
		return "vdsm_notify_call_scene"
	case pbufTypeVdsmNotifySaveScene:
		return "vdsm_notify_save_scene"
	case pbufTypeVdsmNotifyUndoScene:
		return "vdsm_notify_undo_scene"
	case pbufTypeVdsmNotifySetLocalPrio:
		return "vdsm_notify_set_local_prio"
	case pbufTypeVdsmNotifyCallMinScene:
		return "vdsm_notify_call_min_scene"
	case pbufTypeVdsmNotifyIdentify:
		return "vdsm_notify_identify"
	case pbufTypeVdsmNotifySetControlValue:
		return "vdsm_notify_set_control_value"
	case pbufTypeVdcSendIdentify:
		return "vdc_send_identify"
	case pbufTypeVdcSendAnnounceVdc:
		return "vdc_send_announce_vdc"
	case pbufTypeVdsmNotifyDimChannel:
		return "vdsm_notify_dim_channel"
	case pbufTypeVdsmNotifySetOutputChannelValue:
		return "vdsm_notify_set_output_channel_value"
	case pbufTypeVdsmRequestGenericReq:
		return "vdsm_request_generic_request"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func (s *PbufServer) processPbufMessageForConn(payload []byte, sess *session, conn net.Conn) ([][]byte, bool) {
	msgType, msgID, hasID, subMessages, err := parsePbufEnvelope(payload)
	if err != nil {
		return [][]byte{buildPbufGenericResponse(0, false, pbufResultMessageUnknown, err.Error())}, false
	}
	logging.Debug("pbuf_recv", logging.Fields{
		"type":       pbufTypeName(int(msgType)),
		"type_num":   msgType,
		"has_msg_id": hasID,
		"msg_id":     msgID,
	})

	// Emit a trace event for every received frame.
	s.traceRx(payload, msgType, msgID, hasID, subMessages)

	switch msgType {
	case pbufTypeVdsmRequestHello:
		hello, err := mustPbufSubMessage(subMessages, 100, "vdsm_request_hello")
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		vdsmDSUID, version, err := parsePbufHelloRequest(hello)
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		logging.Debug("pbuf_hello_request", logging.Fields{
			"api_version":  version,
			"vdsm_dsuid":   vdsmDSUID,
			"session_open": sess.active,
		})
		if version < APIVersionMin || version > APIVersionMax {
			logging.Warn("pbuf_hello_rejected", logging.Fields{"reason": "incompatible_api_version", "api_version": version, "expected_min": APIVersionMin, "expected_max": APIVersionMax})
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultIncompatibleAPI, fmt.Sprintf("Incompatible vDC API version - found %d, expected %d..%d", version, APIVersionMin, APIVersionMax))}, false
		}
		if vdsmDSUID == "" {
			logging.Warn("pbuf_hello_rejected", logging.Fields{"reason": "missing_dsuid"})
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, "missing dSUID")}, false
		}
		if sess.active && !strings.EqualFold(sess.vdsmDSUID, vdsmDSUID) {
			logging.Warn("pbuf_hello_rejected", logging.Fields{"reason": "session_conflict", "active_vdsm_dsuid": sess.vdsmDSUID, "incoming_vdsm_dsuid": vdsmDSUID})
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultServiceUnavailable, fmt.Sprintf("this vDC already has an active session with %s", sess.vdsmDSUID))}, false
		}
		activeDSUID, replacedConn, ok := s.claimSessionOwner(conn, vdsmDSUID)
		if !ok {
			logging.Warn("pbuf_hello_rejected", logging.Fields{"reason": "session_conflict", "active_vdsm_dsuid": activeDSUID, "incoming_vdsm_dsuid": vdsmDSUID})
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultServiceUnavailable, fmt.Sprintf("this vDC already has an active session with %s", activeDSUID))}, false
		}
		if replacedConn != nil {
			// Close the old connection immediately so its handleConn goroutine
			// unblocks from readPbufFrame and cleans up its resources.
			_ = replacedConn.Close()
			logging.Info("pbuf_session_replaced", logging.Fields{"vdsm_dsuid": vdsmDSUID})
		}
		sess.active = true
		sess.vdsmDSUID = vdsmDSUID
		sess.apiVersion = version
		logging.Info("pbuf_hello_accepted", logging.Fields{"api_version": version, "vdsm_dsuid": vdsmDSUID})
		s.statusSession.setConnected(conn, vdsmDSUID, version)
		snapshot := ExternalSnapshot{}
		if s.State != nil {
			snapshot = s.State.Snapshot()
		}
		_, _, devices := buildPropertyTree(s.DSUID, s.Description, snapshot, s.Scenes, s.Config)
		currentDSUIDs := make([]string, 0, len(devices))
		for dsuid := range devices {
			currentDSUIDs = append(currentDSUIDs, dsuid)
		}
		frames := [][]byte{buildPbufHelloResponse(msgID, hasID, s.DSUID), buildPbufAnnounceVdc(s.DSUID)}
		// Reconcile: vanish any devices DSS knew about that are no longer active.
		if s.Config != nil {
			for _, staleDSUID := range s.Config.StaleAnnouncedDSUIDs(currentDSUIDs) {
				logging.Info("pbuf_vanish_stale_device", logging.Fields{"dsuid": staleDSUID})
				frames = append(frames, buildPbufVanish(staleDSUID))
			}
		}
		for dsuid, d := range devices {
			active, _ := d["active"].(bool)
			// Vanish the device first so DSS treats it as brand-new and re-queries all
			// configuration properties (outputDescription, channelDescriptions, etc.).
			// Without this, DSS reuses its database entry and skips the full config query.
			logging.Info("pbuf_vanish_before_announce", logging.Fields{"dsuid": dsuid})
			frames = append(frames, buildPbufVanish(dsuid))
			logging.Info("pbuf_announce_device_on_hello", logging.Fields{
				"dsuid":  dsuid,
				"name":   d["name"],
				"active": active,
			})
			frames = append(frames, buildPbufAnnounceDevice(dsuid, s.DSUID))
			changed := map[string]any{
				"channelStates": d["channelStates"],
			}
			if v, ok := d["active"].(bool); ok {
				changed["active"] = v
			}
			logging.Debug("pbuf_push_notification_on_hello", logging.Fields{"dsuid": dsuid, "active": active})
			frames = append(frames, buildPbufPushNotification(dsuid, changed))
		}
		return frames, false

	case pbufTypeVdsmRequestGetProp:
		if !sess.active || !s.isSessionOwner(conn) {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotAuthorized, "no vDC session - cannot call method")}, false
		}
		body, err := mustPbufSubMessage(subMessages, 102, "vdsm_request_get_property")
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		target, query, err := parsePbufGetPropertyRequest(body)
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		return s.handlePbufGetProperty(msgID, hasID, target, query), false

	case pbufTypeVdsmRequestSetProp:
		if !sess.active || !s.isSessionOwner(conn) {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotAuthorized, "no vDC session - cannot call method")}, false
		}
		body, err := mustPbufSubMessage(subMessages, 104, "vdsm_request_set_property")
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		target, props, err := parsePbufSetPropertyRequest(body)
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		return s.handlePbufSetProperty(msgID, hasID, target, props), false

	case pbufTypeVdsmSendRemove:
		if !sess.active || !s.isSessionOwner(conn) {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotAuthorized, "no vDC session - cannot call method")}, false
		}
		body, err := mustPbufSubMessage(subMessages, 110, "vdsm_send_remove")
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		target := parsePbufDSUIDField(body)
		if target == "" {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, "missing dSUID")}, false
		}
		logging.Info("vdsm_remove_request", logging.Fields{"dsuid": target})
		if err := s.methodService().removeDeviceByDSUID(target); err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotFound, err.Error())}, false
		}
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}, false

	case pbufTypeVdsmRequestGenericReq:
		if !sess.active || !s.isSessionOwner(conn) {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultNotAuthorized, "no vDC session - cannot call method")}, false
		}
		body, err := mustPbufSubMessage(subMessages, 123, "vdsm_request_generic_request")
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		target, method, params, err := parsePbufGenericRequest(body)
		if err != nil {
			return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
		}
		logging.Debug("pbuf_generic_request", logging.Fields{"method": method, "target": target, "param_count": len(params)})
		return s.handlePbufGenericRequest(msgID, hasID, target, method, params), false

	case pbufTypeVdsmSendBye:
		sess.active = false
		sess.vdsmDSUID = ""
		sess.apiVersion = 0
		return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultOK, "")}, true

	case pbufTypeVdsmSendPing:
		pingBody, err := mustPbufSubMessage(subMessages, 105, "vdsm_send_ping")
		if err != nil {
			if hasID {
				return [][]byte{buildPbufGenericResponse(msgID, hasID, pbufResultMessageUnknown, err.Error())}, false
			}
			logging.Warn("pbuf_ping_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		dsuid := parsePbufPingDSUID(pingBody)
		pongDSUID, ok := s.methodService().resolvePongTarget(dsuid)
		if !ok {
			return nil, false
		}
		return [][]byte{buildPbufPong(pongDSUID)}, false

	case pbufTypeVdsmNotifyCallScene:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 112, "vdsm_send_call_scene")
		if err != nil {
			logging.Warn("pbuf_call_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, scene, err := parsePbufCallSceneNotification(body)
		if err != nil {
			logging.Warn("pbuf_call_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		s.methodService().handleCallSceneNotification(targets, false, scene)
		return nil, false

	case pbufTypeVdsmNotifySaveScene:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 113, "vdsm_send_save_scene")
		if err != nil {
			logging.Warn("pbuf_save_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, scene, err := parsePbufCallSceneNotification(body)
		if err != nil {
			logging.Warn("pbuf_save_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		s.methodService().handleSaveSceneNotification(targets, false, scene)
		return nil, false

	case pbufTypeVdsmNotifyUndoScene:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 114, "vdsm_send_undo_scene")
		if err != nil {
			logging.Warn("pbuf_undo_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, _, err := parsePbufCallSceneNotification(body)
		if err != nil {
			logging.Warn("pbuf_undo_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		logging.Debug("pbuf_undo_scene_received", logging.Fields{"target_count": len(targets)})
		return nil, false

	case pbufTypeVdsmNotifySetLocalPrio:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 115, "vdsm_send_set_local_prio")
		if err != nil {
			logging.Warn("pbuf_set_local_prio_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, _, err := parsePbufCallSceneNotification(body)
		if err != nil {
			logging.Warn("pbuf_set_local_prio_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		logging.Debug("pbuf_set_local_prio_received", logging.Fields{"target_count": len(targets)})
		return nil, false

	case pbufTypeVdsmNotifyCallMinScene:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 116, "vdsm_send_call_min_scene")
		if err != nil {
			logging.Warn("pbuf_call_min_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, scene, err := parsePbufCallSceneNotification(body)
		if err != nil {
			logging.Warn("pbuf_call_min_scene_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		s.methodService().handleCallSceneNotification(targets, false, scene)
		return nil, false

	case pbufTypeVdsmNotifyIdentify:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 117, "vdsm_send_identify")
		if err != nil {
			logging.Warn("pbuf_identify_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, err := parsePbufNotificationTargets(body)
		if err != nil {
			logging.Warn("pbuf_identify_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		logging.Debug("pbuf_identify_received", logging.Fields{"target_count": len(targets)})
		frames := make([][]byte, 0, len(targets))
		for _, t := range targets {
			frames = append(frames, buildPbufIdentify(t))
		}
		return frames, false

	case pbufTypeVdsmNotifySetControlValue:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 118, "vdsm_send_set_control_value")
		if err != nil {
			logging.Warn("pbuf_set_control_value_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, name, value, ok, err := parsePbufSetControlValueNotification(body)
		if err != nil {
			logging.Warn("pbuf_set_control_value_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		s.methodService().handleSetControlValueNotification(targets, false, name, value, ok)
		return nil, false

	case pbufTypeVdsmNotifyDimChannel:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 121, "vdsm_send_dim_channel")
		if err != nil {
			logging.Warn("pbuf_dim_channel_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, mode, err := parsePbufDimChannelNotification(body)
		if err != nil {
			logging.Warn("pbuf_dim_channel_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		s.methodService().handleDimChannelNotification(targets, false, mode)
		return nil, false

	case pbufTypeVdsmNotifySetOutputChannelValue:
		if !sess.active || !s.isSessionOwner(conn) {
			return nil, false
		}
		body, err := mustPbufSubMessage(subMessages, 122, "vdsm_send_output_channel_value")
		if err != nil {
			logging.Warn("pbuf_set_output_channel_value_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		targets, value, channelIndex, applyNow, ok, err := parsePbufSetOutputChannelValueNotification(body)
		if err != nil {
			logging.Warn("pbuf_set_output_channel_value_parse_error", logging.Fields{"error": err})
			return nil, false
		}
		s.methodService().handleSetOutputChannelValueNotification(targets, false, channelIndex, value, applyNow, ok)
		return nil, false

	case pbufTypeGenericResponse:
		// DSS sends a generic_response back for vDC-initiated messages that
		// "only expect a response in case of error" (announcedevice, announcevdc,
		// pushNotification).  We don't track pending answers, so just absorb the
		// message silently on success, and log if it carries an error.
		if gen, ok := subMessages[3]; ok {
			code, _, _ := parsePbufGenericResponseCode(gen)
			if code != pbufResultOK {
				logging.Warn("pbuf_generic_response_error", logging.Fields{"code": code, "has_id": hasID, "message_id": msgID})
			} else {
				logging.Debug("pbuf_generic_response_ok", logging.Fields{"has_id": hasID, "message_id": msgID})
			}
		}
		return nil, false

	default:
		logging.Warn("pbuf_unsupported_message", logging.Fields{"message_type": msgType, "has_id": hasID})
		if hasID {
			return [][]byte{buildPbufGenericResponse(msgID, true, pbufResultMessageUnknown, "message unknown")}, false
		}
		return nil, false
	}
}

func mustPbufSubMessage(sub map[int][]byte, field int, label string) ([]byte, error) {
	b, ok := sub[field]
	if !ok {
		return nil, fmt.Errorf("message type and contents do not match: missing %s", label)
	}
	return b, nil
}
