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
	registry       *Registry
	fallback       vdcapi.Commander
	timeout        time.Duration
	activityBuffer *ActivityBuffer
}

// NewCommander wraps the registry around an existing fallback commander.
func NewCommander(r *Registry, fallback vdcapi.Commander) *Commander {
	return &Commander{registry: r, fallback: fallback, timeout: 5 * time.Second}
}

// SetActivityBuffer installs the ActivityBuffer for recording vdsm-originated commands.
func (c *Commander) SetActivityBuffer(ab *ActivityBuffer) {
	c.activityBuffer = ab
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
			if c.activityBuffer != nil {
				v := value
				c.activityBuffer.Publish(DeviceActivity{
					DSUID:    m.DSUID,
					Source:   ActivitySourceVDSM,
					PluginID: m.PluginID,
					Type:     "channel",
					Index:    channelIndex,
					Value:    &v,
				})
			}
			return nil
		}
	}
	if c.fallback == nil {
		return fmt.Errorf("device with uniqueid %q not connected", uniqueID)
	}
	return c.fallback.SetChannelValue(uniqueID, channelIndex, value)
}

// CallScene implements vdcapi.SceneCommander. It only does something for a
// bridged device whose owning plugin implements SceneCaller (currently just
// WLED, recalling one of its own presets) — any other case, including a
// device that IS bridged but whose plugin has no native scene support,
// returns an error so the caller (method_service.go's scene-call fallback)
// falls through to its computed-brightness-level behavior instead.
func (c *Commander) CallScene(uniqueID string, scene int) error {
	if c == nil {
		return fmt.Errorf("commander not configured")
	}
	if c.registry != nil {
		if m, plugin, ok := c.lookupBridge(uniqueID); ok {
			sc, ok := plugin.(SceneCaller)
			if !ok {
				return fmt.Errorf("plugin %q does not support native scene recall", m.PluginID)
			}
			ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
			defer cancel()
			if err := sc.CallScene(ctx, m, scene); err != nil {
				logging.Warn("bridge_call_scene_error", logging.Fields{
					"plugin_id": m.PluginID,
					"dsuid":     m.DSUID,
					"scene":     scene,
					"error":     err.Error(),
				})
				c.registry.EmitPluginEvent(m.PluginID, LevelWarn, CodeApplyFailed,
					"native scene recall failed", map[string]any{
						"dsuid": m.DSUID,
						"scene": scene,
						"error": err.Error(),
					})
				return err
			}
			logging.Info("bridge_call_scene", logging.Fields{
				"plugin_id": m.PluginID,
				"dsuid":     m.DSUID,
				"scene":     scene,
			})
			if c.activityBuffer != nil {
				c.activityBuffer.Publish(DeviceActivity{
					DSUID:    m.DSUID,
					Source:   ActivitySourceVDSM,
					PluginID: m.PluginID,
					Type:     "scene",
					Scene:    scene,
				})
			}
			return nil
		}
	}
	if sc, ok := c.fallback.(vdcapi.SceneCommander); ok {
		return sc.CallScene(uniqueID, scene)
	}
	return fmt.Errorf("device with uniqueid %q does not support native scene recall", uniqueID)
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
