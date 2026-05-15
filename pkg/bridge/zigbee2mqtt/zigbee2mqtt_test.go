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
	eps := bulb.endpointsWithOpts(endpointOpts{})
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
	eps := dev.endpointsWithOpts(endpointOpts{})
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

// ── sensor device fixtures ────────────────────────────────────────────────────

const sensorDevices = `[
  {
    "ieee_address": "0xSENSOR001",
    "friendly_name": "bedroom_climate",
    "type": "EndDevice",
    "supported": true,
    "definition": {
      "model": "WSDCGQ11LM",
      "vendor": "Aqara",
      "exposes": [
        {"type":"numeric","name":"temperature","property":"temperature","unit":"\u00b0C","value_min":-40,"value_max":60,"access":1},
        {"type":"numeric","name":"humidity","property":"humidity","unit":"%","value_min":0,"value_max":100,"access":1},
        {"type":"numeric","name":"pressure","property":"pressure","unit":"hPa","value_min":800,"value_max":1200,"access":1},
        {"type":"numeric","name":"battery","property":"battery","unit":"%","value_min":0,"value_max":100,"access":1},
        {"type":"numeric","name":"linkquality","property":"linkquality","unit":"lqi","value_min":0,"value_max":255,"access":1}
      ]
    }
  },
  {
    "ieee_address": "0xCONTACT001",
    "friendly_name": "door_sensor",
    "type": "EndDevice",
    "supported": true,
    "definition": {
      "model": "MCCGQ11LM",
      "vendor": "Aqara",
      "exposes": [
        {"type":"binary","name":"contact","property":"contact","value_on":true,"value_off":false,"access":1},
        {"type":"numeric","name":"battery","property":"battery","unit":"%","value_min":0,"value_max":100,"access":1},
        {"type":"numeric","name":"linkquality","property":"linkquality","unit":"lqi","access":1}
      ]
    }
  },
  {
    "ieee_address": "0xPLUG001",
    "friendly_name": "smart_plug",
    "type": "Router",
    "supported": true,
    "definition": {
      "model": "SP-EUC01",
      "vendor": "Tuya",
      "exposes": [
        {"type":"switch","features":[{"name":"state","property":"state","access":7}]},
        {"type":"numeric","name":"power","property":"power","unit":"W","value_min":0,"value_max":3000,"access":1},
        {"type":"numeric","name":"energy","property":"energy","unit":"kWh","value_min":0,"value_max":1000,"access":1}
      ]
    }
  }
]`

func TestEndpoints_SensorOnly(t *testing.T) {
	var arr []bridgeDevice
	if err := json.Unmarshal([]byte(sensorDevices), &arr); err != nil {
		t.Fatal(err)
	}
	sensor := arr[0]

	// Without battery/linkquality: 3 sensor features (temp, hum, pressure)
	eps := sensor.endpointsWithOpts(endpointOpts{})
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(eps))
	}
	ep := eps[0]
	if ep.Kind != "sensor" {
		t.Errorf("kind=%q want sensor", ep.Kind)
	}
	if ep.Name != "" {
		t.Errorf("name=%q want empty (sensor-only device)", ep.Name)
	}
	if len(ep.SensorFeatures) != 3 {
		t.Errorf("sensor features=%d want 3 (temp+hum+pressure)", len(ep.SensorFeatures))
	}
	// Check temperature meta
	sf := ep.SensorFeatures[0]
	if sf.Property != "temperature" || sf.Meta.Type != 1 || sf.Meta.Symbol != "°C" {
		t.Errorf("temp feature=%+v", sf)
	}
	// Indices are sequential
	for i, f := range ep.SensorFeatures {
		if f.Index != i {
			t.Errorf("feature[%d].Index=%d want %d", i, f.Index, i)
		}
	}

	// With battery included: 4 sensor features
	eps2 := sensor.endpointsWithOpts(endpointOpts{IncludeBattery: true})
	if len(eps2[0].SensorFeatures) != 4 {
		t.Errorf("sensor features with battery=%d want 4", len(eps2[0].SensorFeatures))
	}

	// With linkquality included: 4 features
	eps3 := sensor.endpointsWithOpts(endpointOpts{IncludeLinkquality: true})
	if len(eps3[0].SensorFeatures) != 4 {
		t.Errorf("sensor features with lqi=%d want 4", len(eps3[0].SensorFeatures))
	}

	// EntityID should just be the IEEE for sensor-only device
	if got := sensor.entityID(ep); got != "0xSENSOR001" {
		t.Errorf("entityID=%q want 0xSENSOR001", got)
	}
	// DisplayName should just be the friendly name
	if got := sensor.displayName(ep); got != "bedroom_climate" {
		t.Errorf("displayName=%q want bedroom_climate", got)
	}
}

func TestEndpoints_ContactSensor(t *testing.T) {
	var arr []bridgeDevice
	_ = json.Unmarshal([]byte(sensorDevices), &arr)
	door := arr[1]

	eps := door.endpointsWithOpts(endpointOpts{})
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(eps))
	}
	ep := eps[0]
	// Only binary feature → kind should be binaryInput
	if ep.Kind != "binaryInput" {
		t.Errorf("kind=%q want binaryInput", ep.Kind)
	}
	if len(ep.BinaryFeatures) != 1 {
		t.Errorf("binary features=%d want 1", len(ep.BinaryFeatures))
	}
	bf := ep.BinaryFeatures[0]
	if bf.Property != "contact" || bf.Function != 13 {
		t.Errorf("contact feature=%+v", bf)
	}
}

func TestEndpoints_SmartPlug_WithSensor(t *testing.T) {
	var arr []bridgeDevice
	_ = json.Unmarshal([]byte(sensorDevices), &arr)
	plug := arr[2]

	eps := plug.endpointsWithOpts(endpointOpts{})
	// switch endpoint + sensor endpoint
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d: %+v", len(eps), eps)
	}
	switchEp := eps[0]
	sensorEp := eps[1]
	if switchEp.Kind != "light" {
		t.Errorf("switch ep kind=%q", switchEp.Kind)
	}
	if sensorEp.Kind != "sensor" {
		t.Errorf("sensor ep kind=%q", sensorEp.Kind)
	}
	// Mixed device: sensor endpoint gets Name="sensor"
	if sensorEp.Name != "sensor" {
		t.Errorf("sensor ep name=%q want sensor", sensorEp.Name)
	}
	if len(sensorEp.SensorFeatures) != 2 {
		t.Errorf("sensor features=%d want 2 (power+energy)", len(sensorEp.SensorFeatures))
	}
	// DisplayName for sensor endpoint should show "· sensors"
	if got := plug.displayName(sensorEp); got != "smart_plug · sensors" {
		t.Errorf("displayName=%q want 'smart_plug · sensors'", got)
	}
}

func TestBoolField(t *testing.T) {
	m := stateMessage{
		"contact":    true,
		"occupancy":  false,
		"water_leak": "true",
		"smoke":      "ON",
		"power":      1.0,
		"absent":     nil,
	}
	cases := []struct {
		key  string
		want bool
		ok   bool
	}{
		{"contact", true, true},
		{"occupancy", false, true},
		{"water_leak", true, true},
		{"smoke", true, true},
		{"power", true, true},
		{"missing", false, false},
	}
	for _, tc := range cases {
		got, gotOK := m.boolField(tc.key)
		if got != tc.want || gotOK != tc.ok {
			t.Errorf("boolField(%q)=(%v,%v) want (%v,%v)", tc.key, got, gotOK, tc.want, tc.ok)
		}
	}
}
