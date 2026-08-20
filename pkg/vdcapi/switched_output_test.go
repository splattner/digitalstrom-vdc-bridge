package vdcapi

import (
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

// getDeviceProperties fetches the full property tree for the single device
// registered in state via a getProperty(root) request, for assertions.
func getDeviceProperties(t *testing.T, state *StateStore) map[string]any {
	t.Helper()
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
	return dev
}

func TestLightOutputDimmableByDefault(t *testing.T) {
	state := NewStateStore()
	// No Dimmable set — must default to dimmable (existing behavior preserved).
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "lamp", UniqueID: "u-dim1"})
	dev := getDeviceProperties(t, state)

	if dev["outputType"] != "dimmer" {
		t.Errorf("outputType = %v, want dimmer", dev["outputType"])
	}
	outDesc := dev["outputDescription"].(map[string]any)
	if outDesc["function"] != 1 {
		t.Errorf("outputDescription.function = %v, want 1 (dimmer)", outDesc["function"])
	}
	if outDesc["variableRamp"] != true {
		t.Errorf("outputDescription.variableRamp = %v, want true", outDesc["variableRamp"])
	}
	outSet := dev["outputSettings"].(map[string]any)
	if outSet["mode"] != 2 {
		t.Errorf("outputSettings.mode = %v, want 2 (gradual)", outSet["mode"])
	}
	mf := dev["modelFeatures"].(map[string]any)
	if mf["outmode"] != true || mf["outmodeswitch"] != false || mf["transt"] != true {
		t.Errorf("modelFeatures = %+v, want outmode=true outmodeswitch=false transt=true", mf)
	}
}

func TestLightOutputExplicitlyDimmable(t *testing.T) {
	state := NewStateStore()
	dimmable := true
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "lamp", UniqueID: "u-dim2", Dimmable: &dimmable})
	dev := getDeviceProperties(t, state)

	if dev["outputType"] != "dimmer" {
		t.Errorf("outputType = %v, want dimmer", dev["outputType"])
	}
	outSet := dev["outputSettings"].(map[string]any)
	if outSet["mode"] != 2 {
		t.Errorf("outputSettings.mode = %v, want 2 (gradual)", outSet["mode"])
	}
}

func TestSwitchedOutputNotDimmable(t *testing.T) {
	state := NewStateStore()
	dimmable := false
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "relay", UniqueID: "u-sw1", Dimmable: &dimmable})
	dev := getDeviceProperties(t, state)

	if dev["outputType"] != "light" {
		t.Errorf("outputType = %v, want light", dev["outputType"])
	}
	outDesc := dev["outputDescription"].(map[string]any)
	if outDesc["function"] != 0 {
		t.Errorf("outputDescription.function = %v, want 0 (on/off only)", outDesc["function"])
	}
	if outDesc["variableRamp"] != false {
		t.Errorf("outputDescription.variableRamp = %v, want false", outDesc["variableRamp"])
	}
	outSet := dev["outputSettings"].(map[string]any)
	if outSet["mode"] != 1 {
		t.Errorf("outputSettings.mode = %v, want 1 (binary)", outSet["mode"])
	}
	// onThreshold must stay populated regardless of mode — it's the SW_THR
	// value real dS switched-mode devices threshold at (ds-light.pdf §2.3).
	if outSet["onThreshold"] != 50.0 {
		t.Errorf("outputSettings.onThreshold = %v, want 50.0", outSet["onThreshold"])
	}
	mf := dev["modelFeatures"].(map[string]any)
	if mf["outmode"] != true || mf["outmodeswitch"] != true || mf["transt"] != false {
		t.Errorf("modelFeatures = %+v, want outmode=true outmodeswitch=true transt=false", mf)
	}
	// Still belongs to the yellow/light group like any other light output.
	if dev["primaryGroup"] != 1 {
		t.Errorf("primaryGroup = %v, want 1 (class_yellow_light)", dev["primaryGroup"])
	}
	// Still gets a brightness channel (0-100%) and scene support — only the
	// dimmable-ness differs, per ds-light.pdf's "same internal output value"
	// model for switched devices.
	chDescs := dev["channelDescriptions"].(map[string]any)
	if _, ok := chDescs["0"]; !ok {
		t.Error("expected channel 0 (brightness) to still be present for a switched output")
	}
}

func TestSwitchedOutputStillIsLightOutputForBroadcastAddressing(t *testing.T) {
	// Regression guard: resolveNotificationTargets/isLightOutput must keep
	// treating Output=="light" as light-ish for scene/dim broadcast, whether
	// the device is dimmable or not — both use the same Output string, only
	// the new Dimmable field differs.
	if !isLightOutput("light") {
		t.Fatal("isLightOutput(\"light\") must remain true regardless of Dimmable")
	}
}
