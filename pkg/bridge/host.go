package bridge

import (
	"context"

	"github.com/splattner/vdcgo/pkg/runtime"
	"github.com/splattner/vdcgo/pkg/services/mqtt"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

// Host is the facade given to plugins for announcing and updating vDC devices.
// All methods are safe for concurrent use.
type Host interface {
	// DeriveDSUID returns the deterministic dSUID for a (pluginID, remoteEntityID) pair.
	DeriveDSUID(pluginID, remoteEntityID string) string
	// AnnounceDevice registers a bridge device in the vDC state store.
	// m.DSUID must already be set (use DeriveDSUID).
	AnnounceDevice(ctx context.Context, m Mapping) error
	// RemoveDevice removes a bridge device from the vDC state store.
	RemoveDevice(ctx context.Context, dsuid string) error
	// UpdateChannel pushes a channel value update for a device.
	UpdateChannel(ctx context.Context, dsuid string, channelIndex int, value float64) error
	// UpdateSensor pushes a sensor input value update for a device.
	UpdateSensor(ctx context.Context, dsuid string, sensorIndex int, value float64) error
	// SetSensorDescriptor publishes metadata (type, range, unit) for a sensor input.
	// Should be called once per sensor (e.g. on the first state push) so the
	// vDSM/dSS UI can render the value with the correct label and unit.
	SetSensorDescriptor(ctx context.Context, dsuid string, sensorIndex int, desc SensorDescriptor) error
	// UpdateActive sets the online/active state of a device.
	UpdateActive(ctx context.Context, dsuid string, active bool) error
	// MQTT returns the shared MQTT broker manager. May be nil if the runtime
	// has no MQTT support compiled in (in practice always present).
	MQTT() *mqtt.Manager
	// Log emits a structured diagnostic event for the plugin that holds this
	// Host. The event is routed through the Registry's EventSink (if set) and
	// forwarded to WebSocket subscribers / REST snapshots.
	// Use the Code constants defined in plugin_event.go for well-known events.
	Log(level LogLevel, code, message string, fields map[string]any)
}

// SensorDescriptor is re-exported from the runtime package for plugin use.
type SensorDescriptor = runtime.SensorDescriptor

// hostImpl wires bridge callbacks into the shared vdcapi.StateStore.
type hostImpl struct {
	state *vdcapi.StateStore
	mqtt  *mqtt.Manager
}

// NewHost creates a Host backed by the given StateStore. The MQTT manager
// (which may be empty) lets bridge plugins look up shared broker connections
// owned by the "mqtt" plugin.
func NewHost(state *vdcapi.StateStore, mq *mqtt.Manager) Host {
	if mq == nil {
		mq = mqtt.NewManager()
	}
	return &hostImpl{state: state, mqtt: mq}
}

func (h *hostImpl) MQTT() *mqtt.Manager { return h.mqtt }

// Log is a no-op on the base hostImpl. Plugins receive a pluginHost wrapper
// (created by the Registry) that overrides this with real event emission.
func (h *hostImpl) Log(_ LogLevel, _, _ string, _ map[string]any) {}

func (h *hostImpl) DeriveDSUID(pluginID, remoteEntityID string) string {
	return vdcapi.BridgeDSUID(pluginID, remoteEntityID)
}

func (h *hostImpl) AnnounceDevice(_ context.Context, m Mapping) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventInit,
		UniqueID: m.DSUID,
		Name:     m.Name,
		Output:   kindToOutput(m.Kind),
	})
	return nil
}

func (h *hostImpl) RemoveDevice(_ context.Context, dsuid string) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventRemove,
		UniqueID: dsuid,
	})
	return nil
}

func (h *hostImpl) UpdateChannel(_ context.Context, dsuid string, channelIndex int, value float64) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventChannel,
		UniqueID: dsuid,
		Index:    channelIndex,
		Value:    value,
	})
	return nil
}

func (h *hostImpl) UpdateSensor(_ context.Context, dsuid string, sensorIndex int, value float64) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventSensor,
		UniqueID: dsuid,
		Index:    sensorIndex,
		Value:    value,
	})
	return nil
}

func (h *hostImpl) SetSensorDescriptor(_ context.Context, dsuid string, sensorIndex int, desc SensorDescriptor) error {
	d := desc
	h.state.HandleEvent(runtime.Event{
		Type:             runtime.EventSensorDescriptor,
		UniqueID:         dsuid,
		Index:            sensorIndex,
		SensorDescriptor: &d,
	})
	return nil
}

func (h *hostImpl) UpdateActive(_ context.Context, dsuid string, active bool) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventActive,
		UniqueID: dsuid,
		Active:   active,
	})
	return nil
}

// kindToOutput maps a bridge entity kind to a vDC output type string.
func kindToOutput(kind string) string {
	switch kind {
	case "light", "dimmer":
		return "light"
	case "colorlight":
		return "colorlight"
	case "movinglight":
		return "movinglight"
	case "sensor":
		return "sensor"
	case "binary":
		return "binaryInput"
	default:
		if kind == "" {
			return "light"
		}
		return kind
	}
}
