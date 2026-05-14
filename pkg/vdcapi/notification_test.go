package vdcapi

import (
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func TestProcessRequestNotificationDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-json-direct"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-json-direct"], "uid:u-json-direct")
	mc := &mockCommander{}

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "14",
		Notification: "setOutputChannelValue",
		Params: map[string]any{
			"dSUID": target,
			"value": 67.0,
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 0 {
		t.Fatalf("expected notification success, got %+v", r)
	}
	if !mc.called || mc.uniqueID != "u-json-direct" || mc.value != 67.0 {
		t.Fatalf("expected commander call uniqueid=u-json-direct value=67, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessRequestNotificationUnknownRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "15",
		Notification: "unknownNotification",
		Params:       map[string]any{},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 404 {
		t.Fatalf("expected unknown notification rejection, got %+v", r)
	}
	if r.ErrorMsg != "unknown notification 'unknownNotification'" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestNotificationIdentifyMissingAudienceRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "20",
		Notification: "identify",
		Params:       map[string]any{"duration": "abc"},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 400 {
		t.Fatalf("expected identify missing-audience rejection, got %+v", r)
	}
	if r.ErrorMsg != "notification needs dSUID, itemSpec or zone_id/group parameters" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestNotificationIdentifyInvalidDurationRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "20b",
		Notification: "identify",
		Params: map[string]any{
			"dSUID":    "root",
			"duration": "abc",
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 415 {
		t.Fatalf("expected identify invalid-duration rejection, got %+v", r)
	}
	if r.ErrorMsg != "invalid duration: must be numeric" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestNotificationIdentifyUnknownTargetRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "22",
		Notification: "identify",
		Params: map[string]any{
			"dSUID": "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF",
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 404 {
		t.Fatalf("expected identify unknown-target rejection, got %+v", r)
	}
	if r.ErrorMsg != "addressable not found" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestNotificationSetOutputChannelValueMissingAudienceRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "24",
		Notification: "setOutputChannelValue",
		Params:       map[string]any{"value": 50.0},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 400 {
		t.Fatalf("expected missing-audience rejection, got %+v", r)
	}
	if r.ErrorMsg != "notification needs dSUID, itemSpec or zone_id/group parameters" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestNotificationSetOutputChannelValueDSUIDArrayIgnoresUnknownMembers(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-array"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-array"], "uid:u-array")
	mc := &mockCommander{}

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "25",
		Notification: "setOutputChannelValue",
		Params: map[string]any{
			"dSUID": []any{"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", target},
			"value": 61.0,
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 0 {
		t.Fatalf("expected notification success, got %+v", r)
	}
	if !mc.called || mc.uniqueID != "u-array" || mc.value != 61.0 {
		t.Fatalf("expected commander call uniqueid=u-array value=61, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessRequestNotificationSetOutputChannelValueItemSpecRootAccepted(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-itemspec"})
	mc := &mockCommander{}

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "26",
		Notification: "setOutputChannelValue",
		Params: map[string]any{
			"itemSpec": "root",
			"value":    42.0,
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 0 {
		t.Fatalf("expected notification success, got %+v", r)
	}
	if !mc.called || mc.uniqueID != "u-itemspec" || mc.value != 42.0 {
		t.Fatalf("expected commander call uniqueid=u-itemspec value=42, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessRequestNotificationSetOutputChannelValueInvalidItemSpecRejected(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "27",
		Notification: "setOutputChannelValue",
		Params: map[string]any{
			"itemSpec": "invalid:spec",
			"value":    42.0,
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 404 {
		t.Fatalf("expected invalid itemSpec rejection, got %+v", r)
	}
	if r.ErrorMsg != "missing/invalid itemSpec" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
}

func TestProcessRequestNotificationSetOutputChannelValueZoneGroupAccepted(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u-zone"})
	mc := &mockCommander{}

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, closeAfter := s.processRequest(request{
		ID:           "28",
		Notification: "setOutputChannelValue",
		Params: map[string]any{
			"zone_id": 1,
			"group":   2,
			"value":   77.0,
		},
	}, sess)
	if closeAfter {
		t.Fatal("notification must not close connection")
	}
	if r == nil || r.Error != 0 {
		t.Fatalf("expected zone/group notification success, got %+v", r)
	}
	if !mc.called || mc.uniqueID != "u-zone" || mc.value != 77.0 {
		t.Fatalf("expected commander call uniqueid=u-zone value=77, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}
