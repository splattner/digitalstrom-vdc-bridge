package vdcapi

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"
)

// OutputSettingsEntry holds persisted outputSettings overrides for a device.
// Pointer fields: nil means "use default". Groups maps channel-group-index string to membership bool.
type OutputSettingsEntry struct {
	Mode            *int            `json:"mode,omitempty"`
	PushChanges     *bool           `json:"pushChanges,omitempty"`
	Groups          map[string]bool `json:"groups,omitempty"`
	OnThreshold     *float64        `json:"onThreshold,omitempty"`
	MinBrightness   *float64        `json:"minBrightness,omitempty"`
	DimTimeUp       *int            `json:"dimTimeUp,omitempty"`
	DimTimeDown     *int            `json:"dimTimeDown,omitempty"`
	DimTimeUpAlt1   *int            `json:"dimTimeUpAlt1,omitempty"`
	DimTimeDownAlt1 *int            `json:"dimTimeDownAlt1,omitempty"`
	DimTimeUpAlt2   *int            `json:"dimTimeUpAlt2,omitempty"`
	DimTimeDownAlt2 *int            `json:"dimTimeDownAlt2,omitempty"`
}

// OutputStateEntry holds runtime-only (non-persisted) output state overrides for a device.
type OutputStateEntry struct {
	LocalPriority  bool
	TransitionTime float64 // seconds
}

// ButtonInputSettingsEntry holds writable button input settings per button index.
// Group/Mode/Function are pointers: nil means "use default".
type ButtonInputSettingsEntry struct {
	SetsLocalPriority bool `json:"setsLocalPriority"`
	CallsPresent      bool `json:"callsPresent"`
	Group             *int `json:"group,omitempty"`
	Mode              *int `json:"mode,omitempty"`
	Function          *int `json:"function,omitempty"`
}

// BinaryInputSettingsEntry holds writable binary input settings per input index.
// Group is a pointer: nil means "use default".
type BinaryInputSettingsEntry struct {
	SensorFunction int  `json:"sensorFunction"`
	Group          *int `json:"group,omitempty"`
}

// SensorSettingsEntry holds writable sensor settings per sensor index.
// Pointer fields: nil means "use default / not set".
type SensorSettingsEntry struct {
	Group               *int     `json:"group,omitempty"`
	Function            *int     `json:"function,omitempty"`
	Channel             *int     `json:"channel,omitempty"`
	MinPushInterval     *float64 `json:"minPushInterval,omitempty"`
	ChangesOnlyInterval *float64 `json:"changesOnlyInterval,omitempty"`
}

// configStoreJSON is the on-disk format.
type configStoreJSON struct {
	DeviceNames          map[string]string                   `json:"deviceNames"`
	DeviceZoneIDs        map[string]int                      `json:"deviceZoneIDs,omitempty"`
	DeviceOutputSettings map[string]OutputSettingsEntry      `json:"deviceOutputSettings,omitempty"`
	ButtonInputSettings  map[string]ButtonInputSettingsEntry `json:"buttonInputSettings,omitempty"`
	BinaryInputSettings  map[string]BinaryInputSettingsEntry `json:"binaryInputSettings,omitempty"`
	SensorSettings       map[string]SensorSettingsEntry      `json:"sensorSettings,omitempty"`
	AnnouncedDSUIDs      []string                            `json:"announcedDSUIDs,omitempty"`
}

// ConfigStore persists writable device configuration.
// It is concurrency-safe and supports write-through persistence via SetAutoSave.
type ConfigStore struct {
	mu                   sync.Mutex
	deviceNames          map[string]string                   // deviceDSUID -> user-set name
	deviceZoneIDs        map[string]int                      // deviceDSUID -> zone ID
	deviceOutputSettings map[string]OutputSettingsEntry      // deviceDSUID -> persisted output settings overlay
	deviceOutputStates   map[string]OutputStateEntry         // deviceDSUID -> runtime output state (not persisted)
	buttonInputSettings  map[string]ButtonInputSettingsEntry // "deviceDSUID:idx" -> settings
	binaryInputSettings  map[string]BinaryInputSettingsEntry // "deviceDSUID:idx" -> settings
	sensorSettings       map[string]SensorSettingsEntry      // "deviceDSUID:idx" -> settings
	announcedDSUIDs      map[string]bool                     // DSUIDs currently known to DSS
	autoSavePath         string
}

// inputSettingKey returns the map key for per-device per-index input settings.
func inputSettingKey(deviceDSUID string, idx int) string {
	return deviceDSUID + ":" + strconv.Itoa(idx)
}

// NewConfigStore returns an empty ConfigStore.
func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		deviceNames:          make(map[string]string),
		deviceZoneIDs:        make(map[string]int),
		deviceOutputSettings: make(map[string]OutputSettingsEntry),
		deviceOutputStates:   make(map[string]OutputStateEntry),
		buttonInputSettings:  make(map[string]ButtonInputSettingsEntry),
		binaryInputSettings:  make(map[string]BinaryInputSettingsEntry),
		sensorSettings:       make(map[string]SensorSettingsEntry),
		announcedDSUIDs:      make(map[string]bool),
	}
}

// StaleAnnouncedDSUIDs returns DSUIDs that were previously announced to DSS but
// are not in currentDSUIDs. It then replaces the stored set with currentDSUIDs
// and persists. Call this on every successful hello to reconcile with DSS.
func (cs *ConfigStore) StaleAnnouncedDSUIDs(currentDSUIDs []string) []string {
	currentSet := make(map[string]bool, len(currentDSUIDs))
	for _, d := range currentDSUIDs {
		currentSet[d] = true
	}
	cs.mu.Lock()
	var stale []string
	for d := range cs.announcedDSUIDs {
		if !currentSet[d] {
			stale = append(stale, d)
		}
	}
	cs.announcedDSUIDs = currentSet
	cs.mu.Unlock()
	cs.saveIfConfigured()
	return stale
}

// MarkDSUIDAdded adds a DSUID to the announced set and persists.
func (cs *ConfigStore) MarkDSUIDAdded(dsuid string) {
	cs.mu.Lock()
	cs.announcedDSUIDs[dsuid] = true
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// MarkDSUIDRemoved removes a DSUID from the announced set and persists.
func (cs *ConfigStore) MarkDSUIDRemoved(dsuid string) {
	cs.mu.Lock()
	delete(cs.announcedDSUIDs, dsuid)
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// ClearAnnouncedDSUIDs forgets all DSUIDs previously announced to a vDSM.
// The next hello/announce cycle will re-announce all currently known devices.
// Returns the number of entries cleared.
func (cs *ConfigStore) ClearAnnouncedDSUIDs() int {
	cs.mu.Lock()
	n := len(cs.announcedDSUIDs)
	cs.announcedDSUIDs = make(map[string]bool)
	cs.mu.Unlock()
	cs.saveIfConfigured()
	return n
}

// SetAutoSave configures a file path for write-through persistence.
func (cs *ConfigStore) SetAutoSave(path string) {
	cs.mu.Lock()
	cs.autoSavePath = path
	cs.mu.Unlock()
}

// SetDeviceName stores a user-set name override for a device.
func (cs *ConfigStore) SetDeviceName(deviceDSUID, name string) {
	cs.mu.Lock()
	cs.deviceNames[deviceDSUID] = name
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// GetDeviceName returns the stored name override for a device, if any.
func (cs *ConfigStore) GetDeviceName(deviceDSUID string) (string, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	n, ok := cs.deviceNames[deviceDSUID]
	return n, ok
}

// SetDeviceZoneID stores the zone ID for a device.
func (cs *ConfigStore) SetDeviceZoneID(deviceDSUID string, zoneID int) {
	cs.mu.Lock()
	cs.deviceZoneIDs[deviceDSUID] = zoneID
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// GetDeviceZoneID returns the stored zone ID for a device (0 = not set).
func (cs *ConfigStore) GetDeviceZoneID(deviceDSUID string) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.deviceZoneIDs[deviceDSUID]
}

// MergeDeviceOutputSettings merges non-nil fields from overlay into the stored entry for deviceDSUID.
func (cs *ConfigStore) MergeDeviceOutputSettings(deviceDSUID string, overlay OutputSettingsEntry) {
	cs.mu.Lock()
	entry := cs.deviceOutputSettings[deviceDSUID]
	if overlay.Mode != nil {
		entry.Mode = overlay.Mode
	}
	if overlay.PushChanges != nil {
		entry.PushChanges = overlay.PushChanges
	}
	if overlay.Groups != nil {
		if entry.Groups == nil {
			entry.Groups = make(map[string]bool)
		}
		for k, v := range overlay.Groups {
			entry.Groups[k] = v
		}
	}
	if overlay.OnThreshold != nil {
		entry.OnThreshold = overlay.OnThreshold
	}
	if overlay.MinBrightness != nil {
		entry.MinBrightness = overlay.MinBrightness
	}
	if overlay.DimTimeUp != nil {
		entry.DimTimeUp = overlay.DimTimeUp
	}
	if overlay.DimTimeDown != nil {
		entry.DimTimeDown = overlay.DimTimeDown
	}
	if overlay.DimTimeUpAlt1 != nil {
		entry.DimTimeUpAlt1 = overlay.DimTimeUpAlt1
	}
	if overlay.DimTimeDownAlt1 != nil {
		entry.DimTimeDownAlt1 = overlay.DimTimeDownAlt1
	}
	if overlay.DimTimeUpAlt2 != nil {
		entry.DimTimeUpAlt2 = overlay.DimTimeUpAlt2
	}
	if overlay.DimTimeDownAlt2 != nil {
		entry.DimTimeDownAlt2 = overlay.DimTimeDownAlt2
	}
	cs.deviceOutputSettings[deviceDSUID] = entry
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// GetDeviceOutputSettings returns the stored outputSettings overlay for a device.
func (cs *ConfigStore) GetDeviceOutputSettings(deviceDSUID string) OutputSettingsEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.deviceOutputSettings[deviceDSUID]
}

// SetDeviceOutputState stores runtime output state for a device (not persisted).
func (cs *ConfigStore) SetDeviceOutputState(deviceDSUID string, overlay OutputStateEntry) {
	cs.mu.Lock()
	entry := cs.deviceOutputStates[deviceDSUID]
	entry.LocalPriority = overlay.LocalPriority
	if overlay.TransitionTime > 0 {
		entry.TransitionTime = overlay.TransitionTime
	}
	cs.deviceOutputStates[deviceDSUID] = entry
	cs.mu.Unlock()
}

// GetDeviceOutputState returns the runtime output state for a device.
func (cs *ConfigStore) GetDeviceOutputState(deviceDSUID string) OutputStateEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.deviceOutputStates[deviceDSUID]
}

// AllDeviceNames returns a copy of all stored name overrides.
func (cs *ConfigStore) AllDeviceNames() map[string]string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make(map[string]string, len(cs.deviceNames))
	for k, v := range cs.deviceNames {
		out[k] = v
	}
	return out
}

// SaveToFile writes the store atomically to path.
func (cs *ConfigStore) SaveToFile(path string) error {
	cs.mu.Lock()
	data := cs.toJSON()
	cs.mu.Unlock()
	return atomicWriteJSON(path, data)
}

// LoadFromFile replaces the store contents from path.
// Returns nil if the file does not exist.
func (cs *ConfigStore) LoadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var data configStoreJSON
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.deviceNames = make(map[string]string)
	for k, v := range data.DeviceNames {
		cs.deviceNames[k] = v
	}
	cs.buttonInputSettings = make(map[string]ButtonInputSettingsEntry)
	for k, v := range data.ButtonInputSettings {
		cs.buttonInputSettings[k] = v
	}
	cs.binaryInputSettings = make(map[string]BinaryInputSettingsEntry)
	for k, v := range data.BinaryInputSettings {
		cs.binaryInputSettings[k] = v
	}
	cs.sensorSettings = make(map[string]SensorSettingsEntry)
	for k, v := range data.SensorSettings {
		cs.sensorSettings[k] = v
	}
	cs.deviceZoneIDs = make(map[string]int)
	for k, v := range data.DeviceZoneIDs {
		cs.deviceZoneIDs[k] = v
	}
	cs.deviceOutputSettings = make(map[string]OutputSettingsEntry)
	for k, v := range data.DeviceOutputSettings {
		cs.deviceOutputSettings[k] = v
	}
	cs.deviceOutputStates = make(map[string]OutputStateEntry)
	cs.announcedDSUIDs = make(map[string]bool)
	for _, d := range data.AnnouncedDSUIDs {
		cs.announcedDSUIDs[d] = true
	}
	return nil
}

// GetButtonInputSettings returns the stored button input settings for a device+index.
func (cs *ConfigStore) GetButtonInputSettings(deviceDSUID string, idx int) ButtonInputSettingsEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.buttonInputSettings[inputSettingKey(deviceDSUID, idx)]
}

// SetButtonSetsLocalPriority persists the setsLocalPriority flag for a device+button index.
func (cs *ConfigStore) SetButtonSetsLocalPriority(deviceDSUID string, idx int, val bool) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.buttonInputSettings[key]
	e.SetsLocalPriority = val
	cs.buttonInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// SetButtonCallsPresent persists the callsPresent flag for a device+button index.
func (cs *ConfigStore) SetButtonCallsPresent(deviceDSUID string, idx int, val bool) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.buttonInputSettings[key]
	e.CallsPresent = val
	cs.buttonInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// SetButtonGroup persists the group for a device+button index.
func (cs *ConfigStore) SetButtonGroup(deviceDSUID string, idx int, val int) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.buttonInputSettings[key]
	v := val
	e.Group = &v
	cs.buttonInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// SetButtonMode persists the mode for a device+button index.
func (cs *ConfigStore) SetButtonMode(deviceDSUID string, idx int, val int) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.buttonInputSettings[key]
	v := val
	e.Mode = &v
	cs.buttonInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// SetButtonFunction persists the function for a device+button index.
func (cs *ConfigStore) SetButtonFunction(deviceDSUID string, idx int, val int) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.buttonInputSettings[key]
	v := val
	e.Function = &v
	cs.buttonInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// GetBinaryInputSettings returns the stored binary input settings for a device+index.
func (cs *ConfigStore) GetBinaryInputSettings(deviceDSUID string, idx int) BinaryInputSettingsEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.binaryInputSettings[inputSettingKey(deviceDSUID, idx)]
}

// SetBinaryInputSensorFunction persists the sensorFunction for a device+input index.
func (cs *ConfigStore) SetBinaryInputSensorFunction(deviceDSUID string, idx int, fn int) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.binaryInputSettings[key]
	e.SensorFunction = fn
	cs.binaryInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// SetBinaryInputGroup persists the group for a device+input index.
func (cs *ConfigStore) SetBinaryInputGroup(deviceDSUID string, idx int, val int) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	e := cs.binaryInputSettings[key]
	v := val
	e.Group = &v
	cs.binaryInputSettings[key] = e
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

// GetSensorSettings returns the stored sensor settings for a device+sensor index.
func (cs *ConfigStore) GetSensorSettings(deviceDSUID string, idx int) SensorSettingsEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.sensorSettings[inputSettingKey(deviceDSUID, idx)]
}

// MergeSensorSettings merges non-nil fields from overlay into the stored entry.
func (cs *ConfigStore) MergeSensorSettings(deviceDSUID string, idx int, overlay SensorSettingsEntry) {
	cs.mu.Lock()
	key := inputSettingKey(deviceDSUID, idx)
	entry := cs.sensorSettings[key]
	if overlay.Group != nil {
		entry.Group = overlay.Group
	}
	if overlay.Function != nil {
		entry.Function = overlay.Function
	}
	if overlay.Channel != nil {
		entry.Channel = overlay.Channel
	}
	if overlay.MinPushInterval != nil {
		entry.MinPushInterval = overlay.MinPushInterval
	}
	if overlay.ChangesOnlyInterval != nil {
		entry.ChangesOnlyInterval = overlay.ChangesOnlyInterval
	}
	cs.sensorSettings[key] = entry
	cs.mu.Unlock()
	cs.saveIfConfigured()
}

func (cs *ConfigStore) toJSON() configStoreJSON {
	data := configStoreJSON{
		DeviceNames:          make(map[string]string),
		DeviceZoneIDs:        make(map[string]int),
		DeviceOutputSettings: make(map[string]OutputSettingsEntry),
		ButtonInputSettings:  make(map[string]ButtonInputSettingsEntry),
		BinaryInputSettings:  make(map[string]BinaryInputSettingsEntry),
		SensorSettings:       make(map[string]SensorSettingsEntry),
	}
	for k, v := range cs.deviceNames {
		data.DeviceNames[k] = v
	}
	for k, v := range cs.deviceZoneIDs {
		data.DeviceZoneIDs[k] = v
	}
	for k, v := range cs.deviceOutputSettings {
		data.DeviceOutputSettings[k] = v
	}
	for k, v := range cs.buttonInputSettings {
		data.ButtonInputSettings[k] = v
	}
	for k, v := range cs.binaryInputSettings {
		data.BinaryInputSettings[k] = v
	}
	for k, v := range cs.sensorSettings {
		data.SensorSettings[k] = v
	}
	for d := range cs.announcedDSUIDs {
		data.AnnouncedDSUIDs = append(data.AnnouncedDSUIDs, d)
	}
	return data
}

func (cs *ConfigStore) saveIfConfigured() {
	cs.mu.Lock()
	path := cs.autoSavePath
	if path == "" {
		cs.mu.Unlock()
		return
	}
	data := cs.toJSON()
	cs.mu.Unlock()
	_ = atomicWriteJSON(path, data)
}
