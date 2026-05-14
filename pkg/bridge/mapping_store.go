package bridge

import (
	"encoding/json"
	"os"
	"sync"
)

// MappingStore persists plugin→remoteEntity→DSUID mappings across restarts.
// Backed by a single JSON file; all methods are safe for concurrent use.
type MappingStore struct {
	mu       sync.Mutex
	mappings map[string]Mapping // keyed by DSUID
	path     string
}

// NewMappingStore returns an empty in-memory store.
// Call LoadFromFile to populate from disk, then SetAutoSave to enable write-through.
func NewMappingStore() *MappingStore {
	return &MappingStore{mappings: make(map[string]Mapping)}
}

// SetAutoSave configures the file to write on every mutation.
func (ms *MappingStore) SetAutoSave(path string) {
	ms.mu.Lock()
	ms.path = path
	ms.mu.Unlock()
}

// LoadFromFile reads mappings from a JSON file. Missing file is silently ignored.
func (ms *MappingStore) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []Mapping
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	ms.mu.Lock()
	for _, m := range list {
		ms.mappings[m.DSUID] = m
	}
	ms.mu.Unlock()
	return nil
}

// Save writes all mappings to the configured auto-save file.
func (ms *MappingStore) Save() error {
	ms.mu.Lock()
	path := ms.path
	list := ms.list()
	ms.mu.Unlock()
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Add persists a new mapping. Returns true if it was newly added.
func (ms *MappingStore) Add(m Mapping) (bool, error) {
	ms.mu.Lock()
	_, exists := ms.mappings[m.DSUID]
	ms.mappings[m.DSUID] = m
	path := ms.path
	list := ms.list()
	ms.mu.Unlock()
	if path != "" {
		if data, err := json.MarshalIndent(list, "", "  "); err == nil {
			_ = os.WriteFile(path, append(data, '\n'), 0o644)
		}
	}
	return !exists, nil
}

// Remove deletes the mapping for the given DSUID. Returns true if it existed.
func (ms *MappingStore) Remove(dsuid string) (bool, error) {
	ms.mu.Lock()
	_, exists := ms.mappings[dsuid]
	delete(ms.mappings, dsuid)
	path := ms.path
	list := ms.list()
	ms.mu.Unlock()
	if path != "" {
		if data, err := json.MarshalIndent(list, "", "  "); err == nil {
			_ = os.WriteFile(path, append(data, '\n'), 0o644)
		}
	}
	return exists, nil
}

// Get returns the mapping for the given DSUID.
func (ms *MappingStore) Get(dsuid string) (Mapping, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m, ok := ms.mappings[dsuid]
	return m, ok
}

// GetByRemote returns the mapping for a (pluginID, remoteEntityID) pair.
func (ms *MappingStore) GetByRemote(pluginID, remoteEntityID string) (Mapping, bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for _, m := range ms.mappings {
		if m.PluginID == pluginID && m.RemoteEntityID == remoteEntityID {
			return m, true
		}
	}
	return Mapping{}, false
}

// List returns all current mappings (copy, safe to mutate).
func (ms *MappingStore) List() []Mapping {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.list()
}

// ListForPlugin returns all mappings belonging to pluginID.
func (ms *MappingStore) ListForPlugin(pluginID string) []Mapping {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	var out []Mapping
	for _, m := range ms.mappings {
		if m.PluginID == pluginID {
			out = append(out, m)
		}
	}
	return out
}

// list returns a snapshot; caller must hold mu.
func (ms *MappingStore) list() []Mapping {
	out := make([]Mapping, 0, len(ms.mappings))
	for _, m := range ms.mappings {
		out = append(out, m)
	}
	return out
}
