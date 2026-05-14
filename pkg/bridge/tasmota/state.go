package tasmota

import (
	"fmt"
	"math"
)

// stateMessage is the relevant subset of `tele/<t>/STATE` and the JSON envelope
// of `stat/<t>/RESULT` (which carries the same keys when responding to a power
// or dimmer command). Fields not present in a given message are zero / nil.
//
// Tasmota only sends the keys that changed in RESULT; STATE always contains
// the full snapshot.
type stateMessage struct {
	POWER    string `json:"POWER"`
	POWER1   string `json:"POWER1"`
	POWER2   string `json:"POWER2"`
	POWER3   string `json:"POWER3"`
	POWER4   string `json:"POWER4"`
	POWER5   string `json:"POWER5"`
	POWER6   string `json:"POWER6"`
	POWER7   string `json:"POWER7"`
	POWER8   string `json:"POWER8"`
	Dimmer   *int   `json:"Dimmer,omitempty"`
	HSBColor string `json:"HSBColor,omitempty"`
	Color    string `json:"Color,omitempty"`
	CT       *int   `json:"CT,omitempty"`
	White    *int   `json:"White,omitempty"`
	Channel  []int  `json:"Channel,omitempty"`
}

// powerForRelay returns the value of POWER<n> for the (1-based) relay, falling
// back to "POWER" for single-relay devices. Returns ("", false) when the
// message did not include a state for that relay.
func (m *stateMessage) powerForRelay(relay int) (string, bool) {
	var v string
	switch relay {
	case 1:
		v = m.POWER1
		if v == "" {
			v = m.POWER
		}
	case 2:
		v = m.POWER2
	case 3:
		v = m.POWER3
	case 4:
		v = m.POWER4
	case 5:
		v = m.POWER5
	case 6:
		v = m.POWER6
	case 7:
		v = m.POWER7
	case 8:
		v = m.POWER8
	}
	return v, v != ""
}

// briToVDC converts a Tasmota Dimmer (0–100) to a vDC percentage.
// When `on` is false the brightness is forced to 0 so the channel reflects
// the off state immediately.
func briToVDC(dimmer int, on bool) float64 {
	if !on {
		return 0
	}
	return clampF(float64(dimmer), 0, 100)
}

// vdcToDimmer converts a vDC percentage to a Tasmota Dimmer value (1–100).
// 0 is special: the caller should send POWER OFF instead.
func vdcToDimmer(pct float64) int {
	v := int(math.Round(clampF(pct, 0, 100)))
	if v < 1 {
		v = 1
	}
	if v > 100 {
		v = 100
	}
	return v
}

// parseHSB parses a Tasmota HSBColor string ("h,s,v" with h:0-360, s:0-100, v:0-100).
func parseHSB(s string) (h, sat, v float64, ok bool) {
	if s == "" {
		return
	}
	var hi, si, vi int
	n, err := fmt.Sscanf(s, "%d,%d,%d", &hi, &si, &vi)
	if err != nil || n != 3 {
		return
	}
	return float64(hi), float64(si), float64(vi), true
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
