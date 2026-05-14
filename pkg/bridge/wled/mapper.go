package wled

import "math"

// wledState is the relevant subset of the WLED JSON state object.
type wledState struct {
	On  bool `json:"on"`
	Bri int  `json:"bri"` // 0-255
	Seg []struct {
		Col [][]int `json:"col"` // [[r,g,b,...], ...]
		CCT int     `json:"cct"` // 0-255 relative, or Kelvin if >255
	} `json:"seg"`
}

// wledInfo is the relevant subset of the WLED JSON info object.
type wledInfo struct {
	Name string `json:"name"`
	MAC  string `json:"mac"`
	Ver  string `json:"ver"`
	Leds struct {
		Count int `json:"count"`
		LC    int `json:"lc"` // light capability bitmask: bit0=RGB, bit1=white, bit2=CCT
	} `json:"leds"`
}

// wledSI is the combined state+info response from /json/si (and WS push).
type wledSI struct {
	State wledState `json:"state"`
	Info  wledInfo  `json:"info"`
}

// lcBitRGB indicates the segment supports RGB color.
const lcBitRGB = 0x01

// lcBitCCT indicates the segment supports color temperature.
const lcBitCCT = 0x04

// classifyKind maps a WLED light-capability bitmask to a bridge kind string.
func classifyKind(lc int) string {
	if lc&lcBitRGB != 0 {
		return "colorlight"
	}
	// white-only or CCT-only → expose as dimmable
	return "dimmer"
}

// briToVDC converts a WLED brightness (0–255) to a vDC percentage (0.0–100.0).
func briToVDC(bri int, on bool) float64 {
	if !on {
		return 0
	}
	return clampF(float64(bri)/255.0*100.0, 0, 100)
}

// vdcToBri converts a vDC percentage (0.0–100.0) to a WLED brightness (0–255).
func vdcToBri(pct float64) int {
	v := int(math.Round(pct / 100.0 * 255.0))
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return v
}

// rgbToHueSat extracts hue (0–360) and saturation (0–100) from an RGB triplet.
// Returns (0,0) for achromatic colours.
func rgbToHueSat(r, g, b int) (hue, sat float64) {
	rf, gf, bf := float64(r)/255.0, float64(g)/255.0, float64(b)/255.0
	mx := math.Max(rf, math.Max(gf, bf))
	mn := math.Min(rf, math.Min(gf, bf))
	delta := mx - mn

	if delta == 0 {
		return 0, 0
	}
	sat = delta / mx * 100.0
	switch mx {
	case rf:
		hue = 60.0 * math.Mod((gf-bf)/delta, 6)
	case gf:
		hue = 60.0 * ((bf-rf)/delta + 2)
	default:
		hue = 60.0 * ((rf-gf)/delta + 4)
	}
	if hue < 0 {
		hue += 360
	}
	return hue, sat
}

// hueSatToRGB converts hue (0–360) and saturation (0–100) to an RGB triplet.
// Brightness is kept at the maximum (value=1 in HSV).
func hueSatToRGB(hue, sat float64) (r, g, b int) {
	s := sat / 100.0
	c := s // V = 1.0, so C = V*S = S
	x := c * (1 - math.Abs(math.Mod(hue/60.0, 2)-1))
	m := 1.0 - c

	var rf, gf, bf float64
	switch {
	case hue < 60:
		rf, gf, bf = c, x, 0
	case hue < 120:
		rf, gf, bf = x, c, 0
	case hue < 180:
		rf, gf, bf = 0, c, x
	case hue < 240:
		rf, gf, bf = 0, x, c
	case hue < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	r = int(math.Round((rf + m) * 255))
	g = int(math.Round((gf + m) * 255))
	b = int(math.Round((bf + m) * 255))
	return
}

// cctToMired converts a WLED CCT value (0–255 relative, or Kelvin) to mired.
// WLED 0–255 maps to ~warmest (~2700 K = 370 mired) … coldest (~6500 K = 154 mired).
func cctToMired(cct int) float64 {
	if cct > 255 {
		// Already a Kelvin value from a newer WLED.
		return 1e6 / float64(cct)
	}
	// Linear interpolation: cct=0 → 2700 K, cct=255 → 6500 K
	kelvin := 2700.0 + float64(cct)/255.0*(6500.0-2700.0)
	return 1e6 / kelvin
}

// miredToCCT converts mired to a WLED CCT value (0–255 relative).
func miredToCCT(mired float64) int {
	if mired <= 0 {
		return 0
	}
	kelvin := 1e6 / mired
	v := int(math.Round((kelvin - 2700.0) / (6500.0 - 2700.0) * 255.0))
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return v
}

// primaryRGB extracts the first (primary) segment's first colour as R, G, B.
// Returns (0,0,0) if not present.
func primaryRGB(s wledState) (r, g, b int) {
	if len(s.Seg) == 0 || len(s.Seg[0].Col) == 0 {
		return 0, 0, 0
	}
	col := s.Seg[0].Col[0]
	if len(col) < 3 {
		return 0, 0, 0
	}
	return col[0], col[1], col[2]
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

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
