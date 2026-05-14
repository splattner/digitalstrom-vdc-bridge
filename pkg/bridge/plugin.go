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
}

// Factory creates a new Plugin instance for a given plugin type.
// The id parameter is the instance identifier from PluginConfig.ID.
type Factory func(id string) Plugin
