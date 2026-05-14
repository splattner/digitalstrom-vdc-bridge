package vdcapi

import (
	"encoding/binary"
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

func buildPbufPropertyResult(query []pbufPropertyElement, full map[string]any) []pbufPropertyElement {
	if len(query) == 0 {
		res := make([]pbufPropertyElement, 0, len(full))
		for k, v := range full {
			res = append(res, convertValueToProperty(k, v, nil))
		}
		return res
	}
	res := make([]pbufPropertyElement, 0, len(query))
	for _, q := range query {
		name := q.Name
		// dSS sentinels for "all properties at this level":
		//   ""  – missing/empty name
		//   " " – single space (preserved verbatim, NOT TrimSpace'd)
		//   "*" – wildcard
		if name == "" || name == " " || name == "*" {
			for k, v := range full {
				res = append(res, convertValueToProperty(k, v, nil))
			}
			continue
		}
		if v, ok := full[name]; ok {
			res = append(res, convertValueToProperty(name, v, q.Elements))
		}
	}
	return res
}

func convertValueToProperty(name string, value any, queryChildren []pbufPropertyElement) pbufPropertyElement {
	e := pbufPropertyElement{Name: name}
	switch v := value.(type) {
	case map[string]any:
		if len(queryChildren) == 0 {
			for k, subv := range v {
				e.Elements = append(e.Elements, convertValueToProperty(k, subv, nil))
			}
			return e
		}
		for _, q := range queryChildren {
			qn := q.Name
			if qn == "" || qn == " " || qn == "*" {
				for k, subv := range v {
					e.Elements = append(e.Elements, convertValueToProperty(k, subv, nil))
				}
				continue
			}
			if subv, ok := v[qn]; ok {
				e.Elements = append(e.Elements, convertValueToProperty(qn, subv, q.Elements))
			}
		}
		return e
	case []any:
		for i := range v {
			e.Elements = append(e.Elements, convertValueToProperty(fmt.Sprintf("%d", i), v[i], nil))
		}
		return e
	case []string:
		arr := make([]any, 0, len(v))
		for i := range v {
			arr = append(arr, v[i])
		}
		return convertValueToProperty(name, arr, queryChildren)
	default:
		e.Value = value
		return e
	}
}

func buildPbufFrame(payload []byte) []byte {
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame[:2], uint16(len(payload)))
	copy(frame[2:], payload)
	return frame
}

func buildPbufGenericResponse(msgID uint32, hasID bool, code uint64, description string) []byte {
	resp := make([]byte, 0, 128)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeGenericResponse)
	if hasID {
		resp = protowire.AppendTag(resp, 2, protowire.VarintType)
		resp = protowire.AppendVarint(resp, uint64(msgID))
	}
	gen := make([]byte, 0, 64)
	gen = protowire.AppendTag(gen, 1, protowire.VarintType)
	gen = protowire.AppendVarint(gen, code)
	if description != "" {
		gen = protowire.AppendTag(gen, 2, protowire.BytesType)
		gen = protowire.AppendString(gen, description)
	}
	resp = protowire.AppendTag(resp, 3, protowire.BytesType)
	resp = protowire.AppendBytes(resp, gen)
	return buildPbufFrame(resp)
}

func buildPbufHelloResponse(msgID uint32, hasID bool, dsuid string) []byte {
	resp := make([]byte, 0, 128)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcResponseHello)
	if hasID {
		resp = protowire.AppendTag(resp, 2, protowire.VarintType)
		resp = protowire.AppendVarint(resp, uint64(msgID))
	}
	sub := make([]byte, 0, 64)
	sub = protowire.AppendTag(sub, 1, protowire.BytesType)
	sub = protowire.AppendString(sub, dsuid)
	resp = protowire.AppendTag(resp, 101, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func buildPbufGetPropertyResponse(msgID uint32, hasID bool, props []pbufPropertyElement) []byte {
	resp := make([]byte, 0, 512)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcResponseGetProp)
	if hasID {
		resp = protowire.AppendTag(resp, 2, protowire.VarintType)
		resp = protowire.AppendVarint(resp, uint64(msgID))
	}
	getProp := make([]byte, 0, 512)
	for i := range props {
		pe := encodePropertyElement(props[i])
		getProp = protowire.AppendTag(getProp, 1, protowire.BytesType)
		getProp = protowire.AppendBytes(getProp, pe)
	}
	resp = protowire.AppendTag(resp, 103, protowire.BytesType)
	resp = protowire.AppendBytes(resp, getProp)
	return buildPbufFrame(resp)
}

func buildPbufPong(dsuid string) []byte {
	resp := make([]byte, 0, 96)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcSendPong)
	sub := make([]byte, 0, 48)
	if dsuid != "" {
		sub = protowire.AppendTag(sub, 1, protowire.BytesType)
		sub = protowire.AppendString(sub, dsuid)
	}
	resp = protowire.AppendTag(resp, 106, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func buildPbufAnnounceVdc(vdcDSUID string) []byte {
	resp := make([]byte, 0, 96)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcSendAnnounceVdc)
	sub := make([]byte, 0, 48)
	if vdcDSUID != "" {
		sub = protowire.AppendTag(sub, 1, protowire.BytesType)
		sub = protowire.AppendString(sub, vdcDSUID)
	}
	resp = protowire.AppendTag(resp, 120, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func buildPbufAnnounceDevice(deviceDSUID, vdcDSUID string) []byte {
	resp := make([]byte, 0, 128)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcSendAnnounceDevice)
	sub := make([]byte, 0, 64)
	if deviceDSUID != "" {
		sub = protowire.AppendTag(sub, 1, protowire.BytesType)
		sub = protowire.AppendString(sub, deviceDSUID)
	}
	if vdcDSUID != "" {
		sub = protowire.AppendTag(sub, 2, protowire.BytesType)
		sub = protowire.AppendString(sub, vdcDSUID)
	}
	resp = protowire.AppendTag(resp, 107, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func buildPbufVanish(deviceDSUID string) []byte {
	resp := make([]byte, 0, 96)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcSendVanish)
	sub := make([]byte, 0, 48)
	if deviceDSUID != "" {
		sub = protowire.AppendTag(sub, 1, protowire.BytesType)
		sub = protowire.AppendString(sub, deviceDSUID)
	}
	resp = protowire.AppendTag(resp, 108, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func buildPbufIdentify(dsuid string) []byte {
	resp := make([]byte, 0, 96)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcSendIdentify)
	sub := make([]byte, 0, 48)
	if dsuid != "" {
		sub = protowire.AppendTag(sub, 1, protowire.BytesType)
		sub = protowire.AppendString(sub, dsuid)
	}
	resp = protowire.AppendTag(resp, 119, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func buildPbufPushNotification(deviceDSUID string, changed map[string]any) []byte {
	resp := make([]byte, 0, 256)
	resp = protowire.AppendTag(resp, 1, protowire.VarintType)
	resp = protowire.AppendVarint(resp, pbufTypeVdcSendPushNotification)
	sub := make([]byte, 0, 192)
	if deviceDSUID != "" {
		sub = protowire.AppendTag(sub, 1, protowire.BytesType)
		sub = protowire.AppendString(sub, deviceDSUID)
	}
	// field 2 = changedproperties, field 3 = deviceevents (per vdcapi.proto)
	for k, v := range changed {
		if k == "deviceevents" {
			continue
		}
		pe := encodePropertyElement(convertValueToProperty(k, v, nil))
		sub = protowire.AppendTag(sub, 2, protowire.BytesType)
		sub = protowire.AppendBytes(sub, pe)
	}
	if events, ok := changed["deviceevents"].([]any); ok {
		for _, ev := range events {
			pe := encodePropertyElement(convertValueToProperty("deviceevent", ev, nil))
			sub = protowire.AppendTag(sub, 3, protowire.BytesType)
			sub = protowire.AppendBytes(sub, pe)
		}
	}
	resp = protowire.AppendTag(resp, 109, protowire.BytesType)
	resp = protowire.AppendBytes(resp, sub)
	return buildPbufFrame(resp)
}

func encodePropertyElement(e pbufPropertyElement) []byte {
	out := make([]byte, 0, 128)
	if e.Name != "" {
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendString(out, e.Name)
	}
	if e.Value != nil {
		pv := encodePropertyValue(e.Value)
		if len(pv) > 0 {
			out = protowire.AppendTag(out, 2, protowire.BytesType)
			out = protowire.AppendBytes(out, pv)
		}
	}
	for i := range e.Elements {
		child := encodePropertyElement(e.Elements[i])
		out = protowire.AppendTag(out, 3, protowire.BytesType)
		out = protowire.AppendBytes(out, child)
	}
	return out
}

func encodePropertyValue(v any) []byte {
	out := make([]byte, 0, 64)
	switch x := v.(type) {
	case bool:
		out = protowire.AppendTag(out, 1, protowire.VarintType)
		if x {
			out = protowire.AppendVarint(out, 1)
		} else {
			out = protowire.AppendVarint(out, 0)
		}
	case uint64:
		out = protowire.AppendTag(out, 2, protowire.VarintType)
		out = protowire.AppendVarint(out, x)
	case int64:
		// Match C++ vdcd convention: many integer device properties (mode, groups,
		// primaryGroup, zoneID, channelType, dsIndex, …) are declared apivalue_uint64.
		// DSS reads v_uint64 (field 2) and ignores v_int64 (field 3) for those, so
		// emit non-negative ints as v_uint64 to be compatible with both signedness
		// expectations.
		if x >= 0 {
			out = protowire.AppendTag(out, 2, protowire.VarintType)
			out = protowire.AppendVarint(out, uint64(x))
		} else {
			out = protowire.AppendTag(out, 3, protowire.VarintType)
			out = protowire.AppendVarint(out, uint64(x))
		}
	case int:
		if x >= 0 {
			out = protowire.AppendTag(out, 2, protowire.VarintType)
			out = protowire.AppendVarint(out, uint64(x))
		} else {
			out = protowire.AppendTag(out, 3, protowire.VarintType)
			out = protowire.AppendVarint(out, uint64(int64(x)))
		}
	case float64:
		out = protowire.AppendTag(out, 4, protowire.Fixed64Type)
		out = protowire.AppendFixed64(out, math.Float64bits(x))
	case string:
		out = protowire.AppendTag(out, 5, protowire.BytesType)
		out = protowire.AppendString(out, x)
	case []byte:
		out = protowire.AppendTag(out, 6, protowire.BytesType)
		out = protowire.AppendBytes(out, x)
	}
	return out
}
