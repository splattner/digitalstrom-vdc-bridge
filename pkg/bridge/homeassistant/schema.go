package homeassistant

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the full bridge.FactoryEntry for the Home Assistant
// plugin, including its config schema for the UI form renderer.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "Home Assistant",
		Description: "Bridge entities (lights, switches, sensors) from a Home Assistant instance via its WebSocket API.",
		Factory:     Factory(),
		Schema: bridge.ConfigSchema{
			Fields: []bridge.ConfigField{
				{
					Key:         "url",
					Label:       "WebSocket URL",
					Type:        "string",
					Required:    true,
					Placeholder: "ws://homeassistant.local:8123/api/websocket",
					Help:        "Full URL of the Home Assistant WebSocket API endpoint.",
				},
				{
					Key:         "token",
					Label:       "Long-lived access token",
					Type:        "password",
					Required:    true,
					Placeholder: "Created in HA → user profile → Long-lived access tokens",
				},
			},
		},
	}
}
