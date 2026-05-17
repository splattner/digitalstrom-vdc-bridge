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
	// UpdateButton pushes a raw button input state change (1=pressed, 0=released).
	// Bridge plugins that receive only pre-aggregated click events (HA, Z2M) may
	// pulse this 1→0 around a SetButtonAction call so that observers see the
	// button as briefly active.
	UpdateButton(ctx context.Context, dsuid string, buttonIndex int, value float64) error
	// SetButtonAction publishes a digitalSTROM-style click type
	// ("tip", "tip2", "tip3", "tip4", "hold") for a button input. This is the
	// preferred entry point for bridges that already receive aggregated button
	// events (HA event entities, Z2M action enum values).
	SetButtonAction(ctx context.Context, dsuid string, buttonIndex int, action string) error
	// UpdateSensor pushes a sensor input value update for a device.
	UpdateSensor(ctx context.Context, dsuid string, sensorIndex int, value float64) error
	// SetSensorDescriptor publishes metadata (type, range, unit) for a sensor input.
	// Should be called once per sensor (e.g. on the first state push) so the
	// vDSM/dSS UI can render the value with the correct label and unit.
	SetSensorDescriptor(ctx context.Context, dsuid string, sensorIndex int, desc SensorDescriptor) error
	// SetButtonDescriptor publishes metadata (type, element, group) for a button input.
	// Should be called once per button so the vDSM/dSS UI classifies the button correctly.
	// If index is not yet in the button map, a zero-value state entry is created on first update.
	SetButtonDescriptor(ctx context.Context, dsuid string, buttonIndex int, desc ButtonDescriptor) error
	// AnnounceRichDevice registers a device and atomically pushes all descriptor
	// metadata (button, sensor, binary-input) in one call, followed by a ReAnnounce
	// so the vDSM re-queries all configuration properties. Use this instead of
	// separate AnnounceDevice + SetXxxDescriptor + ReAnnounce calls when the full
	// descriptor is known up front (e.g. in an external-device plugin's Discover).
	AnnounceRichDevice(ctx context.Context, m Mapping, d DeviceDescriptor) error
	// UpdateInput pushes a binary input state change for a device.
	// value 1.0 = active/true, 0.0 = inactive/false.
	UpdateInput(ctx context.Context, dsuid string, inputIndex int, value float64) error
	// SetBinaryInputDescriptor publishes metadata (name, sensorFunction) for a binary input.
	// Should be called once per input so the vDSM/dSS UI classifies the input correctly.
	SetBinaryInputDescriptor(ctx context.Context, dsuid string, inputIndex int, desc BinaryInputDescriptor) error
	// ReAnnounce triggers a full device property re-announcement to the vDSM.
	// Call this after pushing descriptor metadata (sensor types, binary input functions)
	// that was not yet available when the device was first announced, so the vDSM/dSS
	// re-queries all configuration properties including descriptions.
	ReAnnounce(ctx context.Context, dsuid string) error
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

// BinaryInputDescriptor is re-exported from the runtime package for plugin use.
type BinaryInputDescriptor = runtime.BinaryInputDescriptor

// ButtonDescriptor is re-exported from the runtime package for plugin use.
type ButtonDescriptor = runtime.ButtonDescriptor

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

func (h *hostImpl) UpdateButton(_ context.Context, dsuid string, buttonIndex int, value float64) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventButton,
		UniqueID: dsuid,
		Index:    buttonIndex,
		Value:    value,
	})
	return nil
}

func (h *hostImpl) SetButtonAction(_ context.Context, dsuid string, buttonIndex int, action string) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventButtonAction,
		UniqueID: dsuid,
		Index:    buttonIndex,
		Action:   action,
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

func (h *hostImpl) UpdateInput(_ context.Context, dsuid string, inputIndex int, value float64) error {
	h.state.HandleEvent(runtime.Event{
		Type:     runtime.EventInput,
		UniqueID: dsuid,
		Index:    inputIndex,
		Value:    value,
	})
	return nil
}

func (h *hostImpl) SetBinaryInputDescriptor(_ context.Context, dsuid string, inputIndex int, desc BinaryInputDescriptor) error {
	d := desc
	h.state.HandleEvent(runtime.Event{
		Type:                  runtime.EventBinaryInputDescriptor,
		UniqueID:              dsuid,
		Index:                 inputIndex,
		BinaryInputDescriptor: &d,
	})
	return nil
}

func (h *hostImpl) SetButtonDescriptor(_ context.Context, dsuid string, buttonIndex int, desc ButtonDescriptor) error {
	d := desc
	h.state.HandleEvent(runtime.Event{
		Type:             runtime.EventButtonDescriptor,
		UniqueID:         dsuid,
		Index:            buttonIndex,
		ButtonDescriptor: &d,
	})
	return nil
}

func (h *hostImpl) AnnounceRichDevice(ctx context.Context, m Mapping, desc DeviceDescriptor) error {
	if err := h.AnnounceDevice(ctx, m); err != nil {
		return err
	}
	for i, s := range desc.Sensors {
		if err := h.SetSensorDescriptor(ctx, m.DSUID, i, s); err != nil {
			return err
		}
	}
	for i, inp := range desc.Inputs {
		if err := h.SetBinaryInputDescriptor(ctx, m.DSUID, i, inp); err != nil {
			return err
		}
	}
	for i, b := range desc.Buttons {
		if err := h.SetButtonDescriptor(ctx, m.DSUID, i, b); err != nil {
			return err
		}
	}
	hasDescriptors := len(desc.Sensors) > 0 || len(desc.Inputs) > 0 || len(desc.Buttons) > 0
	if hasDescriptors {
		return h.ReAnnounce(ctx, m.DSUID)
	}
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

func (h *hostImpl) ReAnnounce(_ context.Context, dsuid string) error {
	h.state.ReAnnounce(dsuid)
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
	case "button":
		return "button"
	default:
		if kind == "" {
			return "light"
		}
		return kind
	}
}
