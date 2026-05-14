package vdcapi

import (
	"testing"
)

func TestSceneStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/scenes.json"

	s := NewSceneStore()
	s.SaveScene("dev1", 5, map[int]float64{0: 88.5})
	s.SetSceneChannelValue("dev1", 5, 0, 88.5)

	if err := s.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	s2 := NewSceneStore()
	if err := s2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	e, ok := s2.GetScene("dev1", 5)
	if !ok {
		t.Fatal("expected scene entry after load")
	}
	if v, ok2 := e.Channels[0]; !ok2 || v.Value != 88.5 {
		t.Fatalf("expected channel 0 = 88.5, got %v", e.Channels)
	}
}

func TestSceneStoreAutoSave(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/scenes.json"

	s := NewSceneStore()
	s.SetAutoSave(path)
	s.SetSceneDontCare("dev2", 3, true)

	s2 := NewSceneStore()
	if err := s2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile after auto-save: %v", err)
	}
	e, ok := s2.GetScene("dev2", 3)
	if !ok || !e.DontCare {
		t.Fatalf("expected dontCare=true after auto-save, got %+v ok=%v", e, ok)
	}
}

func TestSceneStoreLoadNonExistent(t *testing.T) {
	s := NewSceneStore()
	if err := s.LoadFromFile("/tmp/nonexistent-vdcgo-test-scenes.json"); err != nil {
		t.Fatalf("LoadFromFile on missing file should return nil, got %v", err)
	}
}

func TestConfigStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	cs := NewConfigStore()
	cs.SetDeviceName("DEADBEEF", "My Lamp")
	if err := cs.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	cs2 := NewConfigStore()
	if err := cs2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	name, ok := cs2.GetDeviceName("DEADBEEF")
	if !ok || name != "My Lamp" {
		t.Fatalf("expected 'My Lamp', got %q ok=%v", name, ok)
	}
}

func TestConfigStoreLoadNonExistent(t *testing.T) {
	cs := NewConfigStore()
	if err := cs.LoadFromFile("/tmp/nonexistent-vdcgo-test-config.json"); err != nil {
		t.Fatalf("LoadFromFile on missing file should return nil, got %v", err)
	}
}
