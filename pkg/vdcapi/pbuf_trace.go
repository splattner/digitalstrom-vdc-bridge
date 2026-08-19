package vdcapi

import (
	"encoding/hex"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

// traceRx emits a received-frame trace event if OnTrace is configured.
func (s *PbufServer) traceRx(raw []byte, msgType uint64, msgID uint32, hasID bool, sub map[int][]byte) {
	if s.OnTrace == nil {
		return
	}
	dec := decodePbufMessage(msgType, sub)
	s.OnTrace(PbufTraceFrame{
		Time:        time.Now(),
		Direction:   "rx",
		TypeNum:     int(msgType),
		TypeName:    pbufTypeName(int(msgType)),
		MsgID:       msgID,
		HasMsgID:    hasID,
		DeviceDSUID: extractDecodedDSUID(dec),
		Decoded:     dec,
		RawHex:      hex.EncodeToString(raw),
	})
}

// traceTxFrames traces a slice of outbound frames by parsing their envelopes.
func (s *PbufServer) traceTxFrames(frames [][]byte) {
	if s.OnTrace == nil {
		return
	}
	for _, f := range frames {
		// frames are length-prefixed (2 bytes big-endian), strip header
		payload := f
		if len(f) > 2 {
			payload = f[2:]
		}
		msgType, msgID, hasID, sub, err := parsePbufEnvelope(payload)
		if err != nil {
			continue
		}
		dec := decodePbufMessage(msgType, sub)
		s.OnTrace(PbufTraceFrame{
			Time:        time.Now(),
			Direction:   "tx",
			TypeNum:     int(msgType),
			TypeName:    pbufTypeName(int(msgType)),
			MsgID:       msgID,
			HasMsgID:    hasID,
			DeviceDSUID: extractDecodedDSUID(dec),
			Decoded:     dec,
			RawHex:      hex.EncodeToString(payload),
		})
	}
}

// decodePbufMessage extracts a human-readable map from the already-parsed sub-messages.
// See also: extractDecodedDSUID.
func decodePbufMessage(msgType uint64, sub map[int][]byte) map[string]any {
	d := map[string]any{}
	switch int(msgType) {

	// ── RX: requests / notifications from vdSM ─────────────────────────────

	case pbufTypeVdsmRequestHello:
		if body, ok := sub[100]; ok {
			if dsuid, version, err := parsePbufHelloRequest(body); err == nil {
				d["dsuid"] = dsuid
				d["apiVersion"] = version
			}
		}

	case pbufTypeVdsmRequestGetProp:
		if body, ok := sub[102]; ok {
			if target, query, err := parsePbufGetPropertyRequest(body); err == nil {
				d["target"] = target
				d["query"] = pbufElemsToAny(query)
			}
		}

	case pbufTypeVdsmRequestSetProp:
		if body, ok := sub[104]; ok {
			if target, props, err := parsePbufSetPropertyRequest(body); err == nil {
				d["target"] = target
				d["properties"] = pbufElemsToAny(props)
			}
		}

	case pbufTypeVdsmSendPing:
		if body, ok := sub[105]; ok {
			d["dsuid"] = parsePbufPingDSUID(body)
		}

	case pbufTypeVdsmSendRemove:
		if body, ok := sub[110]; ok {
			d["dsuid"] = parsePbufDSUIDField(body)
		}

	case pbufTypeVdsmNotifyCallScene:
		if body, ok := sub[112]; ok {
			if targets, scene, err := parsePbufCallSceneNotification(body); err == nil {
				d["targets"] = targets
				d["scene"] = scene
			}
		}

	case pbufTypeVdsmNotifySaveScene:
		if body, ok := sub[113]; ok {
			if targets, scene, err := parsePbufCallSceneNotification(body); err == nil {
				d["targets"] = targets
				d["scene"] = scene
			}
		}

	case pbufTypeVdsmNotifyUndoScene:
		if body, ok := sub[114]; ok {
			if targets, _, err := parsePbufCallSceneNotification(body); err == nil {
				d["targets"] = targets
			}
		}

	case pbufTypeVdsmNotifySetLocalPrio:
		if body, ok := sub[115]; ok {
			if targets, _, err := parsePbufCallSceneNotification(body); err == nil {
				d["targets"] = targets
			}
		}

	case pbufTypeVdsmNotifyCallMinScene:
		if body, ok := sub[116]; ok {
			if targets, scene, err := parsePbufCallSceneNotification(body); err == nil {
				d["targets"] = targets
				d["scene"] = scene
			}
		}

	case pbufTypeVdsmNotifyIdentify:
		if body, ok := sub[117]; ok {
			if targets, err := parsePbufNotificationTargets(body); err == nil {
				d["targets"] = targets
			}
		}

	case pbufTypeVdsmNotifySetControlValue:
		if body, ok := sub[118]; ok {
			if targets, name, value, hasValue, err := parsePbufSetControlValueNotification(body); err == nil {
				d["targets"] = targets
				d["name"] = name
				if hasValue {
					d["value"] = value
				}
			}
		}

	case pbufTypeVdsmNotifyDimChannel:
		if body, ok := sub[121]; ok {
			if targets, mode, err := parsePbufDimChannelNotification(body); err == nil {
				d["targets"] = targets
				d["mode"] = mode
			}
		}

	case pbufTypeVdsmNotifySetOutputChannelValue:
		if body, ok := sub[122]; ok {
			if targets, value, channelIndex, applyNow, hasValue, err := parsePbufSetOutputChannelValueNotification(body); err == nil {
				d["targets"] = targets
				d["channelIndex"] = channelIndex
				d["applyNow"] = applyNow
				if hasValue {
					d["value"] = value
				}
			}
		}

	case pbufTypeVdsmRequestGenericReq:
		if body, ok := sub[123]; ok {
			if target, method, params, err := parsePbufGenericRequest(body); err == nil {
				d["target"] = target
				d["method"] = method
				d["params"] = pbufElemsToAny(params)
			}
		}

	// ── Both directions: generic response ──────────────────────────────────

	case pbufTypeGenericResponse:
		if gen, ok := sub[3]; ok {
			code, desc, errType := parsePbufGenericResponseCode(gen)
			d["code"] = code
			if desc != "" {
				d["description"] = desc
			}
			if errType != 0 {
				d["errorType"] = errType
			}
		}

	// ── TX: responses / notifications from vDC ─────────────────────────────

	case pbufTypeVdcResponseHello:
		if body, ok := sub[101]; ok {
			d["dsuid"] = parsePbufDSUIDField(body)
		}

	case pbufTypeVdcResponseGetProp:
		if body, ok := sub[103]; ok {
			d["properties"] = pbufElemsToAny(parsePbufGetPropResponseBody(body))
		}

	case pbufTypeVdcSendPong:
		if body, ok := sub[106]; ok {
			d["dsuid"] = parsePbufDSUIDField(body)
		}

	case pbufTypeVdcSendAnnounceDevice:
		if body, ok := sub[107]; ok {
			deviceDSUID, vdcDSUID := parsePbufAnnounceDeviceBody(body)
			d["deviceDSUID"] = deviceDSUID
			d["vdcDSUID"] = vdcDSUID
		}

	case pbufTypeVdcSendVanish:
		if body, ok := sub[108]; ok {
			d["dsuid"] = parsePbufDSUIDField(body)
		}

	case pbufTypeVdcSendPushNotification:
		if body, ok := sub[109]; ok {
			deviceDSUID, changed := parsePbufPushNotificationBody(body)
			d["deviceDSUID"] = deviceDSUID
			d["changed"] = pbufElemsToAny(changed)
		}

	case pbufTypeVdcSendIdentify:
		if body, ok := sub[119]; ok {
			d["dsuid"] = parsePbufDSUIDField(body)
		}

	case pbufTypeVdcSendAnnounceVdc:
		if body, ok := sub[120]; ok {
			d["dsuid"] = parsePbufDSUIDField(body)
		}
	}

	if len(d) == 0 {
		return nil
	}
	return d
}

// extractDecodedDSUID returns the primary device DSUID from a decoded message map,
// or "" when the message does not address a specific device (e.g. targeting the
// vDC root). It checks, in order:
//
//   - "target" (getProperty/setProperty/genericRequest) — only if it looks like
//     a real DSUID (34 ASCII hex chars or 17 raw binary bytes); path targets
//     like "root" or "vdc" are skipped.
//   - "targets" (scene/channel notifications) — first entry.
//   - "dsuid" (hello/ping/remove/announcements).
func extractDecodedDSUID(decoded map[string]any) string {
	if decoded == nil {
		return ""
	}
	if s, ok := decoded["target"].(string); ok {
		if d := normaliseDSUID(s); d != "" {
			return d
		}
	}
	if ts, ok := decoded["targets"].([]string); ok && len(ts) > 0 {
		return ts[0]
	}
	// TX messages (push notification, announce device) store it as "deviceDSUID"
	if s, ok := decoded["deviceDSUID"].(string); ok && s != "" {
		return s
	}
	if s, ok := decoded["dsuid"].(string); ok && s != "" {
		return s
	}
	return ""
}

// normaliseDSUID converts a raw DSUID string to its canonical 34-char uppercase
// hex form, returning "" if s does not look like a DSUID at all.
//
// The vdSM sends DSUIDs in two ways depending on the message direction:
//   - 34 ASCII hex chars (vdcgo TX side, C++ side may mirror same)
//   - 17 raw binary bytes (C++ vdSM requests use native binary representation)
func normaliseDSUID(s string) string {
	switch len(s) {
	case 34:
		// Must be all hex digits.
		for _, c := range s {
			if (c < '0' || c > '9') && (c < 'A' || c > 'F') && (c < 'a' || c > 'f') {
				return ""
			}
		}
		return strings.ToUpper(s)
	case 17:
		// Raw binary — hex-encode to canonical form.
		return strings.ToUpper(hex.EncodeToString([]byte(s)))
	default:
		return ""
	}
}

// pbufElemToMap converts a single pbufPropertyElement to a JSON-friendly map.
func pbufElemToMap(e pbufPropertyElement) map[string]any {
	m := map[string]any{}
	if e.Name != "" {
		m["name"] = e.Name
	}
	if e.Value != nil {
		m["value"] = e.Value
	}
	if len(e.Elements) > 0 {
		m["children"] = pbufElemsToAny(e.Elements)
	}
	return m
}

// pbufElemsToAny converts a slice of pbufPropertyElement to a JSON-friendly slice.
func pbufElemsToAny(elems []pbufPropertyElement) []any {
	out := make([]any, len(elems))
	for i, e := range elems {
		out[i] = pbufElemToMap(e)
	}
	return out
}

// parsePbufGetPropResponseBody parses the get-property response body (sub[103]).
// Format: repeated (tag 1, bytes) where each bytes is an encoded pbufPropertyElement.
func parsePbufGetPropResponseBody(body []byte) []pbufPropertyElement {
	var props []pbufPropertyElement
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			break
		}
		body = body[n:]
		if num == 1 {
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				break
			}
			body = body[n:]
			if e, err := parsePbufPropertyElement(b); err == nil {
				props = append(props, e)
			}
			continue
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			break
		}
		body = body[n:]
	}
	return props
}

// parsePbufAnnounceDeviceBody extracts deviceDSUID and vdcDSUID from sub[107].
func parsePbufAnnounceDeviceBody(body []byte) (deviceDSUID, vdcDSUID string) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return
		}
		body = body[n:]
		switch num {
		case 1:
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return
			}
			deviceDSUID = v
			body = body[n:]
		case 2:
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return
			}
			vdcDSUID = v
			body = body[n:]
		default:
			n = protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return
			}
			body = body[n:]
		}
	}
	return
}

// parsePbufPushNotificationBody extracts deviceDSUID and changed property elements from sub[109].
func parsePbufPushNotificationBody(body []byte) (deviceDSUID string, changed []pbufPropertyElement) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return
		}
		body = body[n:]
		switch num {
		case 1:
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return
			}
			deviceDSUID = v
			body = body[n:]
		case 2, 3:
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return
			}
			body = body[n:]
			if e, err := parsePbufPropertyElement(b); err == nil {
				changed = append(changed, e)
			}
		default:
			n = protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return
			}
			body = body[n:]
		}
	}
	return
}
