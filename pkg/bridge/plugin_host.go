package bridge

import "time"

// pluginHost wraps a shared Host and provides a per-plugin Log implementation
// that auto-tags events with the plugin ID and forwards them to the Registry's
// current EventSink. The sink is resolved at call time via a closure so that
// plugins constructed before SetEventSink is called still emit events once a
// sink is registered.
type pluginHost struct {
	Host
	pluginID string
	getSink  func() EventSink
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
