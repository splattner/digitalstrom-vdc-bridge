package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMappingStoreAddGetRemove(t *testing.T) {
	ms := NewMappingStore()

	m := Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1", Kind: "light", Name: "Lamp"}
	added, err := ms.Add(m)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !added {
		t.Fatal("expected Add to report newly added")
	}

	got, ok := ms.Get("D1")
	if !ok || got != m {
		t.Fatalf("Get(D1) = %+v, %t; want %+v, true", got, ok, m)
	}

	// Re-adding the same DSUID updates in place and reports not-new.
	m2 := m
	m2.Name = "Lamp Renamed"
	added, err = ms.Add(m2)
	if err != nil {
		t.Fatalf("Add (update): %v", err)
	}
	if added {
		t.Fatal("expected re-Add of existing DSUID to report not newly added")
	}
	got, _ = ms.Get("D1")
	if got.Name != "Lamp Renamed" {
		t.Fatalf("expected update to take effect, got name=%q", got.Name)
	}

	removed, err := ms.Remove("D1")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatal("expected Remove to report existed")
	}
	if _, ok := ms.Get("D1"); ok {
		t.Fatal("expected mapping gone after Remove")
	}

	removed, err = ms.Remove("D1")
	if err != nil {
		t.Fatalf("Remove (again): %v", err)
	}
	if removed {
		t.Fatal("expected second Remove to report not-existed")
	}
}

func TestMappingStoreGetByRemote(t *testing.T) {
	ms := NewMappingStore()
	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1"})
	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e2", DSUID: "D2"})
	_, _ = ms.Add(Mapping{PluginID: "p2", RemoteEntityID: "e1", DSUID: "D3"})

	m, ok := ms.GetByRemote("p1", "e2")
	if !ok || m.DSUID != "D2" {
		t.Fatalf("GetByRemote(p1,e2) = %+v, %t; want DSUID=D2, true", m, ok)
	}

	if _, ok := ms.GetByRemote("p1", "nope"); ok {
		t.Fatal("expected GetByRemote to report not found for unknown remote entity")
	}
	if _, ok := ms.GetByRemote("nope", "e1"); ok {
		t.Fatal("expected GetByRemote to report not found for unknown plugin")
	}
}

func TestMappingStoreListAndListForPlugin(t *testing.T) {
	ms := NewMappingStore()
	if got := ms.List(); len(got) != 0 {
		t.Fatalf("expected empty List() on fresh store, got %+v", got)
	}

	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1"})
	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e2", DSUID: "D2"})
	_, _ = ms.Add(Mapping{PluginID: "p2", RemoteEntityID: "e1", DSUID: "D3"})

	all := ms.List()
	if len(all) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(all))
	}

	p1 := ms.ListForPlugin("p1")
	if len(p1) != 2 {
		t.Fatalf("expected 2 mappings for p1, got %d: %+v", len(p1), p1)
	}
	for _, m := range p1 {
		if m.PluginID != "p1" {
			t.Fatalf("ListForPlugin(p1) returned foreign mapping %+v", m)
		}
	}

	if got := ms.ListForPlugin("nope"); len(got) != 0 {
		t.Fatalf("expected 0 mappings for unknown plugin, got %+v", got)
	}
}

func TestMappingStoreListIsACopy(t *testing.T) {
	ms := NewMappingStore()
	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1", Name: "orig"})

	got := ms.List()
	got[0].Name = "mutated by caller"

	fresh, _ := ms.Get("D1")
	if fresh.Name != "orig" {
		t.Fatalf("expected List() to return a copy that mutation doesn't leak back, got name=%q", fresh.Name)
	}
}

func TestMappingStoreLoadFromFileMissingIsNotError(t *testing.T) {
	ms := NewMappingStore()
	if err := ms.LoadFromFile(filepath.Join(t.TempDir(), "does-not-exist.json")); err != nil {
		t.Fatalf("expected missing file to be silently ignored, got %v", err)
	}
	if got := ms.List(); len(got) != 0 {
		t.Fatalf("expected empty store after loading missing file, got %+v", got)
	}
}

func TestMappingStorePersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridges.json")

	ms := NewMappingStore()
	ms.SetAutoSave(path)

	m1 := Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1", Kind: "light", Name: "Lamp"}
	m2 := Mapping{PluginID: "p2", RemoteEntityID: "e2", DSUID: "D2", Kind: "sensor", Name: "Temp"}
	if _, err := ms.Add(m1); err != nil {
		t.Fatalf("Add m1: %v", err)
	}
	if _, err := ms.Add(m2); err != nil {
		t.Fatalf("Add m2: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected auto-save to write %s on Add, got %v", path, err)
	}

	// Load into a fresh store and verify both mappings survive.
	loaded := NewMappingStore()
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if got := loaded.List(); len(got) != 2 {
		t.Fatalf("expected 2 mappings after reload, got %d: %+v", len(got), got)
	}
	if got, ok := loaded.Get("D1"); !ok || got != m1 {
		t.Fatalf("reloaded D1 = %+v, %t; want %+v, true", got, ok, m1)
	}

	// Removing a mapping auto-saves too.
	if _, err := ms.Remove("D1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	reloaded := NewMappingStore()
	if err := reloaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile after remove: %v", err)
	}
	if got := reloaded.List(); len(got) != 1 || got[0].DSUID != "D2" {
		t.Fatalf("expected only D2 to remain after Remove+reload, got %+v", got)
	}
}

func TestMappingStoreSaveWithoutAutoSavePathIsNoop(t *testing.T) {
	ms := NewMappingStore()
	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1"})
	if err := ms.Save(); err != nil {
		t.Fatalf("expected Save() with no configured path to be a no-op, got error: %v", err)
	}
}

func TestMappingStoreSaveWritesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "bridges.json")
	// Save must not create missing parent directories — it uses os.WriteFile,
	// which requires the directory to already exist.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ms := NewMappingStore()
	ms.SetAutoSave(path)
	_, _ = ms.Add(Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1"})

	// Overwrite the file to prove Save() rewrites it from current state.
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}
	if err := ms.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded := NewMappingStore()
	if err := reloaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if got := reloaded.List(); len(got) != 1 || got[0].DSUID != "D1" {
		t.Fatalf("expected Save() to rewrite the file with current state, got %+v", got)
	}
}
