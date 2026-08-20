package shelly

import "testing"

func TestEntityIDRoundTrip(t *testing.T) {
	cases := []struct {
		devID string
		c     component
	}{
		{"shellyplus1pm-441793a66a4c", component{Kind: "switch", Index: 0}},
		{"shellypro2-aabbcc", component{Kind: "light", Index: 3}},
	}
	for _, tc := range cases {
		id := entityID(tc.devID, tc.c)
		gotDev, gotC, ok := parseEntityID(id)
		if !ok {
			t.Fatalf("parseEntityID(%q) ok=false", id)
		}
		if gotDev != tc.devID || gotC != tc.c {
			t.Errorf("parseEntityID(%q) = (%q, %+v), want (%q, %+v)", id, gotDev, gotC, tc.devID, tc.c)
		}
	}
}

func TestParseEntityIDRejectsMalformed(t *testing.T) {
	for _, id := range []string{"", "noindex", "onlyonecolon:switch", "dev:switch:notanumber", ":switch:0"} {
		if _, _, ok := parseEntityID(id); ok {
			t.Errorf("parseEntityID(%q) unexpectedly succeeded", id)
		}
	}
}

func TestParseComponents(t *testing.T) {
	status := map[string]map[string]any{
		"switch:0": {"output": false},
		"input:1":  {"state": false},
		"sys":      {"time": "12:00"}, // no numeric suffix, must be skipped
		"pm1:0":    {"voltage": 230.0},
	}
	got := parseComponents(status)
	want := []component{{Kind: "input", Index: 1}, {Kind: "pm1", Index: 0}, {Kind: "switch", Index: 0}}
	if len(got) != len(want) {
		t.Fatalf("parseComponents = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseComponents[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBuildEntitiesSwitchOnlyNoMetering(t *testing.T) {
	// Pro1 shape: a switch with no power metering (no apower/voltage/current/
	// aenergy), just output+counts+temperature.
	components := []component{{Kind: "switch", Index: 0}}
	status := map[string]map[string]any{
		"switch:0": {"output": false, "temperature": map[string]any{"tC": 35.2}},
	}
	entities := buildEntities(components, status, nil)
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities (light + sensor for temperature), got %d: %+v", len(entities), entities)
	}
	if entities[0].Kind != "light" || entities[0].Component != (component{Kind: "switch", Index: 0}) {
		t.Errorf("entities[0] = %+v, want light/switch:0", entities[0])
	}
	if entities[1].Kind != "sensor" {
		t.Errorf("entities[1].Kind = %q, want sensor", entities[1].Kind)
	}
	if len(entities[1].SensorFeatures) != 1 || entities[1].SensorFeatures[0].Field != "temperature.tC" {
		t.Errorf("expected exactly a temperature.tC sensor feature, got %+v", entities[1].SensorFeatures)
	}
}

func TestBuildEntitiesMeteredSwitch(t *testing.T) {
	// Plus1PM shape: full power metering on the switch itself.
	components := []component{{Kind: "switch", Index: 0}}
	status := map[string]map[string]any{
		"switch:0": {
			"output": true, "apower": 12.3, "voltage": 230.0, "current": 0.05,
			"aenergy":     map[string]any{"total": 1500.0},
			"temperature": map[string]any{"tC": 40.0},
		},
	}
	entities := buildEntities(components, status, nil)
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %d: %+v", len(entities), entities)
	}
	sensor := entities[1]
	if sensor.Kind != "sensor" {
		t.Fatalf("entities[1].Kind = %q, want sensor", sensor.Kind)
	}
	if len(sensor.SensorFeatures) != 5 {
		t.Fatalf("expected 5 sensor features (apower/voltage/current/aenergy.total/temperature.tC), got %d: %+v",
			len(sensor.SensorFeatures), sensor.SensorFeatures)
	}
	for i, sf := range sensor.SensorFeatures {
		if sf.Index != i {
			t.Errorf("SensorFeatures[%d].Index = %d, want %d (dense 0-based)", i, sf.Index, i)
		}
	}
}

func TestBuildEntitiesPM1Only(t *testing.T) {
	components := []component{{Kind: "pm1", Index: 0}}
	status := map[string]map[string]any{
		"pm1:0": {"voltage": 228.0, "current": 1.2, "apower": 220.4, "aenergy": map[string]any{"total": 3000.0}},
	}
	entities := buildEntities(components, status, nil)
	if len(entities) != 1 {
		t.Fatalf("expected exactly 1 entity (sensor only, no output), got %d: %+v", len(entities), entities)
	}
	if entities[0].Kind != "sensor" || entities[0].Component != (component{Kind: "sensor", Index: 0}) {
		t.Errorf("entities[0] = %+v, want sensor/sensor:0", entities[0])
	}
	if len(entities[0].SensorFeatures) != 4 {
		t.Errorf("expected 4 sensor features, got %d: %+v", len(entities[0].SensorFeatures), entities[0].SensorFeatures)
	}
}

func TestBuildEntitiesSwitchTypeInputsBecomeBinary(t *testing.T) {
	components := []component{
		{Kind: "switch", Index: 0},
		{Kind: "input", Index: 0},
		{Kind: "input", Index: 1},
	}
	status := map[string]map[string]any{
		"switch:0": {"output": false},
		"input:0":  {"state": false},
		"input:1":  {"state": true},
	}
	entities := buildEntities(components, status, map[int]string{0: "switch", 1: "switch"})
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities (light + binary), got %d: %+v", len(entities), entities)
	}
	binary := entities[1]
	if binary.Kind != "binary" {
		t.Fatalf("entities[1].Kind = %q, want binary (no numerics present)", binary.Kind)
	}
	if len(binary.SensorFeatures) != 0 {
		t.Errorf("expected no sensor features, got %+v", binary.SensorFeatures)
	}
	if len(binary.BinaryFeatures) != 2 {
		t.Fatalf("expected 2 binary features, got %d: %+v", len(binary.BinaryFeatures), binary.BinaryFeatures)
	}
	for i, bf := range binary.BinaryFeatures {
		if bf.Index != i {
			t.Errorf("BinaryFeatures[%d].Index = %d, want %d", i, bf.Index, i)
		}
		if bf.Field != "state" {
			t.Errorf("BinaryFeatures[%d].Field = %q, want state", i, bf.Field)
		}
	}
}

func TestBuildEntitiesButtonTypeInputGetsOwnEntity(t *testing.T) {
	components := []component{
		{Kind: "switch", Index: 0},
		{Kind: "input", Index: 0}, // switch-type -> folded into binary entity
		{Kind: "input", Index: 1}, // button-type -> own entity
	}
	status := map[string]map[string]any{
		"switch:0": {"output": false},
		"input:0":  {"state": false},
		"input:1":  {"state": false},
	}
	entities := buildEntities(components, status, map[int]string{0: "switch", 1: "button"})
	if len(entities) != 3 {
		t.Fatalf("expected 3 entities (light, button, binary), got %d: %+v", len(entities), entities)
	}
	var gotButton, gotBinary bool
	for _, e := range entities {
		switch e.Kind {
		case "button":
			gotButton = true
			if e.Component != (component{Kind: "input", Index: 1}) {
				t.Errorf("button entity Component = %+v, want input:1", e.Component)
			}
		case "binary":
			gotBinary = true
			if len(e.BinaryFeatures) != 1 || e.BinaryFeatures[0].Source != (component{Kind: "input", Index: 0}) {
				t.Errorf("binary entity features = %+v, want exactly input:0", e.BinaryFeatures)
			}
		}
	}
	if !gotButton || !gotBinary {
		t.Fatalf("expected both a button and a binary entity, got %+v", entities)
	}
}

func TestBuildEntitiesNoExtrasWhenNothingPresent(t *testing.T) {
	components := []component{{Kind: "switch", Index: 0}}
	status := map[string]map[string]any{"switch:0": {"output": false}}
	entities := buildEntities(components, status, nil)
	if len(entities) != 1 {
		t.Fatalf("expected exactly 1 entity (light only), got %d: %+v", len(entities), entities)
	}
}

func TestEntityForComponent(t *testing.T) {
	entities := []entitySpec{
		{Component: component{Kind: "switch", Index: 0}, Kind: "light"},
		{Component: component{Kind: "sensor", Index: 0}, Kind: "sensor"},
	}
	if spec, ok := entityForComponent(entities, component{Kind: "switch", Index: 0}); !ok || spec.Kind != "light" {
		t.Errorf("entityForComponent(switch:0) = (%+v, %v), want (light, true)", spec, ok)
	}
	if _, ok := entityForComponent(entities, component{Kind: "light", Index: 0}); ok {
		t.Error("expected entityForComponent to report not found for an absent component")
	}
}

func TestSameComponents(t *testing.T) {
	a := []component{{Kind: "switch", Index: 0}, {Kind: "input", Index: 0}}
	b := []component{{Kind: "switch", Index: 0}, {Kind: "input", Index: 0}}
	c := []component{{Kind: "switch", Index: 0}}
	if !sameComponents(a, b) {
		t.Error("expected identical slices to compare equal")
	}
	if sameComponents(a, c) {
		t.Error("expected slices of different length to compare unequal")
	}
}
