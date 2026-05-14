package zigbee2mqtt

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

// PluginType is the registry key for zigbee2mqtt plugins.
const PluginType = "zigbee2mqtt"

const defaultBaseTopic = "zigbee2mqtt"

// Factory returns a bridge.Factory producing zigbee2mqtt plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{
			id:         id,
			devices:    make(map[string]*discoveredDevice),
			subscribed: make(map[string]*deviceSub),
		}
	}
}

// Plugin is the bridge.Plugin implementation for zigbee2mqtt.
type Plugin struct {
	id     string
	host   bridge.Host
	broker mqttsvc.Broker
	cfg    config

	devicesSub mqttsvc.Subscription

	mu sync.Mutex
	// devices keyed by IEEE address
	devices map[string]*discoveredDevice
	// subscribed keyed by mapping DSUID
	subscribed map[string]*deviceSub
}

type discoveredDevice struct {
	dev       bridgeDevice
	endpoints []endpoint
}

// endpoint lookup by name (or "" for single-endpoint).
func (d *discoveredDevice) endpoint(name string) (endpoint, bool) {
	for _, ep := range d.endpoints {
		if ep.Name == name {
			return ep, true
		}
	}
	return endpoint{}, false
}

type deviceSub struct {
	mapping bridge.Mapping
	ieee    string
	epName  string
	subs    []mqttsvc.Subscription
	// cached HSV used to merge single-axis hue/sat updates.
	hue float64
	sat float64
}

// ID returns the plugin instance id.
func (p *Plugin) ID() string { return p.id }

// Status reports broker status plus device/mapping counts.
func (p *Plugin) Status() string {
	if p.broker == nil {
		return "not_initialized"
	}
	s := p.broker.Status()
	p.mu.Lock()
	d := len(p.devices)
	a := len(p.subscribed)
	p.mu.Unlock()
	return fmt.Sprintf("%s · %d device(s) · %d active", s, d, a)
}

// Init resolves the broker and subscribes to <base>/bridge/devices.
func (p *Plugin) Init(ctx context.Context, raw map[string]any, host bridge.Host) error {
	c, err := parseConfig(raw)
	if err != nil {
		return fmt.Errorf("zigbee2mqtt %q: %w", p.id, err)
	}
	p.host = host
	p.cfg = c

	mgr := host.MQTT()
	if mgr == nil {
		return errors.New("zigbee2mqtt: MQTT manager is not available")
	}
	b, err := mgr.Get(c.broker)
	if err != nil {
		return fmt.Errorf("zigbee2mqtt: broker %q not registered: %w", c.broker, err)
	}
	p.broker = b

	topic := c.baseTopic + "/bridge/devices"
	sub, err := b.Subscribe(topic, 0, func(_ string, payload []byte, _ bool) {
		p.onDevices(ctx, payload)
	})
	if err != nil {
		return fmt.Errorf("zigbee2mqtt: subscribe %s: %w", topic, err)
	}
	p.devicesSub = sub

	logging.Info("zigbee2mqtt_plugin_started", logging.Fields{
		"id":     p.id,
		"broker": c.broker,
		"base":   c.baseTopic,
	})
	return nil
}

// Discover returns one entity per bridgeable endpoint of every known device.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]bridge.RemoteEntity, 0, len(p.devices))
	for _, d := range p.devices {
		for _, ep := range d.endpoints {
			attrs := map[string]any{
				"ieee":      d.dev.IEEE,
				"friendly":  d.dev.FriendlyName,
				"power":     d.dev.PowerSource,
				"sw":        d.dev.SoftwareBuildID,
				"supported": d.dev.Supported,
			}
			if d.dev.Definition != nil {
				attrs["model"] = d.dev.Definition.Model
				attrs["vendor"] = d.dev.Definition.Vendor
			}
			if ep.Name != "" {
				attrs["endpoint"] = ep.Name
			}
			out = append(out, bridge.RemoteEntity{
				ID:         d.dev.entityID(ep),
				Name:       d.dev.displayName(ep),
				Kind:       ep.Kind,
				Attributes: attrs,
			})
		}
	}
	return out, nil
}

// Subscribe records the mapping and (if device is known) installs state subs.
func (p *Plugin) Subscribe(_ context.Context, m bridge.Mapping) error {
	ieee, epName := parseEntityID(m.RemoteEntityID)
	sub := &deviceSub{mapping: m, ieee: ieee, epName: epName}
	p.mu.Lock()
	p.subscribed[m.DSUID] = sub
	dev := p.devices[ieee]
	p.mu.Unlock()

	if dev != nil {
		p.activate(sub, dev)
	} else {
		logging.Info("zigbee2mqtt_subscribe_pending", logging.Fields{
			"dsuid": m.DSUID, "ieee": ieee,
		})
	}
	return nil
}

// Unsubscribe tears down per-mapping topic subscriptions.
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

// Apply translates a vDC command into a z2m JSON `set` publish.
func (p *Plugin) Apply(ctx context.Context, m bridge.Mapping, cmd bridge.Command) error {
	p.mu.Lock()
	sub, ok := p.subscribed[m.DSUID]
	dev := p.devices[sub.ieee]
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("zigbee2mqtt: not subscribed: %s", m.DSUID)
	}
	if dev == nil {
		return fmt.Errorf("zigbee2mqtt: device %q not yet discovered", sub.ieee)
	}
	ep, ok := dev.endpoint(sub.epName)
	if !ok {
		return fmt.Errorf("zigbee2mqtt: endpoint %q not found on %s", sub.epName, sub.ieee)
	}

	payload := map[string]any{}

	switch cmd.Type {
	case "setActive":
		payload[ep.StateProp] = onOff(cmd.Active)

	case "callScene":
		payload[ep.StateProp] = onOff(cmd.Scene != 0)

	case "setChannel":
		switch cmd.Channel {
		case 0: // brightness / on-off
			if cmd.Value <= 0 {
				payload[ep.StateProp] = "OFF"
			} else if !ep.HasBrightness {
				payload[ep.StateProp] = "ON"
			} else {
				payload[ep.BrightnessProp] = vdcToBri(cmd.Value)
			}
		case 1, 2: // hue / saturation
			if !ep.HasColor {
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
			payload[ep.ColorProp] = map[string]any{"hue": int(h), "saturation": int(s)}
		default:
			return nil
		}

	default:
		return nil
	}

	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.publish(ctx, dev.dev.setTopic(p.cfg.baseTopic), body)
}

// Close tears down all subscriptions.
func (p *Plugin) Close() error {
	if p.devicesSub != nil {
		_ = p.devicesSub.Close()
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

// onDevices handles the retained <base>/bridge/devices array.
func (p *Plugin) onDevices(_ context.Context, payload []byte) {
	if len(payload) == 0 {
		return
	}
	var arr []bridgeDevice
	if err := json.Unmarshal(payload, &arr); err != nil {
		logging.Warn("zigbee2mqtt_devices_parse", logging.Fields{"error": err.Error()})
		return
	}
	seen := make(map[string]struct{}, len(arr))
	pending := []*deviceSub{}
	added := 0

	p.mu.Lock()
	for i := range arr {
		bd := arr[i]
		if bd.IEEE == "" || bd.Disabled || bd.Type == "Coordinator" {
			continue
		}
		eps := bd.endpoints()
		if len(eps) == 0 {
			continue
		}
		seen[bd.IEEE] = struct{}{}
		if _, existed := p.devices[bd.IEEE]; !existed {
			added++
		}
		p.devices[bd.IEEE] = &discoveredDevice{dev: bd, endpoints: eps}
		for _, sub := range p.subscribed {
			if sub.ieee == bd.IEEE && len(sub.subs) == 0 {
				pending = append(pending, sub)
			}
		}
	}
	// Drop devices that disappeared from the list.
	for ieee := range p.devices {
		if _, ok := seen[ieee]; !ok {
			delete(p.devices, ieee)
		}
	}
	p.mu.Unlock()

	if added > 0 {
		logging.Info("zigbee2mqtt_devices_updated", logging.Fields{
			"added": added, "total": len(seen),
		})
	}
	for _, sub := range pending {
		dev := p.devices[sub.ieee]
		if dev != nil {
			p.activate(sub, dev)
		}
	}
}

// activate subscribes the per-mapping state and availability topics.
func (p *Plugin) activate(sub *deviceSub, dev *discoveredDevice) {
	base := p.cfg.baseTopic
	stateTopic := dev.dev.stateTopic(base)
	availTopic := dev.dev.availabilityTopic(base)

	subs := make([]mqttsvc.Subscription, 0, 2)

	if s, err := p.broker.Subscribe(stateTopic, 0, func(_ string, payload []byte, _ bool) {
		var msg stateMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return
		}
		p.applyState(sub, dev, msg)
	}); err == nil {
		subs = append(subs, s)
	} else {
		logging.Warn("zigbee2mqtt_state_subscribe", logging.Fields{
			"topic": stateTopic, "error": err.Error(),
		})
	}

	// Availability is optional in z2m. Subscribing to a non-existent topic is
	// harmless — we just never receive anything.
	if s, err := p.broker.Subscribe(availTopic, 0, func(_ string, payload []byte, _ bool) {
		v := strings.TrimSpace(string(payload))
		// payload may be "online"/"offline" or {"state":"online"} (JSON form).
		if strings.HasPrefix(v, "{") {
			var j struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(payload, &j); err == nil {
				v = j.State
			}
		}
		_ = p.host.UpdateActive(context.Background(), sub.mapping.DSUID, strings.EqualFold(v, "online"))
	}); err == nil {
		subs = append(subs, s)
	}

	p.mu.Lock()
	sub.subs = subs
	p.mu.Unlock()

	logging.Info("zigbee2mqtt_subscribed", logging.Fields{
		"dsuid":    sub.mapping.DSUID,
		"ieee":     sub.ieee,
		"endpoint": sub.epName,
		"topic":    stateTopic,
	})
}

// applyState forwards a state JSON to the host's channel/active state.
func (p *Plugin) applyState(sub *deviceSub, dev *discoveredDevice, msg stateMessage) {
	ctx := context.Background()
	ep, ok := dev.endpoint(sub.epName)
	if !ok {
		return
	}

	on := true
	if s, ok := msg.stringField(ep.StateProp); ok {
		on = stateOn(s)
		if !on {
			_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, 0)
		} else if !ep.HasBrightness {
			_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, 100)
		}
	}

	if ep.HasBrightness {
		if bri, ok := msg.numberField(ep.BrightnessProp); ok {
			_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, briToVDC(bri, on))
		}
	}

	if ep.HasColor {
		if c := msg.colorField(ep.ColorProp); c != nil {
			h, hOK := c["hue"].(float64)
			s, sOK := c["saturation"].(float64)
			if hOK && sOK {
				p.mu.Lock()
				sub.hue = h
				sub.sat = s
				p.mu.Unlock()
				_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 1, h)
				_ = p.host.UpdateChannel(ctx, sub.mapping.DSUID, 2, s)
			}
		}
	}
}

// publish wraps the broker Publish with a debug log on error.
func (p *Plugin) publish(ctx context.Context, topic string, payload []byte) error {
	if err := p.broker.Publish(ctx, topic, payload, 0, false); err != nil {
		logging.Warn("zigbee2mqtt_publish", logging.Fields{
			"topic": topic, "error": err.Error(),
		})
		return err
	}
	return nil
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// ── config ────────────────────────────────────────────────────────────────────

type config struct {
	broker    string
	baseTopic string
}

func parseConfig(raw map[string]any) (config, error) {
	c := config{baseTopic: defaultBaseTopic}
	if v, ok := raw["broker"].(string); ok {
		c.broker = strings.TrimSpace(v)
	}
	if c.broker == "" {
		return c, errors.New("'broker' is required (id of the MQTT plugin instance)")
	}
	if v, ok := raw["baseTopic"].(string); ok {
		v = strings.TrimSpace(strings.TrimRight(v, "/"))
		if v != "" {
			c.baseTopic = v
		}
	}
	return c, nil
}
