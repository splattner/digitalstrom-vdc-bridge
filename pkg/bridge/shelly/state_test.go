package shelly

import "testing"

func TestDeviceStatusMergeAndOverwrite(t *testing.T) {
	s := newDeviceStatus()
	s.merge(map[string]any{
		"ts":       123.0,
		"switch:0": map[string]any{"output": false, "apower": 0.0},
	})
	fields, ok := s.component("switch:0")
	if !ok {
		t.Fatal("expected switch:0 component")
	}
	if fields["output"] != false {
		t.Errorf("output = %v, want false", fields["output"])
	}

	// A delta only carries the fields that changed.
	s.merge(map[string]any{"switch:0": map[string]any{"output": true}})
	fields, _ = s.component("switch:0")
	if fields["output"] != true {
		t.Errorf("output after delta = %v, want true", fields["output"])
	}
	if fields["apower"] != 0.0 {
		t.Errorf("apower should survive an unrelated delta, got %v", fields["apower"])
	}
}

func TestDeviceStatusMergeNullDeletesField(t *testing.T) {
	s := newDeviceStatus()
	s.merge(map[string]any{"switch:0": map[string]any{"output": true, "timer_started_at": 123.0}})
	s.merge(map[string]any{"switch:0": map[string]any{"timer_started_at": nil}})
	fields, _ := s.component("switch:0")
	if _, ok := fields["timer_started_at"]; ok {
		t.Errorf("expected timer_started_at to be removed after a null delta, got %+v", fields)
	}
	if fields["output"] != true {
		t.Errorf("unrelated field should be untouched, got %v", fields["output"])
	}
}

func TestDeviceStatusIgnoresTsKey(t *testing.T) {
	s := newDeviceStatus()
	s.merge(map[string]any{"ts": 123.0})
	if _, ok := s.component("ts"); ok {
		t.Error("expected 'ts' not to be treated as a component")
	}
}

func TestDeviceStatusUnknownComponentNotFound(t *testing.T) {
	s := newDeviceStatus()
	if _, ok := s.component("switch:0"); ok {
		t.Error("expected component() to report not found before any merge")
	}
}

func TestSwitchChannelValue(t *testing.T) {
	cases := []struct {
		fields map[string]any
		wantV  float64
		wantOK bool
	}{
		{map[string]any{"output": true}, 100, true},
		{map[string]any{"output": false}, 0, true},
		{map[string]any{}, 0, false},
	}
	for _, tc := range cases {
		v, ok := switchChannelValue(tc.fields)
		if v != tc.wantV || ok != tc.wantOK {
			t.Errorf("switchChannelValue(%+v) = (%v, %v), want (%v, %v)", tc.fields, v, ok, tc.wantV, tc.wantOK)
		}
	}
}

func TestLightChannelValue(t *testing.T) {
	cases := []struct {
		fields map[string]any
		wantV  float64
		wantOK bool
	}{
		{map[string]any{"output": true, "brightness": 42.0}, 42, true},
		{map[string]any{"output": false, "brightness": 42.0}, 0, true},
		{map[string]any{"output": true}, 100, true},                      // no brightness reported — assume full
		{map[string]any{"output": true, "brightness": 150.0}, 100, true}, // clamp
		{map[string]any{}, 0, false},
	}
	for _, tc := range cases {
		v, ok := lightChannelValue(tc.fields)
		if v != tc.wantV || ok != tc.wantOK {
			t.Errorf("lightChannelValue(%+v) = (%v, %v), want (%v, %v)", tc.fields, v, ok, tc.wantV, tc.wantOK)
		}
	}
}
