package protocol

import "testing"

func TestParseSimpleLine(t *testing.T) {
	tests := []struct {
		line string
		tag  string
		cmd  string
		val  string
	}{
		{"C0=50", "", "C0", "50"},
		{"A:C0=12.5", "A", "C0", "12.5"},
		{"SYNC", "", "SYNC", ""},
	}
	for _, tc := range tests {
		m, err := ParseSimpleLine(tc.line)
		if err != nil {
			t.Fatalf("unexpected parse error for %q: %v", tc.line, err)
		}
		if m.Tag != tc.tag || m.Cmd != tc.cmd || m.Value != tc.val {
			t.Fatalf("unexpected parse result for %q: %+v", tc.line, m)
		}
	}
}

func TestFormatSimpleStatus(t *testing.T) {
	if got := FormatSimpleStatus(true, "", ""); got != "OK" {
		t.Fatalf("unexpected OK status: %s", got)
	}
	if got := FormatSimpleStatus(false, "boom", "A"); got != "A:ERROR=boom" {
		t.Fatalf("unexpected tagged error status: %s", got)
	}
}
