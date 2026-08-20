package shelly

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the full bridge.FactoryEntry for the Shelly plugin.
// The Shelly plugin currently has no required configuration — Gen2+ devices
// are auto-discovered via mDNS and controlled over their local RPC API
// (WebSocket for push, HTTP for commands). No cloud account, no MQTT broker,
// and no per-device setup are needed.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "Shelly",
		Description: "Discover Shelly Gen2+ devices on the local network via mDNS and bridge their relays and dimmers.",
		Factory:     Factory(),
		Schema:      bridge.ConfigSchema{},
	}
}
