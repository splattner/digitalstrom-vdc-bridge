package vdcapi

import (
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func TestGetPropertyColorlightHasAllChannels(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "cl1", UniqueID: "u-cl1"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl1", Index: 0, Value: 80.0})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl1", Index: 1, Value: 180.0})
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	devices := r.Result.(map[string]any)["devices"].(map[string]any)
	var dev map[string]any
	for _, d := range devices {
		dev = d.(map[string]any)
	}
	if dev == nil {
		t.Fatal("no device found")
	}
	chDescs := dev["channelDescriptions"].(map[string]any)
	chStates := dev["channelStates"].(map[string]any)
	for _, k := range []string{"0", "1", "2", "3", "4", "5"} {
		if _, ok := chDescs[k]; !ok {
			t.Errorf("missing channelDescriptions[%s]", k)
		}
		if _, ok := chStates[k]; !ok {
			t.Errorf("missing channelStates[%s]", k)
		}
	}
	if desc, ok := chDescs["0"].(map[string]any); ok {
		if desc["name"] != "brightness" {
			t.Errorf("channel 0 name = %v, want brightness", desc["name"])
		}
	}
	if desc, ok := chDescs["1"].(map[string]any); ok {
		if desc["name"] != "hue" {
			t.Errorf("channel 1 name = %v, want hue", desc["name"])
		}
	}
	if st, ok := chStates["0"].(map[string]any); ok {
		if v, _ := st["value"].(float64); v != 80.0 {
			t.Errorf("channel 0 value = %v, want 80", v)
		}
	}
	if st, ok := chStates["1"].(map[string]any); ok {
		if v, _ := st["value"].(float64); v != 180.0 {
			t.Errorf("channel 1 value = %v, want 180", v)
		}
	}
}

func TestNotificationSetOutputChannelValueColorChannel(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "cl2", UniqueID: "u-cl2"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl2", Index: 0, Value: 50.0})
	mc := &mockCommander{}
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	var target string
	for dsuid := range pr.Result.(map[string]any)["devices"].(map[string]any) {
		target = dsuid
	}
	if target == "" {
		t.Fatal("no device found")
	}

	resp, _ := s.processRequest(request{
		ID:     "n1",
		Method: "genericRequest",
		Params: map[string]any{
			"methodname": "setOutputChannelValue",
			"dSUID":      target,
			"params": map[string]any{
				"channelIndex": float64(1),
				"value":        180.0,
				"apply_now":    true,
			},
		},
	}, sess)
	if resp != nil && resp.Error != 0 {
		t.Fatalf("setOutputChannelValue ch1 returned error %d", resp.Error)
	}
	if !mc.colorCalled {
		t.Fatal("expected SetColorChannelValue to be called for channel 1")
	}
	if mc.colorChannel != 1 {
		t.Errorf("expected colorChannel=1, got %d", mc.colorChannel)
	}
	if mc.colorValue != 180.0 {
		t.Errorf("expected colorValue=180, got %f", mc.colorValue)
	}

	mc.colorCalled = false
	mc.called = false
	resp2, _ := s.processRequest(request{
		ID:     "n2",
		Method: "genericRequest",
		Params: map[string]any{
			"methodname": "setOutputChannelValue",
			"dSUID":      target,
			"params": map[string]any{
				"channelIndex": float64(0),
				"value":        75.0,
				"apply_now":    true,
			},
		},
	}, sess)
	if resp2 != nil && resp2.Error != 0 {
		t.Fatalf("setOutputChannelValue ch0 returned error %d", resp2.Error)
	}
	if !mc.called {
		t.Fatal("expected SetLightLevel to be called for channel 0")
	}
	if mc.colorCalled {
		t.Fatal("SetColorChannelValue must not be called for channel 0")
	}
}

func TestPbufCallSceneRestoresColorlightChannels(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "cl3", UniqueID: "u-cl3"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl3", Index: 0, Value: 60.0})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl3", Index: 1, Value: 200.0})

	scenes := NewSceneStore()
	mc := &mockCommander{}
	ps := &PbufServer{ServerConfig: ServerConfig{
		DSUID:     "0123456789ABCDEFFEDCBA9876543210AA",
		State:     state,
		Scenes:    scenes,
		Commander: mc,
	}}

	snapshot := state.Snapshot()
	var devDSUID string
	for k, d := range snapshot.Devices {
		devDSUID = deviceDSUID(ps.DSUID, d, k)
	}
	if devDSUID == "" {
		t.Fatal("no device found")
	}

	scenes.SetSceneChannelValue(devDSUID, 5, 0, 80.0)
	scenes.SetSceneChannelValue(devDSUID, 5, 1, 150.0)

	ps.methodService().handleCallSceneNotification([]string{devDSUID}, false, 5)

	if !mc.called {
		t.Fatal("expected SetLightLevel to be called for brightness channel")
	}
	if mc.value != 80.0 {
		t.Errorf("expected brightness=80, got %f", mc.value)
	}
	if !mc.colorCalled {
		t.Fatal("expected SetColorChannelValue to be called for hue channel")
	}
	if mc.colorChannel != 1 {
		t.Errorf("expected colorChannel=1, got %d", mc.colorChannel)
	}
	if mc.colorValue != 150.0 {
		t.Errorf("expected hue=150, got %f", mc.colorValue)
	}
}

// TestSetPropertyChannelStatesPerChannel verifies that setProperty against
// channelStates.{N}.value dispatches to the commander for any channel index
// (not only brightness via the legacy SetLightLevel path).
func TestSetPropertyChannelStatesPerChannel(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "cl-set", UniqueID: "u-cl-set"})
	mc := &mockCommander{}
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	snapshot := state.Snapshot()
	var target string
	for k, d := range snapshot.Devices {
		target = deviceDSUID(s.DSUID, d, k)
	}
	if target == "" {
		t.Fatal("no device found")
	}

	// Path-form write to channel 3 (cieX) — DSS may use this form.
	resp, _ := s.processRequest(request{
		ID:     "set-ch3",
		Method: "setProperty",
		Params: map[string]any{"dSUID": target, "name": "channelStates/3/value", "value": 0.42},
	}, sess)
	if resp != nil && resp.Error != 0 {
		t.Fatalf("setProperty channelStates/3/value returned error %d", resp.Error)
	}
	if !mc.colorCalled || mc.colorChannel != 3 || mc.colorValue != 0.42 {
		t.Fatalf("expected SetChannelValue(uid, 3, 0.42); got called=%t ch=%d val=%f", mc.colorCalled, mc.colorChannel, mc.colorValue)
	}

	// Nested-form write to channel 1 (hue).
	mc.colorCalled = false
	resp2, _ := s.processRequest(request{
		ID:     "set-ch1",
		Method: "setProperty",
		Params: map[string]any{"dSUID": target, "properties": map[string]any{
			"channelStates": map[string]any{"1": map[string]any{"value": 120.0}},
		}},
	}, sess)
	if resp2 != nil && resp2.Error != 0 {
		t.Fatalf("setProperty nested channelStates returned error %d", resp2.Error)
	}
	if !mc.colorCalled || mc.colorChannel != 1 || mc.colorValue != 120.0 {
		t.Fatalf("expected SetChannelValue(uid, 1, 120); got called=%t ch=%d val=%f", mc.colorCalled, mc.colorChannel, mc.colorValue)
	}
}

func TestColorlightSceneRoundTrip(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "cl4", UniqueID: "u-cl4"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl4", Index: 0, Value: 70.0})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl4", Index: 1, Value: 120.0})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-cl4", Index: 2, Value: 80.0})

	scenes := NewSceneStore()
	mc := &mockCommander{}
	ps := &PbufServer{ServerConfig: ServerConfig{
		DSUID:     "0123456789ABCDEFFEDCBA9876543210AA",
		State:     state,
		Scenes:    scenes,
		Commander: mc,
	}}

	snapshot := state.Snapshot()
	var devDSUID string
	for k, d := range snapshot.Devices {
		devDSUID = deviceDSUID(ps.DSUID, d, k)
	}
	if devDSUID == "" {
		t.Fatal("no device found")
	}

	ps.methodService().handleSaveSceneNotification([]string{devDSUID}, false, 3)

	entry, ok := scenes.GetScene(devDSUID, 3)
	if !ok {
		t.Fatal("expected scene 3 to be saved")
	}
	if v := entry.Channels[0].Value; v != 70.0 {
		t.Errorf("saved brightness = %f, want 70", v)
	}
	if v := entry.Channels[1].Value; v != 120.0 {
		t.Errorf("saved hue = %f, want 120", v)
	}
	if v := entry.Channels[2].Value; v != 80.0 {
		t.Errorf("saved saturation = %f, want 80", v)
	}

	ps.methodService().handleCallSceneNotification([]string{devDSUID}, false, 3)

	if mc.value != 70.0 {
		t.Errorf("recalled brightness = %f, want 70", mc.value)
	}
	if !mc.colorCalled {
		t.Fatal("expected color channel recall")
	}
}

// TestMovinglightSceneRoundTrip verifies a save→call cycle preserves both
// position (ch0) and tilt (ch1) channels for movinglight devices.
func TestMovinglightSceneRoundTrip(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "movinglight", Name: "ml1", UniqueID: "u-ml1"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-ml1", Index: 0, Value: 55.0})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u-ml1", Index: 1, Value: 30.0})

	scenes := NewSceneStore()
	mc := &mockCommander{}
	ps := &PbufServer{ServerConfig: ServerConfig{
		DSUID:     "0123456789ABCDEFFEDCBA9876543210AA",
		State:     state,
		Scenes:    scenes,
		Commander: mc,
	}}

	snapshot := state.Snapshot()
	var devDSUID string
	for k, d := range snapshot.Devices {
		devDSUID = deviceDSUID(ps.DSUID, d, k)
	}
	if devDSUID == "" {
		t.Fatal("no device found")
	}

	ps.methodService().handleSaveSceneNotification([]string{devDSUID}, false, 7)

	entry, ok := scenes.GetScene(devDSUID, 7)
	if !ok {
		t.Fatal("expected scene 7 to be saved")
	}
	if v := entry.Channels[0].Value; v != 55.0 {
		t.Errorf("saved position = %f, want 55", v)
	}
	if v := entry.Channels[1].Value; v != 30.0 {
		t.Errorf("saved tilt = %f, want 30", v)
	}

	ps.methodService().handleCallSceneNotification([]string{devDSUID}, false, 7)

	if mc.value != 55.0 {
		t.Errorf("recalled position = %f, want 55", mc.value)
	}
	if !mc.colorCalled || mc.colorChannel != 1 || mc.colorValue != 30.0 {
		t.Errorf("expected tilt recall ch=1 val=30; got called=%t ch=%d val=%f", mc.colorCalled, mc.colorChannel, mc.colorValue)
	}
}
