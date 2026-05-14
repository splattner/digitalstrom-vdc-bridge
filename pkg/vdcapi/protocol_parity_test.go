package vdcapi

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

type phase1ParityCase struct {
	name                 string
	jsonReq              request
	jsonSessionActive    bool
	wantJSONError        int
	wantJSONMsgContains  string
	pbufPayload          []byte
	pbufSessionActive    bool
	wantPbufCode         uint64
	wantPbufDescContains string
	wantPbufFrames       int
	wantPbufExtraType    uint64
}

func TestProtocolParityMatrix(t *testing.T) {
	const (
		vdcDSUID     = "0123456789ABCDEFFEDCBA9876543210AA"
		unknownDSUID = "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	)

	rows := []phase1ParityCase{
		{
			name:                 "session gating getProperty",
			jsonReq:              request{ID: "j1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}},
			jsonSessionActive:    false,
			wantJSONError:        401,
			wantJSONMsgContains:  "no vDC session - cannot call method",
			pbufPayload:          buildPbufGetPropertyMatrixRequest("root"),
			pbufSessionActive:    false,
			wantPbufCode:         pbufResultNotAuthorized,
			wantPbufDescContains: "no vDC session - cannot call method",
			wantPbufFrames:       1,
		},
		{
			name:                 "genericRequest recursive guard",
			jsonReq:              request{ID: "j2", Method: "genericRequest", Params: map[string]any{"methodname": "genericRequest"}},
			jsonSessionActive:    true,
			wantJSONError:        415,
			wantJSONMsgContains:  "recursive call of genericRequest",
			pbufPayload:          buildPbufGenericMatrixRequest(vdcDSUID, "genericRequest", nil),
			pbufSessionActive:    true,
			wantPbufCode:         pbufResultInvalidValueType,
			wantPbufDescContains: "recursive call of genericRequest",
			wantPbufFrames:       1,
		},
		{
			name:                 "genericRequest unknown notification fallback",
			jsonReq:              request{ID: "j3", Method: "genericRequest", Params: map[string]any{"methodname": "unknownNotificationLike", "params": map[string]any{}}},
			jsonSessionActive:    true,
			wantJSONError:        404,
			wantJSONMsgContains:  "unknown notification 'unknownNotificationLike'",
			pbufPayload:          buildPbufGenericMatrixRequest(vdcDSUID, "unknownNotificationLike", nil),
			pbufSessionActive:    true,
			wantPbufCode:         pbufResultNotImplemented,
			wantPbufDescContains: "unknown notification 'unknownNotificationLike'",
			wantPbufFrames:       1,
		},
		{
			name:                 "genericRequest loglevel missing value",
			jsonReq:              request{ID: "j4", Method: "genericRequest", Params: map[string]any{"methodname": "loglevel", "params": map[string]any{}}},
			jsonSessionActive:    true,
			wantJSONError:        405,
			wantJSONMsgContains:  "missing value",
			pbufPayload:          buildPbufGenericMatrixRequest(vdcDSUID, "loglevel", nil),
			pbufSessionActive:    true,
			wantPbufCode:         pbufResultMessageUnknown,
			wantPbufDescContains: "missing value",
			wantPbufFrames:       1,
		},
		{
			name: "genericRequest identify invalid duration",
			jsonReq: request{ID: "j5", Method: "genericRequest", Params: map[string]any{
				"methodname": "identify",
				"dSUID":      "root",
				"params":     map[string]any{"duration": "abc"},
			}},
			jsonSessionActive:   true,
			wantJSONError:       415,
			wantJSONMsgContains: "invalid duration: must be numeric",
			pbufPayload: buildPbufGenericMatrixRequest("root", "identify", []pbufPropertyElement{
				{Name: "duration", Value: "abc"},
			}),
			pbufSessionActive:    true,
			wantPbufCode:         pbufResultInvalidValueType,
			wantPbufDescContains: "invalid duration: must be numeric",
			wantPbufFrames:       1,
		},
		{
			name: "genericRequest identify unknown target",
			jsonReq: request{ID: "j6", Method: "genericRequest", Params: map[string]any{
				"methodname": "identify",
				"dSUID":      unknownDSUID,
				"params":     map[string]any{},
			}},
			jsonSessionActive:    true,
			wantJSONError:        404,
			wantJSONMsgContains:  "addressable not found",
			pbufPayload:          buildPbufGenericMatrixRequest(unknownDSUID, "identify", nil),
			pbufSessionActive:    true,
			wantPbufCode:         pbufResultNotFound,
			wantPbufDescContains: "addressable not found",
			wantPbufFrames:       1,
		},
		{
			name: "genericRequest ping root side effect",
			jsonReq: request{ID: "j7", Method: "genericRequest", Params: map[string]any{
				"methodname": "ping",
				"dSUID":      "root",
				"params":     map[string]any{},
			}},
			jsonSessionActive:    true,
			wantJSONError:        0,
			pbufPayload:          buildPbufGenericMatrixRequest("root", "ping", nil),
			pbufSessionActive:    true,
			wantPbufCode:         pbufResultOK,
			wantPbufFrames:       2,
			wantPbufExtraType:    pbufTypeVdcSendPong,
			wantPbufDescContains: "",
		},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			jsonServer := &Server{ServerConfig: ServerConfig{DSUID: vdcDSUID, Description: "phase1"}}
			jsonSess := &session{}
			if tc.jsonSessionActive {
				jsonSess.active = true
				jsonSess.vdsmDSUID = "0011"
				jsonSess.apiVersion = APIVersionMax
			}
			jr, closeAfter := jsonServer.processRequest(tc.jsonReq, jsonSess)
			if closeAfter {
				t.Fatal("json request must not close connection")
			}
			if jr == nil {
				t.Fatal("expected json response")
			}
			if jr.Error != tc.wantJSONError {
				t.Fatalf("unexpected json error code: got=%d want=%d resp=%+v", jr.Error, tc.wantJSONError, jr)
			}
			if tc.wantJSONMsgContains != "" && !strings.Contains(jr.ErrorMsg, tc.wantJSONMsgContains) {
				t.Fatalf("unexpected json error msg: got=%q want contains=%q", jr.ErrorMsg, tc.wantJSONMsgContains)
			}

			pbufServer := &PbufServer{ServerConfig: ServerConfig{DSUID: vdcDSUID, Description: "phase1"}}
			pbufSess := &session{}
			if tc.pbufSessionActive {
				pbufSess.active = true
				pbufSess.vdsmDSUID = "0011"
				pbufSess.apiVersion = APIVersionMax
			}
			frames, closeAfter := pbufServer.processPbufMessage(tc.pbufPayload, pbufSess)
			if closeAfter {
				t.Fatal("pbuf request must not close connection")
			}
			if len(frames) != tc.wantPbufFrames {
				t.Fatalf("unexpected pbuf frame count: got=%d want=%d", len(frames), tc.wantPbufFrames)
			}
			if len(frames) == 0 {
				t.Fatal("expected at least one pbuf frame")
			}

			msgType, _, _, sub, err := parsePbufEnvelope(frames[0][2:])
			if err != nil {
				t.Fatalf("parse pbuf response envelope: %v", err)
			}
			if msgType != pbufTypeGenericResponse {
				t.Fatalf("expected generic response, got message type %d", msgType)
			}
			if code := parsePbufGenericCode(sub[3]); code != tc.wantPbufCode {
				t.Fatalf("unexpected pbuf code: got=%d want=%d", code, tc.wantPbufCode)
			}
			desc := parsePbufGenericDescription(sub[3])
			if tc.wantPbufDescContains != "" && !strings.Contains(desc, tc.wantPbufDescContains) {
				t.Fatalf("unexpected pbuf description: got=%q want contains=%q", desc, tc.wantPbufDescContains)
			}

			if tc.wantPbufExtraType != 0 {
				extraType, _, _, _, err := parsePbufEnvelope(frames[1][2:])
				if err != nil {
					t.Fatalf("parse pbuf extra frame: %v", err)
				}
				if extraType != tc.wantPbufExtraType {
					t.Fatalf("unexpected pbuf side effect type: got=%d want=%d", extraType, tc.wantPbufExtraType)
				}
			}
		})
	}
}

func buildPbufGetPropertyMatrixRequest(target string) []byte {
	req := make([]byte, 0, 96)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 9001)
	body := make([]byte, 0, 48)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	req = protowire.AppendTag(req, 102, protowire.BytesType)
	req = protowire.AppendBytes(req, body)
	return req
}

func buildPbufGenericMatrixRequest(target, method string, params []pbufPropertyElement) []byte {
	req := make([]byte, 0, 192)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGenericReq)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 9002)

	body := make([]byte, 0, 160)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, target)
	body = protowire.AppendTag(body, 2, protowire.BytesType)
	body = protowire.AppendString(body, method)
	for i := range params {
		pe := encodePropertyElement(params[i])
		body = protowire.AppendTag(body, 3, protowire.BytesType)
		body = protowire.AppendBytes(body, pe)
	}

	req = protowire.AppendTag(req, 123, protowire.BytesType)
	req = protowire.AppendBytes(req, body)
	return req
}
