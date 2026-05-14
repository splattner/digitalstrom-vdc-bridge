package vdcapi

import (
	"fmt"
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func TestProcessRequestGenericRequestScanDevices(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "10",
		Method: "genericRequest",
		Params: map[string]any{
			"methodname": "scanDevices",
			"params": map[string]any{
				"incremental": true,
				"exhaustive":  false,
			},
		},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected genericRequest scanDevices success, got %+v", r)
	}
	res, ok := r.Result.(map[string]any)
	if !ok || res["status"] != "started" {
		t.Fatalf("unexpected scanDevices result: %+v", r.Result)
	}
}

func TestProcessRequestGenericRequestPairTimeoutAbort(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "11",
		Method: "genericRequest",
		Params: map[string]any{
			"methodname": "pair",
			"params": map[string]any{
				"timeout": 0,
			},
		},
	}, sess)
	if r == nil || r.Error != 404 {
		t.Fatalf("expected pair abort error, got %+v", r)
	}
}

func TestProcessRequestGenericRequestRecursiveRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "12",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "genericRequest"},
	}, sess)
	if r == nil || r.Error != 415 {
		t.Fatalf("expected recursive genericRequest rejection, got %+v", r)
	}
}

func TestProcessRequestGenericRequestNotificationFallbackDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-json-notify"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-json-notify"], "uid:u-json-notify")
	mc := &mockCommander{}

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "13",
		Method: "genericRequest",
		Params: map[string]any{
			"methodname": "setOutputChannelValue",
			"params": map[string]any{
				"dSUID": target,
				"value": 55.0,
			},
		},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected genericRequest notification fallback success, got %+v", r)
	}
	if !mc.called || mc.uniqueID != "u-json-notify" || mc.value != 55.0 {
		t.Fatalf("expected commander call uniqueid=u-json-notify value=55, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessRequestGenericRequestUnknownNotificationPropagatesError(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "16",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "unknownNotificationLike", "params": map[string]any{}},
	}, sess)
	if r == nil || r.Error != 404 {
		t.Fatalf("expected genericRequest notification error propagation, got %+v", r)
	}
	if r.ErrorMsg != "unknown notification 'unknownNotificationLike'" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestLogLevelRequiresValue(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "17",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "loglevel", "params": map[string]any{}},
	}, sess)
	if r == nil || r.Error != 405 {
		t.Fatalf("expected loglevel validation error, got %+v", r)
	}
	if r.ErrorMsg != "missing value" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestLogLevelRejectsRange(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "18",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "loglevel", "params": map[string]any{"value": 9}},
	}, sess)
	if r == nil || r.Error != 405 {
		t.Fatalf("expected invalid loglevel error, got %+v", r)
	}
	if r.ErrorMsg != "invalid log level 9" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestLogLevelOffsetRejectsUnknownTopic(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "19",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "logleveloffset", "params": map[string]any{"value": 2, "topic": "foo"}},
	}, sess)
	if r == nil || r.Error != 405 {
		t.Fatalf("expected unknown topic error, got %+v", r)
	}
	if r.ErrorMsg != "unknown logging topic 'foo'" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestIdentifyInvalidDurationRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "21",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "identify", "dSUID": "root", "params": map[string]any{"duration": "abc"}},
	}, sess)
	if r == nil || r.Error != 415 {
		t.Fatalf("expected genericRequest identify invalid-duration rejection, got %+v", r)
	}
	if r.ErrorMsg != "invalid duration: must be numeric" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestIdentifyUsesTopLevelTarget(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "23",
		Method: "genericRequest",
		Params: map[string]any{
			"methodname": "identify",
			"dSUID":      "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF",
			"params":     map[string]any{},
		},
	}, sess)
	if r == nil || r.Error != 404 {
		t.Fatalf("expected genericRequest identify target rejection, got %+v", r)
	}
	if r.ErrorMsg != "addressable not found" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestRemoveTargetParity(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-remove"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-remove"], "uid:u-remove")

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	okResp, _ := s.processRequest(request{
		ID:     "29",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "remove", "dSUID": target, "params": map[string]any{}},
	}, sess)
	if okResp == nil || okResp.Error != 0 {
		t.Fatalf("expected genericRequest remove success, got %+v", okResp)
	}

	notFoundResp, _ := s.processRequest(request{
		ID:     "30",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "remove", "dSUID": "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", "params": map[string]any{}},
	}, sess)
	if notFoundResp == nil || notFoundResp.Error != 404 {
		t.Fatalf("expected genericRequest remove unknown-target rejection, got %+v", notFoundResp)
	}
	if notFoundResp.ErrorMsg != "addressable not found" {
		t.Fatalf("unexpected error message: %q", notFoundResp.ErrorMsg)
	}
}

func TestProcessRequestGenericRequestControlMethodFallbackDispatch(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-generic-control"})
	mc := &mockCommander{}

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	tests := []struct {
		name       string
		methodName string
		params     map[string]any
		wantValue  float64
	}{
		{name: "callScene", methodName: "callScene", params: map[string]any{"scene": 5}, wantValue: 100},
		{name: "dimChannel", methodName: "dimChannel", params: map[string]any{"mode": -1}, wantValue: 0},
		{name: "setControlValue", methodName: "setControlValue", params: map[string]any{"name": "brightness", "value": 33.0}, wantValue: 33.0},
		{name: "setOutputChannelValue", methodName: "setOutputChannelValue", params: map[string]any{"value": 44.0}, wantValue: 44.0},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc.called = false
			mc.uniqueID = ""
			mc.value = 0
			r, _ := s.processRequest(request{
				ID:     fmt.Sprintf("31-%d", i),
				Method: "genericRequest",
				Params: map[string]any{"methodname": tc.methodName, "dSUID": "root", "params": tc.params},
			}, sess)
			if r == nil || r.Error != 0 {
				t.Fatalf("expected genericRequest %s success, got %+v", tc.methodName, r)
			}
			if !mc.called || mc.uniqueID != "u-generic-control" || mc.value != tc.wantValue {
				t.Fatalf("expected commander call uniqueid=u-generic-control value=%f, got called=%t uid=%s value=%f", tc.wantValue, mc.called, mc.uniqueID, mc.value)
			}
		})
	}
}

func TestRemoveLifecycle(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "lamp", UniqueID: "u-lifecycle"})
	snap := state.Snapshot()
	dsuid := "0123456789ABCDEFFEDCBA9876543210AA"
	target := deviceDSUID(dsuid, snap.Devices["uid:u-lifecycle"], "uid:u-lifecycle")

	subID, updates := state.Subscribe()
	defer state.Unsubscribe(subID)

	s := &Server{ServerConfig: ServerConfig{DSUID: dsuid, Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	resp, _ := s.processRequest(request{
		ID:     "r1",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "remove", "dSUID": target, "params": map[string]any{}},
	}, sess)
	if resp == nil || resp.Error != 0 {
		t.Fatalf("expected remove success, got %+v", resp)
	}

	after := state.Snapshot()
	if _, still := after.Devices["uid:u-lifecycle"]; still {
		t.Fatal("device still present in snapshot after remove")
	}

	select {
	case u := <-updates:
		if u.Type != runtime.EventRemove {
			t.Fatalf("expected EventRemove broadcast, got type %q", u.Type)
		}
		if u.Device.UniqueID != "u-lifecycle" {
			t.Fatalf("broadcast device UniqueID = %q, want %q", u.Device.UniqueID, "u-lifecycle")
		}
	default:
		t.Fatal("no StateUpdate broadcast received after remove")
	}

	resp2, _ := s.processRequest(request{
		ID:     "r2",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "remove", "dSUID": target, "params": map[string]any{}},
	}, sess)
	if resp2 == nil || resp2.Error != 404 {
		t.Fatalf("expected 404 on second remove, got %+v", resp2)
	}
}
