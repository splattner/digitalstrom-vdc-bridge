package shelly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/splattner/vdcgo/pkg/bridge"
)

// newTestPluginWithClient returns a Plugin with a single subscribed
// component whose shared deviceClient's HTTP RPC calls go to ts.
func newTestPluginWithClient(t *testing.T, ts *httptest.Server, comp component) (*Plugin, bridge.Mapping) {
	t.Helper()
	addr := strings.TrimPrefix(ts.URL, "http://")
	devID := "shellytest-aabbcc"
	m := bridge.Mapping{PluginID: "shelly1", RemoteEntityID: entityID(devID, comp), DSUID: "D1", Kind: "light", Name: "Relay"}
	sub := &deviceSub{mapping: m, deviceID: devID, identity: comp, activated: true}
	c := newDeviceClient(addr, devID, "vdcgo-shelly1")
	p := &Plugin{
		id:         "shelly1",
		subscribed: map[string]*deviceSub{m.DSUID: sub},
		clients:    map[string]*sharedClient{devID: {client: c, refs: 1}},
	}
	return p, m
}

// rpcTestServer decodes one JSON-RPC request, records its method and params,
// and replies with the given result.
func rpcTestServer(t *testing.T, gotMethod *string, gotParams *map[string]any, result json.RawMessage) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if gotMethod != nil {
			*gotMethod = req.Method
		}
		if gotParams != nil {
			*gotParams, _ = req.Params.(map[string]any)
		}
		reqID := req.ID
		resp := rpcFrame{ID: &reqID, Result: result}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func TestApplySwitchOn(t *testing.T) {
	var gotMethod string
	var gotParams map[string]any
	ts := rpcTestServer(t, &gotMethod, &gotParams, json.RawMessage(`{"was_on":false}`))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts, component{Kind: "switch", Index: 0})
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 0, Value: 100}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gotMethod != "Switch.Set" {
		t.Errorf("method = %q, want Switch.Set", gotMethod)
	}
	if gotParams["on"] != true {
		t.Errorf("params[on] = %v, want true", gotParams["on"])
	}
	if id, ok := gotParams["id"].(float64); !ok || int(id) != 0 {
		t.Errorf("params[id] = %v, want 0", gotParams["id"])
	}
}

func TestApplySwitchOff(t *testing.T) {
	var gotParams map[string]any
	ts := rpcTestServer(t, nil, &gotParams, json.RawMessage(`{"was_on":true}`))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts, component{Kind: "switch", Index: 0})
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 0, Value: 0}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gotParams["on"] != false {
		t.Errorf("params[on] = %v, want false", gotParams["on"])
	}
}

func TestApplyLightBrightness(t *testing.T) {
	var gotMethod string
	var gotParams map[string]any
	ts := rpcTestServer(t, &gotMethod, &gotParams, json.RawMessage(`{}`))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts, component{Kind: "light", Index: 0})
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 0, Value: 42}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gotMethod != "Light.Set" {
		t.Errorf("method = %q, want Light.Set", gotMethod)
	}
	if gotParams["on"] != true {
		t.Errorf("params[on] = %v, want true", gotParams["on"])
	}
	if bri, ok := gotParams["brightness"].(float64); !ok || bri != 42 {
		t.Errorf("params[brightness] = %v, want 42", gotParams["brightness"])
	}
}

func TestApplyLightZeroTurnsOffWithoutBrightness(t *testing.T) {
	var gotParams map[string]any
	ts := rpcTestServer(t, nil, &gotParams, json.RawMessage(`{}`))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts, component{Kind: "light", Index: 0})
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 0, Value: 0}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gotParams["on"] != false {
		t.Errorf("params[on] = %v, want false", gotParams["on"])
	}
	if _, ok := gotParams["brightness"]; ok {
		t.Errorf("expected no brightness field when turning off, got params=%+v", gotParams)
	}
}

func TestApplyUnknownDeviceErrors(t *testing.T) {
	p := &Plugin{subscribed: map[string]*deviceSub{}, clients: map[string]*sharedClient{}}
	m := bridge.Mapping{RemoteEntityID: "nope:switch:0", DSUID: "DNE"}
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 0, Value: 1}); err == nil {
		t.Fatal("expected error for a device with no active client")
	}
}

func TestApplyIgnoresNonChannelZero(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("expected no RPC call for an unsupported channel")
	}))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts, component{Kind: "switch", Index: 0})
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 1, Value: 1}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func TestApplySurfacesHTTPErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p, m := newTestPluginWithClient(t, ts, component{Kind: "switch", Index: 0})
	if err := p.Apply(context.Background(), m, bridge.Command{Type: "setChannel", Channel: 0, Value: 1}); err == nil {
		t.Fatal("expected error to propagate from a failed HTTP POST")
	}
}

func TestDisplayNameFallsBackToModelThenID(t *testing.T) {
	cases := []struct {
		dev  discoveredDevice
		want string
	}{
		{discoveredDevice{ID: "shelly1-aabbcc", Model: "Plus1PM", Name: "Kitchen"}, "Kitchen"},
		{discoveredDevice{ID: "shelly1-aabbcc", Model: "Plus1PM"}, "Plus1PM"},
		{discoveredDevice{ID: "shelly1-aabbcc"}, "shelly1-aabbcc"},
	}
	for _, tc := range cases {
		if got := displayName(tc.dev); got != tc.want {
			t.Errorf("displayName(%+v) = %q, want %q", tc.dev, got, tc.want)
		}
	}
}

// shellyPluginImplementsPlugin is a compile-time assertion that *Plugin
// satisfies bridge.Plugin.
var _ bridge.Plugin = (*Plugin)(nil)
