package homeassistant

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
			lastButtonEvent:  make(map[string]string),
			buttonHeld:       make(map[string]bool),
			buttonHoldTimers: make(map[string]*time.Timer),
		}
	}
}

// Plugin is the bridge.Plugin implementation for Home Assistant.
type Plugin struct {
	id     string
	host   bridge.Host
	client *wsClient
	cancel context.CancelFunc

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
	// lastButtonEvent stores the last seen state (timestamp) of an event
	// entity per DSUID. The first observation seeds the value without firing
	// an action so that snapshot replay on (re)connect doesn't replay the
	// last button press.
	lastButtonEvent map[string]string
	// buttonHeld tracks whether a button DSUID is currently in a sustained
	// hold so we know whether to honour an incoming release event.
	buttonHeld map[string]bool
	// buttonHoldTimers carries the watchdog cancellation per held DSUID so
	// silently dropped release events still cause dSS to stop dimming.
	buttonHoldTimers map[string]*time.Timer
	// registries holds the most recent area/device/entity registry snapshot
	// fetched from HA on (re)connect. Used to enrich Discover() attributes.
	registries haRegistries
	// filter is the resolved entity-filter configuration (ignore_integrations,
	// ignore_zigbee2mqtt, ignore_entity_prefixes). Constant after Init.
	filter entityFilter
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

// Stats returns the number of discovered entities (those matching the filter)
// and the number that are currently bridged (active subscriptions).
func (p *Plugin) Stats() bridge.PluginStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// Count only entities classifyEntity would accept and that the filter
	// doesn't hide — same logic as Discover() without allocating the slice.
	discovered := 0
	for _, e := range p.latestStates {
		if classifyEntity(e) == "" {
			continue
		}
		if p.registries.shouldIgnore(e.EntityID, p.filter) {
			continue
		}
		discovered++
	}
	return bridge.PluginStats{Discovered: discovered, Active: len(p.subscribed)}
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
	p.filter = parseEntityFilter(cfg)

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

	// Derive our own cancellable context rather than using ctx directly: ctx
	// is the Registry's shared, process-lifetime runtime context, never
	// cancelled per-plugin — Close() needs something it can actually stop.
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	go p.client.Run(runCtx)
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
		if p.registries.shouldIgnore(e.EntityID, p.filter) {
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
	manufacturer, model, swVersion := p.registries.deviceMeta(m.RemoteEntityID)
	p.mu.Unlock()

	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "entity subscribed",
		map[string]any{"dsuid": m.DSUID, "entity_id": m.RemoteEntityID})

	// Forward device metadata to digitalSTROM (HA device registry provides
	// manufacturer, model, and sw_version). Empty strings are silently ignored.
	if manufacturer != "" || model != "" || swVersion != "" {
		_ = p.host.UpdateDeviceMeta(ctx, m.DSUID, manufacturer, model, swVersion)
	}

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
	delete(p.lastButtonEvent, dsuid)
	delete(p.buttonHeld, dsuid)
	if t, ok := p.buttonHoldTimers[dsuid]; ok {
		t.Stop()
		delete(p.buttonHoldTimers, dsuid)
	}
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
	}
	return nil
}

// Close shuts the plugin down, stopping the WS client's background Run loop.
func (p *Plugin) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// Suggest returns dynamic option lists for schema fields whose OptionsSource
// is "plugin". Currently supported field: "ignore_integrations" — returns the
// distinct entity registry platforms (each with the count of entities backed
// by that integration) that the connected HA instance currently exposes.
func (p *Plugin) Suggest(_ context.Context, field string) ([]bridge.SuggestOption, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch field {
	case "ignore_integrations":
		counts := map[string]int{}
		for _, e := range p.registries.Entities {
			if e.Platform == "" {
				continue
			}
			counts[e.Platform]++
		}
		out := make([]bridge.SuggestOption, 0, len(counts))
		for plat, c := range counts {
			out = append(out, bridge.SuggestOption{
				Value: plat,
				Label: fmt.Sprintf("%s (%d)", plat, c),
				Count: c,
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
		return out, nil
	}
	return nil, nil
}

// handleRegistries stores the most recent area / device / entity registry
// snapshot so Discover() can enrich entries with device + area names.
func (p *Plugin) handleRegistries(regs haRegistries) {
	p.mu.Lock()
	p.registries = regs
	filter := p.filter
	p.mu.Unlock()

	// Diagnostics: count how many devices the Z2M heuristic matches and how
	// many entities would be filtered. Lets the user verify the toggle is
	// actually doing something against their HA data.
	z2mDevs := 0
	for id := range regs.Devices {
		if regs.isZigbee2MQTTDevice(id) {
			z2mDevs++
		}
	}
	z2mEnts := 0
	for _, e := range regs.Entities {
		if regs.isZigbee2MQTTDevice(e.DeviceID) {
			z2mEnts++
		}
	}
	p.host.Log(bridge.LevelInfo, bridge.CodeDiscoveryDone, "HA registries loaded",
		map[string]any{
			"areas":               len(regs.Areas),
			"devices":             len(regs.Devices),
			"entities":            len(regs.Entities),
			"z2m_devices":         z2mDevs,
			"z2m_entities":        z2mEnts,
			"ignore_zigbee2mqtt":  filter.ignoreZigbee2MQTT,
			"ignore_integrations": keysOf(filter.ignoreIntegrations),
		})
}

// handleSnapshot stores the initial state map and emits updates for any
// already-subscribed mappings.
func (p *Plugin) handleSnapshot(snap map[string]haEntity) {
	p.mu.Lock()
	p.latestStates = snap
	visible := 0
	filtered := 0
	for _, e := range snap {
		if classifyEntity(e) == "" {
			continue
		}
		if p.registries.shouldIgnore(e.EntityID, p.filter) {
			filtered++
			continue
		}
		visible++
	}
	// Take a copy of the subscriptions so we can release the lock before
	// invoking Host callbacks.
	subs := make([]bridge.Mapping, 0, len(p.subscribed))
	for _, m := range p.subscribed {
		subs = append(subs, m)
	}
	p.mu.Unlock()

	p.host.Log(bridge.LevelInfo, bridge.CodeDiscoveryDone, "HA state snapshot received",
		map[string]any{"total": len(snap), "visible": visible, "filtered": filtered})

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
		if classifyEntity(*sc.NewState) != "" && !p.registries.shouldIgnore(sc.EntityID, p.filter) {
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
					Usage:      meta.Usage,
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
	case "button":
		// HA event entities advance their state to the timestamp of the
		// most recent event. Compare with the last seen value: if it changed
		// (and we had a previous value), the attached event_type tells us
		// which click verb to forward.
		p.mu.Lock()
		prev, hadPrev := p.lastButtonEvent[m.DSUID]
		p.lastButtonEvent[m.DSUID] = e.State
		p.mu.Unlock()
		if !hadPrev || prev == e.State {
			return
		}
		evType, _ := e.Attributes["event_type"].(string)
		mapped := mapHAEventType(evType)
		if mapped == "" {
			return
		}
		p.dispatchButtonAction(ctx, m.DSUID, 0, mapped)
	}
}

// buttonHoldWatchdog auto-releases a held button if no release event arrives
// within this window. HA integrations occasionally drop the long_release; the
// watchdog keeps dSS from dimming forever in that case.
const buttonHoldWatchdog = 30 * time.Second

// dispatchButtonAction translates a logical action verb (tip/tip2/.../hold/release)
// into the right sequence of host calls so dSS sees a coherent
// pressed → released gesture even when HA only sends discrete event_type strings.
//
// Behaviour:
//   - tip / tip2 / tip3 / tip4: synthetic 1 → action → 0 pulse.
//   - hold: latch value=1, remember the held state, arm a watchdog that
//     auto-releases after buttonHoldWatchdog if no real release arrives.
//   - release: only emit value=0 if we previously latched a hold. Cancels
//     the watchdog. Spurious releases are dropped.
func (p *Plugin) dispatchButtonAction(ctx context.Context, dsuid string, index int, action string) {
	switch action {
	case "release":
		p.mu.Lock()
		held := p.buttonHeld[dsuid]
		if held {
			delete(p.buttonHeld, dsuid)
			if t := p.buttonHoldTimers[dsuid]; t != nil {
				t.Stop()
				delete(p.buttonHoldTimers, dsuid)
			}
		}
		p.mu.Unlock()
		if !held {
			return
		}
		_ = p.host.SetButtonAction(ctx, dsuid, index, "release")
		_ = p.host.UpdateButton(ctx, dsuid, index, 0)
		return

	case "hold":
		p.mu.Lock()
		if t := p.buttonHoldTimers[dsuid]; t != nil {
			t.Stop()
		}
		p.buttonHeld[dsuid] = true
		p.buttonHoldTimers[dsuid] = time.AfterFunc(buttonHoldWatchdog, func() {
			p.autoReleaseHold(dsuid, index)
		})
		p.mu.Unlock()
		_ = p.host.UpdateButton(ctx, dsuid, index, 1)
		_ = p.host.SetButtonAction(ctx, dsuid, index, "hold")
		return

	default:
		_ = p.host.UpdateButton(ctx, dsuid, index, 1)
		_ = p.host.SetButtonAction(ctx, dsuid, index, action)
		_ = p.host.UpdateButton(ctx, dsuid, index, 0)
	}
}

// autoReleaseHold fires when the watchdog expires without a real release.
func (p *Plugin) autoReleaseHold(dsuid string, index int) {
	p.mu.Lock()
	if !p.buttonHeld[dsuid] {
		p.mu.Unlock()
		return
	}
	delete(p.buttonHeld, dsuid)
	delete(p.buttonHoldTimers, dsuid)
	p.mu.Unlock()
	logging.Warn("homeassistant_button_hold_watchdog", logging.Fields{
		"dsuid": dsuid,
		"index": index,
	})
	ctx := context.Background()
	_ = p.host.SetButtonAction(ctx, dsuid, index, "release")
	_ = p.host.UpdateButton(ctx, dsuid, index, 0)
}

// keysOf returns the sorted keys of a string-keyed map (used for stable log output).
func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
