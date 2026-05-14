package tasmota

import (
	"encoding/json"
	"testing"
)

const sampleDiscovery = `{
  "ip":"192.168.1.10","dn":"Light","fn":["Lamp",null,null,null,null,null,null,null],
  "hn":"tasmota-1","mac":"AABBCCDDEEFF","md":"Sonoff Mini","ty":0,"if":0,
  "ofln":"Offline","onln":"Online","state":["OFF","ON","TOGGLE","HOLD"],
  "sw":"12.5.0","t":"tasmota_AABBCC","ft":"%prefix%/%topic%/","tp":["cmnd","stat","tele"],
  "rl":[1,0,0,0,0,0,0,0],"lt_st":0,"ver":1
}`

func TestDiscoveryParse_SingleRelay(t *testing.T) {
	var d discoveryConfig
	if err := json.Unmarshal([]byte(sampleDiscovery), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.relayCount() != 1 {
		t.Fatalf("relayCount = %d, want 1", d.relayCount())
	}
	if got := d.entityID(1); got != "AABBCCDDEEFF" {
		t.Fatalf("entityID(1) = %q, want plain MAC", got)
	}
	if got := d.powerSuffix(1); got != "POWER" {
		t.Fatalf("powerSuffix(1) = %q, want POWER", got)
	}
	if got := d.cmndTopic("POWER"); got != "cmnd/tasmota_AABBCC/POWER" {
		t.Fatalf("cmndTopic = %q", got)
	}
	if got := d.teleTopic("LWT"); got != "tele/tasmota_AABBCC/LWT" {
		t.Fatalf("teleTopic = %q", got)
	}
	if got := d.kindFor(1); got != "light" {
		t.Fatalf("kindFor = %q, want light", got)
	}
	if got := d.friendlyName(1); got != "Lamp" {
		t.Fatalf("friendlyName = %q", got)
	}
}

func TestDiscoveryParse_MultiRelay(t *testing.T) {
	d := discoveryConfig{
		MAC: "AABBCCDDEEFF", Topic: "tasmota_x", FullTop: "%prefix%/%topic%/",
		Prefix:  []string{"cmnd", "stat", "tele"},
		FNames:  []any{"R1", "R2", nil, nil},
		RL:      []int{1, 1, 0, 0, 0, 0, 0, 0},
		DevName: "Strip",
	}
	if d.relayCount() != 2 {
		t.Fatalf("relayCount = %d", d.relayCount())
	}
	if got := d.entityID(2); got != "AABBCCDDEEFF:R2" {
		t.Fatalf("entityID(2) = %q", got)
	}
	if got := d.powerSuffix(2); got != "POWER2" {
		t.Fatalf("powerSuffix(2) = %q", got)
	}
}

func TestDiscoveryParse_RGBLight(t *testing.T) {
	d := discoveryConfig{
		MAC: "M", Topic: "t", FullTop: "%prefix%/%topic%/",
		Prefix: []string{"cmnd", "stat", "tele"},
		RL:     []int{2, 0, 0, 0, 0, 0, 0, 0},
		LtSt:   3, // RGB
	}
	if got := d.kindFor(1); got != "colorlight" {
		t.Fatalf("kindFor = %q, want colorlight", got)
	}
}

func TestParseEntityID(t *testing.T) {
	for _, tc := range []struct {
		in        string
		wantMAC   string
		wantRelay int
	}{
		{"AABB", "AABB", 1},
		{"AABB:R3", "AABB", 3},
		{"AABB:R0", "AABB", 1}, // 0 → defaults to 1
	} {
		mac, relay := parseEntityID(tc.in)
		if mac != tc.wantMAC || relay != tc.wantRelay {
			t.Errorf("parseEntityID(%q) = (%q, %d), want (%q, %d)",
				tc.in, mac, relay, tc.wantMAC, tc.wantRelay)
		}
	}
}

func TestMacFromDiscoveryTopic(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"tasmota/discovery/AABBCCDDEEFF/config", "AABBCCDDEEFF"},
		{"foo/bar/AABB/config", "AABB"},
		{"foo/bar/AABB/sensors", ""},
		{"too/short", ""},
	} {
		if got := macFromDiscoveryTopic(tc.in); got != tc.want {
			t.Errorf("macFromDiscoveryTopic(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStateMessage_PowerForRelay(t *testing.T) {
	m := stateMessage{POWER: "ON"}
	if v, ok := m.powerForRelay(1); !ok || v != "ON" {
		t.Fatalf("single POWER fallback: got (%q,%v)", v, ok)
	}
	m2 := stateMessage{POWER2: "OFF"}
	if v, ok := m2.powerForRelay(2); !ok || v != "OFF" {
		t.Fatalf("POWER2: got (%q,%v)", v, ok)
	}
	if _, ok := m2.powerForRelay(3); ok {
		t.Fatalf("POWER3 absent should return false")
	}
}

func TestParseHSB(t *testing.T) {
	h, s, v, ok := parseHSB("180,50,75")
	if !ok || h != 180 || s != 50 || v != 75 {
		t.Fatalf("parseHSB = (%v,%v,%v,%v)", h, s, v, ok)
	}
	if _, _, _, ok := parseHSB(""); ok {
		t.Fatal("empty should fail")
	}
}
