package vdcapi

import (
	"errors"
	"strconv"
	"strings"

	"github.com/splattner/vdcgo/pkg/logging"
	"github.com/splattner/vdcgo/pkg/runtime"
)

var (
	errControlPathUnavailable     = errors.New("control path unavailable")
	errAddressableNotFound        = errors.New("addressable not found")
	errSetPropertyLightOnly       = errors.New("setProperty is only supported for light outputs")
	errSetPropertyRequiresChannel = errors.New("setProperty requires channelStates.0.value")
	errPairingAborted             = errors.New("pairing/unpairing aborted")
)

type methodService struct {
	dsuid       string
	description string
	state       *StateStore
	commander   Commander
	scenes      *SceneStore
	config      *ConfigStore
}

func newMethodService(dsuid, description string, state *StateStore, commander Commander, scenes *SceneStore, config *ConfigStore) methodService {
	return methodService{dsuid: dsuid, description: description, state: state, commander: commander, scenes: scenes, config: config}
}

func (m methodService) resolveGetPropertyTarget(target string) (map[string]any, error) {
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	root, vdc, devices := buildPropertyTree(m.dsuid, m.description, snapshot, m.scenes, m.config)
	full := root
	if target != "" && !strings.EqualFold(target, "root") {
		switch {
		case strings.EqualFold(target, m.dsuid):
			full = root
		default:
			// Case-insensitive device DSUID lookup (DSS may normalize case).
			upperTarget := strings.ToUpper(target)
			if d, ok := devices[upperTarget]; ok {
				full = d
				break
			}
			if strings.EqualFold(target, stringFromAny(vdc["dSUID"])) {
				full = vdc
			} else {
				return nil, errAddressableNotFound
			}
		}
	}
	return full, nil
}

func (m methodService) setPropertyFromJSON(params map[string]any) error {
	// Handle scene property writes first
	if sw, ok := parseScenePathJSON(params); ok {
		target := stringFromAny(params["dSUID"])
		return m.applySceneWrite(target, sw)
	}
	// Handle input setting writes (buttonInputSettings, binaryInputSettings)
	if isw, ok := parseInputSettingPathJSON(params); ok {
		target := stringFromAny(params["dSUID"])
		return m.applyInputSettingWrite(target, isw)
	}
	// Handle device name write
	if name, ok := params["value"].(string); ok {
		if propName := strings.TrimSpace(stringFromAny(params["name"])); strings.EqualFold(propName, "name") {
			target := stringFromAny(params["dSUID"])
			return m.applyNameWrite(target, name)
		}
	}
	// Handle generic per-channel writes: channelStates/N/value path or nested form.
	if cw, ok := parseChannelStatesPathJSON(params); ok {
		target := stringFromAny(params["dSUID"])
		return m.setChannelValue(target, cw.ChannelIdx, cw.Value)
	}
	value, ok := extractBrightnessValueFromJSON(params)
	if !ok {
		return errSetPropertyRequiresChannel
	}
	target := stringFromAny(params["dSUID"])
	return m.setLightLevel(target, value)
}

func (m methodService) setPropertyFromPbuf(target string, props []pbufPropertyElement) error {
	// Handle scene property writes first (these are container writes with specific structure).
	if sw, ok := extractSceneWriteFromPbuf(props); ok {
		return m.applySceneWrite(target, sw)
	}
	// Handle input setting writes.
	if isw, ok := extractInputSettingFromPbuf(props); ok {
		return m.applyInputSettingWrite(target, isw)
	}

	// Iterate top-level properties and dispatch by name.
	for _, p := range props {
		switch strings.ToLower(strings.TrimSpace(p.Name)) {
		case "name":
			if s, ok := p.Value.(string); ok {
				return m.applyNameWrite(target, s)
			}
		case "zoneid":
			return m.applyZoneIDWrite(target, p.Value)
		case "outputsettings":
			return m.applyOutputSettingsWrite(target, p.Elements)
		case "outputstate":
			return m.applyOutputStateWrite(target, p.Elements)
		case "channelstates":
			// Generic per-channel value write: iterate channelStates.{idx}.value.
			return m.applyChannelStatesWrite(target, p.Elements)
		}
	}

	// Fallback: try the legacy top-level channel write format.
	if value, ok := extractBrightnessFromPbufProperties(props); ok {
		return m.setLightLevel(target, value)
	}

	// Unknown properties — silently accept. DSS writes many config properties.
	return nil
}

func (m methodService) setLightLevel(target string, value float64) error {
	if m.commander == nil {
		return errControlPathUnavailable
	}
	if err := validateLightControlTarget(target, m.dsuid); err != nil {
		return err
	}
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	dev, ok := resolveDeviceByDSUID(m.dsuid, target, snapshot)
	if !ok {
		return errAddressableNotFound
	}
	if !isLightOutput(dev.Output) {
		return errSetPropertyLightOnly
	}
	if err := m.commander.SetLightLevel(dev.UniqueID, value); err != nil {
		return err
	}
	return nil
}

// setChannelValue dispatches a generic per-channel value write to the commander.
// Channel 0 is the primary (brightness/position), 1+ are output-kind specific.
func (m methodService) setChannelValue(target string, channelIndex int, value float64) error {
	if m.commander == nil {
		return errControlPathUnavailable
	}
	if err := validateLightControlTarget(target, m.dsuid); err != nil {
		return err
	}
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	dev, ok := resolveDeviceByDSUID(m.dsuid, target, snapshot)
	if !ok {
		return errAddressableNotFound
	}
	if !isLightOutput(dev.Output) {
		return errSetPropertyLightOnly
	}
	return m.commander.SetChannelValue(dev.UniqueID, channelIndex, value)
}

// applyChannelStatesWrite handles setProperty writes against the channelStates
// container. Iterates each child {channelIndex}.value and dispatches a per-
// channel commander call. Unknown sub-fields (e.g. age) are silently ignored.
func (m methodService) applyChannelStatesWrite(target string, elements []pbufPropertyElement) error {
	for _, child := range elements {
		idx, err := strconv.Atoi(strings.TrimSpace(child.Name))
		if err != nil {
			continue
		}
		for _, grand := range child.Elements {
			if !strings.EqualFold(strings.TrimSpace(grand.Name), "value") {
				continue
			}
			if v, ok := asFloat(grand.Value); ok {
				if err := m.setChannelValue(target, idx, v); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// applyInputSettingWrite stores a button or binary input setting in the config store.
func (m methodService) applyInputSettingWrite(target string, isw inputSettingWrite) error {
	if m.config == nil {
		return nil // silently accept
	}
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, m.dsuid) {
		return nil
	}
	if isw.Field == "" {
		return nil // unrecognised sub-field — accept silently
	}
	switch isw.Field {
	case "setsLocalPriority":
		m.config.SetButtonSetsLocalPriority(target, isw.Index, isw.BoolVal)
	case "callsPresent":
		m.config.SetButtonCallsPresent(target, isw.Index, isw.BoolVal)
	case "buttonGroup":
		m.config.SetButtonGroup(target, isw.Index, isw.IntVal)
	case "buttonMode":
		m.config.SetButtonMode(target, isw.Index, isw.IntVal)
	case "buttonFunction":
		m.config.SetButtonFunction(target, isw.Index, isw.IntVal)
	case "sensorFunction":
		m.config.SetBinaryInputSensorFunction(target, isw.Index, isw.IntVal)
	case "binaryInputGroup":
		m.config.SetBinaryInputGroup(target, isw.Index, isw.IntVal)
	case "group":
		v := isw.IntVal
		m.config.MergeSensorSettings(target, isw.Index, SensorSettingsEntry{Group: &v})
	case "function":
		v := isw.IntVal
		m.config.MergeSensorSettings(target, isw.Index, SensorSettingsEntry{Function: &v})
	case "channel":
		v := isw.IntVal
		m.config.MergeSensorSettings(target, isw.Index, SensorSettingsEntry{Channel: &v})
	case "minPushInterval":
		v := isw.FloatVal
		m.config.MergeSensorSettings(target, isw.Index, SensorSettingsEntry{MinPushInterval: &v})
	case "changesOnlyInterval":
		v := isw.FloatVal
		m.config.MergeSensorSettings(target, isw.Index, SensorSettingsEntry{ChangesOnlyInterval: &v})
	}
	return nil
}

// applyNameWrite stores a user-set device name in the config store.
func (m methodService) applyNameWrite(target, name string) error {
	if m.config == nil {
		return nil // silently accept
	}
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, m.dsuid) {
		return nil // vDC-level name change ignored for now
	}
	m.config.SetDeviceName(target, strings.TrimSpace(name))
	return nil
}

// applyZoneIDWrite stores a zone ID write from DSS in the config store.
func (m methodService) applyZoneIDWrite(target string, value any) error {
	if m.config == nil {
		return nil // silently accept
	}
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, m.dsuid) {
		return nil
	}
	var zoneID int
	switch v := value.(type) {
	case int64:
		zoneID = int(v)
	case uint64:
		zoneID = int(v)
	case int:
		zoneID = v
	case float64:
		zoneID = int(v)
	default:
		return nil // unrecognised type — accept silently
	}
	m.config.SetDeviceZoneID(target, zoneID)
	return nil
}

// applyOutputSettingsWrite persists outputSettings sub-property writes.
// DSS sends: outputSettings → [mode, pushChanges, groups, onThreshold, minBrightness, dimTime*]
func (m methodService) applyOutputSettingsWrite(target string, elems []pbufPropertyElement) error {
	if m.config == nil {
		return nil
	}
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, m.dsuid) {
		return nil
	}
	var overlay OutputSettingsEntry
	for _, e := range elems {
		switch strings.ToLower(strings.TrimSpace(e.Name)) {
		case "mode":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.Mode = &v
			}
		case "pushchanges":
			b := asBool(e.Value)
			overlay.PushChanges = &b
		case "groups":
			// groups is a container: elements are {"0": true, "1": false, ...}
			overlay.Groups = make(map[string]bool)
			for _, g := range e.Elements {
				overlay.Groups[strings.TrimSpace(g.Name)] = asBool(g.Value)
			}
		case "onthreshold":
			if f, ok := asFloat(e.Value); ok {
				overlay.OnThreshold = &f
			}
		case "minbrightness":
			if f, ok := asFloat(e.Value); ok {
				overlay.MinBrightness = &f
			}
		case "dimtimeup":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.DimTimeUp = &v
			}
		case "dimtimedown":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.DimTimeDown = &v
			}
		case "dimtimeupalt1":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.DimTimeUpAlt1 = &v
			}
		case "dimtimedownalt1":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.DimTimeDownAlt1 = &v
			}
		case "dimtimeupalt2":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.DimTimeUpAlt2 = &v
			}
		case "dimtimedownalt2":
			if f, ok := asFloat(e.Value); ok {
				v := int(f)
				overlay.DimTimeDownAlt2 = &v
			}
		}
	}
	m.config.MergeDeviceOutputSettings(target, overlay)
	return nil
}

// applyOutputStateWrite stores runtime output state writes (localPriority, transitionTime).
func (m methodService) applyOutputStateWrite(target string, elems []pbufPropertyElement) error {
	if m.config == nil {
		return nil
	}
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, m.dsuid) {
		return nil
	}
	current := m.config.GetDeviceOutputState(target)
	for _, e := range elems {
		switch strings.ToLower(strings.TrimSpace(e.Name)) {
		case "localpriority":
			current.LocalPriority = asBool(e.Value)
		case "transitiontime":
			if f, ok := asFloat(e.Value); ok {
				current.TransitionTime = f
			}
		}
	}
	m.config.SetDeviceOutputState(target, current)
	return nil
}

// applySceneWrite applies a decoded scene property write to the scene store.
// target may be a device dSUID or root/vDC dSUID; we resolve to the device DSUID.
func (m methodService) applySceneWrite(target string, sw sceneWrite) error {
	if m.scenes == nil {
		return nil // silently accept if no scene store
	}
	// Resolve target to a concrete device DSUID
	deviceDSUID := target
	if target == "" || strings.EqualFold(target, "root") || strings.EqualFold(target, m.dsuid) {
		// scene write on vDC/root: not meaningful per spec, ignore
		return nil
	}
	switch sw.Field {
	case "channelValue":
		m.scenes.SetSceneChannelValue(deviceDSUID, sw.SceneNum, sw.ChannelIdx, sw.FloatVal)
	case "channelDontCare":
		m.scenes.SetSceneChannelDontCare(deviceDSUID, sw.SceneNum, sw.ChannelIdx, sw.BoolVal)
	case "effect":
		m.scenes.SetSceneEffect(deviceDSUID, sw.SceneNum, sw.IntVal)
	case "dontCare":
		m.scenes.SetSceneDontCare(deviceDSUID, sw.SceneNum, sw.BoolVal)
	case "ignoreLocalPriority":
		m.scenes.SetSceneIgnoreLocalPriority(deviceDSUID, sw.SceneNum, sw.BoolVal)
	}
	return nil
}

func (m methodService) ensurePairTimeout(timeout int) error {
	if timeout <= 0 {
		return errPairingAborted
	}
	return nil
}

func (m methodService) resolvePongTarget(target string) (string, bool) {
	pongDSUID := strings.TrimSpace(target)
	if pongDSUID == "" || strings.EqualFold(pongDSUID, "root") || strings.EqualFold(pongDSUID, m.dsuid) {
		return m.dsuid, true
	}
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	if _, ok := resolveDeviceByDSUID(m.dsuid, pongDSUID, snapshot); ok {
		return pongDSUID, true
	}
	return "", false
}

func (m methodService) ensureAddressableTarget(target string) error {
	t := strings.TrimSpace(target)
	if t == "" || strings.EqualFold(t, "root") || strings.EqualFold(t, m.dsuid) {
		return nil
	}
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	if _, ok := resolveDeviceByDSUID(m.dsuid, t, snapshot); ok {
		return nil
	}
	return errAddressableNotFound
}

func (m methodService) ensureRemovableTarget(target string) error {
	t := strings.TrimSpace(target)
	if t == "" || strings.EqualFold(t, "root") || strings.EqualFold(t, m.dsuid) {
		return errAddressableNotFound
	}
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	if _, ok := resolveDeviceByDSUID(m.dsuid, t, snapshot); ok {
		return nil
	}
	return errAddressableNotFound
}

// removeDeviceByDSUID validates the target dSUID refers to a known device, then fires
// EventRemove on the state store so the push loops emit a vanish notification and the
// device is removed from the registry.
func (m methodService) removeDeviceByDSUID(target string) error {
	t := strings.TrimSpace(target)
	if t == "" || strings.EqualFold(t, "root") || strings.EqualFold(t, m.dsuid) {
		return errAddressableNotFound
	}
	if m.state == nil {
		return errAddressableNotFound
	}
	snapshot := m.state.Snapshot()
	dev, ok := resolveDeviceByDSUID(m.dsuid, t, snapshot)
	if !ok {
		return errAddressableNotFound
	}
	m.state.HandleEvent(runtime.Event{Type: runtime.EventRemove, UniqueID: dev.UniqueID})
	return nil
}

// --- Notification dispatchers (shared by JSON and protobuf servers) ---

// setPropertyJSONStatusCode maps a setProperty error to a JSON API error code.
func setPropertyJSONStatusCode(err error) int {
	switch {
	case errors.Is(err, errControlPathUnavailable):
		return 503
	case errors.Is(err, errAddressableNotFound):
		return 404
	case errors.Is(err, errSetPropertyLightOnly):
		return 405
	case errors.Is(err, errSetPropertyRequiresChannel), errors.Is(err, errSetPropertyTargetRequired):
		return 400
	default:
		return 503
	}
}

// setPropertyPbufStatusCode maps a setProperty error to a protobuf result code.
func setPropertyPbufStatusCode(err error) uint64 {
	switch {
	case errors.Is(err, errControlPathUnavailable):
		return pbufResultServiceUnavailable
	case errors.Is(err, errAddressableNotFound):
		return pbufResultNotFound
	case errors.Is(err, errSetPropertyLightOnly):
		return pbufResultForbidden
	case errors.Is(err, errSetPropertyRequiresChannel), errors.Is(err, errSetPropertyTargetRequired):
		return pbufResultMissingData
	default:
		return pbufResultServiceUnavailable
	}
}

func clampLevel(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func copyChannels(channels map[int]float64) map[int]float64 {
	out := make(map[int]float64, len(channels))
	for k, v := range channels {
		out[k] = v
	}
	return out
}

func (m methodService) resolveNotificationTargets(targets []string, all bool) []ExternalDeviceState {
	snapshot := ExternalSnapshot{}
	if m.state != nil {
		snapshot = m.state.Snapshot()
	}
	if len(snapshot.Devices) == 0 {
		return nil
	}

	if !all {
		for _, t := range targets {
			if strings.TrimSpace(t) == "" || strings.EqualFold(t, "root") || strings.EqualFold(t, m.dsuid) {
				all = true
				break
			}
		}
	}

	if all {
		result := make([]ExternalDeviceState, 0, len(snapshot.Devices))
		for _, d := range snapshot.Devices {
			if d.Output != "" && !isLightOutput(d.Output) {
				continue
			}
			result = append(result, d)
		}
		return result
	}

	targetSet := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t != "" {
			targetSet[strings.ToUpper(t)] = struct{}{}
		}
	}

	seen := map[string]struct{}{}
	result := make([]ExternalDeviceState, 0, len(targetSet))
	for key, d := range snapshot.Devices {
		if d.Output != "" && !isLightOutput(d.Output) {
			continue
		}
		dsuid := strings.ToUpper(deviceDSUID(m.dsuid, d, key))
		if _, ok := targetSet[dsuid]; !ok {
			continue
		}
		if _, ok := seen[d.UniqueID]; ok {
			continue
		}
		seen[d.UniqueID] = struct{}{}
		result = append(result, d)
	}
	return result
}

func (m methodService) applyLevelToNotificationTargets(targets []string, all bool, value float64) {
	if m.commander == nil {
		return
	}
	value = clampLevel(value)
	for _, d := range m.resolveNotificationTargets(targets, all) {
		if err := m.commander.SetLightLevel(d.UniqueID, value); err != nil {
			logging.Warn("notification_control_error", logging.Fields{"unique_id": d.UniqueID, "error": err})
		}
	}
}

func (m methodService) applyColorChannelToNotificationTargets(targets []string, all bool, channelIndex int, value float64) {
	if m.commander == nil {
		return
	}
	for _, d := range m.resolveNotificationTargets(targets, all) {
		if err := applyColorChannelValue(m.commander, d.UniqueID, channelIndex, value); err != nil {
			logging.Warn("notification_color_channel_error", logging.Fields{"unique_id": d.UniqueID, "channel": channelIndex, "error": err})
		}
	}
}

func (m methodService) handleCallSceneNotification(targets []string, all bool, scene int) {
	if m.scenes != nil && !all && len(targets) > 0 {
		for _, devDSUID := range targets {
			if entry, ok := m.scenes.GetScene(devDSUID, scene); ok && !entry.DontCare {
				applied := false
				for chIdx, ch := range entry.Channels {
					if ch.DontCare {
						continue
					}
					if chIdx == 0 {
						m.applyLevelToNotificationTargets([]string{devDSUID}, false, ch.Value)
					} else if m.commander != nil {
						snapshot := ExternalSnapshot{}
						if m.state != nil {
							snapshot = m.state.Snapshot()
						}
						if dev, ok2 := resolveDeviceByDSUID(m.dsuid, devDSUID, snapshot); ok2 {
							if err := applyColorChannelValue(m.commander, dev.UniqueID, chIdx, ch.Value); err != nil {
								logging.Warn("notification_call_scene_color_error", logging.Fields{"unique_id": dev.UniqueID, "channel": chIdx, "error": err})
							}
						}
					}
					applied = true
				}
				if applied {
					continue
				}
			}
			m.applyStaticSceneAction([]string{devDSUID}, false, scene)
		}
		return
	}
	m.applyStaticSceneAction(targets, all, scene)
}

func (m methodService) applyStaticSceneAction(targets []string, all bool, scene int) {
	action := mapSceneAction(scene)
	switch action.kind {
	case sceneActionSetLevel:
		m.applyLevelToNotificationTargets(targets, all, action.level)
	case sceneActionDimUp:
		m.applyLevelToNotificationTargets(targets, all, 100)
	case sceneActionDimDown:
		m.applyLevelToNotificationTargets(targets, all, 0)
	case sceneActionStop:
		return
	default:
		logging.Debug("notification_call_scene_ignored", logging.Fields{"scene": scene})
	}
}

func (m methodService) handleSaveSceneNotification(targets []string, all bool, sceneNum int) {
	if m.scenes == nil || m.state == nil {
		return
	}
	snapshot := m.state.Snapshot()
	if all {
		for k, d := range snapshot.Devices {
			devDSUID := deviceDSUID(m.dsuid, d, k)
			m.scenes.SaveScene(devDSUID, sceneNum, copyChannels(d.Channels))
		}
		return
	}
	for _, targetDSUID := range targets {
		for k, d := range snapshot.Devices {
			if strings.EqualFold(deviceDSUID(m.dsuid, d, k), targetDSUID) {
				m.scenes.SaveScene(targetDSUID, sceneNum, copyChannels(d.Channels))
				break
			}
		}
	}
}

func (m methodService) handleSetControlValueNotification(targets []string, all bool, name string, value float64, hasValue bool) {
	if !hasValue {
		return
	}
	if name != "" && !strings.EqualFold(name, "brightness") && !strings.EqualFold(name, "output") && !strings.EqualFold(name, "value") {
		logging.Debug("notification_set_control_value_ignored", logging.Fields{"name": name})
		return
	}
	m.applyLevelToNotificationTargets(targets, all, value)
}

func (m methodService) handleDimChannelNotification(targets []string, all bool, mode int) {
	switch {
	case mode == 0:
		return
	case mode > 0:
		m.applyLevelToNotificationTargets(targets, all, 100)
	case mode < 0:
		m.applyLevelToNotificationTargets(targets, all, 0)
	}
}

func (m methodService) handleSetOutputChannelValueNotification(targets []string, all bool, channelIndex int, value float64, applyNow bool, hasValue bool) {
	if !applyNow || !hasValue {
		return
	}
	if channelIndex == 0 {
		m.applyLevelToNotificationTargets(targets, all, value)
		return
	}
	m.applyColorChannelToNotificationTargets(targets, all, channelIndex, value)
}
