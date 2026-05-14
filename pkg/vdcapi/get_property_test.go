package vdcapi

import (
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func TestProcessRequestGetProperty(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "2",
		Method: "getProperty",
		Params: map[string]any{"dSUID": "root", "name": " "},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty success, got %+v", r)
	}
	result, ok := r.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", r.Result)
	}
	if result["dSUID"] != s.DSUID {
		t.Fatalf("unexpected dSUID: %+v", result)
	}
	if result["vdcs"] == nil || result["devices"] == nil {
		t.Fatalf("expected p44 discovery lists, got %+v", result)
	}
	vdcs, ok := result["vdcs"].(map[string]any)
	if !ok || len(vdcs) == 0 {
		t.Fatalf("expected non-empty vdcs map, got %+v", result["vdcs"])
	}
}

func TestProcessRequestGetPropertyQueryReturnsRegisteredDevice(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "my light", UniqueID: "u-q1"})
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	query := map[string]any{
		"vdcs": map[string]any{
			"*": map[string]any{
				"devices": map[string]any{
					"*": map[string]any{"dSUID": nil, "name": nil, "active": nil},
				},
			},
		},
	}
	r, _ := s.processRequest(request{
		ID:     "4",
		Method: "getProperty",
		Params: map[string]any{"dSUID": "root", "query": query},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty query success, got %+v", r)
	}
	result, ok := r.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", r.Result)
	}
	vdcs, ok := result["vdcs"].(map[string]any)
	if !ok || len(vdcs) != 1 {
		t.Fatalf("expected one vdc in query result, got %+v", result)
	}
	for _, vv := range vdcs {
		vdc, ok := vv.(map[string]any)
		if !ok {
			t.Fatalf("unexpected vdc entry: %+v", vv)
		}
		devices, ok := vdc["devices"].(map[string]any)
		if !ok || len(devices) == 0 {
			t.Fatalf("expected at least one device in query result, got %+v", vdc)
		}
		for _, dv := range devices {
			device, ok := dv.(map[string]any)
			if !ok {
				t.Fatalf("unexpected device entry: %+v", dv)
			}
			if _, ok := device["dSUID"].(string); !ok {
				t.Fatalf("expected device dSUID, got %+v", device)
			}
		}
	}
}

func TestProcessRequestGetPropertyUsesExternalState(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext dimmer", UniqueID: "u1"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u1", Index: 0, Value: 77.5})
	state.HandleEvent(runtime.Event{Type: runtime.EventActive, UniqueID: "u1", Active: true})

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "5",
		Method: "getProperty",
		Params: map[string]any{"dSUID": "root", "name": " "},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty success, got %+v", r)
	}
	result, ok := r.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", r.Result)
	}
	devices, ok := result["devices"].(map[string]any)
	if !ok || len(devices) == 0 {
		t.Fatalf("expected sample device list, got %+v", result["devices"])
	}
	for _, dv := range devices {
		device, ok := dv.(map[string]any)
		if !ok {
			t.Fatalf("unexpected device entry: %+v", dv)
		}
		if device["name"] != "ext dimmer" {
			t.Fatalf("expected state-backed name, got %+v", device["name"])
		}
		chStates, ok := device["channelStates"].(map[string]any)
		if !ok {
			t.Fatalf("missing channelStates: %+v", device)
		}
		ch0, ok := chStates["0"].(map[string]any)
		if !ok || ch0["value"] != 77.5 {
			t.Fatalf("expected channel 0 value 77.5, got %+v", chStates)
		}
	}
}

func TestProcessRequestGetPropertyIncludesSensorState(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "temp", UniqueID: "u9"})
	state.HandleEvent(runtime.Event{Type: runtime.EventSensor, UniqueID: "u9", Index: 0, Value: 22.75})

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "7",
		Method: "getProperty",
		Params: map[string]any{"dSUID": "root", "name": " "},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty success, got %+v", r)
	}
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	found := false
	for _, dv := range devices {
		device, ok := dv.(map[string]any)
		if !ok || device["name"] != "temp" {
			continue
		}
		found = true
		mf, ok := device["modelFeatures"].(map[string]any)
		if !ok {
			t.Fatalf("expected modelFeatures map, got %+v", device)
		}
		if _, hasIdent := mf["identification"]; !hasIdent {
			t.Fatalf("expected standard dS modelFeatures (identification key), got %+v", mf)
		}
		sensorStates, ok := device["sensorStates"].(map[string]any)
		if !ok {
			t.Fatalf("expected sensorStates map, got %+v", device)
		}
		s0, ok := sensorStates["0"].(map[string]any)
		if !ok || s0["value"] != 22.75 {
			t.Fatalf("expected sensor value 22.75, got %+v", sensorStates)
		}
	}
	if !found {
		t.Fatal("expected sensor device in property tree")
	}
}

func TestProcessRequestGetPropertyIncludesButtonAction(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "button", Name: "btn", UniqueID: "u11"})
	state.HandleEvent(runtime.Event{Type: runtime.EventButton, UniqueID: "u11", Index: 0, Value: 0})
	state.HandleEvent(runtime.Event{Type: runtime.EventButtonAction, UniqueID: "u11", Index: 0, Action: "hold"})

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{
		ID:     "9",
		Method: "getProperty",
		Params: map[string]any{"dSUID": "root", "name": " "},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty success, got %+v", r)
	}
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	found := false
	for _, dv := range devices {
		device, ok := dv.(map[string]any)
		if !ok || device["name"] != "btn" {
			continue
		}
		found = true
		states, ok := device["buttonInputStates"].(map[string]any)
		if !ok {
			t.Fatalf("expected buttonInputStates map, got %+v", device)
		}
		s0, ok := states["0"].(map[string]any)
		if !ok || s0["action"] != "hold" {
			t.Fatalf("expected button action hold, got %+v", states)
		}
	}
	if !found {
		t.Fatal("expected button device in property tree")
	}
}

func TestProcessRequestGetPropertyQueryCanonicalOutputSchemas(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "temp", UniqueID: "u20"})
	state.HandleEvent(runtime.Event{Type: runtime.EventSensor, UniqueID: "u20", Index: 1, Value: 21.5})
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "button", Name: "btn", UniqueID: "u21"})
	state.HandleEvent(runtime.Event{Type: runtime.EventButton, UniqueID: "u21", Index: 0, Value: 1})
	state.HandleEvent(runtime.Event{Type: runtime.EventButtonAction, UniqueID: "u21", Index: 0, Action: "scene5"})
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "input", Name: "in", UniqueID: "u22"})
	state.HandleEvent(runtime.Event{Type: runtime.EventInput, UniqueID: "u22", Index: 0, Value: 1})

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	query := map[string]any{
		"devices": map[string]any{
			"*": map[string]any{
				"name":                    nil,
				"outputDescription":       nil,
				"modelFeatures":           map[string]any{"identification": nil, "pushbutton": nil, "outmode": nil, "transt": nil, "outputchannels": nil},
				"sensorDescriptions":      map[string]any{"0": map[string]any{"name": nil}},
				"buttonInputDescriptions": map[string]any{"0": map[string]any{"name": nil, "supportsActions": nil}},
				"binaryInputDescriptions": map[string]any{"0": map[string]any{"name": nil}},
				"deviceevents":            nil,
			},
		},
	}
	r, _ := s.processRequest(request{ID: "c1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "query": query}}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty query success, got %+v", r)
	}
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	if len(devices) == 0 {
		t.Fatalf("expected queried devices, got %+v", result)
	}

	foundSensor, foundButton, foundInput := false, false, false

	for _, dv := range devices {
		device, ok := dv.(map[string]any)
		if !ok {
			continue
		}
		mf, _ := device["modelFeatures"].(map[string]any)
		events, _ := device["deviceevents"].([]any)
		if _, ok := mf["identification"]; !ok {
			t.Fatalf("expected standard dS modelFeatures (identification key), got %+v", mf)
		}

		name, _ := device["name"].(string)
		switch name {
		case "temp":
			foundSensor = true
		case "btn":
			foundButton = true
			if mf["pushbutton"] != true {
				t.Fatalf("expected button device with pushbutton=true, got %+v", mf)
			}
			if _, ok := device["buttonInputDescriptions"].(map[string]any); !ok {
				t.Fatalf("expected buttonInputDescriptions map, got %+v", device)
			}
			if len(events) == 0 {
				t.Fatalf("expected deviceevents for button device, got %+v", device)
			}
		case "in":
			foundInput = true
			if _, ok := device["binaryInputDescriptions"].(map[string]any); !ok {
				t.Fatalf("expected binaryInputDescriptions map, got %+v", device)
			}
		}
	}

	if !foundSensor || !foundButton || !foundInput {
		t.Fatalf("expected sensor/button/input devices in queried result, got sensor=%t button=%t input=%t", foundSensor, foundButton, foundInput)
	}
}

func TestProcessRequestGetPropertyIncludesColorlightSchemaAndChannels(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "colorlight", Name: "rgb", UniqueID: "u30"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u30", Index: 0, Value: 80})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u30", Index: 1, Value: 12})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u30", Index: 2, Value: 34})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u30", Index: 3, Value: 56})

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "d1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty success, got %+v", r)
	}
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	found := false
	for _, dv := range devices {
		device, ok := dv.(map[string]any)
		if !ok || device["name"] != "rgb" {
			continue
		}
		found = true
		od, _ := device["outputDescription"].(map[string]any)
		if _, ok := od["function"]; !ok {
			t.Fatalf("expected colorlight outputDescription, got %+v", od)
		}
		chs, _ := device["channelStates"].(map[string]any)
		if len(chs) < 4 {
			t.Fatalf("expected 4 channel states, got %+v", chs)
		}
		cds, _ := device["channelDescriptions"].(map[string]any)
		if len(cds) < 4 {
			t.Fatalf("expected 4 channel descriptions, got %+v", cds)
		}
		if cds["1"].(map[string]any)["name"] != "hue" {
			t.Fatalf("expected hue channel description, got %+v", cds["1"])
		}
	}
	if !found {
		t.Fatal("expected colorlight device in property tree")
	}
}

func TestProcessRequestGetPropertyIncludesMovinglightSchema(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "movinglight", Name: "mover", UniqueID: "u31"})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u31", Index: 0, Value: 20})
	state.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u31", Index: 1, Value: 65})

	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "d2", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected getProperty success, got %+v", r)
	}
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	found := false
	for _, dv := range devices {
		device, ok := dv.(map[string]any)
		if !ok || device["name"] != "mover" {
			continue
		}
		found = true
		od, _ := device["outputDescription"].(map[string]any)
		if _, ok := od["function"]; !ok {
			t.Fatalf("expected movinglight outputDescription, got %+v", od)
		}
		cds, _ := device["channelDescriptions"].(map[string]any)
		if cds["0"].(map[string]any)["name"] != "position" || cds["1"].(map[string]any)["name"] != "tilt" {
			t.Fatalf("unexpected movinglight channel descriptions: %+v", cds)
		}
	}
	if !found {
		t.Fatal("expected movinglight device in property tree")
	}
}

func TestGetPropertyDeviceHasHardwareAndModelFields(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "lamp", UniqueID: "u-hw"})
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "hw1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	for _, dv := range devices {
		device := dv.(map[string]any)
		if device["name"] != "lamp" {
			continue
		}
		if _, ok := device["hardwareVersion"]; !ok {
			t.Error("expected hardwareVersion field on device")
		}
		if _, ok := device["hardwareGuid"]; !ok {
			t.Error("expected hardwareGuid field on device")
		}
		if device["modelVersion"] != "1.0" {
			t.Errorf("expected modelVersion=1.0, got %v", device["modelVersion"])
		}
		return
	}
	t.Fatal("device not found")
}

func TestGetPropertyVdcHasHardwareAndModelFields(t *testing.T) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc"}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "hw2", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	result, _ := r.Result.(map[string]any)
	if _, ok := result["modelVersion"]; !ok {
		t.Error("expected modelVersion on root")
	}
	if _, ok := result["hardwareVersion"]; !ok {
		t.Error("expected hardwareVersion on root")
	}
	vdcs, _ := result["vdcs"].(map[string]any)
	for _, vv := range vdcs {
		vdc := vv.(map[string]any)
		if _, ok := vdc["hardwareGuid"]; !ok {
			t.Error("expected hardwareGuid on vDC")
		}
	}
}

func TestGetPropertyLightOutputDescriptionHasMaxPower(t *testing.T) {
	for _, output := range []string{"light", "colorlight", "movinglight"} {
		t.Run(output, func(t *testing.T) {
			state := NewStateStore()
			state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: output, Name: "dev", UniqueID: "u-mp-" + output})
			s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
			sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
			r, _ := s.processRequest(request{ID: "mp1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
			result, _ := r.Result.(map[string]any)
			devices, _ := result["devices"].(map[string]any)
			for _, dv := range devices {
				device := dv.(map[string]any)
				od, _ := device["outputDescription"].(map[string]any)
				if od == nil {
					continue
				}
				if _, ok := od["maxPower"]; !ok {
					t.Errorf("expected maxPower in outputDescription for %s, got %+v", output, od)
				}
				return
			}
			t.Fatalf("no output-bearing device found for %s", output)
		})
	}
}

func TestGetPropertyButtonDescriptionHasNewFields(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "button", Name: "btn2", UniqueID: "u-btnf"})
	state.HandleEvent(runtime.Event{Type: runtime.EventButton, UniqueID: "u-btnf", Index: 0, Value: 0})
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "bf1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	for _, dv := range devices {
		device := dv.(map[string]any)
		if device["name"] != "btn2" {
			continue
		}
		// setsLocalPriority and callsPresent must be in buttonInputSettings, NOT descriptions
		bids, _ := device["buttonInputDescriptions"].(map[string]any)
		b0, _ := bids["0"].(map[string]any)
		if _, ok := b0["setsLocalPriority"]; ok {
			t.Error("setsLocalPriority must NOT appear in buttonInputDescriptions")
		}
		if _, ok := b0["callsPresent"]; ok {
			t.Error("callsPresent must NOT appear in buttonInputDescriptions")
		}
		biss, _ := device["buttonInputSettings"].(map[string]any)
		s0, _ := biss["0"].(map[string]any)
		if _, ok := s0["setsLocalPriority"]; !ok {
			t.Error("expected setsLocalPriority in buttonInputSettings")
		}
		if _, ok := s0["callsPresent"]; !ok {
			t.Error("expected callsPresent in buttonInputSettings")
		}
		return
	}
	t.Fatal("button device not found")
}

func TestGetPropertySensorDescriptionHasNewFields(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "snsr", UniqueID: "u-snsr"})
	state.HandleEvent(runtime.Event{Type: runtime.EventSensor, UniqueID: "u-snsr", Index: 0, Value: 10.0})
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "sf1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	for _, dv := range devices {
		device := dv.(map[string]any)
		if device["name"] != "snsr" {
			continue
		}
		sds, _ := device["sensorDescriptions"].(map[string]any)
		s0, _ := sds["0"].(map[string]any)
		if _, ok := s0["minPushInterval"]; !ok {
			t.Error("expected minPushInterval in sensorDescriptions")
		}
		if _, ok := s0["changesOnlyInterval"]; !ok {
			t.Error("expected changesOnlyInterval in sensorDescriptions")
		}
		return
	}
	t.Fatal("sensor device not found")
}

func TestGetPropertySensorAgeReflectsUpdateTime(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "agetest", UniqueID: "u-sage"})
	state.HandleEvent(runtime.Event{Type: runtime.EventSensor, UniqueID: "u-sage", Index: 0, Value: 5.0})
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	r, _ := s.processRequest(request{ID: "sa1", Method: "getProperty", Params: map[string]any{"dSUID": "root", "name": " "}}, sess)
	result, _ := r.Result.(map[string]any)
	devices, _ := result["devices"].(map[string]any)
	for _, dv := range devices {
		device := dv.(map[string]any)
		if device["name"] != "agetest" {
			continue
		}
		ss, _ := device["sensorStates"].(map[string]any)
		s0, _ := ss["0"].(map[string]any)
		age, _ := s0["age"].(float64)
		if age < 0 || age > 2.0 {
			t.Errorf("expected sensor age 0..2s after update, got %v", age)
		}
		return
	}
	t.Fatal("sensor device not found")
}
