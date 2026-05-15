package homeassistant

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/logging"
)

// PluginType is the registered Factory key for Home Assistant plugins.
const PluginType = "homeassistant"

// Factory returns a bridge.Factory that produces Home Assistant plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{
			id:               id,
			subscribed:       make(map[string]bridge.Mapping),
			subByEntity:      make(map[string]string),
			latestStates:     make(map[string]haEntity),
			colorState:       make(map[string]colorChannels),
			sensorDescPushed: make(map[string]bool),
		}
	}
}

// Plugin is the bridge.Plugin implementation for Home Assistant.
type Plugin struct {
	id     string
	host   bridge.Host
	client *wsClient

	url   string
	token string

	statusVal atomic.Value // string

	mu sync.RWMutex
	// subscribed maps DSUID → Mapping (active forwards)
	subscribed map[string]bridge.Mapping
	// subByEntity maps entity_id → DSUID for fast event dispatch
	subByEntity map[string]string
	// latestStates is the last-known HA state per entity (snapshot + live updates)
	latestStates map[string]haEntity
	// colorState caches the most recent hue/sat per DSUID so single-channel
	// updates can be combined into the HA `hs_color` attribute.
	colorState map[string]colorChannels
	// sensorDescPushed remembers DSUIDs for which we already published the
	// sensorDescriptor so we don't re-emit it on every state_changed.
	sensorDescPushed map[string]bool
	// registries holds the most recent area/device/entity registry snapshot
	// fetched from HA on (re)connect. Used to enrich Discover() attributes.
	registries haRegistries
}

// colorChannels caches the most recent per-channel color values for a colorlight.
type colorChannels struct {
	hue, sat float64
	hueSet   bool
	satSet   bool
}

// ID returns the configured instance id.
func (p *Plugin) ID() string { return p.id }

// Status returns the current connection status.
func (p *Plugin) Status() string {
	if v := p.statusVal.Load(); v != nil {
		return v.(string)
	}
	return "starting"
}

// Init reads the config, validates it, and starts the WS client in the background.
func (p *Plugin) Init(ctx context.Context, cfg map[string]any, host bridge.Host) error {
	p.host = host
	p.statusVal.Store("starting")

	url, _ := cfg["url"].(string)
	token, _ := cfg["token"].(string)
	url = strings.TrimSpace(url)
	token = strings.TrimSpace(token)
	if url == "" {
		return fmt.Errorf("homeassistant: missing config.url")
	}
	if token == "" {
		return fmt.Errorf("homeassistant: missing config.token")
	}
	p.url = url
	p.token = token

	p.client = newWSClient(url, token)
	p.client.onSnapshot = p.handleSnapshot
	p.client.onStateChange = p.handleStateChange
	p.client.onStatus = func(s string) {
		p.statusVal.Store(s)
		switch s {
		case "connected":
			host.Log(bridge.LevelInfo, bridge.CodeConnectOK, "connected to Home Assistant",
				map[string]any{"url": url})
		case "auth_failed":
			host.Log(bridge.LevelError, bridge.CodeAuthFailed, "Home Assistant authentication failed",
				map[string]any{"url": url})
		case "reconnecting":
			host.Log(bridge.LevelWarn, bridge.CodeConnectFailed, "reconnecting to Home Assistant",
				map[string]any{"url": url})
		}
	}
	p.client.onRegistries = p.handleRegistries
	p.client.onWarn = func(code, message string, fields map[string]any) {
		host.Log(bridge.LevelWarn, code, message, fields)
	}

	go p.client.Run(ctx)
	logging.Info("ha_plugin_started", logging.Fields{"id": p.id, "url": url})
	return nil
}

// Discover returns all HA entities that classifyEntity recognises.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]bridge.RemoteEntity, 0, len(p.latestStates))
	for _, e := range p.latestStates {
		kind := classifyEntity(e)
		if kind == "" {
			continue
		}
		attrs := map[string]any{
			"state":        e.State,
			"device_class": e.Attributes["device_class"],
			"entity_id":    e.EntityID,
		}
		// Merge in plugin-specific extras from the HA registries (device,
		// area, manufacturer, model). Anything missing is simply absent.
		for k, v := range p.registries.resolveExtras(e.EntityID) {
			attrs[k] = v
		}
		out = append(out, bridge.RemoteEntity{
			ID:         e.EntityID,
			Name:       friendlyName(e),
			Kind:       kind,
			Attributes: attrs,
		})
	}
	return out, nil
}

// Subscribe registers a mapping and pushes the current state once if known.
func (p *Plugin) Subscribe(ctx context.Context, m bridge.Mapping) error {
	p.mu.Lock()
	p.subscribed[m.DSUID] = m
	p.subByEntity[m.RemoteEntityID] = m.DSUID
	state, hasState := p.latestStates[m.RemoteEntityID]
	p.mu.Unlock()

	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "entity subscribed",
		map[string]any{"dsuid": m.DSUID, "entity_id": m.RemoteEntityID})

	if hasState {
		p.pushState(ctx, m, state)
	}
	return nil
}

// Unsubscribe removes a mapping.
func (p *Plugin) Unsubscribe(_ context.Context, dsuid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m, ok := p.subscribed[dsuid]; ok {
		delete(p.subByEntity, m.RemoteEntityID)
		delete(p.subscribed, dsuid)
	}
	delete(p.colorState, dsuid)
	delete(p.sensorDescPushed, dsuid)
	return nil
}

// Apply translates a vDC command to a Home Assistant service call.
func (p *Plugin) Apply(ctx context.Context, m bridge.Mapping, cmd bridge.Command) error {
	if p.client == nil || !p.client.Connected() {
		return fmt.Errorf("homeassistant: not connected")
	}
	domain, _, ok := splitEntityID(m.RemoteEntityID)
	if !ok {
		return fmt.Errorf("homeassistant: invalid entity_id %q", m.RemoteEntityID)
	}
	target := map[string]any{"entity_id": m.RemoteEntityID}

	switch domain {
	case "light":
		return p.applyLight(ctx, m, cmd, target)
	default:
		// Sensors and other read-only types: nothing to do.
		return nil
	}
}

func (p *Plugin) applyLight(ctx context.Context, m bridge.Mapping, cmd bridge.Command, target map[string]any) error {
	switch cmd.Type {
	case "setActive":
		service := "turn_on"
		if !cmd.Active {
			service = "turn_off"
		}
		return p.client.callService(ctx, "light", service, target, nil)
	case "setChannel":
		// Channels other than 0 are color attributes for colorlights and
		// have no off/on semantics — turn the light on while applying them.
		switch cmd.Channel {
		case 0:
			// Brightness on channel 0 (vDC dimmer convention).
			if cmd.Value <= 0 {
				return p.client.callService(ctx, "light", "turn_off", target, nil)
			}
			bri := int(cmd.Value/100.0*255.0 + 0.5)
			if bri > 255 {
				bri = 255
			}
			return p.client.callService(ctx, "light", "turn_on", target, map[string]any{
				"brightness": bri,
			})
		case 1, 2:
			// Hue (1) or saturation (2): combine with the cached counterpart
			// because HA expects both as `hs_color`.
			p.mu.Lock()
			cs := p.colorState[m.DSUID]
			if cmd.Channel == 1 {
				cs.hue = clamp(cmd.Value, 0, 360)
				cs.hueSet = true
			} else {
				cs.sat = clamp(cmd.Value, 0, 100)
				cs.satSet = true
			}
			p.colorState[m.DSUID] = cs
			hue, sat := cs.hue, cs.sat
			p.mu.Unlock()
			return p.client.callService(ctx, "light", "turn_on", target, map[string]any{
				"hs_color": []any{hue, sat},
			})
		case 3:
			// Color temperature in mired (channel value already in mired).
			mired := int(cmd.Value + 0.5)
			if mired < 1 {
				mired = 1
			}
			return p.client.callService(ctx, "light", "turn_on", target, map[string]any{
				"color_temp": mired,
			})
		case 4, 5:
			// CIE x/y not yet supported — ignore silently.
			return nil
		}
		return nil
	case "callScene":
		// Minimal scene mapping: scene 0 = off, anything else = on at 100%.
		if cmd.Scene == 0 {
			return p.client.callService(ctx, "light", "turn_off", target, nil)
		}
		return p.client.callService(ctx, "light", "turn_on", target, nil)
	}
	return nil
}

// Close shuts the plugin down.
func (p *Plugin) Close() error {
	// The wsClient stops when the Init context is cancelled; nothing else to do.
	return nil
}

// handleRegistries stores the most recent area / device / entity registry
// snapshot so Discover() can enrich entries with device + area names.
func (p *Plugin) handleRegistries(regs haRegistries) {
	p.mu.Lock()
	p.registries = regs
	p.mu.Unlock()
}

// handleSnapshot stores the initial state map and emits updates for any
// already-subscribed mappings.
func (p *Plugin) handleSnapshot(snap map[string]haEntity) {
	p.mu.Lock()
	p.latestStates = snap
	visible := 0
	for _, e := range snap {
		if classifyEntity(e) != "" {
			visible++
		}
	}
	// Take a copy of the subscriptions so we can release the lock before
	// invoking Host callbacks.
	subs := make([]bridge.Mapping, 0, len(p.subscribed))
	for _, m := range p.subscribed {
		subs = append(subs, m)
	}
	p.mu.Unlock()

	p.host.Log(bridge.LevelInfo, bridge.CodeDiscoveryDone, "HA state snapshot received",
		map[string]any{"total": len(snap), "visible": visible})

	ctx := context.Background()
	for _, m := range subs {
		if e, ok := snap[m.RemoteEntityID]; ok {
			p.pushState(ctx, m, e)
		}
	}
}

// handleStateChange updates the cache and forwards the new state if a mapping exists.
func (p *Plugin) handleStateChange(sc stateChange) {
	// Removal: HA fires state_changed with new_state=nil when an entity is
	// deleted (e.g. integration removed). Surface as entity_removed so the
	// operator can see vanishing devices in the Logs drawer.
	if sc.NewState == nil {
		p.mu.Lock()
		_, existed := p.latestStates[sc.EntityID]
		delete(p.latestStates, sc.EntityID)
		p.mu.Unlock()
		if existed {
			p.host.Log(bridge.LevelInfo, bridge.CodeEntityRemoved, "HA entity removed",
				map[string]any{"entity_id": sc.EntityID})
		}
		return
	}

	p.mu.Lock()
	_, existed := p.latestStates[sc.EntityID]
	p.latestStates[sc.EntityID] = *sc.NewState
	dsuid, hasSub := p.subByEntity[sc.EntityID]
	var m bridge.Mapping
	if hasSub {
		m = p.subscribed[dsuid]
	}
	p.mu.Unlock()

	// Newly-appearing entity (HA fires new_state set + old_state=nil when an
	// integration adds a brand-new entity after our initial snapshot). Only
	// log it if we'd actually classify it as a bridgeable device — otherwise
	// every weather/sun/script entity would spam the log.
	if !existed && sc.OldState == nil {
		if classifyEntity(*sc.NewState) != "" {
			p.host.Log(bridge.LevelInfo, bridge.CodeEntityAdded, "HA entity appeared",
				map[string]any{
					"entity_id": sc.EntityID,
					"kind":      classifyEntity(*sc.NewState),
					"state":     sc.NewState.State,
				})
		}
	}

	if hasSub {
		p.pushState(context.Background(), m, *sc.NewState)
	}
}

// pushState translates an HA entity state into Host callback updates for the device.
func (p *Plugin) pushState(ctx context.Context, m bridge.Mapping, e haEntity) {
	if p.host == nil {
		return
	}
	// active = is the entity available?
	active := e.State != "unavailable" && e.State != "unknown" && e.State != ""
	_ = p.host.UpdateActive(ctx, m.DSUID, active)
	if !active {
		return
	}

	switch m.Kind {
	case "light", "dimmer":
		if v, ok := brightnessFromState(e); ok {
			_ = p.host.UpdateChannel(ctx, m.DSUID, 0, v)
		}
	case "colorlight":
		if v, ok := brightnessFromState(e); ok {
			_ = p.host.UpdateChannel(ctx, m.DSUID, 0, v)
		}
		if h, s, ok := hueSatFromState(e); ok {
			_ = p.host.UpdateChannel(ctx, m.DSUID, 1, h)
			_ = p.host.UpdateChannel(ctx, m.DSUID, 2, s)
			p.mu.Lock()
			cs := p.colorState[m.DSUID]
			cs.hue, cs.sat = h, s
			cs.hueSet, cs.satSet = true, true
			p.colorState[m.DSUID] = cs
			p.mu.Unlock()
		}
		if mired, ok := colorTempMiredFromState(e); ok {
			_ = p.host.UpdateChannel(ctx, m.DSUID, 3, mired)
		}
	case "sensor":
		p.mu.Lock()
		alreadyPushed := p.sensorDescPushed[m.DSUID]
		p.mu.Unlock()
		if !alreadyPushed {
			if meta, ok := sensorMetaFor(e); ok {
				_ = p.host.SetSensorDescriptor(ctx, m.DSUID, 0, bridge.SensorDescriptor{
					Type:       meta.Type,
					Name:       meta.Name,
					Min:        meta.Min,
					Max:        meta.Max,
					Resolution: meta.Resolution,
					SIUnit:     meta.SIUnit,
					Symbol:     meta.Symbol,
				})
				p.mu.Lock()
				p.sensorDescPushed[m.DSUID] = true
				p.mu.Unlock()
			}
		}
		if v, ok := sensorValueFromState(e); ok {
			_ = p.host.UpdateSensor(ctx, m.DSUID, 0, v)
		}
	}
}
