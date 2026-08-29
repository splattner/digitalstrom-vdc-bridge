package shelly

import (
	"context"
	"sync"
	"testing"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/services/mqtt"
)

// fakeHost is a minimal bridge.Host recording every call this test file
// cares about, mirroring pkg/bridge/registry_test.go's fakeHost — that one
// is unexported in package bridge, so it can't be reused here.
type fakeHost struct {
	mu sync.Mutex

	channels          map[string]map[int]float64
	sensors           map[string]map[int]float64
	inputs            map[string]map[int]float64
	active            map[string]bool
	buttonPulses      []buttonPulse
	buttonActions     []buttonActionCall
	sensorDescriptors map[string]map[int]bridge.SensorDescriptor
	binaryDescriptors map[string]map[int]bridge.BinaryInputDescriptor
	reannounced       []string
}

type buttonPulse struct {
	dsuid string
	index int
	value float64
}

type buttonActionCall struct {
	dsuid  string
	index  int
	action string
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		channels:          make(map[string]map[int]float64),
		sensors:           make(map[string]map[int]float64),
		inputs:            make(map[string]map[int]float64),
		active:            make(map[string]bool),
		sensorDescriptors: make(map[string]map[int]bridge.SensorDescriptor),
		binaryDescriptors: make(map[string]map[int]bridge.BinaryInputDescriptor),
	}
}

func (h *fakeHost) DeriveDSUID(_, remoteEntityID string) string          { return "DSUID-" + remoteEntityID }
func (h *fakeHost) AnnounceDevice(context.Context, bridge.Mapping) error { return nil }
func (h *fakeHost) RemoveDevice(context.Context, string) error           { return nil }

func (h *fakeHost) UpdateChannel(_ context.Context, dsuid string, idx int, v float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[dsuid] == nil {
		h.channels[dsuid] = make(map[int]float64)
	}
	h.channels[dsuid][idx] = v
	return nil
}

func (h *fakeHost) UpdateButton(_ context.Context, dsuid string, idx int, v float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buttonPulses = append(h.buttonPulses, buttonPulse{dsuid, idx, v})
	return nil
}

func (h *fakeHost) SetButtonAction(_ context.Context, dsuid string, idx int, action string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buttonActions = append(h.buttonActions, buttonActionCall{dsuid, idx, action})
	return nil
}

func (h *fakeHost) UpdateSensor(_ context.Context, dsuid string, idx int, v float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sensors[dsuid] == nil {
		h.sensors[dsuid] = make(map[int]float64)
	}
	h.sensors[dsuid][idx] = v
	return nil
}

func (h *fakeHost) SetSensorDescriptor(_ context.Context, dsuid string, idx int, d bridge.SensorDescriptor) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sensorDescriptors[dsuid] == nil {
		h.sensorDescriptors[dsuid] = make(map[int]bridge.SensorDescriptor)
	}
	h.sensorDescriptors[dsuid][idx] = d
	return nil
}

func (h *fakeHost) SetButtonDescriptor(context.Context, string, int, bridge.ButtonDescriptor) error {
	return nil
}

func (h *fakeHost) AnnounceRichDevice(ctx context.Context, m bridge.Mapping, _ bridge.DeviceDescriptor) error {
	return h.AnnounceDevice(ctx, m)
}

func (h *fakeHost) UpdateInput(_ context.Context, dsuid string, idx int, v float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.inputs[dsuid] == nil {
		h.inputs[dsuid] = make(map[int]float64)
	}
	h.inputs[dsuid][idx] = v
	return nil
}

func (h *fakeHost) SetBinaryInputDescriptor(_ context.Context, dsuid string, idx int, d bridge.BinaryInputDescriptor) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.binaryDescriptors[dsuid] == nil {
		h.binaryDescriptors[dsuid] = make(map[int]bridge.BinaryInputDescriptor)
	}
	h.binaryDescriptors[dsuid][idx] = d
	return nil
}

func (h *fakeHost) UpdateDeviceMeta(context.Context, string, string, string, string) error {
	return nil
}

func (h *fakeHost) ReAnnounce(_ context.Context, dsuid string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reannounced = append(h.reannounced, dsuid)
	return nil
}

func (h *fakeHost) UpdateActive(_ context.Context, dsuid string, active bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active[dsuid] = active
	return nil
}

func (h *fakeHost) HasDevice(string) bool                               { return true }
func (h *fakeHost) MQTT() *mqtt.Manager                                 { return nil }
func (h *fakeHost) Log(bridge.LogLevel, string, string, map[string]any) {}
func (h *fakeHost) NotifyDiscoveryChanged()                             {}

// newTestPluginForEntities returns a Plugin wired to a fresh fakeHost, with
// empty subscribed/clients maps — enough to drive pushDescriptors/
// handleStatus/handleEvents directly without any real network client.
func newTestPluginForEntities(id string) (*Plugin, *fakeHost) {
	h := newFakeHost()
	p := &Plugin{
		id:         id,
		host:       h,
		subscribed: make(map[string]*deviceSub),
		clients:    make(map[string]*sharedClient),
	}
	return p, h
}

func TestPushDescriptorsAndApplySpecForSensorEntity(t *testing.T) {
	p, h := newTestPluginForEntities("shelly1")

	spec := entitySpec{
		Component: component{Kind: "sensor", Index: 0}, Kind: "sensor",
		SensorFeatures: []sensorFeature{
			{Source: component{Kind: "switch", Index: 0}, Field: "apower", Index: 0, Meta: shellySensorMeta["apower"]},
			{Source: component{Kind: "switch", Index: 0}, Field: "aenergy.total", Index: 1, Meta: shellySensorMeta["aenergy.total"]},
		},
	}
	m := bridge.Mapping{DSUID: "SENSORDSUID"}
	sub := &deviceSub{mapping: m, deviceID: "dev1", identity: spec.Component, spec: spec, activated: true}
	p.subscribed[m.DSUID] = sub

	p.pushDescriptors(sub)

	if got := h.sensorDescriptors["SENSORDSUID"][0].Type; got != 14 {
		t.Errorf("descriptor[0].Type = %d, want 14 (power)", got)
	}
	if got := h.sensorDescriptors["SENSORDSUID"][1].Type; got != 16 {
		t.Errorf("descriptor[1].Type = %d, want 16 (energy)", got)
	}
	if len(h.reannounced) != 1 || h.reannounced[0] != "SENSORDSUID" {
		t.Errorf("expected exactly one ReAnnounce for SENSORDSUID, got %+v", h.reannounced)
	}

	p.handleStatus("dev1", map[string]map[string]any{
		"switch:0": {"apower": 42.0, "aenergy": map[string]any{"total": 1500.0}},
	})

	if v := h.sensors["SENSORDSUID"][0]; v != 42.0 {
		t.Errorf("sensor[0] = %v, want 42.0 (apower)", v)
	}
	if v := h.sensors["SENSORDSUID"][1]; v != 1.5 {
		t.Errorf("sensor[1] = %v, want 1.5 (aenergy.total Wh->kWh)", v)
	}
	if !h.active["SENSORDSUID"] {
		t.Error("expected UpdateActive(true) after a status push")
	}
}

func TestApplySpecForBinaryEntity(t *testing.T) {
	p, h := newTestPluginForEntities("shelly1")

	spec := entitySpec{
		Component: component{Kind: "sensor", Index: 0}, Kind: "binary",
		BinaryFeatures: []binaryFeature{
			{Source: component{Kind: "input", Index: 0}, Field: "state", Index: 0},
			{Source: component{Kind: "input", Index: 1}, Field: "state", Index: 1},
		},
	}
	m := bridge.Mapping{DSUID: "BINARYDSUID"}
	sub := &deviceSub{mapping: m, deviceID: "dev1", identity: spec.Component, spec: spec, activated: true}
	p.subscribed[m.DSUID] = sub

	p.pushDescriptors(sub)
	if len(h.binaryDescriptors["BINARYDSUID"]) != 2 {
		t.Fatalf("expected 2 binary descriptors, got %d", len(h.binaryDescriptors["BINARYDSUID"]))
	}
	if len(h.sensorDescriptors["BINARYDSUID"]) != 0 {
		t.Errorf("expected no sensor descriptors for a pure-binary entity, got %+v", h.sensorDescriptors["BINARYDSUID"])
	}

	p.handleStatus("dev1", map[string]map[string]any{
		"input:0": {"state": false},
		"input:1": {"state": true},
	})

	if v := h.inputs["BINARYDSUID"][0]; v != 0.0 {
		t.Errorf("input[0] = %v, want 0.0", v)
	}
	if v := h.inputs["BINARYDSUID"][1]; v != 1.0 {
		t.Errorf("input[1] = %v, want 1.0", v)
	}
}

func TestHandleEventsDispatchesButtonActions(t *testing.T) {
	p, h := newTestPluginForEntities("shelly1")

	spec := entitySpec{Component: component{Kind: "input", Index: 1}, Kind: "button"}
	m := bridge.Mapping{DSUID: "BUTTONDSUID"}
	sub := &deviceSub{mapping: m, deviceID: "dev1", identity: spec.Component, spec: spec, activated: true}
	p.subscribed[m.DSUID] = sub

	p.handleEvents("dev1", []shellyEvent{{Component: "input:1", Event: "single_push"}})

	if len(h.buttonActions) != 1 || h.buttonActions[0].action != "tip" {
		t.Fatalf("expected one 'tip' action, got %+v", h.buttonActions)
	}
	if len(h.buttonPulses) != 2 || h.buttonPulses[0].value != 1 || h.buttonPulses[1].value != 0 {
		t.Fatalf("expected a 1->0 UpdateButton pulse, got %+v", h.buttonPulses)
	}
}

func TestHandleEventsIgnoresOtherDeviceAndUnmappedEvents(t *testing.T) {
	p, h := newTestPluginForEntities("shelly1")
	spec := entitySpec{Component: component{Kind: "input", Index: 0}, Kind: "button"}
	m := bridge.Mapping{DSUID: "BUTTONDSUID"}
	sub := &deviceSub{mapping: m, deviceID: "dev1", identity: spec.Component, spec: spec, activated: true}
	p.subscribed[m.DSUID] = sub

	p.handleEvents("otherdev", []shellyEvent{{Component: "input:0", Event: "single_push"}})
	p.handleEvents("dev1", []shellyEvent{{Component: "input:0", Event: "config_changed"}})

	if len(h.buttonActions) != 0 {
		t.Fatalf("expected no button actions, got %+v", h.buttonActions)
	}
}

func TestMapInputEvent(t *testing.T) {
	cases := map[string]string{
		"single_push": "tip",
		"double_push": "tip2",
		"triple_push": "tip3",
		"long_push":   "hold",
		"btn_down":    "",
	}
	for event, want := range cases {
		if got := mapInputEvent(event); got != want {
			t.Errorf("mapInputEvent(%q) = %q, want %q", event, got, want)
		}
	}
}

func TestDiscoverNamesMultiEntityDevicesWithSuffix(t *testing.T) {
	p, _ := newTestPluginForEntities("shelly1")
	p.scanner = newScanner(nil, nil, nil)
	dev := discoveredDevice{
		ID: "dev1", Model: "Plus1PM",
		Entities: []entitySpec{
			{Component: component{Kind: "switch", Index: 0}, Kind: "light"},
			{Component: component{Kind: "sensor", Index: 0}, Kind: "sensor"},
		},
	}
	p.scanner.devices[dev.ID] = dev

	entities, err := p.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %d: %+v", len(entities), entities)
	}
	names := make(map[string]string)
	for _, e := range entities {
		names[e.Kind] = e.Name
	}
	if names["light"] != "Plus1PM · switch:0" {
		t.Errorf("light entity name = %q, want %q", names["light"], "Plus1PM · switch:0")
	}
	if names["sensor"] != "Plus1PM · sensors" {
		t.Errorf("sensor entity name = %q, want %q", names["sensor"], "Plus1PM · sensors")
	}
}

func TestStatsCountsEntitiesAcrossDevices(t *testing.T) {
	p, _ := newTestPluginForEntities("shelly1")
	p.scanner = newScanner(nil, nil, nil)
	p.scanner.devices["dev1"] = discoveredDevice{ID: "dev1", Entities: []entitySpec{{Kind: "light"}, {Kind: "sensor"}}}
	p.scanner.devices["dev2"] = discoveredDevice{ID: "dev2", Entities: []entitySpec{{Kind: "light"}}}

	stats := p.Stats()
	if stats.Discovered != 3 {
		t.Errorf("Discovered = %d, want 3", stats.Discovered)
	}
}

// fakeHostImplementsBridgeHost is a compile-time assertion.
var _ bridge.Host = (*fakeHost)(nil)
