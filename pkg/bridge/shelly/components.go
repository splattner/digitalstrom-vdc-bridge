package shelly

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// component identifies one Shelly RPC component instance, e.g. "switch:0".
type component struct {
	Kind  string
	Index int
}

func (c component) key() string {
	return fmt.Sprintf("%s:%d", c.Kind, c.Index)
}

// entityID builds the bridge.RemoteEntity.ID for one component of a device.
func entityID(devID string, c component) string {
	return devID + ":" + c.key()
}

// parseEntityID reverses entityID, splitting off the trailing "kind:index".
func parseEntityID(id string) (devID string, c component, ok bool) {
	idx := strings.LastIndex(id, ":")
	if idx <= 0 || idx == len(id)-1 {
		return "", component{}, false
	}
	n, err := strconv.Atoi(id[idx+1:])
	if err != nil {
		return "", component{}, false
	}
	rest := id[:idx]
	idx2 := strings.LastIndex(rest, ":")
	if idx2 <= 0 {
		return "", component{}, false
	}
	return rest[:idx2], component{Kind: rest[idx2+1:], Index: n}, true
}

// parseComponents extracts the component list from a Shelly.GetStatus result
// keyed by "kind:index" (e.g. "switch:0", "input:1"). Non-component keys
// (top-level services like "sys", "wifi", "cloud", "mqtt", "ble") have no
// numeric suffix and are skipped.
func parseComponents(status map[string]map[string]any) []component {
	var out []component
	for key := range status {
		idx := strings.LastIndex(key, ":")
		if idx <= 0 || idx == len(key)-1 {
			continue
		}
		n, err := strconv.Atoi(key[idx+1:])
		if err != nil {
			continue
		}
		out = append(out, component{Kind: key[:idx], Index: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Index < out[j].Index
	})
	return out
}

func sameComponents(a, b []component) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sensorMeta is the dS sensor descriptor metadata for one status field,
// reusing the same dS type numbers Zigbee2MQTT already established
// (pkg/bridge/zigbee2mqtt/discovery.go's z2mNumericMeta) so the two plugins
// present the same kind of field the same way.
type sensorMeta struct {
	Type       int
	Usage      int
	Name       string
	Min, Max   float64
	Resolution float64
	SIUnit     string
	Symbol     string
}

// shellySensorMeta maps a Shelly status field name (dotted for nested
// fields, e.g. "aenergy.total") to its dS sensor descriptor metadata.
var shellySensorMeta = map[string]sensorMeta{
	"voltage":        {Type: 4, Name: "voltage", Min: 0, Max: 500, Resolution: 0.1, SIUnit: "volt", Symbol: "V"},
	"apower":         {Type: 14, Name: "power", Min: 0, Max: 10000, Resolution: 1, SIUnit: "watt", Symbol: "W"},
	"current":        {Type: 15, Name: "current", Min: 0, Max: 100, Resolution: 0.1, SIUnit: "ampere", Symbol: "A"},
	"aenergy.total":  {Type: 16, Name: "energy", Min: 0, Max: 1e7, Resolution: 0.1, SIUnit: "kilowatt_hour", Symbol: "kWh"},
	"temperature.tC": {Type: 1, Usage: 1, Name: "temperature", Min: -40, Max: 125, Resolution: 0.1, SIUnit: "celsius", Symbol: "°C"},
}

// sensorFeature is one numeric field folded into a device's merged
// sensor/binary entity, e.g. the "apower" field of "switch:0".
type sensorFeature struct {
	Source component // the component the field is read from at status time
	Field  string    // status field name, dotted for nested fields
	Index  int       // dense 0-based sensor descriptor index within the entity
	Meta   sensorMeta
}

// binaryFeature is one boolean field folded into a device's merged
// sensor/binary entity — currently always a switch-type input's "state".
type binaryFeature struct {
	Source component
	Field  string
	Index  int // dense 0-based binary descriptor index within the entity
}

// entitySpec describes one bridgeable entity derived from a device's
// components, precomputed once during scanner enrichment so Discover() and
// Subscribe() stay O(1) in-memory.
type entitySpec struct {
	// Component identifies this entity for entityID()/parseEntityID(). For a
	// primary output (switch/light) it's that component itself, so existing
	// bridge mappings from before this entity model existed keep working
	// unchanged. For a button-type input it's that input component. For the
	// merged sensor/binary entity it's the synthetic {Kind: "sensor", Index: 0}
	// — "sensor" is never a real Shelly component kind, so this can't collide
	// with an actual component.
	Component component
	Kind      string // bridge.RemoteEntity Kind ("light", "dimmer", "sensor", "binary", "button")

	// SensorFeatures and BinaryFeatures are only set for the merged
	// sensor/binary entity (Kind == "sensor" or "binary").
	SensorFeatures []sensorFeature
	BinaryFeatures []binaryFeature
}

// buildEntities derives the bridgeable entities for a device from its
// components, its status snapshot at discovery time, and each input's
// configured type. Mirrors Zigbee2MQTT's endpointsWithOpts: primary outputs
// (switch/light) are one entity each; every numeric (power/energy/
// temperature) field actually present, plus every switch-type input's
// boolean state, are merged into a single "sensor" (or "binary", if there
// are no numeric fields) entity — a relay+meter+input device doesn't need
// three separate bridge mappings for what is clearly one physical thing.
// Button-type inputs get their own "button" entity, since they're a
// fundamentally different kind of dS device (events, not a channel/sensor
// value).
func buildEntities(components []component, status map[string]map[string]any, inputTypes map[int]string) []entitySpec {
	var out []entitySpec
	var sensorFeats []sensorFeature
	var binaryFeats []binaryFeature

	for _, c := range components {
		switch c.Kind {
		case "switch":
			out = append(out, entitySpec{Component: c, Kind: "light"})
			sensorFeats = appendPresentSensorFeatures(sensorFeats, c, status[c.key()],
				"apower", "voltage", "current", "aenergy.total", "temperature.tC")
		case "light":
			out = append(out, entitySpec{Component: c, Kind: "dimmer"})
			sensorFeats = appendPresentSensorFeatures(sensorFeats, c, status[c.key()],
				"apower", "voltage", "current", "aenergy.total", "temperature.tC")
		case "pm1":
			sensorFeats = appendPresentSensorFeatures(sensorFeats, c, status[c.key()],
				"apower", "voltage", "current", "aenergy.total")
		case "input":
			if inputTypes[c.Index] == "button" {
				out = append(out, entitySpec{Component: c, Kind: "button"})
			} else {
				binaryFeats = append(binaryFeats, binaryFeature{Source: c, Field: "state", Index: len(binaryFeats)})
			}
		}
	}

	if len(sensorFeats) > 0 || len(binaryFeats) > 0 {
		kind := "binary"
		if len(sensorFeats) > 0 {
			kind = "sensor"
		}
		out = append(out, entitySpec{
			Component:      component{Kind: "sensor", Index: 0},
			Kind:           kind,
			SensorFeatures: sensorFeats,
			BinaryFeatures: binaryFeats,
		})
	}

	return out
}

// appendPresentSensorFeatures appends a sensorFeature for each of names that
// is actually present in fields — a plain (non-PM) switch component, for
// example, has no apower/voltage/current/aenergy at all.
func appendPresentSensorFeatures(feats []sensorFeature, src component, fields map[string]any, names ...string) []sensorFeature {
	for _, name := range names {
		if _, ok := lookupDotted(fields, name); !ok {
			continue
		}
		meta, ok := shellySensorMeta[name]
		if !ok {
			continue
		}
		feats = append(feats, sensorFeature{Source: src, Field: name, Index: len(feats), Meta: meta})
	}
	return feats
}

// entityForComponent finds the entitySpec identified by c among entities.
func entityForComponent(entities []entitySpec, c component) (entitySpec, bool) {
	for _, e := range entities {
		if e.Component == c {
			return e, true
		}
	}
	return entitySpec{}, false
}
