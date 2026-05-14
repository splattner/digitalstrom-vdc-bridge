package vdcapi

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

func parsePbufEnvelope(payload []byte) (msgType uint64, msgID uint32, hasID bool, sub map[int][]byte, err error) {
	sub = make(map[int][]byte)
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return 0, 0, false, nil, fmt.Errorf("invalid protobuf tag")
		}
		payload = payload[n:]
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return 0, 0, false, nil, fmt.Errorf("invalid type field")
			}
			v, n := protowire.ConsumeVarint(payload)
			if n < 0 {
				return 0, 0, false, nil, fmt.Errorf("invalid type varint")
			}
			msgType = v
			payload = payload[n:]
		case 2:
			if typ != protowire.VarintType {
				return 0, 0, false, nil, fmt.Errorf("invalid message_id field")
			}
			v, n := protowire.ConsumeVarint(payload)
			if n < 0 {
				return 0, 0, false, nil, fmt.Errorf("invalid message_id varint")
			}
			hasID = true
			msgID = uint32(v)
			payload = payload[n:]
		default:
			if typ == protowire.BytesType {
				b, n := protowire.ConsumeBytes(payload)
				if n < 0 {
					return 0, 0, false, nil, fmt.Errorf("invalid bytes field")
				}
				sub[int(num)] = append([]byte(nil), b...)
				payload = payload[n:]
				continue
			}
			n := protowire.ConsumeFieldValue(num, typ, payload)
			if n < 0 {
				return 0, 0, false, nil, fmt.Errorf("invalid field value")
			}
			payload = payload[n:]
		}
	}
	if msgType == 0 {
		return 0, 0, false, nil, fmt.Errorf("missing message type")
	}
	return msgType, msgID, hasID, sub, nil
}

func parsePbufHelloRequest(body []byte) (dsuid string, apiVersion int, err error) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return "", 0, fmt.Errorf("invalid hello tag")
		}
		body = body[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return "", 0, fmt.Errorf("invalid hello dSUID field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return "", 0, fmt.Errorf("invalid hello dSUID")
			}
			dsuid = v
			body = body[n:]
		case 2:
			if typ != protowire.VarintType {
				return "", 0, fmt.Errorf("invalid hello api_version field")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return "", 0, fmt.Errorf("invalid hello api_version")
			}
			apiVersion = int(v)
			body = body[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return "", 0, fmt.Errorf("invalid hello field value")
			}
			body = body[n:]
		}
	}
	return dsuid, apiVersion, nil
}

func parsePbufGetPropertyRequest(body []byte) (target string, query []pbufPropertyElement, err error) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return "", nil, fmt.Errorf("invalid getProperty tag")
		}
		body = body[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return "", nil, fmt.Errorf("invalid getProperty dSUID field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid getProperty dSUID")
			}
			target = v
			body = body[n:]
		case 2:
			if typ != protowire.BytesType {
				return "", nil, fmt.Errorf("invalid getProperty query field")
			}
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid getProperty query")
			}
			e, err := parsePbufPropertyElement(b)
			if err != nil {
				return "", nil, err
			}
			query = append(query, e)
			body = body[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid getProperty field value")
			}
			body = body[n:]
		}
	}
	return target, query, nil
}

func parsePbufPropertyElement(body []byte) (pbufPropertyElement, error) {
	e := pbufPropertyElement{}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return pbufPropertyElement{}, fmt.Errorf("invalid property element tag")
		}
		body = body[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return pbufPropertyElement{}, fmt.Errorf("invalid property name field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return pbufPropertyElement{}, fmt.Errorf("invalid property name")
			}
			e.Name = v
			body = body[n:]
		case 2:
			if typ != protowire.BytesType {
				return pbufPropertyElement{}, fmt.Errorf("invalid property value field")
			}
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return pbufPropertyElement{}, fmt.Errorf("invalid property value")
			}
			v, err := parsePbufPropertyValue(b)
			if err != nil {
				return pbufPropertyElement{}, err
			}
			e.Value = v
			body = body[n:]
		case 3:
			if typ != protowire.BytesType {
				return pbufPropertyElement{}, fmt.Errorf("invalid property child field")
			}
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return pbufPropertyElement{}, fmt.Errorf("invalid property child")
			}
			child, err := parsePbufPropertyElement(b)
			if err != nil {
				return pbufPropertyElement{}, err
			}
			e.Elements = append(e.Elements, child)
			body = body[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return pbufPropertyElement{}, fmt.Errorf("invalid property element value")
			}
			body = body[n:]
		}
	}
	return e, nil
}

func parsePbufPropertyValue(body []byte) (any, error) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, fmt.Errorf("invalid property value tag")
		}
		body = body[n:]
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return nil, fmt.Errorf("invalid bool property value")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid bool property value")
			}
			return v != 0, nil
		case 2:
			if typ != protowire.VarintType {
				return nil, fmt.Errorf("invalid uint64 property value")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid uint64 property value")
			}
			return v, nil
		case 3:
			if typ != protowire.VarintType {
				return nil, fmt.Errorf("invalid int64 property value")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid int64 property value")
			}
			return int64(v), nil
		case 4:
			if typ != protowire.Fixed64Type {
				return nil, fmt.Errorf("invalid double property value")
			}
			v, n := protowire.ConsumeFixed64(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid double property value")
			}
			return math.Float64frombits(v), nil
		case 5:
			if typ != protowire.BytesType {
				return nil, fmt.Errorf("invalid string property value")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid string property value")
			}
			return v, nil
		case 6:
			if typ != protowire.BytesType {
				return nil, fmt.Errorf("invalid bytes property value")
			}
			v, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid bytes property value")
			}
			return v, nil
		default:
			n := protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return nil, fmt.Errorf("invalid property value field")
			}
			body = body[n:]
		}
	}
	return nil, nil
}

func parsePbufSetPropertyRequest(body []byte) (target string, props []pbufPropertyElement, err error) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return "", nil, fmt.Errorf("invalid setProperty tag")
		}
		body = body[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return "", nil, fmt.Errorf("invalid setProperty dSUID field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid setProperty dSUID")
			}
			target = v
			body = body[n:]
		case 2:
			if typ != protowire.BytesType {
				return "", nil, fmt.Errorf("invalid setProperty properties field")
			}
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid setProperty properties")
			}
			e, err := parsePbufPropertyElement(b)
			if err != nil {
				return "", nil, err
			}
			props = append(props, e)
			body = body[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return "", nil, fmt.Errorf("invalid setProperty field value")
			}
			body = body[n:]
		}
	}
	return target, props, nil
}

func parsePbufGenericRequest(body []byte) (target, method string, params []pbufPropertyElement, err error) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return "", "", nil, fmt.Errorf("invalid genericRequest tag")
		}
		body = body[n:]
		switch num {
		case 1:
			if typ != protowire.BytesType {
				return "", "", nil, fmt.Errorf("invalid genericRequest dSUID field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return "", "", nil, fmt.Errorf("invalid genericRequest dSUID")
			}
			target = v
			body = body[n:]
		case 2:
			if typ != protowire.BytesType {
				return "", "", nil, fmt.Errorf("invalid genericRequest methodname field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return "", "", nil, fmt.Errorf("invalid genericRequest methodname")
			}
			method = v
			body = body[n:]
		case 3:
			if typ != protowire.BytesType {
				return "", "", nil, fmt.Errorf("invalid genericRequest params field")
			}
			b, n := protowire.ConsumeBytes(body)
			if n < 0 {
				return "", "", nil, fmt.Errorf("invalid genericRequest params")
			}
			e, err := parsePbufPropertyElement(b)
			if err != nil {
				return "", "", nil, err
			}
			params = append(params, e)
			body = body[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return "", "", nil, fmt.Errorf("invalid genericRequest field value")
			}
			body = body[n:]
		}
	}
	return target, method, params, nil
}

func parsePbufNotificationTargets(body []byte) (targets []string, err error) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, fmt.Errorf("invalid notification tag")
		}
		body = body[n:]
		if num == 1 {
			if typ != protowire.BytesType {
				return nil, fmt.Errorf("invalid notification dSUID field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return nil, fmt.Errorf("invalid notification dSUID")
			}
			targets = append(targets, v)
			body = body[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			return nil, fmt.Errorf("invalid notification field value")
		}
		body = body[n:]
	}
	return targets, nil
}

func parsePbufCallSceneNotification(body []byte) (targets []string, scene int, err error) {
	targets, err = parsePbufNotificationTargets(body)
	if err != nil {
		return nil, 0, err
	}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, 0, fmt.Errorf("invalid callScene tag")
		}
		body = body[n:]
		if num == 2 {
			if typ != protowire.VarintType {
				return nil, 0, fmt.Errorf("invalid callScene scene field")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, 0, fmt.Errorf("invalid callScene scene")
			}
			scene = int(v)
			body = body[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			return nil, 0, fmt.Errorf("invalid callScene field value")
		}
		body = body[n:]
	}
	return targets, scene, nil
}

func parsePbufSetControlValueNotification(body []byte) (targets []string, name string, value float64, hasValue bool, err error) {
	targets, err = parsePbufNotificationTargets(body)
	if err != nil {
		return nil, "", 0, false, err
	}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, "", 0, false, fmt.Errorf("invalid setControlValue tag")
		}
		body = body[n:]
		switch num {
		case 2:
			if typ != protowire.BytesType {
				return nil, "", 0, false, fmt.Errorf("invalid setControlValue name field")
			}
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return nil, "", 0, false, fmt.Errorf("invalid setControlValue name")
			}
			name = v
			body = body[n:]
		case 3:
			if typ != protowire.Fixed64Type {
				return nil, "", 0, false, fmt.Errorf("invalid setControlValue value field")
			}
			v, n := protowire.ConsumeFixed64(body)
			if n < 0 {
				return nil, "", 0, false, fmt.Errorf("invalid setControlValue value")
			}
			value = math.Float64frombits(v)
			hasValue = true
			body = body[n:]
		default:
			n = protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return nil, "", 0, false, fmt.Errorf("invalid setControlValue field value")
			}
			body = body[n:]
		}
	}
	return targets, name, value, hasValue, nil
}

func parsePbufDimChannelNotification(body []byte) (targets []string, mode int, err error) {
	targets, err = parsePbufNotificationTargets(body)
	if err != nil {
		return nil, 0, err
	}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, 0, fmt.Errorf("invalid dimChannel tag")
		}
		body = body[n:]
		if num == 3 {
			if typ != protowire.VarintType {
				return nil, 0, fmt.Errorf("invalid dimChannel mode field")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, 0, fmt.Errorf("invalid dimChannel mode")
			}
			mode = int(int64(v))
			body = body[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			return nil, 0, fmt.Errorf("invalid dimChannel field value")
		}
		body = body[n:]
	}
	return targets, mode, nil
}

func parsePbufSetOutputChannelValueNotification(body []byte) (targets []string, value float64, channelIndex int, applyNow bool, hasValue bool, err error) {
	applyNow = true
	targets, err = parsePbufNotificationTargets(body)
	if err != nil {
		return nil, 0, 0, true, false, err
	}
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue tag")
		}
		body = body[n:]
		switch num {
		case 2:
			if typ != protowire.VarintType {
				return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue apply_now field")
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue apply_now")
			}
			applyNow = v != 0
			body = body[n:]
		case 3:
			if typ != protowire.VarintType {
				n = protowire.ConsumeFieldValue(num, typ, body)
				if n < 0 {
					return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue channelIndex field")
				}
				body = body[n:]
				break
			}
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue channelIndex")
			}
			channelIndex = int(v)
			body = body[n:]
		case 4:
			if typ != protowire.Fixed64Type {
				return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue value field")
			}
			v, n := protowire.ConsumeFixed64(body)
			if n < 0 {
				return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue value")
			}
			value = math.Float64frombits(v)
			hasValue = true
			body = body[n:]
		default:
			n = protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return nil, 0, 0, true, false, fmt.Errorf("invalid setOutputChannelValue field value")
			}
			body = body[n:]
		}
	}
	return targets, value, channelIndex, applyNow, hasValue, nil
}

func extractBrightnessFromPbufProperties(props []pbufPropertyElement) (float64, bool) {
	for _, p := range props {
		if strings.EqualFold(p.Name, "channelStates") {
			for _, child := range p.Elements {
				if strings.TrimSpace(child.Name) == "0" {
					for _, grand := range child.Elements {
						if strings.EqualFold(grand.Name, "value") {
							if v, ok := asFloat(grand.Value); ok {
								return v, true
							}
						}
					}
				}
			}
		}
	}
	return 0, false
}

// extractSceneWriteFromPbuf extracts the first scene property write from pbuf properties.
// Expected tree: scenes → {sceneNum} → channels → {channelIdx} → value/dontCare
// or: scenes → {sceneNum} → effect/dontCare/ignoreLocalPriority
func extractSceneWriteFromPbuf(props []pbufPropertyElement) (sceneWrite, bool) {
	for _, p := range props {
		if !strings.EqualFold(p.Name, "scenes") {
			continue
		}
		for _, sceneElem := range p.Elements {
			sceneNum, err := strconv.Atoi(strings.TrimSpace(sceneElem.Name))
			if err != nil {
				continue
			}
			for _, field := range sceneElem.Elements {
				switch strings.ToLower(strings.TrimSpace(field.Name)) {
				case "channels":
					for _, chElem := range field.Elements {
						chIdx, err := strconv.Atoi(strings.TrimSpace(chElem.Name))
						if err != nil {
							continue
						}
						for _, vElem := range chElem.Elements {
							switch strings.ToLower(strings.TrimSpace(vElem.Name)) {
							case "value":
								if f, ok := asFloat(vElem.Value); ok {
									return sceneWrite{SceneNum: sceneNum, ChannelIdx: chIdx, Field: "channelValue", FloatVal: f}, true
								}
							case "dontcare":
								return sceneWrite{SceneNum: sceneNum, ChannelIdx: chIdx, Field: "channelDontCare", BoolVal: asBool(vElem.Value)}, true
							}
						}
					}
				case "effect":
					if f, ok := asFloat(field.Value); ok {
						return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "effect", IntVal: int(f)}, true
					}
				case "dontcare":
					return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "dontCare", BoolVal: asBool(field.Value)}, true
				case "ignorelocalpriority":
					return sceneWrite{SceneNum: sceneNum, ChannelIdx: -1, Field: "ignoreLocalPriority", BoolVal: asBool(field.Value)}, true
				}
			}
		}
	}
	return sceneWrite{}, false
}

// extractInputSettingFromPbuf extracts the first input/sensor setting write from pbuf props.
// Expected tree: buttonInputSettings → {idx} → setsLocalPriority/callsPresent
//
//	binaryInputSettings → {idx} → sensorFunction
//	sensorSettings      → {idx} → group/function/channel/minPushInterval/changesOnlyInterval
func extractInputSettingFromPbuf(props []pbufPropertyElement) (inputSettingWrite, bool) {
	for _, p := range props {
		pLow := strings.ToLower(strings.TrimSpace(p.Name))
		var family string
		switch pLow {
		case "buttoninputsettings":
			family = "button"
		case "binaryinputsettings":
			family = "binaryinput"
		case "sensorsettings":
			family = "sensor"
		default:
			continue
		}
		for _, idxElem := range p.Elements {
			idx, err := strconv.Atoi(strings.TrimSpace(idxElem.Name))
			if err != nil {
				continue
			}
			for _, fieldElem := range idxElem.Elements {
				if isw, ok := makeInputSettingWrite(family, idx, fieldElem.Name, fieldElem.Value); ok {
					return isw, true
				}
			}
		}
	}
	return inputSettingWrite{}, false
}

func parsePbufPingDSUID(body []byte) string {
	return parsePbufDSUIDField(body)
}

func parsePbufDSUIDField(body []byte) string {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return ""
		}
		body = body[n:]
		if num == 1 && typ == protowire.BytesType {
			v, n := protowire.ConsumeString(body)
			if n < 0 {
				return ""
			}
			return v
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			return ""
		}
		body = body[n:]
	}
	return ""
}

// parsePbufGenericResponseCode extracts (code, description, errorType) from the
// GenericResponse sub-message (field 3 of the outer Message envelope).
// Per messages.proto: field 1 = required ResultCode code, field 2 = optional string description.
func parsePbufGenericResponseCode(body []byte) (code uint64, description string, errorType uint64) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return 0, "", 0
		}
		body = body[n:]
		switch num {
		case 1:
			if typ == protowire.VarintType {
				v, n := protowire.ConsumeVarint(body)
				if n > 0 {
					code = v
				}
				body = body[max(n, 1):]
			}
		case 2:
			if typ == protowire.BytesType {
				v, n := protowire.ConsumeString(body)
				if n > 0 {
					description = v
				}
				body = body[max(n, 1):]
			}
		case 3:
			if typ == protowire.VarintType {
				v, n := protowire.ConsumeVarint(body)
				if n > 0 {
					errorType = v
				}
				body = body[max(n, 1):]
			}
		default:
			n = protowire.ConsumeFieldValue(num, typ, body)
			if n < 0 {
				return code, description, errorType
			}
			body = body[n:]
		}
	}
	return code, description, errorType
}
