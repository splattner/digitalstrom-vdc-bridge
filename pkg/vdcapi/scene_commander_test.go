package vdcapi

import (
	"testing"

	"github.com/splattner/vdcgo/pkg/runtime"
)

// callScene5 broadcasts digitalSTROM scene 5 ("Preset 1", a
// sceneActionSetLevel scene per ds-light.pdf's default scenetable) via the
// genericRequest JSON-RPC path, mirroring the pattern already used in
// generic_request_test.go. Each test registers exactly one device, so this
// reaches it unambiguously.
func callScene5(t *testing.T, s *Server, sess *session) {
	t.Helper()
	r, _ := s.processRequest(request{
		ID:     "call-scene-5",
		Method: "genericRequest",
		Params: map[string]any{"methodname": "callScene", "dSUID": "root", "params": map[string]any{"scene": 5}},
	}, sess)
	if r == nil || r.Error != 0 {
		t.Fatalf("expected callScene genericRequest success, got %+v", r)
	}
}

func newSceneTestServer(state *StateStore, commander Commander) (*Server, *session) {
	s := &Server{ServerConfig: ServerConfig{DSUID: "0123456789ABCDEFFEDCBA9876543210AA", Description: "testdesc", State: state, Commander: commander}}
	sess := &session{active: true, vdsmDSUID: "0011", apiVersion: 2}
	return s, sess
}

func TestCallSceneUsesNativeRecallWhenAvailable(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "strip", UniqueID: "u-native"})

	mc := newMockSceneCommander()
	s, sess := newSceneTestServer(state, mc)

	callScene5(t, s, sess)

	calls := mc.sceneCallSnapshot()
	if len(calls) != 1 || calls[0].uniqueID != "u-native" || calls[0].scene != 5 {
		t.Fatalf("expected one native CallScene(u-native, 5), got %+v", calls)
	}
	// Native recall succeeded — the computed-brightness fallback must not
	// also have fired for the same device.
	if called, _, _ := mc.snapshot(); called {
		t.Fatal("expected SetLightLevel fallback to be skipped when native CallScene succeeds")
	}
}

func TestCallSceneFallsBackWhenNativeRecallErrors(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "lamp", UniqueID: "u-unsupported"})

	mc := newMockSceneCommander()
	mc.sceneErr = errSceneNotSupportedForTest
	s, sess := newSceneTestServer(state, mc)

	callScene5(t, s, sess)

	calls := mc.sceneCallSnapshot()
	if len(calls) != 1 {
		t.Fatalf("expected native CallScene to be tried once, got %+v", calls)
	}
	called, uid, value := mc.snapshot()
	if !called || uid != "u-unsupported" || value != 100 {
		t.Fatalf("expected fallback SetLightLevel(u-unsupported, 100) after native error, got called=%t uid=%s value=%v", called, uid, value)
	}
}

func TestCallSceneFallsBackWhenCommanderHasNoSceneSupport(t *testing.T) {
	// A plain mockCommander (HA/Z2M/Tasmota's shape today) doesn't implement
	// SceneCommander at all — behavior must be identical to before this
	// feature existed.
	state := NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "lamp", UniqueID: "u-plain"})

	mc := &mockCommander{}
	s, sess := newSceneTestServer(state, mc)

	callScene5(t, s, sess)

	called, uid, value := mc.snapshot()
	if !called || uid != "u-plain" || value != 100 {
		t.Fatalf("expected fallback SetLightLevel(u-plain, 100), got called=%t uid=%s value=%v", called, uid, value)
	}
}

var errSceneNotSupportedForTest = &sceneNotSupportedError{}

type sceneNotSupportedError struct{}

func (*sceneNotSupportedError) Error() string { return "scene not supported (test)" }
