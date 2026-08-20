package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

// newShellyTestServer returns an httptest.Server that answers
// Shelly.GetDeviceInfo and Shelly.GetStatus RPC calls for a single fake device.
func newShellyTestServer(t *testing.T, id, app string, gen int, authEn bool, status map[string]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var result any
		switch req.Method {
		case "Shelly.GetDeviceInfo":
			result = map[string]any{
				"id": id, "model": "SNSW-001", "gen": gen, "app": app, "ver": "1.0.0",
				"auth_en": authEn,
			}
		case "Shelly.GetStatus":
			result = status
		default:
			t.Fatalf("unexpected RPC method %q", req.Method)
		}
		resultBytes, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal result: %v", err)
		}
		reqID := req.ID
		resp := rpcFrame{ID: &reqID, Result: resultBytes}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

// serviceEntryForServer builds a synthetic zeroconf.ServiceEntry pointing at
// ts, as if it had been received from an mDNS browse.
func serviceEntryForServer(t *testing.T, ts *httptest.Server, instance string, gen int) *zeroconf.ServiceEntry {
	t.Helper()
	hostPort := strings.TrimPrefix(ts.URL, "http://")
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("split host port %q: %v", hostPort, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi port %q: %v", portStr, err)
	}
	return &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{Instance: instance},
		Port:          port,
		AddrIPv4:      []net.IP{net.ParseIP(host)},
		Text:          []string{fmt.Sprintf("gen=%d", gen), "app=Plus1PM", "ver=1.7.5"},
	}
}

func TestHandleEntryNewDeviceFiresOnFound(t *testing.T) {
	status := map[string]map[string]any{
		"switch:0": {"output": false},
		"input:0":  {"state": false},
	}
	ts := newShellyTestServer(t, "shellyplus1pm-aabbcc", "Plus1PM", 2, false, status)
	defer ts.Close()

	found := make(chan discoveredDevice, 1)
	s := newScanner(func(d discoveredDevice) { found <- d }, nil, nil)

	entry := serviceEntryForServer(t, ts, "shellyplus1pm-aabbcc", 2)
	s.handleEntry(context.Background(), entry)

	select {
	case d := <-found:
		if d.ID != "shellyplus1pm-aabbcc" {
			t.Errorf("ID = %q, want shellyplus1pm-aabbcc", d.ID)
		}
		if len(d.Components) != 2 {
			t.Errorf("Components = %+v, want 2 entries", d.Components)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for onFound")
	}
}

// TestHandleEntryFriendlyNameDuplicateCollapses covers the fleet behavior
// this plugin was specifically designed around: a device with a configured
// friendly name advertises a second, differently-named _shelly._tcp instance
// at the same address. Both must resolve to exactly one discovered device.
func TestHandleEntryFriendlyNameDuplicateCollapses(t *testing.T) {
	status := map[string]map[string]any{"switch:0": {"output": false}}
	ts := newShellyTestServer(t, "shellyplus1pm-aabbcc", "Plus1PM", 2, false, status)
	defer ts.Close()

	var mu sync.Mutex
	var founds []discoveredDevice
	s := newScanner(func(d discoveredDevice) {
		mu.Lock()
		founds = append(founds, d)
		mu.Unlock()
	}, nil, nil)

	e1 := serviceEntryForServer(t, ts, "shellyplus1pm-aabbcc", 2)
	e2 := serviceEntryForServer(t, ts, "Shelly Geschirrspüler", 2) // same addr, friendly-name instance

	s.handleEntry(context.Background(), e1)
	s.handleEntry(context.Background(), e2)

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(founds)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for onFound")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Give a stray second enrichment (if one somehow started) time to land,
	// so a bug would actually be observed here rather than racing the assert.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(founds) != 1 {
		t.Fatalf("expected exactly 1 onFound for two mDNS instances at the same address, got %d: %+v", len(founds), founds)
	}
	if devs := s.All(); len(devs) != 1 {
		t.Fatalf("expected exactly 1 device recorded, got %d", len(devs))
	}
}

func TestHandleEntrySkipsGen1(t *testing.T) {
	ts := newShellyTestServer(t, "shellyswitch-aabbcc", "SW", 1, false, nil)
	defer ts.Close()

	var found int32
	s := newScanner(func(discoveredDevice) { atomic.AddInt32(&found, 1) }, nil, nil)
	entry := serviceEntryForServer(t, ts, "shellyswitch-aabbcc", 1)
	s.handleEntry(context.Background(), entry)

	time.Sleep(100 * time.Millisecond) // enrichment would have fired onFound by now if it ran
	if atomic.LoadInt32(&found) != 0 {
		t.Fatal("expected a Gen1 advertisement to be skipped, but onFound fired")
	}
	if len(s.All()) != 0 {
		t.Fatal("expected no devices recorded for a Gen1 advertisement")
	}
}

func TestHandleEntrySkipsAuthEnabled(t *testing.T) {
	ts := newShellyTestServer(t, "shellypro1-aabbcc", "Pro1", 2, true, map[string]map[string]any{"switch:0": {"output": false}})
	defer ts.Close()

	errCh := make(chan error, 1)
	s := newScanner(nil, nil, func(err error) { errCh <- err })
	entry := serviceEntryForServer(t, ts, "shellypro1-aabbcc", 2)
	s.handleEntry(context.Background(), entry)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected a non-nil error for an auth-enabled device")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for onError")
	}
	if len(s.All()) != 0 {
		t.Fatal("expected an auth-enabled device not to be recorded")
	}
}

func TestHandleEntryIgnoresEntryWithoutIPv4(t *testing.T) {
	s := newScanner(func(discoveredDevice) { t.Error("onFound should not be called") }, nil, nil)
	entry := &zeroconf.ServiceEntry{ServiceRecord: zeroconf.ServiceRecord{Instance: "no-addr"}, Port: 80}
	s.handleEntry(context.Background(), entry)
	if len(s.All()) != 0 {
		t.Fatal("expected no device recorded for an entry with no AddrIPv4")
	}
}
