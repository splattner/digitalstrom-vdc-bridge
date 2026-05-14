package vdcapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// SceneChannelValue holds a stored scene value and its dontCare flag for one channel.
type SceneChannelValue struct {
	Value    float64 `json:"value"`
	DontCare bool    `json:"dontCare"`
}

// SceneEntry represents one stored scene for a device.
type SceneEntry struct {
	Channels            map[int]SceneChannelValue `json:"channels"`
	Effect              int                       `json:"effect"`
	DontCare            bool                      `json:"dontCare"`
	IgnoreLocalPriority bool                      `json:"ignoreLocalPriority"`
}

// SceneStore is a concurrency-safe in-memory store for device scene values.
// Keyed by device dSUID and scene number.
type SceneStore struct {
	mu           sync.Mutex
	scenes       map[string]map[int]SceneEntry // deviceDSUID -> sceneNum -> entry
	autoSavePath string
}

// NewSceneStore creates an empty SceneStore.
func NewSceneStore() *SceneStore {
	return &SceneStore{
		scenes: make(map[string]map[int]SceneEntry),
	}
}

// SetAutoSave configures a file path for write-through persistence.
// Every mutation will atomically write the store to that path.
func (ss *SceneStore) SetAutoSave(path string) {
	ss.mu.Lock()
	ss.autoSavePath = path
	ss.mu.Unlock()
}

// sceneStoreJSON is the on-disk format.
type sceneStoreJSON struct {
	Scenes map[string]map[string]SceneEntry `json:"scenes"`
}

// SaveToFile writes the store atomically to path.
func (ss *SceneStore) SaveToFile(path string) error {
	ss.mu.Lock()
	data := ss.toJSON()
	ss.mu.Unlock()
	return atomicWriteJSON(path, data)
}

// LoadFromFile replaces the store contents from path.
// Returns nil if the file does not exist.
func (ss *SceneStore) LoadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var data sceneStoreJSON
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.scenes = make(map[string]map[int]SceneEntry)
	for dsuid, scenesMap := range data.Scenes {
		ss.scenes[dsuid] = make(map[int]SceneEntry)
		for sceneKey, entry := range scenesMap {
			num, err := strconv.Atoi(sceneKey)
			if err != nil {
				continue
			}
			if entry.Channels == nil {
				entry.Channels = make(map[int]SceneChannelValue)
			}
			ss.scenes[dsuid][num] = entry
		}
	}
	return nil
}

func (ss *SceneStore) toJSON() sceneStoreJSON {
	data := sceneStoreJSON{Scenes: make(map[string]map[string]SceneEntry)}
	for dsuid, m := range ss.scenes {
		data.Scenes[dsuid] = make(map[string]SceneEntry)
		for num, entry := range m {
			data.Scenes[dsuid][intKey(num)] = entry
		}
	}
	return data
}

func (ss *SceneStore) saveIfConfigured() {
	ss.mu.Lock()
	path := ss.autoSavePath
	if path == "" {
		ss.mu.Unlock()
		return
	}
	data := ss.toJSON()
	ss.mu.Unlock()
	_ = atomicWriteJSON(path, data)
}

// atomicWriteJSON marshals v to JSON and writes it to path atomically.
func atomicWriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// SaveScene stores the current channel values for a device+scene.
// Only updates channels present in the provided map; existing channel entries are preserved.
func (ss *SceneStore) SaveScene(deviceDSUID string, sceneNum int, channels map[int]float64) {
	ss.mu.Lock()
	if ss.scenes[deviceDSUID] == nil {
		ss.scenes[deviceDSUID] = make(map[int]SceneEntry)
	}
	entry := ss.scenes[deviceDSUID][sceneNum]
	if entry.Channels == nil {
		entry.Channels = make(map[int]SceneChannelValue)
	}
	for idx, val := range channels {
		cv := entry.Channels[idx]
		cv.Value = val
		entry.Channels[idx] = cv
	}
	ss.scenes[deviceDSUID][sceneNum] = entry
	ss.mu.Unlock()
	ss.saveIfConfigured()
}

// GetScene returns the stored scene entry for a device+scene, if it exists.
func (ss *SceneStore) GetScene(deviceDSUID string, sceneNum int) (SceneEntry, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if m, ok := ss.scenes[deviceDSUID]; ok {
		if e, ok := m[sceneNum]; ok {
			return e, true
		}
	}
	return SceneEntry{}, false
}

// GetDeviceScenes returns a copy of all stored scenes for a device.
func (ss *SceneStore) GetDeviceScenes(deviceDSUID string) map[int]SceneEntry {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	m, ok := ss.scenes[deviceDSUID]
	if !ok {
		return nil
	}
	result := make(map[int]SceneEntry, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// SetSceneChannelValue updates the stored value for one channel in a scene.
func (ss *SceneStore) SetSceneChannelValue(deviceDSUID string, sceneNum, channelIdx int, value float64) {
	ss.mu.Lock()
	ss.ensureEntry(deviceDSUID, sceneNum)
	entry := ss.scenes[deviceDSUID][sceneNum]
	cv := entry.Channels[channelIdx]
	cv.Value = value
	entry.Channels[channelIdx] = cv
	ss.scenes[deviceDSUID][sceneNum] = entry
	ss.mu.Unlock()
	ss.saveIfConfigured()
}

// SetSceneChannelDontCare updates the dontCare flag for one channel in a scene.
func (ss *SceneStore) SetSceneChannelDontCare(deviceDSUID string, sceneNum, channelIdx int, dontCare bool) {
	ss.mu.Lock()
	ss.ensureEntry(deviceDSUID, sceneNum)
	entry := ss.scenes[deviceDSUID][sceneNum]
	cv := entry.Channels[channelIdx]
	cv.DontCare = dontCare
	entry.Channels[channelIdx] = cv
	ss.scenes[deviceDSUID][sceneNum] = entry
	ss.mu.Unlock()
	ss.saveIfConfigured()
}

// SetSceneEffect updates the effect for a scene.
func (ss *SceneStore) SetSceneEffect(deviceDSUID string, sceneNum, effect int) {
	ss.mu.Lock()
	ss.ensureEntry(deviceDSUID, sceneNum)
	entry := ss.scenes[deviceDSUID][sceneNum]
	entry.Effect = effect
	ss.scenes[deviceDSUID][sceneNum] = entry
	ss.mu.Unlock()
	ss.saveIfConfigured()
}

// SetSceneDontCare updates the global dontCare flag for a scene.
func (ss *SceneStore) SetSceneDontCare(deviceDSUID string, sceneNum int, dontCare bool) {
	ss.mu.Lock()
	ss.ensureEntry(deviceDSUID, sceneNum)
	entry := ss.scenes[deviceDSUID][sceneNum]
	entry.DontCare = dontCare
	ss.scenes[deviceDSUID][sceneNum] = entry
	ss.mu.Unlock()
	ss.saveIfConfigured()
}

// SetSceneIgnoreLocalPriority updates the ignoreLocalPriority flag for a scene.
func (ss *SceneStore) SetSceneIgnoreLocalPriority(deviceDSUID string, sceneNum int, ilp bool) {
	ss.mu.Lock()
	ss.ensureEntry(deviceDSUID, sceneNum)
	entry := ss.scenes[deviceDSUID][sceneNum]
	entry.IgnoreLocalPriority = ilp
	ss.scenes[deviceDSUID][sceneNum] = entry
	ss.mu.Unlock()
	ss.saveIfConfigured()
}

func (ss *SceneStore) ensureEntry(deviceDSUID string, sceneNum int) {
	if ss.scenes[deviceDSUID] == nil {
		ss.scenes[deviceDSUID] = make(map[int]SceneEntry)
	}
	entry := ss.scenes[deviceDSUID][sceneNum]
	if entry.Channels == nil {
		entry.Channels = make(map[int]SceneChannelValue)
		ss.scenes[deviceDSUID][sceneNum] = entry
	}
}
