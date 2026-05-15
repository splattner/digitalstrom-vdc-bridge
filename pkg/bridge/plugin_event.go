package bridge

import "time"

// LogLevel is the severity level attached to a PluginEvent.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// PluginEvent is a structured log/diagnostic event emitted by a bridge plugin
// or the bridge registry (for lifecycle events). Events are streamed live to
// the frontend via WebSocket and available as a snapshot via REST.
type PluginEvent struct {
	Seq      uint64    `json:"seq"`
	Time     time.Time `json:"time"`
	PluginID string    `json:"pluginId"`
	Level    LogLevel  `json:"level"`
	// Code is a machine-readable identifier, e.g. "connect_ok", "device_added".
	// Callers should use the constants defined below for well-known events.
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// Well-known event codes used by the registry and plugins. Individual plugins
// may add their own codes; these are the ones the UI actively styles.
const (
	CodePluginStarted   = "plugin_started"
	CodePluginStopped   = "plugin_stopped"
	CodePluginRestarted = "plugin_restarted"
	CodeConnectAttempt  = "connect_attempt"
	CodeConnectOK       = "connect_ok"
	CodeConnectFailed   = "connect_failed"
	CodeAuthFailed      = "auth_failed"
	CodeDiscoveryStart  = "discovery_started"
	CodeDiscoveryDone   = "discovery_completed"
	CodeEntityAdded     = "entity_added"
	CodeEntityUpdated   = "entity_updated"
	CodeEntityRemoved   = "entity_removed"
	CodeEntitySkipped   = "entity_skipped"
	CodeEntityError     = "entity_error"
	CodeSubscribeOK     = "subscribe_ok"
	CodeSubscribeFailed = "subscribe_failed"
	CodeApplyFailed     = "apply_failed"
	CodeMappingAdded    = "mapping_added"
	CodeMappingRemoved  = "mapping_removed"
)
