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
				{
					Key:           "ignore_integrations",
					Label:         "Ignore integrations",
					Type:          "multiselect",
					OptionsSource: "plugin",
					Help:          "Skip entities backed by these Home Assistant integrations. The list is populated from the platforms currently present in your HA instance — save the plugin first to see the choices.",
				},
				{
					Key:     "ignore_zigbee2mqtt",
					Label:   "Ignore Zigbee2MQTT devices",
					Type:    "bool",
					Default: false,
					Help:    "Skip entities backed by devices discovered via Zigbee2MQTT. Use this when you bridge Z2M directly via the Zigbee2MQTT plugin to avoid duplicates without ignoring all of MQTT.",
				},
				{
					Key:         "ignore_entity_prefixes",
					Label:       "Ignore entity prefixes",
					Type:        "string",
					Placeholder: "sensor.battery_, binary_sensor.linkquality_",
					Help:        "Comma-separated list of entity_id prefixes to ignore.",
				},
			},
		},
	}
}
