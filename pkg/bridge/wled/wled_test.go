package wled

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/splattner/vdcgo/pkg/bridge"
)

// newTestPluginWithClient returns a Plugin with a single subscribed device
// whose deviceClient posts to ts (an httptest.Server standing in for the
// WLED device's HTTP API).
func newTestPluginWithClient(t *testing.T, ts *httptest.Server) (*Plugin, bridge.Mapping) {
	t.Helper()
	addr := strings.TrimPrefix(ts.URL, "http://")
	m := bridge.Mapping{PluginID: "wled1", RemoteEntityID: "aabbccddeeff", DSUID: "D1", Kind: "colorlight", Name: "Strip"}
	p := &Plugin{
		id:         "wled1",
		subscribed: map[string]subscription{m.DSUID: {mapping: m, client: newDeviceClient(addr, m.RemoteEntityID)}},
		byMAC:      map[string]string{m.RemoteEntityID: m.DSUID},
	}
	return p, m
}

func TestCallSceneRecallsPresetForNonZeroScene(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/state" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts)
	if err := p.CallScene(context.Background(), m, 5); err != nil {
		t.Fatalf("CallScene: %v", err)
	}
	if gotBody["on"] != true {
		t.Errorf("body[on] = %v, want true", gotBody["on"])
	}
	if ps, ok := gotBody["ps"].(float64); !ok || int(ps) != 5 {
		t.Errorf("body[ps] = %v, want 5", gotBody["ps"])
	}
}

func TestCallSceneZeroTurnsOff(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts)
	if err := p.CallScene(context.Background(), m, 0); err != nil {
		t.Fatalf("CallScene: %v", err)
	}
	if gotBody["on"] != false {
		t.Errorf("body[on] = %v, want false", gotBody["on"])
	}
	if _, ok := gotBody["ps"]; ok {
		t.Errorf("expected no ps field for scene 0, got body=%+v", gotBody)
	}
}

func TestCallSceneUnknownDeviceErrors(t *testing.T) {
	p := &Plugin{id: "wled1", subscribed: map[string]subscription{}, byMAC: map[string]string{}}
	m := bridge.Mapping{PluginID: "wled1", RemoteEntityID: "nope", DSUID: "DNE"}
	if err := p.CallScene(context.Background(), m, 1); err == nil {
		t.Fatal("expected error for a device with no active client")
	}
}

func TestCallSceneSurfacesHTTPErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts)
	if err := p.CallScene(context.Background(), m, 3); err == nil {
		t.Fatal("expected error to propagate from a failed HTTP POST")
	}
}

// wledPluginImplementsSceneCaller is a compile-time assertion that *Plugin
// satisfies bridge.SceneCaller.
var _ bridge.SceneCaller = (*Plugin)(nil)
