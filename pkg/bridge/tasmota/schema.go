package tasmota

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the FactoryEntry for the Tasmota plugin.
//
// The plugin needs an MQTT broker plugin to talk to — the `broker` field
// holds the id of an `mqtt` plugin instance configured separately.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "Tasmota (MQTT discovery)",
		Description: "Discovers Tasmota devices via MQTT discovery and bridges relays / dimmable & colour lights into the vDC. Requires an MQTT broker plugin instance.",
		Factory:     Factory(),
		Schema: bridge.ConfigSchema{
			Fields: []bridge.ConfigField{
				{
					Key: "broker", Label: "MQTT broker",
					Type: "select", Required: true,
					OptionsSource:    "plugins",
					PluginTypeFilter: "mqtt",
					Help: "The MQTT broker plugin instance to use.",
				},
				{
					Key: "discoveryPrefix", Label: "Discovery topic prefix",
					Type: "string", Default: "tasmota/discovery",
					Help: "Tasmota's `SetOption19` discovery root. Default 'tasmota/discovery'.",
				},
			},
		},
	}
}
