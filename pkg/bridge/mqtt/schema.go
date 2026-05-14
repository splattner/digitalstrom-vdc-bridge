package mqtt

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the FactoryEntry for the MQTT broker plugin.
//
// The MQTT plugin is a "helper" plugin: it owns one MQTT broker connection
// and registers it with the shared MQTT manager so that other bridge plugins
// (Tasmota, Zigbee2MQTT, MQTT discovery, …) can reuse it by referencing the
// MQTT plugin's instance id from their own config.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "MQTT Broker",
		Description: "Connects to an MQTT broker and exposes the connection as a shared service for other plugins.",
		Factory:     Factory(),
		Probe:       Probe,
		Schema: bridge.ConfigSchema{
			Fields: []bridge.ConfigField{
				{
					Key: "host", Label: "Broker host", Type: "string",
					Required: true, Placeholder: "mqtt.local",
				},
				{
					Key: "port", Label: "Broker port", Type: "int",
					Default: 1883, Help: "Plain MQTT default 1883, TLS default 8883.",
					Min: intPtr(1), Max: intPtr(65535),
				},
				{
					Key: "tls", Label: "Use TLS", Type: "bool", Default: false,
				},
				{
					Key: "tlsInsecure", Label: "Skip TLS verification",
					Type: "bool", Default: false,
					Help: "Accept any server certificate. Only enable for testing.",
				},
				{
					Key: "caCert", Label: "CA certificate (PEM)",
					Type: "string",
					Help: "Optional. Pasted PEM block used to verify the broker certificate.",
				},
				{
					Key: "clientId", Label: "Client ID", Type: "string",
					Default: "vdcgo",
				},
				{
					Key: "username", Label: "Username", Type: "string",
				},
				{
					Key: "password", Label: "Password", Type: "password",
				},
				{
					Key: "keepalive", Label: "Keepalive (seconds)", Type: "int",
					Default: 60, Min: intPtr(5), Max: intPtr(3600),
				},
				{
					Key: "cleanSession", Label: "Clean session",
					Type: "bool", Default: true,
				},
				{
					Key: "will", Label: "Last-will message", Type: "object",
					Help: "Optional message published by the broker when this client disconnects unexpectedly.",
					Children: []bridge.ConfigField{
						{Key: "topic", Label: "Topic", Type: "string"},
						{Key: "payload", Label: "Payload", Type: "string"},
						{Key: "qos", Label: "QoS", Type: "int", Default: 0, Min: intPtr(0), Max: intPtr(2)},
						{Key: "retain", Label: "Retain", Type: "bool", Default: false},
					},
				},
			},
		},
	}
}

func intPtr(v int) *int { return &v }
