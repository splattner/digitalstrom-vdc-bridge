package externaldevice

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the bridge.FactoryEntry for the external device plugin.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "External Device API",
		Description: "Host external scripts and programs as digitalSTROM devices via the vdcd external device TCP protocol (JSON or simple-text). Each program connects to the configured TCP port and declares itself with an \"init\" message.",
		Factory:     Factory(),
		Schema: bridge.ConfigSchema{
			Fields: []bridge.ConfigField{
				{
					Key:         "listen",
					Label:       "Listen address",
					Help:        "TCP port number or host:port to listen on. Defaults to 8999 if empty.",
					Type:        "string",
					Default:     "8999",
					Placeholder: "8999",
				},
				{
					Key:   "nonlocal",
					Label: "Accept non-local connections",
					Help:  "When enabled, connections from hosts other than localhost are accepted. Enable only if your external devices run on a different machine.",
					Type:  "bool",
				},
			},
		},
	}
}
