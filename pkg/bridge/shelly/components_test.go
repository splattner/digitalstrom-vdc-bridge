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

func TestBridgeKindFor(t *testing.T) {
	cases := []struct {
		c        component
		wantKind string
		wantOK   bool
	}{
		{component{Kind: "switch", Index: 0}, "light", true},
		{component{Kind: "light", Index: 0}, "dimmer", true},
		{component{Kind: "pm1", Index: 0}, "", false},
		{component{Kind: "input", Index: 0}, "", false},
	}
	for _, tc := range cases {
		kind, ok := bridgeKindFor(tc.c)
		if kind != tc.wantKind || ok != tc.wantOK {
			t.Errorf("bridgeKindFor(%+v) = (%q, %v), want (%q, %v)", tc.c, kind, ok, tc.wantKind, tc.wantOK)
		}
	}
}

func TestBridgeableCount(t *testing.T) {
	components := []component{
		{Kind: "switch", Index: 0},
		{Kind: "input", Index: 0},
		{Kind: "input", Index: 1},
	}
	if got := bridgeableCount(components); got != 1 {
		t.Errorf("bridgeableCount = %d, want 1", got)
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
