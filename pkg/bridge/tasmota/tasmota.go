// Package tasmota is a bridge.Plugin that discovers Tasmota devices via the
// MQTT discovery protocol (`<discoveryPrefix>/<MAC>/config`), bridges each
// relay/light into the vDC device world, and forwards POWER / Dimmer /
// HsbColor commands.
//
// Requires an MQTT broker plugin instance — its id is referenced from the
// `broker` config key and resolved at Init time via host.MQTT().
package tasmota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/logging"
	mqttsvc "github.com/splattner/vdcgo/pkg/services/mqtt"
)

// PluginType is the registered Factory key for Tasmota plugins.
const PluginType = "tasmota"

// defaultDiscoveryPrefix matches Tasmota's out-of-the-box `SetOption19 1`
// (homeassistant-style discovery) topic root.
const defaultDiscoveryPrefix = "tasmota/discovery"

// Factory returns a bridge.Factory producing Tasmota plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{
			id:         id,
			devices:    make(map[string]*discoveredDevice),
			subscribed: make(map[string]*deviceSub),
		}
	}
}

// Plugin is the bridge.Plugin implementation for Tasmota over MQTT.
type Plugin struct {
	id     string
	host   bridge.Host
	broker mqttsvc.Broker
	cfg    config

	discoverySub mqttsvc.Subscription

	mu sync.Mutex
	// devices is the live registry of every Tasmota device seen via discovery,
	// keyed by MAC.
	devices map[string]*discoveredDevice
	// subscribed tracks active mappings (DSUID → per-device state).
	subscribed map[string]*deviceSub
}

// discoveredDevice holds the most recent discovery payload plus runtime info.
type discoveredDevice struct {
	mac    string
	cfg    discoveryConfig
	online bool
}

// deviceSub is the per-mapping runtime state for a subscribed relay.
type deviceSub struct {
	mapping bridge.Mapping
	mac     string
	relay   int
	// Per-device topic subscriptions (LWT + state). Nil until the device's
	// discovery payload arrives — in that case the subscription is "pending"
	// and gets installed by activate() once we know the topics.
	subs []mqttsvc.Subscription
	// cached HSV state used to merge single-axis hue/sat updates.
	hue float64
	sat float64
}

// ID returns the configured plugin instance id.
func (p *Plugin) ID() string { return p.id }

// Status returns the underlying broker connection state.
func (p *Plugin) Status() string {
	if p.broker == nil {
		return "not_initialized"
	}
	return p.broker.Status()
}

// Stats reports the discovered and active (subscribed) device counts.
func (p *Plugin) Stats() bridge.PluginStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return bridge.PluginStats{Discovered: len(p.devices), Active: len(p.subscribed)}
}

// Init resolves the broker and starts the discovery subscription.
func (p *Plugin) Init(ctx context.Context, raw map[string]any, host bridge.Host) error {
	c, err := parseConfig(raw)
	if err != nil {
		return fmt.Errorf("tasmota %q: %w", p.id, err)
	}
	p.host = host
	p.cfg = c

	mgr := host.MQTT()
	if mgr == nil {
		return errors.New("tasmota: MQTT manager is not available")
	}
	b, err := mgr.Get(c.broker)
	if err != nil {
		return fmt.Errorf("tasmota: broker %q not registered: %w", c.broker, err)
	}
	p.broker = b

	topic := strings.TrimRight(c.discoveryPrefix, "/") + "/+/config"
	sub, err := b.Subscribe(topic, 0, func(t string, payload []byte, _ bool) {
		p.onDiscovery(ctx, t, payload)
	})
	if err != nil {
		return fmt.Errorf("tasmota: discovery subscribe: %w", err)
	}
	p.discoverySub = sub

	logging.Info("tasmota_plugin_started", logging.Fields{
		"id":     p.id,
		"broker": c.broker,
		"topic":  topic,
	})
	return nil
}

// Discover returns all relays/lights extracted from currently-known Tasmota
// devices.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]bridge.RemoteEntity, 0, len(p.devices))
	for _, dev := range p.devices {
		n := dev.cfg.relayCount()
		if n == 0 {
			continue
		}
		for relay := 1; relay <= n; relay++ {
			out = append(out, bridge.RemoteEntity{
				ID:   dev.cfg.entityID(relay),
				Name: dev.cfg.friendlyName(relay),
				Kind: dev.cfg.kindFor(relay),
				Attributes: map[string]any{
					"mac":   dev.mac,
					"ip":    dev.cfg.IP,
					"model": dev.cfg.Model,
					"sw":    dev.cfg.SwVer,
					"topic": dev.cfg.Topic,
					"relay": relay,
				},
			})
		}
	}
	return out, nil
}

// Subscribe records the mapping and (if the device is already known) installs
// per-device topic subscriptions.
func (p *Plugin) Subscribe(_ context.Context, m bridge.Mapping) error {
	mac, relay := parseEntityID(m.RemoteEntityID)
	sub := &deviceSub{mapping: m, mac: mac, relay: relay}
	p.mu.Lock()
	p.subscribed[m.DSUID] = sub
	dev := p.devices[mac]
	p.mu.Unlock()

	if dev != nil {
		p.activate(sub, dev)
	} else {
		logging.Info("tasmota_subscribe_pending", logging.Fields{
			"dsuid": m.DSUID, "mac": mac,
		})
	}
	return nil
}

// Unsubscribe stops forwarding state and tears down topic subscriptions.
func (p *Plugin) Unsubscribe(_ context.Context, dsuid string) error {
	p.mu.Lock()
	sub, ok := p.subscribed[dsuid]
	delete(p.subscribed, dsuid)
	p.mu.Unlock()
	if !ok {
		return nil
	}
	for _, s := range sub.subs {
		_ = s.Close()
	}
	return nil
}

// Apply translates a vDC command into Tasmota MQTT publishes.
func (p *Plugin) Apply(ctx context.Context, m bridge.Mapping, cmd bridge.Command) error {
	p.mu.Lock()
	sub, ok := p.subscribed[m.DSUID]
	dev := p.devices[parseMAC(m.RemoteEntityID)]
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("tasmota: not subscribed: %s", m.DSUID)
	}
	if dev == nil {
		return fmt.Errorf("tasmota: device %q not yet discovered", sub.mac)
	}

	powerSuffix := dev.cfg.powerSuffix(sub.relay)
	_, onPayload := dev.cfg.powerStates()
	offPayload := func() string { off, _ := dev.cfg.powerStates(); return off }()

	switch cmd.Type {
	case "setActive":
		payload := offPayload
		if cmd.Active {
			payload = onPayload
		}
		return p.publish(ctx, dev.cfg.cmndTopic(powerSuffix), []byte(payload))

	case "callScene":
		payload := offPayload
		if cmd.Scene != 0 {
			payload = onPayload
		}
		return p.publish(ctx, dev.cfg.cmndTopic(powerSuffix), []byte(payload))

	case "setChannel":
		switch cmd.Channel {
		case 0: // brightness / on-off
			if cmd.Value <= 0 {
				return p.publish(ctx, dev.cfg.cmndTopic(powerSuffix), []byte(offPayload))
			}
			kind := dev.cfg.kindFor(sub.relay)
			if kind == "light" {
				// Pure relay — anything > 0 means ON.
				return p.publish(ctx, dev.cfg.cmndTopic(powerSuffix), []byte(onPayload))
			}
			// Dimmer / colorlight — Tasmota's Dimmer command implicitly turns on.
			return p.publish(ctx, dev.cfg.cmndTopic("Dimmer"),
				[]byte(fmt.Sprintf("%d", vdcToDimmer(cmd.Value))))

		case 1, 2: // hue / saturation — combine with cached counterpart
			if dev.cfg.kindFor(sub.relay) != "colorlight" {
				return nil
			}
			p.mu.Lock()
			if cmd.Channel == 1 {
				sub.hue = clampF(cmd.Value, 0, 360)
			} else {
				sub.sat = clampF(cmd.Value, 0, 100)
			}
			h, s := sub.hue, sub.sat
			p.mu.Unlock()
			// Tasmota uses 100 as "current brightness" — leave V at 100 so the
			// existing Dimmer value drives perceived brightness.
			return p.publish(ctx, dev.cfg.cmndTopic("HsbColor"),
				[]byte(fmt.Sprintf("%d,%d,100", int(h), int(s))))
		}
	}
	return nil
}

// Close tears down all subscriptions.
func (p *Plugin) Close() error {
	if p.discoverySub != nil {
		_ = p.discoverySub.Close()
	}
	p.mu.Lock()
	subs := p.subscribed
	p.subscribed = make(map[string]*deviceSub)
	p.mu.Unlock()
	for _, sub := range subs {
		for _, s := range sub.subs {
			_ = s.Close()
		}
	}
	return nil
}

// ── internals ─────────────────────────────────────────────────────────────────

// onDiscovery handles a retained discovery message. Empty payload → device gone.
func (p *Plugin) onDiscovery(ctx context.Context, topic string, payload []byte) {
	mac := macFromDiscoveryTopic(topic)
	if mac == "" {
		return
	}
	if len(payload) == 0 {
		// Tombstone — Tasmota was un-discovered (rare).
		p.mu.Lock()
		delete(p.devices, mac)
		p.mu.Unlock()
		return
	}
	var c discoveryConfig
	if err := json.Unmarshal(payload, &c); err != nil {
		logging.Warn("tasmota_discovery_parse", logging.Fields{
			"topic": topic, "error": err.Error(),
		})
		p.host.Log(bridge.LevelWarn, bridge.CodeEntityError, "failed to parse Tasmota discovery payload",
			map[string]any{"topic": topic, "error": err.Error()})
		return
	}
	if c.MAC == "" {
		c.MAC = mac
	}
	dev := &discoveredDevice{mac: c.MAC, cfg: c}

	p.mu.Lock()
	first := p.devices[c.MAC] == nil
	p.devices[c.MAC] = dev
	// Activate any pending subscriptions for this MAC.
	pending := []*deviceSub{}
	for _, sub := range p.subscribed {
		if sub.mac == c.MAC && len(sub.subs) == 0 {
			pending = append(pending, sub)
		}
	}
	p.mu.Unlock()

	if first {
		logging.Info("tasmota_device_discovered", logging.Fields{
			"mac": c.MAC, "model": c.Model, "topic": c.Topic, "relays": c.relayCount(),
		})
		p.host.Log(bridge.LevelInfo, bridge.CodeEntityAdded, "Tasmota device discovered",
			map[string]any{"mac": c.MAC, "model": c.Model, "topic": c.Topic, "relays": c.relayCount()})
	}
	for _, sub := range pending {
		p.activate(sub, dev)
	}
	_ = ctx
}

// activate subscribes to per-device LWT + state topics for a single mapping.
func (p *Plugin) activate(sub *deviceSub, dev *discoveredDevice) {
	lwtTopic := dev.cfg.teleTopic("LWT")
	stateTopic := dev.cfg.teleTopic("STATE")
	resultTopic := dev.cfg.statTopic("RESULT")
	powerTopic := dev.cfg.statTopic(dev.cfg.powerSuffix(sub.relay))
	online, offline := dev.cfg.onlineStates()

	subs := make([]mqttsvc.Subscription, 0, 4)

	if s, err := p.broker.Subscribe(lwtTopic, 0, func(_ string, payload []byte, _ bool) {
		v := strings.TrimSpace(string(payload))
		active := v == online && v != offline // online wins; non-online ⇒ offline
		_ = p.host.UpdateActive(context.Background(), sub.mapping.DSUID, active)
	}); err == nil {
		subs = append(subs, s)
	} else {
		logging.Warn("tasmota_lwt_subscribe", logging.Fields{"topic": lwtTopic, "error": err.Error()})
		p.host.Log(bridge.LevelWarn, bridge.CodeSubscribeFailed, "failed to subscribe to Tasmota LWT topic",
			map[string]any{"dsuid": sub.mapping.DSUID, "topic": lwtTopic, "error": err.Error()})
	}

	stateHandler := func(_ string, payload []byte, _ bool) {
		var msg stateMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return
		}
		p.applyState(sub, dev, msg)
	}
	if s, err := p.broker.Subscribe(stateTopic, 0, stateHandler); err == nil {
		subs = append(subs, s)
	}
	if s, err := p.broker.Subscribe(resultTopic, 0, stateHandler); err == nil {
		subs = append(subs, s)
	}
	if s, err := p.broker.Subscribe(powerTopic, 0, func(_ string, payload []byte, _ bool) {
		v := strings.TrimSpace(string(payload))
		_, on := dev.cfg.powerStates()
		isOn := strings.EqualFold(v, on)
		val := 0.0
		if isOn {
			val = 100
		}
		_ = p.host.UpdateChannel(context.Background(), sub.mapping.DSUID, 0, val)
	}); err == nil {
		subs = append(subs, s)
	}

	p.mu.Lock()
	sub.subs = subs
	p.mu.Unlock()

	// Ask the device for its current state — Tasmota responds via stat/RESULT.
	_ = p.publish(context.Background(), dev.cfg.cmndTopic("STATE"), nil)

	logging.Info("tasmota_subscribed", logging.Fields{
		"dsuid": sub.mapping.DSUID,
		"mac":   sub.mac,
		"relay": sub.relay,
	})
	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "device subscribed",
		map[string]any{"dsuid": sub.mapping.DSUID, "mac": sub.mac, "relay": sub.relay})
}

// applyState pushes channel updates derived from a STATE / RESULT payload.
func (p *Plugin) applyState(sub *deviceSub, dev *discoveredDevice, msg stateMessage) {
	ctx := context.Background()
	_, onPayload := dev.cfg.powerStates()

	if power, ok := msg.powerForRelay(sub.relay); ok {
		on := strings.EqualFold(power, onPayload)
		switch dev.cfg.kindFor(sub.relay) {
		case "light":
			val := 0.0
			if on {
				val = 100
			}
			_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, val)
		default:
			// dimmer / colorlight handled below via Dimmer; if we only got POWER,
			// reflect the on/off transition immediately.
			if !on {
				_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, 0)
			} else if msg.Dimmer == nil {
				// keep last brightness — but make sure UI doesn't think it's at 0
				_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, 100)
			}
		}
	}

	if msg.Dimmer != nil && dev.cfg.kindFor(sub.relay) != "light" {
		on := true
		if power, ok := msg.powerForRelay(sub.relay); ok {
			on = strings.EqualFold(power, onPayload)
		}
		_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, briToVDC(*msg.Dimmer, on))
	}

	if dev.cfg.kindFor(sub.relay) == "colorlight" && msg.HSBColor != "" {
		if h, s, _, ok := parseHSB(msg.HSBColor); ok {
			p.mu.Lock()
			sub.hue = h
			sub.sat = s
			p.mu.Unlock()
			_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 1, h)
			_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 2, s)
		}
	}
}

// publish wraps a broker Publish with a debug log.
func (p *Plugin) publish(ctx context.Context, topic string, payload []byte) error {
	if err := p.broker.Publish(ctx, topic, payload, 0, false); err != nil {
		logging.Warn("tasmota_publish", logging.Fields{
			"topic": topic, "error": err.Error(),
		})
		return err
	}
	return nil
}

// macFromDiscoveryTopic extracts the MAC segment from "<prefix>/<MAC>/config".
func macFromDiscoveryTopic(t string) string {
	parts := strings.Split(t, "/")
	if len(parts) < 3 {
		return ""
	}
	if parts[len(parts)-1] != "config" {
		return ""
	}
	return parts[len(parts)-2]
}

// parseMAC extracts the MAC half of a remote entity id (ignores the relay suffix).
func parseMAC(id string) string {
	mac, _ := parseEntityID(id)
	return mac
}

// ── config ────────────────────────────────────────────────────────────────────

type config struct {
	broker          string
	discoveryPrefix string
}

func parseConfig(raw map[string]any) (config, error) {
	c := config{discoveryPrefix: defaultDiscoveryPrefix}
	if v, ok := raw["broker"].(string); ok {
		c.broker = strings.TrimSpace(v)
	}
	if c.broker == "" {
		return c, errors.New("'broker' is required (id of the MQTT plugin instance)")
	}
	if v, ok := raw["discoveryPrefix"].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			c.discoveryPrefix = v
		}
	}
	return c, nil
}
