package vdcapi

import (
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func TestProcessRequestSetPropertyDispatchesCommander(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext dimmer", UniqueID: "u1"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u1"], "uid:u1")

	mc := &mockCommander{}
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "6",
		Method: "setProperty",
		Params: map[string]any{
			"dSUID": target,
			"properties": map[string]any{
				"channelStates": map[string]any{"0": map[string]any{"value": 44.0}},
			},
		},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected setProperty success, got %+v", r)
	}
	if !mc.called || mc.uniqueID != "u1" || mc.value != 44.0 {
		t.Fatalf("expected commander call uniqueid=u1 value=44, got called=%t uid=%s value=%f", mc.called, mc.uniqueID, mc.value)
	}
}

func TestProcessRequestSetPropertyRejectsNonLight(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "temp", UniqueID: "u10"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u10"], "uid:u10")

	mc := &mockCommander{}
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "8",
		Method: "setProperty",
		Params: map[string]any{
			"dSUID": target,
			"properties": map[string]any{
				"channelStates": map[string]any{"0": map[string]any{"value": 33.0}},
			},
		},
	}, sess)
	if r == nil || r.Error != 405 {
		t.Fatalf("expected non-light setProperty rejection, got %+v", r)
	}
	if mc.called {
		t.Fatal("did not expect commander call for non-light setProperty")
	}
}

func TestProcessRequestSetPropertyRejectsRootTarget(t *testing.T) {
	mc := &mockCommander{}
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "8b",
		Method: "setProperty",
		Params: map[string]any{
			"dSUID": "root",
			"properties": map[string]any{
				"channelStates": map[string]any{"0": map[string]any{"value": 33.0}},
			},
		},
	}, sess)
	if r == nil || r.Error != 400 {
		t.Fatalf("expected invalid-target setProperty rejection, got %+v", r)
	}
	if r.ErrorMsg != "setProperty target must be a device dSUID" {
		t.Fatalf("unexpected error message: %q", r.ErrorMsg)
	}
	if mc.called {
		t.Fatal("did not expect commander call for invalid target")
	}
}

func TestProcessRequestSetPropertyDispatchesCommanderForColorlightAndMovinglight(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "rgb", UniqueID: "u32"})
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "movinglight", Name: "mover", UniqueID: "u33"})
	snap := state.Snapshot()
	targetColor := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u32"], "uid:u32")
	targetMover := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u33"], "uid:u33")

	mc := &mockCommander{}
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: mc}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	r, _ := s.processRequest(request{ID: "d3", Method: "setProperty", Params: map[string]any{"dSUID": targetColor, "properties": map[string]any{"channelStates": map[string]any{"0": map[string]any{"value": 44.0}}}}}, sess)
	if r == nil || r.Error != 0 || !mc.called || mc.uniqueID != "u32" {
		t.Fatalf("expected colorlight setProperty commander call, got resp=%+v called=%t uid=%s", r, mc.called, mc.uniqueID)
	}

	mc.called = false
	r, _ = s.processRequest(request{ID: "d4", Method: "setProperty", Params: map[string]any{"dSUID": targetMover, "properties": map[string]any{"channelStates": map[string]any{"0": map[string]any{"value": 55.0}}}}}, sess)
	if r == nil || r.Error != 0 || !mc.called || mc.uniqueID != "u33" {
		t.Fatalf("expected movinglight setProperty commander call, got resp=%+v called=%t uid=%s", r, mc.called, mc.uniqueID)
	}
}

func TestSetPropertySceneChannelValueJSON(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "sc-light", UniqueID: "u-scw"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-scw"], "uid:u-scw")

	scenes := NewSceneStore()
	ms := newMethodService("0123456789ABCDEFFEDCBA9876543210AA", "testdesc", state, &mockCommander{}, scenes, nil, nil)
	err := ms.setPropertyFromJSON(map[string]any{
		"dSUID": target,
		"name":  "scenes/5/channels/0/value",
		"value": 88.0,
	})
	if err != nil {
		t.Fatalf("expected scene write ok, got %v", err)
	}
	entry, ok := scenes.GetScene(target, 5)
	if !ok {
		t.Fatal("expected scene 5 to exist after write")
	}
	ch, ok := entry.Channels[0]
	if !ok || ch.Value != 88.0 {
		t.Fatalf("expected scene 5 channel 0 value=88, got %+v", entry)
	}
}

func TestSetPropertySceneDontCareJSON(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "sc-dc", UniqueID: "u-sdc"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-sdc"], "uid:u-sdc")

	scenes := NewSceneStore()
	ms := newMethodService("0123456789ABCDEFFEDCBA9876543210AA", "testdesc", state, &mockCommander{}, scenes, nil, nil)
	err := ms.setPropertyFromJSON(map[string]any{
		"dSUID": target,
		"name":  "scenes/3/dontCare",
		"value": true,
	})
	if err != nil {
		t.Fatalf("expected scene dontCare write ok, got %v", err)
	}
	entry, ok := scenes.GetScene(target, 3)
	if !ok || !entry.DontCare {
		t.Fatalf("expected scene 3 dontCare=true, got ok=%t entry=%+v", ok, entry)
	}
}

func TestSetPropertySceneIgnoreLocalPriorityJSON(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "sc-ilp", UniqueID: "u-silp"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-silp"], "uid:u-silp")

	scenes := NewSceneStore()
	ms := newMethodService("0123456789ABCDEFFEDCBA9876543210AA", "testdesc", state, &mockCommander{}, scenes, nil, nil)
	err := ms.setPropertyFromJSON(map[string]any{
		"dSUID": target,
		"name":  "scenes/2/ignoreLocalPriority",
		"value": true,
	})
	if err != nil {
		t.Fatalf("expected scene ignoreLocalPriority write ok, got %v", err)
	}
	entry, ok := scenes.GetScene(target, 2)
	if !ok || !entry.IgnoreLocalPriority {
		t.Fatalf("expected scene 2 ignoreLocalPriority=true, got ok=%t entry=%+v", ok, entry)
	}
}

func TestSetPropertySceneWriteReflectedInGetProperty(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "sc-reflect", UniqueID: "u-scr"})
	snap := state.Snapshot()
	target := deviceDSUID("0123456789ABCDEFFEDCBA9876543210AA", snap.Devices["uid:u-scr"], "uid:u-scr")

	scenes := NewSceneStore()
	ms := newMethodService("0123456789ABCDEFFEDCBA9876543210AA", "testdesc", state, &mockCommander{}, scenes, nil, nil)

	_ = ms.setPropertyFromJSON(map[string]any{"dSUID": target, "name": "scenes/5/channels/0/value", "value": 75.0})

	r, _ := ms.resolveGetPropertyTarget(target)
	scenes5Raw, _ := r["scenes"].(map[string]any)
	sc5, _ := scenes5Raw["5"].(map[string]any)
	chs, _ := sc5["channels"].(map[string]any)
	ch0, _ := chs["0"].(map[string]any)
	if ch0["value"] != 75.0 {
		t.Fatalf("expected scene 5 ch0 value=75 in getProperty response, got %+v", ch0)
	}
}

func TestSetPropertyNameReflectedInGetProperty(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "named-lamp", UniqueID: "u-name"})
	snap := state.Snapshot()
	dsuid := "0123456789ABCDEFFEDCBA9876543210AA"
	target := deviceDSUID(dsuid, snap.Devices["uid:u-name"], "uid:u-name")

	cs := NewConfigStore()
	ms := newMethodService(dsuid, "testdesc", state, &mockCommander{}, NewSceneStore(), cs, nil)

	if err := ms.setPropertyFromJSON(map[string]any{
		"dSUID": target,
		"name":  "name",
		"value": "Cool Lamp",
	}); err != nil {
		t.Fatalf("setProperty name: %v", err)
	}

	got, _ := cs.GetDeviceName(target)
	if got != "Cool Lamp" {
		t.Fatalf("expected 'Cool Lamp' in config store, got %q", got)
	}

	r, err := ms.resolveGetPropertyTarget(target)
	if err != nil {
		t.Fatalf("getProperty: %v", err)
	}
	if r["name"] != "Cool Lamp" {
		t.Fatalf("expected name='Cool Lamp' in property tree, got %v", r["name"])
	}
}

func TestSetPropertyButtonInputSettings(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "button", Name: "btnX", UniqueID: "u-btnx"})
	state.HandleEvent(runtime.Event{Type: runtime.EventButton, UniqueID: "u-btnx", Index: 0, Value: 0})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	devices := pr.Result.(map[string]any)["devices"].(map[string]any)
	var target string
	for dsuid := range devices {
		target = dsuid
	}
	if target == "" {
		t.Fatal("no device found")
	}

	resp, _ := s.processRequest(request{
		ID:     "sp1",
		Method: "setProperty",
		Params: map[string]any{
			"dSUID": target,
			"name":  "buttonInputSettings/0/setsLocalPriority",
			"value": true,
		},
	}, sess)
	if resp != nil && resp.Error != 0 {
		t.Fatalf("setProperty returned error %d", resp.Error)
	}

	gr, _ := s.processRequest(request{ID: "g2", Method: "getProperty", Params: map[string]any{"dSUID": target, "name": " "}}, sess)
	dev := gr.Result.(map[string]any)
	biss, _ := dev["buttonInputSettings"].(map[string]any)
	s0, _ := biss["0"].(map[string]any)
	if v, _ := s0["setsLocalPriority"].(bool); !v {
		t.Errorf("expected setsLocalPriority=true after setProperty, got %v", s0["setsLocalPriority"])
	}
}

func TestSetPropertyBinaryInputSettings(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "input", Name: "binX", UniqueID: "u-binx"})
	state.HandleEvent(runtime.Event{Type: runtime.EventInput, UniqueID: "u-binx", Index: 0, Value: 0})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	devices := pr.Result.(map[string]any)["devices"].(map[string]any)
	var target string
	for dsuid := range devices {
		target = dsuid
	}
	if target == "" {
		t.Fatal("no device found")
	}

	resp, _ := s.processRequest(request{
		ID:     "sp2",
		Method: "setProperty",
		Params: map[string]any{
			"dSUID": target,
			"name":  "binaryInputSettings/0/sensorFunction",
			"value": float64(4),
		},
	}, sess)
	if resp != nil && resp.Error != 0 {
		t.Fatalf("setProperty returned error %d", resp.Error)
	}

	gr, _ := s.processRequest(request{ID: "g2", Method: "getProperty", Params: map[string]any{"dSUID": target, "name": " "}}, sess)
	dev := gr.Result.(map[string]any)
	bins, _ := dev["binaryInputSettings"].(map[string]any)
	b0, _ := bins["0"].(map[string]any)
	sf, _ := b0["sensorFunction"].(int)
	if sf != 4 {
		t.Errorf("expected sensorFunction=4 after setProperty, got %v", b0["sensorFunction"])
	}
}

func TestSetPropertySensorSettings(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "sensX", UniqueID: "u-sensx"})
	state.HandleEvent(runtime.Event{Type: runtime.EventSensor, UniqueID: "u-sensx", Index: 0, Value: 21.5})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	devices := pr.Result.(map[string]any)["devices"].(map[string]any)
	var target string
	for dsuid := range devices {
		target = dsuid
	}
	if target == "" {
		t.Fatal("no device found")
	}

	// Verify defaults are exposed.
	gr0, _ := s.processRequest(request{ID: "g0", Method: "getProperty", Params: map[string]any{"dSUID": target, "name": " "}}, sess)
	dev0 := gr0.Result.(map[string]any)
	ss0, ok := dev0["sensorSettings"].(map[string]any)
	if !ok || ss0["0"] == nil {
		t.Fatalf("expected sensorSettings/0 in property tree, got %+v", dev0["sensorSettings"])
	}

	// Write group=8, minPushInterval=2.5
	for _, payload := range []map[string]any{
		{"dSUID": target, "name": "sensorSettings/0/group", "value": float64(8)},
		{"dSUID": target, "name": "sensorSettings/0/minPushInterval", "value": 2.5},
	} {
		resp, _ := s.processRequest(request{ID: "sp", Method: "setProperty", Params: payload}, sess)
		if resp != nil && resp.Error != 0 {
			t.Fatalf("setProperty %v returned error %d", payload["name"], resp.Error)
		}
	}

	gr, _ := s.processRequest(request{ID: "g2", Method: "getProperty", Params: map[string]any{"dSUID": target, "name": " "}}, sess)
	dev := gr.Result.(map[string]any)
	ss, _ := dev["sensorSettings"].(map[string]any)
	s0, _ := ss["0"].(map[string]any)
	if g, _ := s0["group"].(int); g != 8 {
		t.Errorf("expected group=8 after setProperty, got %v", s0["group"])
	}
	if mp, _ := s0["minPushInterval"].(float64); mp != 2.5 {
		t.Errorf("expected minPushInterval=2.5 after setProperty, got %v", s0["minPushInterval"])
	}

	// Persistence round-trip.
	stored := config.GetSensorSettings(target, 0)
	if stored.Group == nil || *stored.Group != 8 {
		t.Errorf("config store group = %v, want 8", stored.Group)
	}
	if stored.MinPushInterval == nil || *stored.MinPushInterval != 2.5 {
		t.Errorf("config store minPushInterval = %v, want 2.5", stored.MinPushInterval)
	}
}

func TestSetPropertyButtonInputSettingsGroupModeFunction(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "button", Name: "btnX", UniqueID: "u-btnx"})
	state.HandleEvent(runtime.Event{Type: runtime.EventButton, UniqueID: "u-btnx", Index: 0, Value: 0})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	devices := pr.Result.(map[string]any)["devices"].(map[string]any)
	var target string
	for dsuid := range devices {
		target = dsuid
	}
	if target == "" {
		t.Fatal("no device found")
	}

	for _, payload := range []map[string]any{
		{"dSUID": target, "name": "buttonInputSettings/0/group", "value": float64(8)},
		{"dSUID": target, "name": "buttonInputSettings/0/mode", "value": float64(2)},
		{"dSUID": target, "name": "buttonInputSettings/0/function", "value": float64(5)},
	} {
		resp, _ := s.processRequest(request{ID: "sp", Method: "setProperty", Params: payload}, sess)
		if resp != nil && resp.Error != 0 {
			t.Fatalf("setProperty %v returned error %d", payload["name"], resp.Error)
		}
	}

	gr, _ := s.processRequest(request{ID: "g2", Method: "getProperty", Params: map[string]any{"dSUID": target, "name": " "}}, sess)
	dev := gr.Result.(map[string]any)
	bs := dev["buttonInputSettings"].(map[string]any)
	b0 := bs["0"].(map[string]any)
	if g, _ := b0["group"].(int); g != 8 {
		t.Errorf("group = %v, want 8", b0["group"])
	}
	if mode, _ := b0["mode"].(int); mode != 2 {
		t.Errorf("mode = %v, want 2", b0["mode"])
	}
	if fn, _ := b0["function"].(int); fn != 5 {
		t.Errorf("function = %v, want 5", b0["function"])
	}

	stored := config.GetButtonInputSettings(target, 0)
	if stored.Group == nil || *stored.Group != 8 || stored.Mode == nil || *stored.Mode != 2 || stored.Function == nil || *stored.Function != 5 {
		t.Errorf("config store entry = %+v", stored)
	}
}

func TestSetPropertyBinaryInputSettingsGroup(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "input", Name: "binY", UniqueID: "u-biny"})
	state.HandleEvent(runtime.Event{Type: runtime.EventInput, UniqueID: "u-biny", Index: 0, Value: 0})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	devices := pr.Result.(map[string]any)["devices"].(map[string]any)
	var target string
	for dsuid := range devices {
		target = dsuid
	}
	if target == "" {
		t.Fatal("no device found")
	}

	resp, _ := s.processRequest(request{ID: "sp", Method: "setProperty", Params: map[string]any{
		"dSUID": target, "name": "binaryInputSettings/0/group", "value": float64(3),
	}}, sess)
	if resp != nil && resp.Error != 0 {
		t.Fatalf("setProperty returned error %d", resp.Error)
	}

	gr, _ := s.processRequest(request{ID: "g2", Method: "getProperty", Params: map[string]any{"dSUID": target, "name": " "}}, sess)
	dev := gr.Result.(map[string]any)
	bs := dev["binaryInputSettings"].(map[string]any)
	b0 := bs["0"].(map[string]any)
	if g, _ := b0["group"].(int); g != 3 {
		t.Errorf("group = %v, want 3", b0["group"])
	}

	stored := config.GetBinaryInputSettings(target, 0)
	if stored.Group == nil || *stored.Group != 3 {
		t.Errorf("config store group = %v", stored.Group)
	}
}

func TestGetPropertySingleSpaceNamePreserved(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "g", UniqueID: "u-sp"})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	for _, name := range []string{"", " "} {
		resp, _ := s.processRequest(request{ID: "g", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": name}}, sess)
		if resp.Error != 0 {
			t.Fatalf("name=%q error=%d", name, resp.Error)
		}
		root, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("name=%q result not a map", name)
		}
		if _, ok := root["devices"]; !ok {
			t.Errorf("name=%q result missing devices key (got %v keys)", name, len(root))
		}
	}
}

func TestGetPropertyEmitsGUIDFields(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "g", UniqueID: "u-g"})
	config := NewConfigStore()
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "test", State: state, Config: config}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}

	pr, _ := s.processRequest(request{ID: "g1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	root := pr.Result.(map[string]any)
	for _, key := range []string{"hardwareModelGuid", "oemGuid", "oemModelGuid", "vendorId", "subdevIdx"} {
		if _, ok := root[key]; !ok {
			t.Errorf("root missing %q", key)
		}
	}
	devices := root["devices"].(map[string]any)
	for dsuid, dv := range devices {
		dev := dv.(map[string]any)
		for _, key := range []string{"hardwareModelGuid", "oemGuid", "oemModelGuid", "vendorId", "subdevIdx"} {
			if _, ok := dev[key]; !ok {
				t.Errorf("device %s missing %q", dsuid, key)
			}
		}
	}
}
