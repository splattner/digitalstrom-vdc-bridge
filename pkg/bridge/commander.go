package bridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/splattner/vdcgo/pkg/logging"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

// Commander adapts a Registry to the vdcapi.Commander interface.  It first
// checks whether the addressed uniqueID belongs to a bridged device; if so it
// dispatches the command to the owning plugin's Apply method, otherwise it
// falls through to the wrapped fallback (typically the external-device server).
type Commander struct {
	registry *Registry
	fallback vdcapi.Commander
	timeout  time.Duration
}

// NewCommander wraps the registry around an existing fallback commander.
func NewCommander(r *Registry, fallback vdcapi.Commander) *Commander {
	return &Commander{registry: r, fallback: fallback, timeout: 5 * time.Second}
}

// SetLightLevel implements vdcapi.Commander.
func (c *Commander) SetLightLevel(uniqueID string, value float64) error {
	return c.SetChannelValue(uniqueID, 0, value)
}

// SetChannelValue implements vdcapi.Commander.
func (c *Commander) SetChannelValue(uniqueID string, channelIndex int, value float64) error {
	if c == nil {
		return fmt.Errorf("commander not configured")
	}
	if c.registry != nil {
		// Bridge mapping lookups are case-insensitive: dSUIDs may arrive uppercased.
		if m, plugin, ok := c.lookupBridge(uniqueID); ok {
			ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
			defer cancel()
			cmd := Command{Type: "setChannel", Channel: channelIndex, Value: value}
			// Light/dimmer/colorlight: a value of 0 on channel 0 also implies "off".
			if channelIndex == 0 {
				cmd.Active = value > 0
			}
			if err := plugin.Apply(ctx, m, cmd); err != nil {
				logging.Warn("bridge_apply_error", logging.Fields{
					"plugin_id": m.PluginID,
					"dsuid":     m.DSUID,
					"channel":   channelIndex,
					"value":     value,
					"error":     err.Error(),
				})
				c.registry.EmitPluginEvent(m.PluginID, LevelWarn, CodeApplyFailed,
					"apply command failed", map[string]any{
						"dsuid":   m.DSUID,
						"channel": channelIndex,
						"value":   value,
						"error":   err.Error(),
					})
				return err
			}
			logging.Info("bridge_apply", logging.Fields{
				"plugin_id": m.PluginID,
				"dsuid":     m.DSUID,
				"channel":   channelIndex,
				"value":     value,
			})
			return nil
		}
	}
	if c.fallback == nil {
		return fmt.Errorf("device with uniqueid %q not connected", uniqueID)
	}
	return c.fallback.SetChannelValue(uniqueID, channelIndex, value)
}

// lookupBridge resolves a uniqueID (case-insensitive) to its mapping + plugin.
func (c *Commander) lookupBridge(uniqueID string) (Mapping, Plugin, bool) {
	store := c.registry.Mappings()
	if store == nil {
		return Mapping{}, nil, false
	}
	if m, ok := store.Get(uniqueID); ok {
		if p, ok := c.registry.Plugin(m.PluginID); ok {
			return m, p, true
		}
		return Mapping{}, nil, false
	}
	// Case-insensitive fallback.
	upper := strings.ToUpper(uniqueID)
	for _, m := range store.List() {
		if strings.ToUpper(m.DSUID) == upper {
			if p, ok := c.registry.Plugin(m.PluginID); ok {
				return m, p, true
			}
			return Mapping{}, nil, false
		}
	}
	return Mapping{}, nil, false
}
