package wled

import "github.com/splattner/vdcgo/pkg/bridge"

// RegisterEntry returns the full bridge.FactoryEntry for the WLED plugin.
// The WLED plugin currently has no required configuration — devices are
// auto-discovered via mDNS.
func RegisterEntry() bridge.FactoryEntry {
	return bridge.FactoryEntry{
		DisplayName: "WLED",
		Description: "Discover WLED LED controllers on the local network via mDNS and bridge them as colour lights.",
		Factory:     Factory(),
		Schema:      bridge.ConfigSchema{},
	}
}
