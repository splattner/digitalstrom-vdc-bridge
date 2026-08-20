package bridge

import (
	"context"
	"time"
)

// pluginHost wraps a shared Host and provides a per-plugin Log implementation
// that auto-tags events with the plugin ID and forwards them to the Registry's
// current EventSink. The sink is resolved at call time via a closure so that
// plugins constructed before SetEventSink is called still emit events once a
// sink is registered.
type pluginHost struct {
	Host
	pluginID          string
	getSink           func() EventSink
	getActivityBuffer func() *ActivityBuffer
	notifyDiscovery   func(pluginID string)
}

// NotifyDiscoveryChanged overrides the base to tell the Registry which
// plugin's discoverable set may have changed.
func (h *pluginHost) NotifyDiscoveryChanged() {
	if h.notifyDiscovery != nil {
		h.notifyDiscovery(h.pluginID)
	}
}

// Log emits a PluginEvent through the Registry's EventSink (if one is set).
func (h *pluginHost) Log(level LogLevel, code, message string, fields map[string]any) {
	s := h.getSink()
	if s == nil {
		return
	}
	s.Publish(PluginEvent{
		Time:     time.Now().UTC(),
		PluginID: h.pluginID,
		Level:    level,
		Code:     code,
		Message:  message,
		Fields:   fields,
	})
}

// UpdateChannel overrides the base to record plugin-direction activity.
func (h *pluginHost) UpdateChannel(ctx context.Context, dsuid string, channelIndex int, value float64) error {
	if err := h.Host.UpdateChannel(ctx, dsuid, channelIndex, value); err != nil {
		return err
	}
	if ab := h.getActivityBuffer(); ab != nil {
		v := value
		ab.Publish(DeviceActivity{
			DSUID:    dsuid,
			Source:   ActivitySourcePlugin,
			PluginID: h.pluginID,
			Type:     "channel",
			Index:    channelIndex,
			Value:    &v,
		})
	}
	return nil
}

// UpdateActive overrides the base to record active-state changes.
func (h *pluginHost) UpdateActive(ctx context.Context, dsuid string, active bool) error {
	if err := h.Host.UpdateActive(ctx, dsuid, active); err != nil {
		return err
	}
	if ab := h.getActivityBuffer(); ab != nil {
		a := active
		ab.Publish(DeviceActivity{
			DSUID:    dsuid,
			Source:   ActivitySourcePlugin,
			PluginID: h.pluginID,
			Type:     "active",
			Active:   &a,
		})
	}
	return nil
}
