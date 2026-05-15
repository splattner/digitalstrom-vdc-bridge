package vdcapi

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Namespace UUIDs matching C++ p44vdc constants (dsuid.hpp).
const (
	// dsuidP44VdcNamespaceUUID is used by the C++ ExternalVdc backend when deriving device DSUIDs
	// from a uniqueID that is not already a valid dSUID.
	dsuidP44VdcNamespaceUUID = "441A1FED-F449-4058-BEBA-13B1C4AB6A93"
	// externalVdcInstanceIDPrefix is vdcClassIdentifier+".instanceNo" from C++ ExternalVdc.
	// Combined with the vDC host DSUID it forms the vdcInstanceIdentifier used in device DSUID
	// derivation.  Go uses the single vDC DSUID as the equivalent of the C++ vdcHost DSUID.
	externalVdcInstanceIDPrefix = "External_Device_Container.1"
)

// validDSUIDRE matches a valid 34-char hex dSUID.
var validDSUIDRE = regexp.MustCompile(`^[0-9A-Fa-f]{34}$`)

// dsuidV5 returns a 34-char dSUID derived via UUIDv5 (RFC 4122), matching
// C++ DsUid::setNameInSpace(name, namespaceUUID).
// namespaceHex is a UUID string (dashes optional, must decode to exactly 16 bytes).
// The 17th byte (subdevice index) is always 0x00.
func dsuidV5(namespaceHex, name string) string {
	nsHex := strings.ReplaceAll(namespaceHex, "-", "")
	nsBytes, _ := hex.DecodeString(nsHex)
	h := sha1.New()
	h.Write(nsBytes[:16])
	h.Write([]byte(name))
	sum := h.Sum(nil)
	// RFC 4122 §4.3: set version field (bits 12-15 of byte 6) to 5
	sum[6] = (sum[6] & 0x0F) | 0x50
	// RFC 4122 §4.3: set variant field (bits 6-7 of byte 8) to 0b10
	sum[8] = (sum[8] & 0x3F) | 0x80
	// dSUID = 16 UUID bytes (upper-hex) + subdevice index byte "00"
	return strings.ToUpper(hex.EncodeToString(sum[:16])) + "00"
}

// DeviceDSUID computes the dSUID for a device given the vDC DSUID and device state.
// Exported so service-layer code can map state updates to device DSUIDs for notifications.
func DeviceDSUID(vdcDSUID string, d ExternalDeviceState, fallbackKey string) string {
	return deviceDSUID(vdcDSUID, d, fallbackKey)
}

// bridgeNamespaceUUID is the stable UUIDv5 namespace used for bridge-derived device DSUIDs.
const bridgeNamespaceUUID = "A4A7A4B5-3C8E-4B2A-9D1F-7E3A5C2B1D4E"

// BridgeDSUID derives a deterministic 34-char dSUID for a bridge-managed device.
// pluginID is the plugin instance identifier; remoteEntityID is the plugin-local entity ID.
// The same (pluginID, remoteEntityID) pair always yields the same DSUID.
func BridgeDSUID(pluginID, remoteEntityID string) string {
	return dsuidV5(bridgeNamespaceUUID, pluginID+":"+remoteEntityID)
}

func deviceDSUID(vdcDSUID string, d ExternalDeviceState, fallbackKey string) string {
	uniqueID := strings.TrimSpace(d.UniqueID)
	if uniqueID == "" {
		uniqueID = fallbackKey
	}
	// If uniqueID is already a valid 34-char dSUID, use it as-is — matches C++ customdevice.cpp
	// which calls mDSUID.setAsString(uniqueid) first and only falls back to UUIDv5 on failure.
	if validDSUIDRE.MatchString(uniqueID) {
		return strings.ToUpper(uniqueID)
	}
	// UUIDv5 with namespace=DSUID_P44VDC_NAMESPACE_UUID,
	// name=vdcInstanceIdentifier+":"+uniqueID
	// where vdcInstanceIdentifier = "External_Device_Container.1@"+vdcDSUID
	// This exactly mirrors C++ customdevice.cpp:
	//   s = mVdcP->vdcInstanceIdentifier();  s += ':';  s += uniqueid;
	//   mDSUID.setNameInSpace(s, vdcNamespace);
	name := externalVdcInstanceIDPrefix + "@" + vdcDSUID + ":" + uniqueID
	return dsuidV5(dsuidP44VdcNamespaceUUID, name)
}

// outputTypeResult holds all output-type-specific device properties populated by per-type builders.
type outputTypeResult struct {
	kind                    string
	model                   string
	outputDescription       any
	outputSettings          any
	outputState             any
	channelStates           map[string]any
	channelDescriptions     map[string]any
	sensorDescriptions      map[string]any
	sensorSettings          map[string]any
	sensorStates            map[string]any
	binaryInputDescriptions map[string]any
	binaryInputSettings     map[string]any
	binaryInputStates       map[string]any
	buttonInputDescriptions map[string]any
	buttonInputSettings     map[string]any
	buttonInputStates       map[string]any
}

func newOutputTypeResult(kind, model string) outputTypeResult {
	return outputTypeResult{
		kind:                    kind,
		model:                   model,
		channelStates:           map[string]any{},
		channelDescriptions:     map[string]any{},
		sensorDescriptions:      map[string]any{},
		sensorSettings:          map[string]any{},
		sensorStates:            map[string]any{},
		binaryInputDescriptions: map[string]any{},
		binaryInputSettings:     map[string]any{},
		binaryInputStates:       map[string]any{},
		buttonInputDescriptions: map[string]any{},
		buttonInputSettings:     map[string]any{},
		buttonInputStates:       map[string]any{},
	}
}

func buildLightTypeProps(d ExternalDeviceState) outputTypeResult {
	tp := newOutputTypeResult("dimmer", "external-light")
	tp.outputDescription = map[string]any{
		"function": 1, "outputUsage": 1, "variableRamp": true, "maxPower": 0,
	}
	tp.outputSettings = buildLightOutputSettings(1)
	tp.outputState = buildOutputState()
	tp.channelStates["0"] = map[string]any{"value": channelValue(d, 0), "age": channelAge(d, 0)}
	tp.channelDescriptions["0"] = map[string]any{
		"name": "brightness", "channelType": 1, "channelIndex": 0, "dsIndex": 0,
		"min": 0.0, "max": 100.0, "resolution": 0.4, "siunit": "%", "symbol": "%",
	}
	return tp
}

func buildColorlightTypeProps(d ExternalDeviceState) outputTypeResult {
	tp := newOutputTypeResult("colorlight", "external-colorlight")
	tp.outputDescription = map[string]any{
		"function": 4, "outputUsage": 1, "variableRamp": true, "maxPower": 0,
	}
	tp.outputSettings = buildLightOutputSettings(1)
	tp.outputState = buildOutputState()
	now := time.Now()
	for idx, chDesc := range colorlightChannelDescriptions() {
		key := intKey(idx)
		age := channelAge(d, idx)
		if age == 0 && !now.IsZero() {
			age = 0.0
		}
		tp.channelStates[key] = map[string]any{"value": channelValue(d, idx), "age": age}
		tp.channelDescriptions[key] = chDesc
	}
	return tp
}

func buildMovinglightTypeProps(d ExternalDeviceState) outputTypeResult {
	tp := newOutputTypeResult("movinglight", "external-movinglight")
	tp.outputDescription = map[string]any{
		"function": 2, "outputUsage": 1, "variableRamp": false, "maxPower": 0,
	}
	tp.outputSettings = buildPositionalOutputSettings(1)
	tp.outputState = buildOutputState()
	tp.channelStates["0"] = map[string]any{"value": channelValue(d, 0), "age": channelAge(d, 0)}
	tp.channelStates["1"] = map[string]any{"value": channelValue(d, 1), "age": channelAge(d, 1)}
	tp.channelDescriptions["0"] = map[string]any{
		"name": "position", "channelType": 7, "channelIndex": 0, "dsIndex": 0,
		"min": 0.0, "max": 100.0, "resolution": 0.4, "siunit": "%", "symbol": "%",
	}
	tp.channelDescriptions["1"] = map[string]any{
		"name": "tilt", "channelType": 9, "channelIndex": 1, "dsIndex": 1,
		"min": 0.0, "max": 100.0, "resolution": 0.4, "siunit": "°", "symbol": "°",
	}
	return tp
}

func buildSensorTypeProps(dsuid string, d ExternalDeviceState, config *ConfigStore) outputTypeResult {
	tp := newOutputTypeResult("sensor", "external-sensor")
	// Build the sensor index set from BOTH descriptor metadata and runtime values.
	// Descriptors are the authoritative count; runtime values arrive later on the
	// first retained state message. Using only d.Sensors would yield no entries
	// before any state message arrives, causing dSS to see a single placeholder
	// sensor with an unknown type.
	sensorIdxSet := make(map[int]struct{})
	for idx := range d.SensorDescriptors {
		sensorIdxSet[idx] = struct{}{}
	}
	for idx := range d.Sensors {
		sensorIdxSet[idx] = struct{}{}
	}
	for idx := range sensorIdxSet {
		key := intKey(idx)
		value := d.Sensors[idx] // zero until first state message arrives
		tp.sensorStates[key] = map[string]any{"value": value, "age": sensorAge(d, idx), "error": 0}
		tp.sensorDescriptions[key] = sensorDescriptionMap(d, idx)
		tp.sensorSettings[key] = sensorSettingsMap(dsuid, idx, config)
	}
	if len(tp.sensorDescriptions) == 0 {
		tp.sensorDescriptions["0"] = sensorDescriptionMap(d, 0)
		tp.sensorStates["0"] = map[string]any{"value": 0.0, "age": 0.0, "error": 0}
		tp.sensorSettings["0"] = sensorSettingsMap(dsuid, 0, config)
	}
	// Also populate any binary inputs on this device (e.g. occupancy alongside
	// temperature on a combined sensor device). Use the descriptor/runtime union
	// so inputs declared at activation time are visible before the first state.
	for idx := range binaryInputIdxSet(d) {
		populateBinaryInput(tp, d, dsuid, idx, config)
	}
	return tp
}

// binaryInputIdxSet returns the union of binary-input indices declared via
// descriptors and those that have already received a runtime value.
func binaryInputIdxSet(d ExternalDeviceState) map[int]struct{} {
	set := make(map[int]struct{})
	for idx := range d.BinaryInputDescriptors {
		set[idx] = struct{}{}
	}
	for idx := range d.Inputs {
		set[idx] = struct{}{}
	}
	return set
}

// populateBinaryInput fills the state/description/settings entries for one
// binary input index into the given output type result.
func populateBinaryInput(tp outputTypeResult, d ExternalDeviceState, dsuid string, idx int, config *ConfigStore) {
	key := intKey(idx)
	value := d.Inputs[idx] // zero (false) until first state message arrives
	tp.binaryInputStates[key] = map[string]any{"value": value > 0, "age": inputAge(d, idx), "error": 0}
	tp.binaryInputDescriptions[key] = binaryInputDescriptionMap(d, dsuid, idx, config)
	var sf int
	if desc, ok := d.BinaryInputDescriptors[idx]; ok {
		sf = desc.Function
	}
	if config != nil {
		if stored := config.GetBinaryInputSettings(dsuid, idx).SensorFunction; stored != 0 {
			sf = stored
		}
	}
	tp.binaryInputSettings[key] = binaryInputSettingsMap(dsuid, idx, sf, config)
}

// sensorDescriptionMap returns the sensorDescription entry for one sensor input,
// preferring the descriptor pushed by the connector (if any) over the defaults.
func sensorDescriptionMap(d ExternalDeviceState, idx int) map[string]any {
	desc, hasDesc := d.SensorDescriptors[idx]
	name := desc.Name
	if name == "" {
		name = "sensor"
	}
	min, max, res := desc.Min, desc.Max, desc.Resolution
	if !hasDesc {
		min, max, res = 0.0, 100.0, 0.1
	}
	out := map[string]any{
		"name":                name,
		"sensorType":          desc.Type,
		"sensorUsage":         desc.Usage,
		"min":                 min,
		"max":                 max,
		"resolution":          res,
		"updateInterval":      0.0,
		"aliveSignInterval":   0.0,
		"minPushInterval":     0.0,
		"changesOnlyInterval": 0.0,
	}
	if desc.SIUnit != "" {
		out["siunit"] = desc.SIUnit
	}
	if desc.Symbol != "" {
		out["symbol"] = desc.Symbol
	}
	return out
}

// sensorSettingsMap builds the sensorSettings entry for the given device+sensor
// index, applying any overrides from the config store on top of the defaults.
func sensorSettingsMap(dsuid string, idx int, config *ConfigStore) map[string]any {
	m := map[string]any{
		"group":               0,
		"function":            0,
		"channel":             0,
		"minPushInterval":     0.0,
		"changesOnlyInterval": 0.0,
	}
	if config == nil {
		return m
	}
	s := config.GetSensorSettings(dsuid, idx)
	if s.Group != nil {
		m["group"] = *s.Group
	}
	if s.Function != nil {
		m["function"] = *s.Function
	}
	if s.Channel != nil {
		m["channel"] = *s.Channel
	}
	if s.MinPushInterval != nil {
		m["minPushInterval"] = *s.MinPushInterval
	}
	if s.ChangesOnlyInterval != nil {
		m["changesOnlyInterval"] = *s.ChangesOnlyInterval
	}
	return m
}

func buildButtonTypeProps(dsuid string, d ExternalDeviceState, config *ConfigStore) outputTypeResult {
	tp := newOutputTypeResult("button", "external-button")
	for idx, value := range d.Buttons {
		key := intKey(idx)
		state := map[string]any{"value": value, "age": buttonAge(d, idx)}
		if action := strings.TrimSpace(d.ButtonActions[idx]); action != "" {
			state["action"] = action
		}
		tp.buttonInputStates[key] = state
		tp.buttonInputDescriptions[key] = map[string]any{
			"name": "button", "buttonID": idx, "supportsLocalKeyMode": false, "supportsActions": true,
		}
		var slp, cp bool
		if config != nil {
			s := config.GetButtonInputSettings(dsuid, idx)
			slp = s.SetsLocalPriority
			cp = s.CallsPresent
		}
		tp.buttonInputSettings[key] = buttonSettingsMap(dsuid, idx, slp, cp, config)
	}
	if len(tp.buttonInputDescriptions) == 0 {
		tp.buttonInputDescriptions["0"] = map[string]any{
			"name": "button", "buttonID": 0, "supportsLocalKeyMode": false, "supportsActions": true,
		}
		var slp, cp bool
		if config != nil {
			s := config.GetButtonInputSettings(dsuid, 0)
			slp = s.SetsLocalPriority
			cp = s.CallsPresent
		}
		tp.buttonInputSettings["0"] = buttonSettingsMap(dsuid, 0, slp, cp, config)
		tp.buttonInputStates["0"] = map[string]any{"value": 0.0, "age": 0.0}
	}
	return tp
}

// buttonSettingsMap renders a buttonInputSettings entry, applying overrides
// from the config store on top of the dS defaults (group=1, mode=0, function=0).
func buttonSettingsMap(dsuid string, idx int, slp, cp bool, config *ConfigStore) map[string]any {
	group, mode, function := 1, 0, 0
	if config != nil {
		s := config.GetButtonInputSettings(dsuid, idx)
		if s.Group != nil {
			group = *s.Group
		}
		if s.Mode != nil {
			mode = *s.Mode
		}
		if s.Function != nil {
			function = *s.Function
		}
	}
	return map[string]any{
		"group": group, "mode": mode, "function": function,
		"setsLocalPriority": slp, "callsPresent": cp,
	}
}

func buildBinaryInputTypeProps(dsuid string, d ExternalDeviceState, config *ConfigStore) outputTypeResult {
	tp := newOutputTypeResult("binaryinput", "external-binaryinput")
	// Iterate the union of descriptor and runtime indices so inputs declared at
	// activation time are visible before the first state message arrives.
	for idx := range binaryInputIdxSet(d) {
		populateBinaryInput(tp, d, dsuid, idx, config)
	}
	if len(tp.binaryInputDescriptions) == 0 {
		tp.binaryInputDescriptions["0"] = binaryInputDescriptionMap(d, dsuid, 0, config)
		var sf int
		if config != nil {
			sf = config.GetBinaryInputSettings(dsuid, 0).SensorFunction
		}
		tp.binaryInputSettings["0"] = binaryInputSettingsMap(dsuid, 0, sf, config)
		tp.binaryInputStates["0"] = map[string]any{"value": false, "age": 0.0, "error": 0}
	}
	return tp
}

// binaryInputDescriptionMap returns the binaryInputDescription entry for one input,
// preferring the descriptor pushed by the connector (if any) over defaults.
func binaryInputDescriptionMap(d ExternalDeviceState, dsuid string, idx int, config *ConfigStore) map[string]any {
	name := "input"
	sf := 0
	if desc, ok := d.BinaryInputDescriptors[idx]; ok {
		if desc.Name != "" {
			name = desc.Name
		}
		sf = desc.Function
	}
	// ConfigStore explicit override takes precedence.
	if config != nil {
		if stored := config.GetBinaryInputSettings(dsuid, idx).SensorFunction; stored != 0 {
			sf = stored
		}
	}
	return map[string]any{
		"name": name, "inputType": 1, "inputUsage": 0,
		"sensorFunction": sf, "updateInterval": 0.0,
	}
}

// binaryInputSettingsMap renders a binaryInputSettings entry, applying the
// stored group override on top of the dS default (group=0).
func binaryInputSettingsMap(dsuid string, idx int, sensorFunction int, config *ConfigStore) map[string]any {
	group := 0
	if config != nil {
		if g := config.GetBinaryInputSettings(dsuid, idx).Group; g != nil {
			group = *g
		}
	}
	return map[string]any{"group": group, "sensorFunction": sensorFunction}
}

func buildGenericTypeProps(output, dsuid string, d ExternalDeviceState, config *ConfigStore) outputTypeResult {
	tp := newOutputTypeResult("generic", "external-device")
	tp.outputDescription = map[string]any{
		"function": 0, "outputUsage": 0, "variableRamp": false, "maxPower": 0,
	}
	tp.outputSettings = map[string]any{"mode": 1, "pushChanges": false}
	tp.outputState = buildOutputState()
	for idx, value := range d.Sensors {
		key := intKey(idx)
		tp.sensorStates[key] = map[string]any{"value": value, "age": sensorAge(d, idx), "error": 0}
		tp.sensorDescriptions[key] = map[string]any{
			"name": "sensor", "sensorType": 0, "sensorUsage": 0,
			"min": 0.0, "max": 100.0, "resolution": 0.1,
			"updateInterval": 0.0, "aliveSignInterval": 0.0,
			"minPushInterval": 0.0, "changesOnlyInterval": 0.0,
		}
		tp.sensorSettings[key] = sensorSettingsMap(dsuid, idx, config)
	}
	for idx := range binaryInputIdxSet(d) {
		populateBinaryInput(tp, d, dsuid, idx, config)
	}
	for idx, value := range d.Buttons {
		key := intKey(idx)
		state := map[string]any{"value": value, "age": buttonAge(d, idx)}
		if action := strings.TrimSpace(d.ButtonActions[idx]); action != "" {
			state["action"] = action
		}
		tp.buttonInputStates[key] = state
		tp.buttonInputDescriptions[key] = map[string]any{
			"name": "button", "buttonID": idx, "supportsLocalKeyMode": false, "supportsActions": true,
		}
		var slp, cp bool
		if config != nil {
			s := config.GetButtonInputSettings(dsuid, idx)
			slp = s.SetsLocalPriority
			cp = s.CallsPresent
		}
		tp.buttonInputSettings[key] = buttonSettingsMap(dsuid, idx, slp, cp, config)
	}
	return tp
}

func buildDeviceProperties(dsuid string, d ExternalDeviceState, scenes *SceneStore, config *ConfigStore) map[string]any {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		name = d.UniqueID
	}
	if config != nil {
		if override, ok := config.GetDeviceName(dsuid); ok && override != "" {
			name = override
		}
	}
	output := strings.TrimSpace(d.Output)
	if output == "" {
		output = "light"
	}

	var tp outputTypeResult
	switch {
	case strings.EqualFold(output, "light"):
		tp = buildLightTypeProps(d)
	case strings.EqualFold(output, "colorlight"):
		tp = buildColorlightTypeProps(d)
	case strings.EqualFold(output, "movinglight"):
		tp = buildMovinglightTypeProps(d)
	case strings.EqualFold(output, "sensor"):
		tp = buildSensorTypeProps(dsuid, d, config)
	case strings.EqualFold(output, "button"):
		tp = buildButtonTypeProps(dsuid, d, config)
	case strings.EqualFold(output, "binaryinput"), strings.EqualFold(output, "input"):
		tp = buildBinaryInputTypeProps(dsuid, d, config)
	default:
		tp = buildGenericTypeProps(output, dsuid, d, config)
	}

	modelFeatures := buildModelFeatures(tp.kind, len(tp.channelStates) > 0,
		len(tp.buttonInputDescriptions) > 0, len(tp.binaryInputDescriptions) > 0, len(tp.sensorDescriptions) > 0)
	deviceEvents := buildDeviceEvents(d)
	scenesMap := buildScenesProperty(dsuid, tp.channelDescriptions, scenes)

	// Overlay persisted outputSettings and runtime outputState from config.
	if config != nil {
		if os, ok := tp.outputSettings.(map[string]any); ok {
			applyOutputSettingsOverlay(os, config.GetDeviceOutputSettings(dsuid))
		}
		if ost, ok := tp.outputState.(map[string]any); ok {
			state := config.GetDeviceOutputState(dsuid)
			ost["localPriority"] = state.LocalPriority
		}
	}

	return map[string]any{
		"dSUID":              dsuid,
		"type":               "vdSD",
		"name":               name,
		"model":              tp.model,
		"modelUID":           "vdcgo-" + tp.kind,
		"modelVersion":       "1.0",
		"hardwareVersion":    "",
		"hardwareGuid":       "",
		"hardwareModelGuid":  "",
		"oemGuid":            "",
		"oemModelGuid":       "",
		"vendorId":           "",
		"subdevIdx":          "",
		"vendorName":         "vdcgo",
		"configURL":          "",
		"primaryGroup":       primaryGroupForKind(tp.kind),
		"deviceClass":        "vdcgo-" + tp.kind,
		"deviceClassVersion": 1,
		"displayId":          shortDisplayID(dsuid),
		"zoneID": func() int {
			if config != nil {
				return config.GetDeviceZoneID(dsuid)
			}
			return 0
		}(),
		"modelFeatures":           modelFeatures,
		"deviceevents":            deviceEvents,
		"active":                  d.Active,
		"outputType":              tp.kind,
		"sensorDescriptions":      tp.sensorDescriptions,
		"sensorSettings":          tp.sensorSettings,
		"sensorStates":            tp.sensorStates,
		"binaryInputDescriptions": tp.binaryInputDescriptions,
		"binaryInputSettings":     tp.binaryInputSettings,
		"binaryInputStates":       tp.binaryInputStates,
		"buttonInputDescriptions": tp.buttonInputDescriptions,
		"buttonInputSettings":     tp.buttonInputSettings,
		"buttonInputStates":       tp.buttonInputStates,
		"channelDescriptions":     tp.channelDescriptions,
		"channelStates":           tp.channelStates,
		"outputDescription":       tp.outputDescription,
		"outputSettings":          tp.outputSettings,
		"outputState":             tp.outputState,
		"scenes":                  scenesMap,
	}
}

// buildModelFeatures produces the modelFeatures map for a given output kind,
// using the standard dS modelFeature keys recognized by DSS (see device.cpp /
// outputbehaviour.cpp / lightbehaviour.cpp / colorlightbehaviour.cpp).
// DSS ignores any keys it does not know, so non-standard keys would result in
// `modelFeatures: {}` being shown in the DSS REST API.
func buildModelFeatures(kind string, hasChannels, hasButtons, hasBinaryInputs, hasSensors bool) map[string]any {
	_ = hasChannels
	_ = hasBinaryInputs
	_ = hasSensors
	dimmable := kind == "dimmer" || kind == "colorlight" || kind == "movinglight"
	isShadow := kind == "shadow" || kind == "movinglight"
	hasOutput := kind != "sensor" && kind != "binaryinput" && kind != "button"
	hasScenes := hasOutput

	mf := map[string]any{
		// device-level (from device.cpp)
		"identification": true,
		"dontcare":       hasScenes,
		"ledauto":        false,
		"leddark":        false,
		"pushbutton":     hasButtons,
		"pushbarea":      hasButtons,
		"pushbadvanced":  hasButtons,
		"pushbsensor":    false,
		"pushbdevice":    false,
		"pushbcombined":  false,
		"twowayconfig":   false,
		"highlevel":      false,
		"jokerconfig":    false,
		"akmsensor":      false,
		"akminput":       false,
		"akmdelay":       false,
	}
	if hasOutput {
		// from outputbehaviour.cpp
		mf["outmodegeneric"] = false // suppressed when we have a more specific outmode
		mf["outvalue8"] = !isShadow
		mf["blink"] = true
	}
	if dimmable {
		// from lightbehaviour.cpp
		mf["outmode"] = true
		mf["outmodeswitch"] = false
		mf["outmodegeneric"] = false
		mf["transt"] = true
	}
	if kind == "colorlight" {
		// from colorlightbehaviour.cpp — required for the multi-channel color UI
		mf["outputchannels"] = true
	}
	if isShadow {
		mf["shadeprops"] = true
		mf["shadeposition"] = true
		mf["motiontimefins"] = kind == "movinglight"
		mf["optypeconfig"] = true
		mf["shadebladeang"] = kind == "movinglight"
	}
	return mf
}

// buildOutputState returns the default outputState property.
func buildOutputState() map[string]any {
	return map[string]any{
		"localPriority": false,
		"error":         0,
	}
}

// buildLightOutputSettings returns outputSettings for a light/dimmer device.
func buildLightOutputSettings(group int) map[string]any {
	return map[string]any{
		"groups":          map[string]any{strconv.Itoa(group): true},
		"mode":            2, // gradual
		"pushChanges":     false,
		"onThreshold":     50.0,
		"minBrightness":   0.0,
		"dimTimeUp":       300,
		"dimTimeDown":     300,
		"dimTimeUpAlt1":   4000,
		"dimTimeDownAlt1": 4000,
		"dimTimeUpAlt2":   8000,
		"dimTimeDownAlt2": 8000,
	}
}

// buildPositionalOutputSettings returns outputSettings for a positional (blind) device.
func buildPositionalOutputSettings(group int) map[string]any {
	return map[string]any{
		"groups":      map[string]any{strconv.Itoa(group): true},
		"mode":        2, // gradual
		"pushChanges": false,
	}
}

// colorlightChannelDescriptions returns spec-correct channel descriptions for a full-color dimmer.
// channelType values: brightness=1, hue=2, saturation=3, colortemp=4, cie_x=5, cie_y=6.
func colorlightChannelDescriptions() map[int]map[string]any {
	return map[int]map[string]any{
		0: {"name": "brightness", "channelType": 1, "channelIndex": 0, "dsIndex": 0, "min": 0.0, "max": 100.0, "resolution": 0.4, "siunit": "%", "symbol": "%"},
		1: {"name": "hue", "channelType": 2, "channelIndex": 1, "dsIndex": 1, "min": 0.0, "max": 360.0, "resolution": 0.1, "siunit": "°", "symbol": "°"},
		2: {"name": "saturation", "channelType": 3, "channelIndex": 2, "dsIndex": 2, "min": 0.0, "max": 100.0, "resolution": 0.4, "siunit": "%", "symbol": "%"},
		3: {"name": "colortemperature", "channelType": 4, "channelIndex": 3, "dsIndex": 3, "min": 153.0, "max": 370.0, "resolution": 1.0, "siunit": "mired", "symbol": "mired"},
		4: {"name": "cieX", "channelType": 5, "channelIndex": 4, "dsIndex": 4, "min": 0.0, "max": 1.0, "resolution": 0.001, "siunit": "", "symbol": ""},
		5: {"name": "cieY", "channelType": 6, "channelIndex": 5, "dsIndex": 5, "min": 0.0, "max": 1.0, "resolution": 0.001, "siunit": "", "symbol": ""},
	}
}

// sensorAge returns the age of a sensor value in seconds, or 0.0 if not yet updated.
func sensorAge(d ExternalDeviceState, index int) float64 {
	if d.SensorUpdatedAt != nil {
		if t, ok := d.SensorUpdatedAt[index]; ok && !t.IsZero() {
			return time.Since(t).Seconds()
		}
	}
	return 0.0
}

// inputAge returns the age of a binary input value in seconds, or 0.0 if not yet updated.
func inputAge(d ExternalDeviceState, index int) float64 {
	if d.InputUpdatedAt != nil {
		if t, ok := d.InputUpdatedAt[index]; ok && !t.IsZero() {
			return time.Since(t).Seconds()
		}
	}
	return 0.0
}

// buttonAge returns the age of a button state in seconds, or 0.0 if not yet updated.
func buttonAge(d ExternalDeviceState, index int) float64 {
	if d.ButtonUpdatedAt != nil {
		if t, ok := d.ButtonUpdatedAt[index]; ok && !t.IsZero() {
			return time.Since(t).Seconds()
		}
	}
	return 0.0
}

// channelAge returns the age of a channel value in seconds, or 0.0 if unknown.
func channelAge(d ExternalDeviceState, index int) float64 {
	if d.ChannelUpdatedAt != nil {
		if t, ok := d.ChannelUpdatedAt[index]; ok && !t.IsZero() {
			return time.Since(t).Seconds()
		}
	}
	return 0.0
}

// buildScenesProperty generates the scenes map for getProperty, merging stored data with defaults.
// Channels without stored entries default to value=0 / dontCare=true.
func buildScenesProperty(dsuid string, channelDescriptions map[string]any, ss *SceneStore) map[string]any {
	result := map[string]any{}

	// Channel index list (derived from channelDescriptions keys)
	chKeys := make([]int, 0, len(channelDescriptions))
	for k := range channelDescriptions {
		if idx, err := strconv.Atoi(k); err == nil {
			chKeys = append(chKeys, idx)
		}
	}
	sort.Ints(chKeys)

	// Default scene values: scene 0=off (brightness=0), scene 5=max (brightness=100)
	defaultSceneValues := map[int]float64{
		0: 0.0,   // off
		1: 75.0,  // area 1 off
		2: 75.0,  // area 2 off
		3: 75.0,  // area 3 off
		4: 75.0,  // area 4 off
		5: 100.0, // max
		6: 33.0,  // area 1 on
		7: 33.0,  // area 2 on
		8: 33.0,  // area 3 on
		9: 33.0,  // area 4 on
	}

	buildSceneEntry := func(sceneNum int, stored *SceneEntry) map[string]any {
		channels := map[string]any{}
		for _, idx := range chKeys {
			chKey := intKey(idx)
			val := 0.0
			dontCare := true
			if idx == 0 {
				// primary channel: use default if no stored value
				if v, ok := defaultSceneValues[sceneNum]; ok {
					val = v
					dontCare = false
				}
			}
			if stored != nil {
				if sv, ok := stored.Channels[idx]; ok {
					val = sv.Value
					dontCare = sv.DontCare
				}
			}
			channels[chKey] = map[string]any{"value": val, "dontCare": dontCare}
		}
		effect := 1 // smooth normal transition (default)
		globalDontCare := false
		ignoreLocalPriority := false
		if stored != nil {
			effect = stored.Effect
			globalDontCare = stored.DontCare
			ignoreLocalPriority = stored.IgnoreLocalPriority
		}
		return map[string]any{
			"channels":            channels,
			"effect":              effect,
			"dontCare":            globalDontCare,
			"ignoreLocalPriority": ignoreLocalPriority,
		}
	}

	if len(chKeys) == 0 {
		// No channels → minimal scene stubs
		return map[string]any{"0": map[string]any{}, "5": map[string]any{}}
	}

	// Emit stored scenes + default scenes 0-9
	storedScenes := map[int]SceneEntry{}
	if ss != nil {
		for num, entry := range ss.GetDeviceScenes(dsuid) {
			storedScenes[num] = entry
		}
	}
	// Always include scenes 0 and 5 as the most important defaults
	for _, num := range []int{0, 5} {
		if _, ok := storedScenes[num]; !ok {
			storedScenes[num] = SceneEntry{} // sentinel so we emit it
		}
	}
	for num := range storedScenes {
		var stored *SceneEntry
		if ss != nil {
			if e, ok := ss.GetScene(dsuid, num); ok {
				stored = &e
			}
		}
		result[intKey(num)] = buildSceneEntry(num, stored)
	}
	return result
}

func buildDeviceEvents(d ExternalDeviceState) []any {
	if len(d.ButtonActions) == 0 {
		return []any{}
	}
	keys := make([]int, 0, len(d.ButtonActions))
	for idx := range d.ButtonActions {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	events := make([]any, 0, len(keys))
	for _, idx := range keys {
		action := strings.TrimSpace(d.ButtonActions[idx])
		if action == "" {
			continue
		}
		events = append(events, map[string]any{
			"index":  idx,
			"type":   "buttonAction",
			"action": action,
		})
	}
	return events
}

func intKey(v int) string {
	return strconv.Itoa(v)
}

func channelValue(d ExternalDeviceState, index int) float64 {
	if d.Channels != nil {
		if v, ok := d.Channels[index]; ok {
			return v
		}
	}
	return 0
}

// BuildPropertyTree is the exported entry-point for building the full vDC
// property tree. It is used by the HTTP API and tests.
func BuildPropertyTree(vdcDSUID, description string, snapshot ExternalSnapshot, scenes *SceneStore, config *ConfigStore) (root map[string]any, vdc map[string]any, devices map[string]map[string]any) {
	return buildPropertyTree(vdcDSUID, description, snapshot, scenes, config)
}

func buildPropertyTree(vdcDSUID, description string, snapshot ExternalSnapshot, scenes *SceneStore, config *ConfigStore) (root map[string]any, vdc map[string]any, devices map[string]map[string]any) {
	devices = make(map[string]map[string]any)
	for k, d := range snapshot.Devices {
		dsuid := deviceDSUID(vdcDSUID, d, k)
		devices[dsuid] = buildDeviceProperties(dsuid, d, scenes, config)
	}
	deviceObjects := make(map[string]any, len(devices))
	for dsuid, d := range devices {
		deviceObjects[dsuid] = d
	}
	vdc = map[string]any{
		"dSUID":             vdcDSUID,
		"name":              description,
		"modelName":         "vdcgo external",
		"modelVersion":      "1.0",
		"hardwareVersion":   "",
		"hardwareGuid":      "",
		"hardwareModelGuid": "",
		"oemGuid":           "",
		"oemModelGuid":      "",
		"vendorId":          "",
		"subdevIdx":         "",
		"displayId":         description,
		"configURL":         "",
		"devices":           deviceObjects,
	}
	root = map[string]any{
		"dSUID":             vdcDSUID,
		"name":              description,
		"modelName":         "vdcgo external",
		"modelVersion":      "1.0",
		"hardwareVersion":   "",
		"hardwareGuid":      "",
		"hardwareModelGuid": "",
		"oemGuid":           "",
		"oemModelGuid":      "",
		"vendorId":          "",
		"subdevIdx":         "",
		"displayId":         description,
		"configURL":         "",
		"vdcs":              map[string]any{vdcDSUID: vdc},
		"devices":           deviceObjects,
	}
	return root, vdc, devices
}

func applyObjectQuery(full map[string]any, query map[string]any) map[string]any {
	if len(query) == 0 {
		return full
	}
	result := make(map[string]any)
	for key, qv := range query {
		if key == "*" {
			qmap, _ := qv.(map[string]any)
			for fk, fv := range full {
				result[fk] = applyQueryValue(fv, qmap)
			}
			continue
		}
		fv, ok := full[key]
		if !ok {
			continue
		}
		qmap, _ := qv.(map[string]any)
		result[key] = applyQueryValue(fv, qmap)
	}
	return result
}

func applyQueryValue(value any, query map[string]any) any {
	if len(query) == 0 {
		return value
	}
	vmap, ok := value.(map[string]any)
	if !ok {
		return value
	}
	return applyObjectQuery(vmap, query)
}

// applyOutputSettingsOverlay merges non-nil fields from an OutputSettingsEntry into an outputSettings map.
func applyOutputSettingsOverlay(m map[string]any, ov OutputSettingsEntry) {
	if ov.Mode != nil {
		m["mode"] = *ov.Mode
	}
	if ov.PushChanges != nil {
		m["pushChanges"] = *ov.PushChanges
	}
	if ov.Groups != nil {
		groups, ok := m["groups"].(map[string]any)
		if !ok {
			groups = make(map[string]any)
			m["groups"] = groups
		}
		for k, v := range ov.Groups {
			groups[k] = v
		}
	}
	if ov.OnThreshold != nil {
		m["onThreshold"] = *ov.OnThreshold
	}
	if ov.MinBrightness != nil {
		m["minBrightness"] = *ov.MinBrightness
	}
	if ov.DimTimeUp != nil {
		m["dimTimeUp"] = *ov.DimTimeUp
	}
	if ov.DimTimeDown != nil {
		m["dimTimeDown"] = *ov.DimTimeDown
	}
	if ov.DimTimeUpAlt1 != nil {
		m["dimTimeUpAlt1"] = *ov.DimTimeUpAlt1
	}
	if ov.DimTimeDownAlt1 != nil {
		m["dimTimeDownAlt1"] = *ov.DimTimeDownAlt1
	}
	if ov.DimTimeUpAlt2 != nil {
		m["dimTimeUpAlt2"] = *ov.DimTimeUpAlt2
	}
	if ov.DimTimeDownAlt2 != nil {
		m["dimTimeDownAlt2"] = *ov.DimTimeDownAlt2
	}
}

// primaryGroupForKind returns the dS color class / primary group for a device kind.
// 1=yellow/light, 2=grey/shadow, 3=blue/climate, 8=black/joker.
func primaryGroupForKind(kind string) int {
	switch kind {
	case "dimmer", "colorlight", "movinglight":
		return 1 // class_yellow_light
	case "button":
		// Buttons default to the yellow/light group so they trigger room light
		// scenes out of the box (matches a stock SW-TKM200 base).
		//
		// NOTE: dSS only exposes a colour-group picker for joker-class (8)
		// devices. Switching to joker, however, currently breaks the proxy
		// rendering of the device (empty buttonInputs / groups), so we keep
		// the device on yellow class and let users retarget the group via the
		// bridge's own UI / config (buttonInputSettings.group override in
		// ConfigStore).
		return 1 // class_yellow_light
	case "shadow":
		return 2 // class_grey_shadow
	case "climate":
		return 3 // class_blue_climate
	default:
		return 8 // class_black_joker (generic)
	}
}

// shortDisplayID returns a short user-facing ID derived from the dSUID.
// DSS uses this as the truncated DisplayID shown in the UI.
func shortDisplayID(dsuid string) string {
	if len(dsuid) >= 8 {
		return dsuid[:8]
	}
	return dsuid
}
