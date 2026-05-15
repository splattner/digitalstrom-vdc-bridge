package vdcapi

import (
	"math"
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestProcessPbufHello(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{}

	req := make([]byte, 0, 64)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestHello)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 7)
	hello := make([]byte, 0, 32)
	hello = protowire.AppendTag(hello, 1, protowire.BytesType)
	hello = protowire.AppendString(hello, "001122")
	hello = protowire.AppendTag(hello, 2, protowire.VarintType)
	hello = protowire.AppendVarint(hello, APIVersionMax)
	req = protowire.AppendTag(req, 100, protowire.BytesType)
	req = protowire.AppendBytes(req, hello)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("hello must not close connection")
	}
	// hello response + announceVdc; no devices registered so no announceDevice frames
	if len(frames) != 2 {
		t.Fatalf("expected two response frames (hello + announceVdc), got %d", len(frames))
	}
	if !sess.active {
		t.Fatal("expected active session")
	}

	msgType, msgID, hasID, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeVdcResponseHello {
		t.Fatalf("unexpected response type %d", msgType)
	}
	if !hasID || msgID != 7 {
		t.Fatalf("unexpected response id has=%t id=%d", hasID, msgID)
	}
	dsuid, _, err := parsePbufHelloRequest(sub[101])
	if err != nil {
		t.Fatalf("parse hello response body: %v", err)
	}
	if dsuid != s.DSUID {
		t.Fatalf("unexpected response dSUID %q", dsuid)
	}

	announceVdcType, _, _, _, err := parsePbufEnvelope(frames[1][2:])
	if err != nil {
		t.Fatalf("parse announce-vdc envelope: %v", err)
	}
	if announceVdcType != pbufTypeVdcSendAnnounceVdc {
		t.Fatalf("unexpected announce-vdc type %d", announceVdcType)
	}
}

// TestProcessPbufHelloVanishesStaleDevices verifies that when a vDC reconnects,
// devices that DSS knew about but are no longer active are vanished.
func TestProcessPbufHelloVanishesStaleDevices(t *testing.T) {
	cfg := NewConfigStore()
	// Simulate a stale DSUID from a previous session.
	staleID := "DEADBEEF01234567ABCDEF0123456789AA"
	cfg.MarkDSUIDAdded(staleID)

	s := &PbufServer{ServerConfig: ServerConfig{
		DSUID:       "0123456789ABCDEFFEDCBA9876543210AA",
		Description: "test",
		Config:      cfg,
	}}
	sess := &session{}

	req := make([]byte, 0, 64)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestHello)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 11)
	hello := make([]byte, 0, 32)
	hello = protowire.AppendTag(hello, 1, protowire.BytesType)
	hello = protowire.AppendString(hello, "001122")
	hello = protowire.AppendTag(hello, 2, protowire.VarintType)
	hello = protowire.AppendVarint(hello, APIVersionMax)
	req = protowire.AppendTag(req, 100, protowire.BytesType)
	req = protowire.AppendBytes(req, hello)

	frames, _ := s.processPbufMessage(req, sess)
	// Expect: hello response + announceVdc + vanish for stale device.
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames (hello + announceVdc + vanish), got %d", len(frames))
	}
	vanishType, _, _, _, err := parsePbufEnvelope(frames[2][2:])
	if err != nil {
		t.Fatalf("parse vanish envelope: %v", err)
	}
	if vanishType != pbufTypeVdcSendVanish {
		t.Fatalf("expected vanish frame type %d, got %d", pbufTypeVdcSendVanish, vanishType)
	}
}

func TestProcessPbufHelloVersionCompatibilityMatrix(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}

	tests := []struct {
		name             string
		includeVersion   bool
		version          int
		wantCode         uint64
		wantSessionAlive bool
	}{
		{
			name:             "missing version falls back to incompatible",
			includeVersion:   false,
			wantCode:         pbufResultIncompatibleAPI,
			wantSessionAlive: false,
		},
		{
			name:             "below minimum version",
			includeVersion:   true,
			version:          APIVersionMin - 1,
			wantCode:         pbufResultIncompatibleAPI,
			wantSessionAlive: false,
		},
		{
			name:             "minimum version",
			includeVersion:   true,
			version:          APIVersionMin,
			wantCode:         pbufResultOK,
			wantSessionAlive: true,
		},
		{
			name:             "maximum version",
			includeVersion:   true,
			version:          APIVersionMax,
			wantCode:         pbufResultOK,
			wantSessionAlive: true,
		},
		{
			name:             "above maximum version",
			includeVersion:   true,
			version:          APIVersionMax + 1,
			wantCode:         pbufResultIncompatibleAPI,
			wantSessionAlive: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sess := &session{}
			frames, closeAfter := s.processPbufMessage(buildPbufHelloRequest(t, tc.includeVersion, tc.version), sess)
			if closeAfter {
				t.Fatal("hello must not close connection")
			}
			if len(frames) == 0 {
				t.Fatal("expected at least one response frame")
			}

			msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
			if err != nil {
				t.Fatalf("parse response envelope: %v", err)
			}

			if tc.wantCode == pbufResultOK {
				if msgType != pbufTypeVdcResponseHello {
					t.Fatalf("expected hello response type, got %d", msgType)
				}
			} else {
				if msgType != pbufTypeGenericResponse {
					t.Fatalf("expected generic response type, got %d", msgType)
				}
				if code := parsePbufGenericCode(sub[3]); code != tc.wantCode {
					t.Fatalf("unexpected error code: got=%d want=%d", code, tc.wantCode)
				}
			}
			if sess.active != tc.wantSessionAlive {
				t.Fatalf("unexpected session active state: got=%t want=%t", sess.active, tc.wantSessionAlive)
			}
		})
	}
}

func buildPbufHelloRequest(t *testing.T, includeVersion bool, version int) []byte {
	t.Helper()

	req := make([]byte, 0, 64)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestHello)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 7)

	hello := make([]byte, 0, 32)
	hello = protowire.AppendTag(hello, 1, protowire.BytesType)
	hello = protowire.AppendString(hello, "001122")
	if includeVersion {
		hello = protowire.AppendTag(hello, 2, protowire.VarintType)
		hello = protowire.AppendVarint(hello, uint64(version))
	}

	req = protowire.AppendTag(req, 100, protowire.BytesType)
	req = protowire.AppendBytes(req, hello)
	return req
}

func TestProcessPbufRejectsGetPropertyWithoutSession(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{}

	req := make([]byte, 0, 80)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 9)
	body := make([]byte, 0, 32)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, "root")
	req = protowire.AppendTag(req, 102, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}

	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("unexpected response type %d", msgType)
	}
	code := parsePbufGenericCode(sub[3])
	if code != pbufResultNotAuthorized {
		t.Fatalf("expected not authorized code, got %d", code)
	}
}

func TestProcessPbufGetPropertyQueryReturnsSampleDevice(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	queryDevice := make([]byte, 0, 64)
	queryDevice = protowire.AppendTag(queryDevice, 1, protowire.BytesType)
	queryDevice = protowire.AppendString(queryDevice, "*")
	childDsuid := make([]byte, 0, 16)
	childDsuid = protowire.AppendTag(childDsuid, 1, protowire.BytesType)
	childDsuid = protowire.AppendString(childDsuid, "dSUID")
	queryDevice = protowire.AppendTag(queryDevice, 3, protowire.BytesType)
	queryDevice = protowire.AppendBytes(queryDevice, childDsuid)
	childBridgeable := make([]byte, 0, 32)
	childBridgeable = protowire.AppendTag(childBridgeable, 1, protowire.BytesType)
	childBridgeable = protowire.AppendString(childBridgeable, "active")
	queryDevice = protowire.AppendTag(queryDevice, 3, protowire.BytesType)
	queryDevice = protowire.AppendBytes(queryDevice, childBridgeable)

	queryDevices := make([]byte, 0, 96)
	queryDevices = protowire.AppendTag(queryDevices, 1, protowire.BytesType)
	queryDevices = protowire.AppendString(queryDevices, "devices")
	queryDevices = protowire.AppendTag(queryDevices, 3, protowire.BytesType)
	queryDevices = protowire.AppendBytes(queryDevices, queryDevice)

	queryVdcAny := make([]byte, 0, 96)
	queryVdcAny = protowire.AppendTag(queryVdcAny, 1, protowire.BytesType)
	queryVdcAny = protowire.AppendString(queryVdcAny, "*")
	queryVdcAny = protowire.AppendTag(queryVdcAny, 3, protowire.BytesType)
	queryVdcAny = protowire.AppendBytes(queryVdcAny, queryDevices)

	queryVdcs := make([]byte, 0, 96)
	queryVdcs = protowire.AppendTag(queryVdcs, 1, protowire.BytesType)
	queryVdcs = protowire.AppendString(queryVdcs, "vdcs")
	queryVdcs = protowire.AppendTag(queryVdcs, 3, protowire.BytesType)
	queryVdcs = protowire.AppendBytes(queryVdcs, queryVdcAny)

	reqBody := make([]byte, 0, 192)
	reqBody = protowire.AppendTag(reqBody, 1, protowire.BytesType)
	reqBody = protowire.AppendString(reqBody, "root")
	reqBody = protowire.AppendTag(reqBody, 2, protowire.BytesType)
	reqBody = protowire.AppendBytes(reqBody, queryVdcs)

	req := make([]byte, 0, 256)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 10)
	req = protowire.AppendTag(req, 102, protowire.BytesType)
	req = protowire.AppendBytes(req, reqBody)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeVdcResponseGetProp {
		t.Fatalf("unexpected response type %d", msgType)
	}
	if len(sub[103]) == 0 {
		t.Fatal("expected getProperty response body")
	}
}

func TestProcessPbufSetPropertyDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	valueProp := make([]byte, 0, 32)
	valueProp = protowire.AppendTag(valueProp, 1, protowire.BytesType)
	valueProp = protowire.AppendString(valueProp, "value")
	valueField := make([]byte, 0, 16)
	valueField = protowire.AppendTag(valueField, 4, protowire.Fixed64Type)
	valueField = protowire.AppendFixed64(valueField, math.Float64bits(66.0))
	valueProp = protowire.AppendTag(valueProp, 2, protowire.BytesType)
	valueProp = protowire.AppendBytes(valueProp, valueField)

	idxProp := make([]byte, 0, 64)
	idxProp = protowire.AppendTag(idxProp, 1, protowire.BytesType)
	idxProp = protowire.AppendString(idxProp, "0")
	idxProp = protowire.AppendTag(idxProp, 3, protowire.BytesType)
	idxProp = protowire.AppendBytes(idxProp, valueProp)

	chProp := make([]byte, 0, 96)
	chProp = protowire.AppendTag(chProp, 1, protowire.BytesType)
	chProp = protowire.AppendString(chProp, "channelStates")
	chProp = protowire.AppendTag(chProp, 3, protowire.BytesType)
	chProp = protowire.AppendBytes(chProp, idxProp)

	body := make([]byte, 0, 192)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendBytes(body, chProp)

	req := make([]byte, 0, 256)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestSetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 11)
	req = protowire.AppendTag(req, 104, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 66.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=66, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufSetPropertyRejectsNonLight(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	valueProp := make([]byte, 0, 32)
	valueProp = protowire.AppendTag(valueProp, 1, protowire.BytesType)
	valueProp = protowire.AppendString(valueProp, "value")
	valueField := make([]byte, 0, 16)
	valueField = protowire.AppendTag(valueField, 4, protowire.Fixed64Type)
	valueField = protowire.AppendFixed64(valueField, math.Float64bits(66.0))
	valueProp = protowire.AppendTag(valueProp, 2, protowire.BytesType)
	valueProp = protowire.AppendBytes(valueProp, valueField)

	idxProp := make([]byte, 0, 64)
	idxProp = protowire.AppendTag(idxProp, 1, protowire.BytesType)
	idxProp = protowire.AppendString(idxProp, "0")
	idxProp = protowire.AppendTag(idxProp, 3, protowire.BytesType)
	idxProp = protowire.AppendBytes(idxProp, valueProp)

	chProp := make([]byte, 0, 96)
	chProp = protowire.AppendTag(chProp, 1, protowire.BytesType)
	chProp = protowire.AppendString(chProp, "channelStates")
	chProp = protowire.AppendTag(chProp, 3, protowire.BytesType)
	chProp = protowire.AppendBytes(chProp, idxProp)

	body := make([]byte, 0, 192)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendBytes(body, chProp)

	req := make([]byte, 0, 256)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestSetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 19)
	req = protowire.AppendTag(req, 104, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultForbidden {
		t.Fatalf("expected forbidden response, got %d", parsePbufGenericCode(sub[3]))
	}
	if mc.called {
		t.Fatal("did not expect commander call for non-light setProperty")
	}
}

func TestProcessPbufSetPropertyRejectsRootTarget(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", Commander: &mockCommander{}}}

	props := []pbufPropertyElement{
		{
			Name: "channelStates",
			Elements: []pbufPropertyElement{
				{
					Name: "0",
					Elements: []pbufPropertyElement{
						{Name: "value", Value: 66.0},
					},
				},
			},
		},
	}

	frames := s.handlePbufSetProperty(23, true, "root", props)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultMissingData {
		t.Fatalf("expected missing-data response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestUnknownMethod(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	param := make([]byte, 0, 24)
	param = protowire.AppendTag(param, 1, protowire.BytesType)
	param = protowire.AppendString(param, "foo")

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "customMethod")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, param)

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 12)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultNotImplemented {
		t.Fatalf("expected not-implemented generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if msg := parsePbufGenericDescription(sub[3]); msg != "unknown notification 'customMethod'" {
		t.Fatalf("unexpected generic response message: %q", msg)
	}
}

func TestProcessPbufGenericRequestScanDevicesOK(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "scanDevices")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 14)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestPairTimeoutAbort(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramTimeout := make([]byte, 0, 32)
	paramTimeout = protowire.AppendTag(paramTimeout, 1, protowire.BytesType)
	paramTimeout = protowire.AppendString(paramTimeout, "timeout")
	paramTimeoutValue := make([]byte, 0, 16)
	paramTimeoutValue = protowire.AppendTag(paramTimeoutValue, 3, protowire.VarintType)
	paramTimeoutValue = protowire.AppendVarint(paramTimeoutValue, 0)
	paramTimeout = protowire.AppendTag(paramTimeout, 2, protowire.BytesType)
	paramTimeout = protowire.AppendBytes(paramTimeout, paramTimeoutValue)

	body := make([]byte, 0, 192)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "pair")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramTimeout)

	req := make([]byte, 0, 256)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 20)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultNotFound {
		t.Fatalf("expected not found code, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestScanDevicesInvalidParamType(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramExhaustive := make([]byte, 0, 32)
	paramExhaustive = protowire.AppendTag(paramExhaustive, 1, protowire.BytesType)
	paramExhaustive = protowire.AppendString(paramExhaustive, "exhaustive")
	paramExhaustiveValue := make([]byte, 0, 32)
	paramExhaustiveValue = protowire.AppendTag(paramExhaustiveValue, 5, protowire.BytesType)
	paramExhaustiveValue = protowire.AppendString(paramExhaustiveValue, "bad")
	paramExhaustive = protowire.AppendTag(paramExhaustive, 2, protowire.BytesType)
	paramExhaustive = protowire.AppendBytes(paramExhaustive, paramExhaustiveValue)

	body := make([]byte, 0, 192)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "scanDevices")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramExhaustive)

	req := make([]byte, 0, 256)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 21)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultInvalidValueType {
		t.Fatalf("expected invalid value type code, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestRecursiveRejected(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "genericRequest")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 15)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultInvalidValueType {
		t.Fatalf("expected invalid value type generic response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestPingReturnsStatusAndPong(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "ping")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 18)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 2 {
		t.Fatalf("expected two response frames, got %d", len(frames))
	}

	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse generic response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}

	pongType, _, _, pongSub, err := parsePbufEnvelope(frames[1][2:])
	if err != nil {
		t.Fatalf("parse pong envelope: %v", err)
	}
	if pongType != pbufTypeVdcSendPong {
		t.Fatalf("expected pong response, got %d", pongType)
	}
	if got := parsePbufDSUIDField(pongSub[106]); got != s.DSUID {
		t.Fatalf("expected pong dSUID %q, got %q", s.DSUID, got)
	}
}

func TestProcessPbufGenericRequestPingUnknownTargetNoPong(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "ping")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 183)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse generic response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultNotFound {
		t.Fatalf("expected not-found generic response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestPingKnownDeviceReturnsPong(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "ping")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 184)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 2 {
		t.Fatalf("expected two response frames, got %d", len(frames))
	}
	pongType, _, _, pongSub, err := parsePbufEnvelope(frames[1][2:])
	if err != nil {
		t.Fatalf("parse pong envelope: %v", err)
	}
	if pongType != pbufTypeVdcSendPong {
		t.Fatalf("expected pong response, got %d", pongType)
	}
	if got := parsePbufDSUIDField(pongSub[106]); got != target {
		t.Fatalf("expected pong dSUID %q, got %q", target, got)
	}
}

func TestProcessPbufDirectPingUnknownTargetNoPong(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{}

	pingBody := make([]byte, 0, 32)
	pingBody = protowire.AppendTag(pingBody, 1, protowire.BytesType)
	pingBody = protowire.AppendString(pingBody, "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")

	req := make([]byte, 0, 80)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmSendPing)
	req = protowire.AppendTag(req, 105, protowire.BytesType)
	req = protowire.AppendBytes(req, pingBody)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("ping must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("expected no pong frame for unknown target, got %d", len(frames))
	}
}

func TestProcessPbufGenericRequestLogLevelRequiresValue(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "loglevel")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 180)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultMessageUnknown {
		t.Fatalf("expected message unknown response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestLogLevelRejectsRange(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramValue := make([]byte, 0, 32)
	paramValue = protowire.AppendTag(paramValue, 1, protowire.BytesType)
	paramValue = protowire.AppendString(paramValue, "value")
	paramValueField := make([]byte, 0, 16)
	paramValueField = protowire.AppendTag(paramValueField, 3, protowire.VarintType)
	paramValueField = protowire.AppendVarint(paramValueField, 9)
	paramValue = protowire.AppendTag(paramValue, 2, protowire.BytesType)
	paramValue = protowire.AppendBytes(paramValue, paramValueField)

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "loglevel")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramValue)

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 181)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultMessageUnknown {
		t.Fatalf("expected message unknown response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestLogLevelOffsetRejectsUnknownTopic(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramValue := make([]byte, 0, 32)
	paramValue = protowire.AppendTag(paramValue, 1, protowire.BytesType)
	paramValue = protowire.AppendString(paramValue, "value")
	paramValueField := make([]byte, 0, 16)
	paramValueField = protowire.AppendTag(paramValueField, 3, protowire.VarintType)
	paramValueField = protowire.AppendVarint(paramValueField, 2)
	paramValue = protowire.AppendTag(paramValue, 2, protowire.BytesType)
	paramValue = protowire.AppendBytes(paramValue, paramValueField)

	paramTopic := make([]byte, 0, 32)
	paramTopic = protowire.AppendTag(paramTopic, 1, protowire.BytesType)
	paramTopic = protowire.AppendString(paramTopic, "topic")
	paramTopicField := make([]byte, 0, 16)
	paramTopicField = protowire.AppendTag(paramTopicField, 5, protowire.BytesType)
	paramTopicField = protowire.AppendString(paramTopicField, "foo")
	paramTopic = protowire.AppendTag(paramTopic, 2, protowire.BytesType)
	paramTopic = protowire.AppendBytes(paramTopic, paramTopicField)

	body := make([]byte, 0, 160)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "logleveloffset")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramValue)
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramTopic)

	req := make([]byte, 0, 224)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 182)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultMessageUnknown {
		t.Fatalf("expected message unknown response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufGenericRequestIdentifyInvalidDuration(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramDuration := make([]byte, 0, 32)
	paramDuration = protowire.AppendTag(paramDuration, 1, protowire.BytesType)
	paramDuration = protowire.AppendString(paramDuration, "duration")
	paramDurationField := make([]byte, 0, 16)
	paramDurationField = protowire.AppendTag(paramDurationField, 5, protowire.BytesType)
	paramDurationField = protowire.AppendString(paramDurationField, "abc")
	paramDuration = protowire.AppendTag(paramDuration, 2, protowire.BytesType)
	paramDuration = protowire.AppendBytes(paramDuration, paramDurationField)

	body := make([]byte, 0, 160)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "identify")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramDuration)

	req := make([]byte, 0, 224)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 185)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultInvalidValueType {
		t.Fatalf("expected invalid value type response, got %d", parsePbufGenericCode(sub[3]))
	}
	if msg := parsePbufGenericDescription(sub[3]); msg != "invalid duration: must be numeric" {
		t.Fatalf("unexpected generic response message: %q", msg)
	}
}

func TestProcessPbufGenericRequestIdentifyValidDuration(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramDuration := make([]byte, 0, 32)
	paramDuration = protowire.AppendTag(paramDuration, 1, protowire.BytesType)
	paramDuration = protowire.AppendString(paramDuration, "duration")
	paramDurationField := make([]byte, 0, 16)
	paramDurationField = protowire.AppendTag(paramDurationField, 3, protowire.VarintType)
	paramDurationField = protowire.AppendVarint(paramDurationField, 12)
	paramDuration = protowire.AppendTag(paramDuration, 2, protowire.BytesType)
	paramDuration = protowire.AppendBytes(paramDuration, paramDurationField)

	body := make([]byte, 0, 160)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "identify")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramDuration)

	req := make([]byte, 0, 224)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 186)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	// Expect genericResponse + vdc_SendIdentify (one per target)
	if len(frames) < 1 {
		t.Fatalf("expected at least one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok response, got %d", parsePbufGenericCode(sub[3]))
	}
	if len(frames) < 2 {
		t.Fatalf("expected vdc_SendIdentify frame, got only %d frames", len(frames))
	}
	identType, _, _, _, err := parsePbufEnvelope(frames[1][2:])
	if err != nil {
		t.Fatalf("parse identify frame: %v", err)
	}
	if identType != pbufTypeVdcSendIdentify {
		t.Fatalf("expected vdc_SendIdentify (type %d), got %d", pbufTypeVdcSendIdentify, identType)
	}
}

func TestProcessPbufGenericRequestIdentifyUnknownTarget(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "identify")

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 187)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultNotFound {
		t.Fatalf("expected not-found response, got %d", parsePbufGenericCode(sub[3]))
	}
	if msg := parsePbufGenericDescription(sub[3]); msg != "addressable not found" {
		t.Fatalf("unexpected generic response message: %q", msg)
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramTarget := make([]byte, 0, 32)
	paramTarget = protowire.AppendTag(paramTarget, 1, protowire.BytesType)
	paramTarget = protowire.AppendString(paramTarget, "dSUID")
	paramTargetValue := make([]byte, 0, 32)
	paramTargetValue = protowire.AppendTag(paramTargetValue, 5, protowire.BytesType)
	paramTargetValue = protowire.AppendString(paramTargetValue, target)
	paramTarget = protowire.AppendTag(paramTarget, 2, protowire.BytesType)
	paramTarget = protowire.AppendBytes(paramTarget, paramTargetValue)

	paramValue := make([]byte, 0, 32)
	paramValue = protowire.AppendTag(paramValue, 1, protowire.BytesType)
	paramValue = protowire.AppendString(paramValue, "value")
	paramValueField := make([]byte, 0, 16)
	paramValueField = protowire.AppendTag(paramValueField, 4, protowire.Fixed64Type)
	paramValueField = protowire.AppendFixed64(paramValueField, math.Float64bits(77.0))
	paramValue = protowire.AppendTag(paramValue, 2, protowire.BytesType)
	paramValue = protowire.AppendBytes(paramValue, paramValueField)

	body := make([]byte, 0, 192)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "setOutputChannelValue")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramTarget)
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramValue)

	req := make([]byte, 0, 256)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 16)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 77.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=77, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueApplyNowFalse(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	paramTarget := make([]byte, 0, 32)
	paramTarget = protowire.AppendTag(paramTarget, 1, protowire.BytesType)
	paramTarget = protowire.AppendString(paramTarget, "dSUID")
	paramTargetValue := make([]byte, 0, 32)
	paramTargetValue = protowire.AppendTag(paramTargetValue, 5, protowire.BytesType)
	paramTargetValue = protowire.AppendString(paramTargetValue, target)
	paramTarget = protowire.AppendTag(paramTarget, 2, protowire.BytesType)
	paramTarget = protowire.AppendBytes(paramTarget, paramTargetValue)

	paramApply := make([]byte, 0, 32)
	paramApply = protowire.AppendTag(paramApply, 1, protowire.BytesType)
	paramApply = protowire.AppendString(paramApply, "apply_now")
	paramApplyValue := make([]byte, 0, 8)
	paramApplyValue = protowire.AppendTag(paramApplyValue, 1, protowire.VarintType)
	paramApplyValue = protowire.AppendVarint(paramApplyValue, 0)
	paramApply = protowire.AppendTag(paramApply, 2, protowire.BytesType)
	paramApply = protowire.AppendBytes(paramApply, paramApplyValue)

	paramValue := make([]byte, 0, 32)
	paramValue = protowire.AppendTag(paramValue, 1, protowire.BytesType)
	paramValue = protowire.AppendString(paramValue, "value")
	paramValueField := make([]byte, 0, 16)
	paramValueField = protowire.AppendTag(paramValueField, 4, protowire.Fixed64Type)
	paramValueField = protowire.AppendFixed64(paramValueField, math.Float64bits(77.0))
	paramValue = protowire.AppendTag(paramValue, 2, protowire.BytesType)
	paramValue = protowire.AppendBytes(paramValue, paramValueField)

	body := make([]byte, 0, 256)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, s.DSUID)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, "setOutputChannelValue")
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramTarget)
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramApply)
	body = protowire.AppendTag(body, 3, protowire.BytesType)
	body = protowire.AppendBytes(body, paramValue)

	req := make([]byte, 0, 320)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 17)
	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if mc.called {
		t.Fatal("expected no commander call when apply_now is false")
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueMissingAudienceRejected(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	frames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("", "setOutputChannelValue", []pbufPropertyElement{{Name: "value", Value: 50.0}}), sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultMessageUnknown {
		t.Fatalf("expected message-unknown response, got %d", parsePbufGenericCode(sub[3]))
	}
	if msg := parsePbufGenericDescription(sub[3]); msg != "notification needs dSUID, itemSpec or zone_id/group parameters" {
		t.Fatalf("unexpected generic response message: %q", msg)
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueDSUIDArrayIgnoresUnknownMembers(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-array"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-array"], "uid:u-array")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	params := []pbufPropertyElement{
		{Name: "dSUID", Value: "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		{Name: "dSUID", Value: target},
		{Name: "value", Value: 61.0},
	}
	frames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("", "setOutputChannelValue", params), sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if !mc.called || mc.uniqueID != "u-array" || mc.value != 61.0 {
		t.Fatalf("expected commander call uniqueid=u-array value=61, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueItemSpecRootAccepted(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-itemspec"})
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	params := []pbufPropertyElement{{Name: "itemSpec", Value: "root"}, {Name: "value", Value: 42.0}}
	frames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("", "setOutputChannelValue", params), sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if !mc.called || mc.uniqueID != "u-itemspec" || mc.value != 42.0 {
		t.Fatalf("expected commander call uniqueid=u-itemspec value=42, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueInvalidItemSpecRejected(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	params := []pbufPropertyElement{{Name: "itemSpec", Value: "invalid:spec"}, {Name: "value", Value: 42.0}}
	frames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("", "setOutputChannelValue", params), sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultNotFound {
		t.Fatalf("expected not-found generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if msg := parsePbufGenericDescription(sub[3]); msg != "missing/invalid itemSpec" {
		t.Fatalf("unexpected generic response message: %q", msg)
	}
}

func TestProcessPbufGenericRequestSetOutputChannelValueZoneGroupAccepted(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-zone"})
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	params := []pbufPropertyElement{
		{Name: "zone_id", Value: 1},
		{Name: "group", Value: 2},
		{Name: "value", Value: 77.0},
	}
	frames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("", "setOutputChannelValue", params), sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
	if !mc.called || mc.uniqueID != "u-zone" || mc.value != 77.0 {
		t.Fatalf("expected commander call uniqueid=u-zone value=77, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufGenericRequestRemoveTargetParity(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-remove"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-remove"], "uid:u-remove")

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	okFrames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest(target, "remove", nil), sess)
	if len(okFrames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(okFrames))
	}
	okType, _, _, okSub, err := parsePbufEnvelope(okFrames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if okType != pbufTypeGenericResponse || parsePbufGenericCode(okSub[3]) != pbufResultOK {
		t.Fatalf("expected remove success generic response, got type=%d code=%d", okType, parsePbufGenericCode(okSub[3]))
	}

	notFoundFrames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", "remove", nil), sess)
	if len(notFoundFrames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(notFoundFrames))
	}
	nfType, _, _, nfSub, err := parsePbufEnvelope(notFoundFrames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if nfType != pbufTypeGenericResponse || parsePbufGenericCode(nfSub[3]) != pbufResultNotFound {
		t.Fatalf("expected remove not-found generic response, got type=%d code=%d", nfType, parsePbufGenericCode(nfSub[3]))
	}
}

func TestProcessPbufGenericRequestControlMethodFallbackDispatch(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-generic-control"})
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	tests := []struct {
		name      string
		method    string
		params    []pbufPropertyElement
		wantValue float64
	}{
		{name: "callScene", method: "callScene", params: []pbufPropertyElement{{Name: "scene", Value: 5}}, wantValue: 100},
		{name: "dimChannel", method: "dimChannel", params: []pbufPropertyElement{{Name: "mode", Value: -1}}, wantValue: 0},
		{name: "setControlValue", method: "setControlValue", params: []pbufPropertyElement{{Name: "name", Value: "brightness"}, {Name: "value", Value: 33.0}}, wantValue: 33.0},
		{name: "setOutputChannelValue", method: "setOutputChannelValue", params: []pbufPropertyElement{{Name: "value", Value: 44.0}}, wantValue: 44.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc.called = false
			mc.uniqueID = ""
			mc.value = 0
			frames, _ := s.processPbufMessage(buildPbufGenericMatrixRequest("root", tc.method, tc.params), sess)
			if len(frames) != 1 {
				t.Fatalf("expected one response frame, got %d", len(frames))
			}
			msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
			if err != nil {
				t.Fatalf("parse response envelope: %v", err)
			}
			if msgType != pbufTypeGenericResponse || parsePbufGenericCode(sub[3]) != pbufResultOK {
				t.Fatalf("expected generic ok response, got type=%d code=%d", msgType, parsePbufGenericCode(sub[3]))
			}
			if !mc.called || mc.uniqueID != "u-generic-control" || mc.value != tc.wantValue {
				t.Fatalf("expected commander call uniqueid=u-generic-control value=%f, got called=%t uid=%s value=%f", tc.wantValue, mc.called, mc.uniqueID, mc.value)
			}
		})
	}
}

func TestProcessPbufRemoveReturnsOK(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-remove-direct"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-remove-direct"], "uid:u-remove-direct")

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 64)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)

	req := make([]byte, 0, 128)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmSendRemove)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 13)
	req = protowire.AppendTag(req, 110, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultOK {
		t.Fatalf("expected ok generic response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufRemoveUnknownTargetRejected(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 64)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")

	req := make([]byte, 0, 128)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmSendRemove)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 130)
	req = protowire.AppendTag(req, 110, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultNotFound {
		t.Fatalf("expected not-found generic response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufNotificationSetOutputChannelValueDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 96)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 4, protowire.Fixed64Type)
	body = protowire.AppendFixed64(body, math.Float64bits(55.0))

	req := make([]byte, 0, 160)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmNotifySetOutputChannelValue)
	req = protowire.AppendTag(req, 122, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("notifications must not respond, got %d frame(s)", len(frames))
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 55.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=55, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufNotificationSetOutputChannelValueApplyNowFalse(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 128)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.VarintType)
	body = protowire.AppendVarint(body, 0) // apply_now = false
	body = protowire.AppendTag(body, 4, protowire.Fixed64Type)
	body = protowire.AppendFixed64(body, math.Float64bits(55.0))

	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmNotifySetOutputChannelValue)
	req = protowire.AppendTag(req, 122, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("notifications must not respond, got %d frame(s)", len(frames))
	}
	if mc.called {
		t.Fatal("expected no commander call when apply_now is false")
	}
}

func TestProcessPbufGetPropertyMissingSubMessageRejected(t *testing.T) {
	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test"}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	req := make([]byte, 0, 32)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 99)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected one response frame, got %d", len(frames))
	}
	msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
	if err != nil {
		t.Fatalf("parse response envelope: %v", err)
	}
	if msgType != pbufTypeGenericResponse {
		t.Fatalf("expected generic response, got %d", msgType)
	}
	if parsePbufGenericCode(sub[3]) != pbufResultMessageUnknown {
		t.Fatalf("expected message unknown generic response, got %d", parsePbufGenericCode(sub[3]))
	}
}

func TestProcessPbufNotificationCallSceneDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 96)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.VarintType)
	body = protowire.AppendVarint(body, 5)

	req := make([]byte, 0, 160)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmNotifyCallScene)
	req = protowire.AppendTag(req, 112, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("notifications must not respond, got %d frame(s)", len(frames))
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 100.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=100, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufNotificationCallScenePresetLevel(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 96)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.VarintType)
	body = protowire.AppendVarint(body, 18) // preset 3 in vdcd default table -> 50

	req := make([]byte, 0, 160)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmNotifyCallScene)
	req = protowire.AppendTag(req, 112, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("notifications must not respond, got %d frame(s)", len(frames))
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 50.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=50, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessPbufNotificationCallSceneStopNoCommand(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 96)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.VarintType)
	body = protowire.AppendVarint(body, 15) // stop scene

	req := make([]byte, 0, 160)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmNotifyCallScene)
	req = protowire.AppendTag(req, 112, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("notifications must not respond, got %d frame(s)", len(frames))
	}
	if mc.called {
		t.Fatal("expected no commander call for stop scene")
	}
}

func TestProcessPbufNotificationDimChannelSignedMode(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")
	mc := &mockCommander{}

	s := &PbufServer{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "001122", apiVersion: APIVersionMax}

	body := make([]byte, 0, 96)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 3, protowire.VarintType)
	body = protowire.AppendVarint(body, ^uint64(0)) // -1 as int32/int64 varint

	req := make([]byte, 0, 160)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmNotifyDimChannel)
	req = protowire.AppendTag(req, 121, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, closeAfter := s.processPbufMessage(req, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if len(frames) != 0 {
		t.Fatalf("notifications must not respond, got %d frame(s)", len(frames))
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 0.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=0, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestChangedStatePayloadIncludesTypedStates(t *testing.T) {
	p := changedStatePayload(ExternalDeviceState{
		Active:        true,
		Channels:      map[int]float64{0: 44.5, 1: 10, 2: 20, 3: 30},
		Buttons:       map[int]float64{1: 1},
		ButtonActions: map[int]string{1: "tip"},
		Inputs:        map[int]float64{0: 0},
		Sensors:       map[int]float64{2: 23.25},
	})

	if p["active"] != true {
		t.Fatalf("expected active=true, got %+v", p["active"])
	}
	ch, ok := p["channelStates"].(map[string]any)
	if !ok {
		t.Fatalf("expected channelStates map, got %T", p["channelStates"])
	}
	c0, ok := ch["0"].(map[string]any)
	if !ok || c0["value"] != 44.5 {
		t.Fatalf("expected channel value 44.5, got %+v", ch)
	}
	if c1, ok := ch["1"].(map[string]any); !ok || c1["value"] != 10.0 {
		t.Fatalf("expected channel 1 value 10, got %+v", ch)
	}
	if c3, ok := ch["3"].(map[string]any); !ok || c3["value"] != 30.0 {
		t.Fatalf("expected channel 3 value 30, got %+v", ch)
	}
	btn, ok := p["buttonInputStates"].(map[string]any)
	if !ok {
		t.Fatalf("expected buttonInputStates map, got %T", p["buttonInputStates"])
	}
	b1, ok := btn["1"].(map[string]any)
	if !ok || b1["value"] != 1.0 {
		t.Fatalf("expected button index 1 value 1, got %+v", btn)
	}
	if b1["clickType"] != 0 {
		t.Fatalf("expected button clickType 0 (ct_tip_1x), got %+v", b1)
	}
	in, ok := p["binaryInputStates"].(map[string]any)
	if !ok {
		t.Fatalf("expected binaryInputStates map, got %T", p["binaryInputStates"])
	}
	i0, ok := in["0"].(map[string]any)
	if !ok || i0["value"] != 0.0 {
		t.Fatalf("expected input index 0 value 0, got %+v", in)
	}
	ss, ok := p["sensorStates"].(map[string]any)
	if !ok {
		t.Fatalf("expected sensorStates map, got %T", p["sensorStates"])
	}
	s2, ok := ss["2"].(map[string]any)
	if !ok || s2["value"] != 23.25 {
		t.Fatalf("expected sensor index 2 value 23.25, got %+v", ss)
	}
	devEvents, ok := p["deviceevents"].([]any)
	if !ok {
		t.Fatalf("expected deviceevents array, got %T", p["deviceevents"])
	}
	if len(devEvents) == 0 {
		t.Fatalf("expected non-empty deviceevents, got %+v", devEvents)
	}
	first, ok := devEvents[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first deviceevent object, got %+v", devEvents[0])
	}
	if first["type"] != "buttonAction" || first["action"] != "tip" {
		t.Fatalf("unexpected first deviceevent, got %+v", first)
	}
}

func parsePbufGenericCode(body []byte) uint64 {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return 0
		}
		body = body[n:]
		if num == 1 && typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(body)
			if n < 0 {
				return 0
			}
			return v
		}
		n = protowire.ConsumeFieldValue(num, typ, body)
		if n < 0 {
			return 0
		}
		body = body[n:]
	}
	return 0
}

func parsePbufGenericDescription(body []byte) string {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return ""
		}
		body = body[n:]
		if num == 2 && typ == protowire.BytesType {
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
