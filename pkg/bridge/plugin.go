// Package bridge defines the plugin interface and shared types for bridge plugins
// that connect external systems (e.g. Home Assistant) to the vDC device world.
package bridge

import "context"

// Plugin is the interface that all bridge plugins must implement.
type Plugin interface {
	// ID returns the unique instance identifier (from config).
	ID() string
	// Init initialises the plugin with its resolved config and a Host facade.
	// ctx is the lifetime context; the plugin should start background work here.
	Init(ctx context.Context, cfg map[string]any, host Host) error
	// Status returns a short, human-readable status string (e.g. "connected", "reconnecting").
	Status() string
	// Discover returns all remote entities currently visible to this plugin.
	// The caller filters out entities that already have a Mapping.
	Discover(ctx context.Context) ([]RemoteEntity, error)
	// Subscribe asks the plugin to start forwarding state updates for mapping m
	// via the Host callbacks.  Called once per Mapping on startup (restore) and
	// whenever a new bridge is created.
	Subscribe(ctx context.Context, m Mapping) error
	// Unsubscribe stops forwarding state updates for the given device DSUID.
	Unsubscribe(ctx context.Context, dsuid string) error
	// Apply sends a command (channel change, scene call, …) to the remote entity.
	Apply(ctx context.Context, m Mapping, cmd Command) error
	// Close shuts the plugin down cleanly.
	Close() error
}

// RemoteEntity is a discoverable entity returned by Plugin.Discover.
type RemoteEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind hints at the vDC output type: "light", "dimmer", "colorlight",
	// "sensor", "binary", …
	Kind string `json:"kind"`
	// Attributes carries plugin-specific metadata for display in the UI.
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Mapping links a remote plugin entity to a vDC device DSUID.
type Mapping struct {
	PluginID       string `json:"pluginId"`
	RemoteEntityID string `json:"remoteEntityId"`
	DSUID          string `json:"dsuid"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
}

// Command is sent by the vDC command path to a plugin via Plugin.Apply.
type Command struct {
	// Type is one of: "setChannel", "callScene", "setActive".
	Type    string  `json:"type"`
	Channel int     `json:"channel,omitempty"`
	Value   float64 `json:"value,omitempty"`
	Scene   int     `json:"scene,omitempty"`
	Active  bool    `json:"active,omitempty"`
}

// PluginConfig is the per-instance plugin configuration entry.
type PluginConfig struct {
	ID string `json:"id"`
	// Type identifies the registered factory (e.g. "homeassistant").
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
	// Disabled, when true, keeps the plugin config (and any bridge mappings
	// it owns) on disk but skips starting the instance. Use a "disabled"
	// rather than "enabled" flag so that a missing/false value in older
	// plugins.json files keeps existing behaviour.
	Disabled bool `json:"disabled,omitempty"`
}

// Factory creates a new Plugin instance for a given plugin type.
// The id parameter is the instance identifier from PluginConfig.ID.
type Factory func(id string) Plugin

// SceneCaller is an optional plugin capability for native remote scene/preset
// recall — e.g. switching a WLED strip to one of its own saved presets.
// Command routing (pkg/vdcapi/method_service.go) tries this first for a
// digitalSTROM scene call that has no explicitly saved per-channel scene for
// the addressed device; an explicit saved scene always wins, since it's a
// precise user intent a generic preset recall might not reproduce. Plugins
// that don't implement it (the default) get the existing computed-brightness
// fallback, unchanged. scene == 0 conventionally means "off" — implementers
// should turn the device off rather than trying to recall a "preset 0".
type SceneCaller interface {
	CallScene(ctx context.Context, m Mapping, scene int) error
}

// Suggester is an optional plugin capability: plugins implementing this
// interface advertise dynamic option lists for "select" / "multiselect"
// schema fields whose OptionsSource is "plugin". The HTTP API exposes them
// at GET /api/plugins/{id}/suggest/{field}.
//
// Implementations should be cheap (in-memory, no blocking I/O) and return an
// empty slice when no suggestions are available yet (e.g. before the plugin
// has connected to its remote system).
type Suggester interface {
	Suggest(ctx context.Context, field string) ([]SuggestOption, error)
}

// SuggestOption is one entry returned by Suggester.Suggest.
type SuggestOption struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	// Count optionally reports how many entities the value applies to,
	// helping users decide whether the option is relevant to ignore.
	Count int `json:"count,omitempty"`
}

// PluginStats is the lightweight counter snapshot returned by plugins that
// implement the optional StatsProvider interface. The numbers are advisory
// (used by the UI to show "discovered / bridged" in the plugin table) and
// must be cheap to compute (in-memory, non-blocking).
type PluginStats struct {
	// Discovered is the number of devices the plugin has currently observed
	// on the remote side (e.g. via MQTT autodiscovery, mDNS, or a snapshot
	// from the remote system). Includes both bridged and unbridged.
	Discovered int `json:"discovered"`
	// Active is the number of those devices that are currently bridged /
	// have an active subscription / forwarder.
	Active int `json:"active"`
}

// StatsProvider is an optional plugin capability for surfacing device
// counters in the API. Plugins that do not implement it simply return no
// stats and the UI falls back to mapping-derived counts.
type StatsProvider interface {
	Stats() PluginStats
}
