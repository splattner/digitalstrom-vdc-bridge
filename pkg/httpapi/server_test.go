package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/splattner/vdcgo/pkg/httpapi"
	"github.com/splattner/vdcgo/pkg/runtime"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	state := vdcapi.NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "Test Light", UniqueID: "test-uid-001"})

	cfg := httpapi.Config{
		Listen:      "127.0.0.1:0",
		DSUID:       "AABBCCDDEEFF00112233445566778899AA",
		Description: "test vdc",
		State:       state,
		Config:      vdcapi.NewConfigStore(),
		Scenes:      vdcapi.NewSceneStore(),
	}
	srv := httpapi.New(cfg)
	return httptest.NewServer(srv.Handler())
}

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Errorf("ok field = %v, want true", body["ok"])
	}
}

func TestDSS(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/dss")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if dsuid, _ := body["vdcDSUID"].(string); dsuid == "" {
		t.Error("vdcDSUID missing in /api/dss response")
	}
}

func TestDevices(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Error("expected at least one device, got empty map")
	}
}

func TestDeviceNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/devices/DOESNOTEXIST")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDeviceFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// First get the list to find the real DSUID
	resp, err := http.Get(ts.URL + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var devices map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	var dsuid string
	for k := range devices {
		dsuid = k
		break
	}
	if dsuid == "" {
		t.Fatal("no devices in list")
	}

	resp2, err := http.Get(ts.URL + "/api/devices/" + dsuid)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for dsuid=%s", resp2.StatusCode, dsuid)
	}
	var dev map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&dev); err != nil {
		t.Fatal(err)
	}
	if dev["dSUID"] != dsuid {
		t.Errorf("device dSUID = %v, want %s", dev["dSUID"], dsuid)
	}
}

func TestCORSHeaders(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("CORS origin = %q, want *", origin)
	}
}

func TestStaticFallback(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}
