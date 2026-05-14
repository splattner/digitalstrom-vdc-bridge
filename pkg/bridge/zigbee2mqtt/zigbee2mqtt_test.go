package zigbee2mqtt

import (
	"encoding/json"
	"testing"
)

const sampleDevices = `[
  {
    "ieee_address": "0x00158d0001234567",
    "friendly_name": "kitchen_lamp",
    "type": "Router",
    "supported": true,
    "power_source": "Mains (single phase)",
    "software_build_id": "2.3.086",
    "definition": {
      "model": "TRADFRI bulb E27 CWS 806lm",
      "vendor": "IKEA",
      "exposes": [{
        "type": "light",
        "features": [
          {"name":"state","property":"state","access":7},
          {"name":"brightness","property":"brightness","value_max":254,"access":7},
          {"name":"color_xy","property":"color","access":7}
        ]
      }]
    }
  },
  {
    "ieee_address": "0x00158d0007654321",
    "friendly_name": "double_switch",
    "type": "Router",
    "supported": true,
    "definition": {
      "model": "QBKG12LM",
      "vendor": "Aqara",
      "exposes": [
        {"type":"switch","endpoint":"left","features":[{"name":"state","property":"state_left","access":7,"endpoint":"left"}]},
        {"type":"switch","endpoint":"right","features":[{"name":"state","property":"state_right","access":7,"endpoint":"right"}]}
      ]
    }
  },
  {
    "ieee_address": "0xCOORD",
    "type": "Coordinator",
    "friendly_name": "Coordinator"
  }
]`

func TestEndpoints_ColorLight(t *testing.T) {
	var arr []bridgeDevice
	if err := json.Unmarshal([]byte(sampleDevices), &arr); err != nil {
		t.Fatal(err)
	}
	bulb := arr[0]
	eps := bulb.endpoints()
	if len(eps) != 1 {
		t.Fatalf("eps=%d", len(eps))
	}
	ep := eps[0]
	if ep.Kind != "colorlight" {
		t.Errorf("kind=%q want colorlight", ep.Kind)
	}
	if !ep.HasBrightness || !ep.HasColor {
		t.Errorf("features lost: %+v", ep)
	}
	if ep.StateProp != "state" || ep.BrightnessProp != "brightness" || ep.ColorProp != "color" {
		t.Errorf("props=%+v", ep)
	}
	if got := bulb.entityID(ep); got != "0x00158d0001234567" {
		t.Errorf("entityID=%q", got)
	}
	if got := bulb.stateTopic("z2m"); got != "z2m/kitchen_lamp" {
		t.Errorf("stateTopic=%q", got)
	}
	if got := bulb.setTopic("z2m"); got != "z2m/kitchen_lamp/set" {
		t.Errorf("setTopic=%q", got)
	}
}

func TestEndpoints_MultiSwitch(t *testing.T) {
	var arr []bridgeDevice
	_ = json.Unmarshal([]byte(sampleDevices), &arr)
	dev := arr[1]
	eps := dev.endpoints()
	if len(eps) != 2 {
		t.Fatalf("eps=%d", len(eps))
	}
	if eps[0].Kind != "light" || eps[0].StateProp != "state_left" {
		t.Errorf("left ep wrong: %+v", eps[0])
	}
	if got := dev.entityID(eps[1]); got != "0x00158d0007654321:right" {
		t.Errorf("entityID=%q", got)
	}
	if got := dev.displayName(eps[1]); got != "double_switch (right)" {
		t.Errorf("displayName=%q", got)
	}
}

func TestParseEntityID(t *testing.T) {
	for _, tc := range []struct{ in, mac, ep string }{
		{"0xAB", "0xAB", ""},
		{"0xAB:l1", "0xAB", "l1"},
		{"", "", ""},
	} {
		m, e := parseEntityID(tc.in)
		if m != tc.mac || e != tc.ep {
			t.Errorf("parseEntityID(%q)=(%q,%q) want (%q,%q)", tc.in, m, e, tc.mac, tc.ep)
		}
	}
}

func TestStateMessage(t *testing.T) {
	m := stateMessage{
		"state":      "ON",
		"brightness": 127.0,
		"color":      map[string]any{"hue": 200.0, "saturation": 50.0},
	}
	if s, ok := m.stringField("state"); !ok || s != "ON" {
		t.Errorf("state=%q,%v", s, ok)
	}
	if !stateOn("on") || stateOn("off") {
		t.Errorf("stateOn broken")
	}
	if v, _ := m.numberField("brightness"); v != 127 {
		t.Errorf("bri=%v", v)
	}
	c := m.colorField("color")
	if c == nil || c["hue"].(float64) != 200 {
		t.Errorf("color=%v", c)
	}
}

func TestBrightnessConversion(t *testing.T) {
	if briToVDC(254, true) < 99 || briToVDC(254, true) > 100.01 {
		t.Errorf("briToVDC(254,on)=%v", briToVDC(254, true))
	}
	if briToVDC(127, false) != 0 {
		t.Errorf("off should be 0")
	}
	if vdcToBri(0) != 1 || vdcToBri(100) != 254 {
		t.Errorf("vdcToBri bounds wrong: %d %d", vdcToBri(0), vdcToBri(100))
	}
}
