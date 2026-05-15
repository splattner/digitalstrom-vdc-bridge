package zigbee2mqtt

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the FactoryEntry for the zigbee2mqtt plugin.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "Zigbee2MQTT",
		Description: "Discovers Zigbee devices via a Zigbee2MQTT bridge and exposes lights / switches / sensors as vDC devices. Requires an MQTT broker plugin instance.",
		Factory:     Factory(),
		Schema: bridge.ConfigSchema{
			Fields: []bridge.ConfigField{
				{
					Key: "broker", Label: "MQTT broker plugin id",
					Type: "string", Required: true,
					Placeholder: "mqtt",
					Help:        "The id of the MQTT plugin instance to use (configure that plugin separately).",
				},
				{
					Key: "baseTopic", Label: "Base topic",
					Type: "string", Default: "zigbee2mqtt",
					Help: "Z2M `mqtt.base_topic` (default 'zigbee2mqtt').",
				},
				{
					Key:   "includeBattery",
					Label: "Include battery sensors",
					Type:  "bool", Default: false,
					Help: "Expose battery level as a vDC sensor input on every battery-powered device.",
				},
				{
					Key:   "includeLinkquality",
					Label: "Include link-quality sensors",
					Type:  "bool", Default: false,
					Help: "Expose link quality (signal strength, lqi) as a vDC sensor input.",
				},
			},
		},
	}
}
