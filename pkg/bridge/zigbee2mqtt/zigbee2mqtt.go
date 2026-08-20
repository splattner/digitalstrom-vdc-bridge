package zigbee2mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
			groups:     make(map[string]*discoveredGroup),
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
	groupsSub  mqttsvc.Subscription

	mu sync.Mutex
	// devices keyed by IEEE address
	devices map[string]*discoveredDevice
	// groups keyed by group entity id ("z2m-group-<id>")
	groups map[string]*discoveredGroup
	// subscribed keyed by mapping DSUID
	subscribed map[string]*deviceSub
}

// discoveredGroup holds a parsed z2m group together with the light endpoint
// capability inferred from its members.
type discoveredGroup struct {
	grp      bridgeGroup
	endpoint endpoint
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

// buttonHoldWatchdog auto-releases a held button if no release event arrives
// within this window. Some Z2M devices drop their *_release message; without
// this safety net dSS would think the button is permanently held and keep
// dimming the room.
const buttonHoldWatchdog = 30 * time.Second

type deviceSub struct {
	mapping bridge.Mapping
	ieee    string
	epName  string
	// isGroup is true when this subscription is for a z2m group.
	// groupID holds the group entity id ("z2m-group-<id>") in that case.
	isGroup bool
	groupID string
	subs    []mqttsvc.Subscription
	// cached HSV used to merge single-axis hue/sat updates.
	hue float64
	sat float64

	// held tracks whether button index i is currently in a sustained hold,
	// indexed by button index. holdTimers carries the watchdog cancellation
	// that fires an automatic release when the upstream silently drops the
	// real *_release event. Both maps are guarded by the parent Plugin mutex.
	held       map[int]bool
	holdTimers map[int]*time.Timer
}

// ID returns the plugin instance id.
func (p *Plugin) ID() string { return p.id }

// Status reports the underlying broker connection state.
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
	return bridge.PluginStats{Discovered: len(p.devices) + len(p.groups), Active: len(p.subscribed)}
}

// Init resolves the broker and subscribes to <base>/bridge/devices and <base>/bridge/groups.
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

	gtopic := c.baseTopic + "/bridge/groups"
	gsub, err := b.Subscribe(gtopic, 0, func(_ string, payload []byte, _ bool) {
		p.onGroups(ctx, payload)
	})
	if err != nil {
		// groups subscription is optional — log but don't fail
		logging.Warn("zigbee2mqtt_groups_subscribe", logging.Fields{"topic": gtopic, "error": err.Error()})
	} else {
		p.groupsSub = gsub
	}

	logging.Info("zigbee2mqtt_plugin_started", logging.Fields{
		"id":     p.id,
		"broker": c.broker,
		"base":   c.baseTopic,
	})
	return nil
}

// Discover returns one entity per bridgeable endpoint of every known device and group.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]bridge.RemoteEntity, 0, len(p.devices)+len(p.groups))
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
	for _, dg := range p.groups {
		out = append(out, bridge.RemoteEntity{
			ID:   dg.grp.entityID(),
			Name: dg.grp.displayName(),
			Kind: dg.endpoint.Kind,
			Attributes: map[string]any{
				"friendly": dg.grp.FriendlyName,
				"group_id": dg.grp.ID,
				"members":  len(dg.grp.Members),
				"type":     "group",
			},
		})
	}
	return out, nil
}

// Subscribe records the mapping and (if device/group is known) installs state subs.
func (p *Plugin) Subscribe(_ context.Context, m bridge.Mapping) error {
	if isGroupEntityID(m.RemoteEntityID) {
		sub := &deviceSub{
			mapping:    m,
			isGroup:    true,
			groupID:    m.RemoteEntityID,
			held:       make(map[int]bool),
			holdTimers: make(map[int]*time.Timer),
		}
		p.mu.Lock()
		p.subscribed[m.DSUID] = sub
		dg := p.groups[m.RemoteEntityID]
		p.mu.Unlock()
		if dg != nil {
			p.activateGroup(sub, dg)
		} else {
			logging.Info("zigbee2mqtt_subscribe_group_pending", logging.Fields{
				"dsuid":    m.DSUID,
				"group_id": m.RemoteEntityID,
			})
		}
		return nil
	}

	ieee, epName := parseEntityID(m.RemoteEntityID)
	sub := &deviceSub{
		mapping:    m,
		ieee:       ieee,
		epName:     epName,
		held:       make(map[int]bool),
		holdTimers: make(map[int]*time.Timer),
	}
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
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("zigbee2mqtt: not subscribed: %s", m.DSUID)
	}

	// Groups: look up discoveredGroup.
	if sub.isGroup {
		p.mu.Lock()
		dg := p.groups[sub.groupID]
		p.mu.Unlock()
		if dg == nil {
			return fmt.Errorf("zigbee2mqtt: group %q not yet discovered", sub.groupID)
		}
		return p.applyToEndpoint(ctx, sub, dg.endpoint, dg.grp.setTopic(p.cfg.baseTopic), cmd)
	}

	// Devices: original path.
	p.mu.Lock()
	dev := p.devices[sub.ieee]
	p.mu.Unlock()
	if dev == nil {
		return fmt.Errorf("zigbee2mqtt: device %q not yet discovered", sub.ieee)
	}
	ep, ok := dev.endpoint(sub.epName)
	if !ok {
		return fmt.Errorf("zigbee2mqtt: endpoint %q not found on %s", sub.epName, sub.ieee)
	}
	return p.applyToEndpoint(ctx, sub, ep, dev.dev.setTopic(p.cfg.baseTopic), cmd)
}

// applyToEndpoint builds and publishes a set-payload for the given endpoint.
func (p *Plugin) applyToEndpoint(ctx context.Context, sub *deviceSub, ep endpoint, setTopic string, cmd bridge.Command) error {
	payload := map[string]any{}

	switch cmd.Type {
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
	return p.publish(ctx, setTopic, body)
}

// Close tears down all subscriptions.
func (p *Plugin) Close() error {
	if p.devicesSub != nil {
		_ = p.devicesSub.Close()
	}
	if p.groupsSub != nil {
		_ = p.groupsSub.Close()
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
		p.host.Log(bridge.LevelWarn, bridge.CodeEntityError, "failed to parse zigbee2mqtt devices payload",
			map[string]any{"error": err.Error()})
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
		eps := bd.endpointsWithOpts(endpointOpts{
			IncludeBattery:     p.cfg.includeBattery,
			IncludeLinkquality: p.cfg.includeLinkquality,
		})
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
		p.host.Log(bridge.LevelInfo, bridge.CodeEntityAdded, "discovered devices updated",
			map[string]any{"added": added, "total": len(seen)})
	}
	for _, sub := range pending {
		dev := p.devices[sub.ieee]
		if dev != nil {
			p.activate(sub, dev)
		}
	}
}

// onGroups handles the retained <base>/bridge/groups array.
func (p *Plugin) onGroups(_ context.Context, payload []byte) {
	if len(payload) == 0 {
		return
	}
	var arr []bridgeGroup
	if err := json.Unmarshal(payload, &arr); err != nil {
		logging.Warn("zigbee2mqtt_groups_parse", logging.Fields{"error": err.Error()})
		p.host.Log(bridge.LevelWarn, bridge.CodeEntityError, "failed to parse zigbee2mqtt groups payload",
			map[string]any{"error": err.Error()})
		return
	}
	seen := make(map[string]struct{}, len(arr))
	pending := []*deviceSub{}
	added := 0

	p.mu.Lock()
	for i := range arr {
		g := arr[i]
		eid := g.entityID()
		seen[eid] = struct{}{}
		if _, existed := p.groups[eid]; !existed {
			added++
		}
		ep := g.inferEndpoint(p.devices)
		p.groups[eid] = &discoveredGroup{grp: g, endpoint: ep}
		for _, sub := range p.subscribed {
			if sub.isGroup && sub.groupID == eid && len(sub.subs) == 0 {
				pending = append(pending, sub)
			}
		}
	}
	// Drop groups that disappeared from the list.
	for eid := range p.groups {
		if _, ok := seen[eid]; !ok {
			delete(p.groups, eid)
		}
	}
	p.mu.Unlock()

	if added > 0 {
		logging.Info("zigbee2mqtt_groups_updated", logging.Fields{
			"added": added, "total": len(seen),
		})
		p.host.Log(bridge.LevelInfo, bridge.CodeEntityAdded, "discovered groups updated",
			map[string]any{"added": added, "total": len(seen)})
	}
	for _, sub := range pending {
		p.mu.Lock()
		dg := p.groups[sub.groupID]
		p.mu.Unlock()
		if dg != nil {
			p.activateGroup(sub, dg)
		}
	}
}

// activate subscribes the per-mapping state and availability topics.
func (p *Plugin) activate(sub *deviceSub, dev *discoveredDevice) {
	base := p.cfg.baseTopic
	stateTopic := dev.dev.stateTopic(base)
	availTopic := dev.dev.availabilityTopic(base)

	subs := make([]mqttsvc.Subscription, 0, 2)

	if s, err := p.broker.Subscribe(stateTopic, 0, func(_ string, payload []byte, retained bool) {
		var msg stateMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			logging.Debug("zigbee2mqtt_state_unmarshal_error", logging.Fields{
				"topic": stateTopic, "error": err.Error(),
			})
			return
		}
		p.applyState(sub, dev, msg, retained)
	}); err == nil {
		subs = append(subs, s)
		logging.Debug("zigbee2mqtt_state_topic_subscribed", logging.Fields{
			"topic":    stateTopic,
			"dsuid":    sub.mapping.DSUID,
			"endpoint": sub.epName,
			"kind":     dev.endpoints[0].Kind,
		})
	} else {
		logging.Warn("zigbee2mqtt_state_subscribe", logging.Fields{
			"topic": stateTopic, "error": err.Error(),
		})
		p.host.Log(bridge.LevelWarn, bridge.CodeSubscribeFailed, "failed to subscribe to zigbee2mqtt state topic",
			map[string]any{"dsuid": sub.mapping.DSUID, "ieee": sub.ieee, "topic": stateTopic, "error": err.Error()})
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

	// Pre-populate sensor / binary-input descriptors for the sensor endpoint
	// (if this subscription is for it). Descriptors are pushed into the state
	// store now so the vDSM property tree can serve them before the first
	// retained state message arrives.
	if ep, ok := dev.endpoint(sub.epName); ok && (ep.Kind == "sensor" || ep.Kind == "binaryInput") {
		ctx := context.Background()
		for _, sf := range ep.SensorFeatures {
			_ = p.host.SetSensorDescriptor(ctx, sub.mapping.DSUID, sf.Index, bridge.SensorDescriptor{
				Type:       sf.Meta.Type,
				Usage:      sf.Meta.Usage,
				Name:       sf.Meta.Name,
				Min:        sf.Meta.Min,
				Max:        sf.Meta.Max,
				Resolution: sf.Meta.Resolution,
				SIUnit:     sf.Meta.SIUnit,
				Symbol:     sf.Meta.Symbol,
			})
		}
		for _, bf := range ep.BinaryFeatures {
			_ = p.host.SetBinaryInputDescriptor(ctx, sub.mapping.DSUID, bf.Index, bridge.BinaryInputDescriptor{
				Name:     bf.Property,
				Function: bf.Function,
			})
		}
		// Trigger a fresh Vanish+AnnounceDevice so vdcd/dSS re-queries all
		// device properties now that sensor descriptors are available.
		_ = p.host.ReAnnounce(ctx, sub.mapping.DSUID)
	}

	// Forward manufacturer/model/firmware metadata to digitalSTROM.
	if def := dev.dev.Definition; def != nil {
		_ = p.host.UpdateDeviceMeta(context.Background(), sub.mapping.DSUID,
			def.Vendor, def.Model, dev.dev.SoftwareBuildID)
	} else if dev.dev.SoftwareBuildID != "" {
		_ = p.host.UpdateDeviceMeta(context.Background(), sub.mapping.DSUID,
			"", "", dev.dev.SoftwareBuildID)
	}

	logging.Info("zigbee2mqtt_subscribed", logging.Fields{
		"dsuid":    sub.mapping.DSUID,
		"ieee":     sub.ieee,
		"endpoint": sub.epName,
		"topic":    stateTopic,
	})
	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "device subscribed",
		map[string]any{"dsuid": sub.mapping.DSUID, "ieee": sub.ieee, "endpoint": sub.epName})
}

// activateGroup subscribes to a z2m group's state topic.
// Groups have no availability topic in z2m (as of 1.x/2.x).
func (p *Plugin) activateGroup(sub *deviceSub, dg *discoveredGroup) {
	base := p.cfg.baseTopic
	stateTopic := dg.grp.stateTopic(base)

	s, err := p.broker.Subscribe(stateTopic, 0, func(_ string, payload []byte, retained bool) {
		var msg stateMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			logging.Debug("zigbee2mqtt_group_state_unmarshal_error", logging.Fields{
				"topic": stateTopic, "error": err.Error(),
			})
			return
		}
		// Re-fetch endpoint snapshot in case groups were updated in the meantime.
		p.mu.Lock()
		latest := p.groups[sub.groupID]
		p.mu.Unlock()
		if latest == nil {
			return
		}
		p.applyGroupState(sub, latest.endpoint, msg, retained)
	})
	if err != nil {
		logging.Warn("zigbee2mqtt_group_state_subscribe", logging.Fields{
			"topic": stateTopic, "error": err.Error(),
		})
		p.host.Log(bridge.LevelWarn, bridge.CodeSubscribeFailed, "failed to subscribe to zigbee2mqtt group state topic",
			map[string]any{"dsuid": sub.mapping.DSUID, "group_id": sub.groupID, "topic": stateTopic, "error": err.Error()})
		return
	}

	p.mu.Lock()
	sub.subs = []mqttsvc.Subscription{s}
	p.mu.Unlock()

	logging.Info("zigbee2mqtt_group_subscribed", logging.Fields{
		"dsuid":    sub.mapping.DSUID,
		"group_id": sub.groupID,
		"topic":    stateTopic,
	})
	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "group subscribed",
		map[string]any{"dsuid": sub.mapping.DSUID, "group_id": sub.groupID})
}

// applyGroupState updates the channel state for a group subscription.
func (p *Plugin) applyGroupState(sub *deviceSub, ep endpoint, msg stateMessage, _ bool) {
	ctx := context.Background()

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

// applyState forwards a state JSON to the host's channel/active state.
func (p *Plugin) applyState(sub *deviceSub, dev *discoveredDevice, msg stateMessage, retained bool) {
	ctx := context.Background()
	ep, ok := dev.endpoint(sub.epName)
	if !ok {
		return
	}

	// Button entities only react to the action property, never to channel
	// state. Retained messages re-fire the last action on plugin restart and
	// are intentionally ignored.
	if ep.Kind == "button" {
		if retained {
			return
		}
		action, ok := msg.stringField(ep.ActionProp)
		if !ok || action == "" {
			return
		}
		prefix, suffix := splitButtonAction(action)
		if suffix == "" {
			logging.Debug("zigbee2mqtt_button_action_unknown", logging.Fields{
				"dsuid":  sub.mapping.DSUID,
				"ieee":   sub.ieee,
				"action": action,
				"reason": "unrecognised verb (not in tip/click/press/double/triple/quadruple/long/hold/release vocabulary)",
			})
			return
		}
		if prefix != ep.ActionPrefix {
			logging.Debug("zigbee2mqtt_button_action_other_button", logging.Fields{
				"dsuid":      sub.mapping.DSUID,
				"ieee":       sub.ieee,
				"action":     action,
				"got_prefix": prefix,
				"my_prefix":  ep.ActionPrefix,
			})
			return
		}
		mapped := mapActionSuffix(suffix)
		if mapped == "" {
			logging.Debug("zigbee2mqtt_button_action_unmapped", logging.Fields{
				"dsuid":  sub.mapping.DSUID,
				"ieee":   sub.ieee,
				"action": action,
				"suffix": suffix,
			})
			return
		}
		logging.Info("zigbee2mqtt_button_action", logging.Fields{
			"dsuid":  sub.mapping.DSUID,
			"ieee":   sub.ieee,
			"action": action,
			"mapped": mapped,
		})
		p.dispatchButtonAction(ctx, sub, 0, mapped)
		return
	}

	// Sensor / binary-input entities: push numeric and boolean readings to the
	// host. Both sensor features (numeric exposes) and binary features (boolean
	// exposes) live in the same z2m state message and are dispatched here.
	if ep.Kind == "sensor" || ep.Kind == "binaryInput" {
		for _, sf := range ep.SensorFeatures {
			if v, ok := msg.numberField(sf.Property); ok {
				_ = p.host.UpdateSensor(ctx, sub.mapping.DSUID, sf.Index, v)
			}
		}
		for _, bf := range ep.BinaryFeatures {
			if v, ok := msg.boolField(bf.Property); ok {
				if bf.Invert {
					v = !v
				}
				inputVal := 0.0
				if v {
					inputVal = 1.0
				}
				_ = p.host.UpdateInput(ctx, sub.mapping.DSUID, bf.Index, inputVal)
			}
		}
		return
	}

	// Diagnostic: a non-button mapping that nevertheless carries an `action`
	// field usually means the device was bridged before the plugin learned to
	// expand action enums. Surface it so the user can re-discover/re-bridge.
	if action, ok := msg.stringField("action"); ok && action != "" {
		logging.Debug("zigbee2mqtt_action_on_non_button", logging.Fields{
			"dsuid":    sub.mapping.DSUID,
			"ieee":     sub.ieee,
			"endpoint": sub.epName,
			"kind":     ep.Kind,
			"action":   action,
			"hint":     "re-discover and re-bridge this device as a button",
		})
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

// ── config ────────────────────────────────────────────────────────────────────

type config struct {
	broker             string
	baseTopic          string
	includeBattery     bool
	includeLinkquality bool
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
	if v, ok := raw["includeBattery"].(bool); ok {
		c.includeBattery = v
	}
	if v, ok := raw["includeLinkquality"].(bool); ok {
		c.includeLinkquality = v
	}
	return c, nil
}

// dispatchButtonAction translates a logical action verb (tip/tip2/.../hold/release)
// into the right sequence of host calls so that dSS sees a coherent
// pressed → released gesture even when the upstream only sends discrete events.
//
// Behaviour:
//   - tip / tip2 / tip3 / tip4: synthetic 1 → action → 0 pulse.
//   - hold: latch value=1 and remember the held state. Arm a watchdog that
//     auto-releases after buttonHoldWatchdog if no real release arrives.
//   - release: only emit value=0 if we previously latched a hold. Cancels
//     the watchdog. Spurious releases are dropped.
func (p *Plugin) dispatchButtonAction(ctx context.Context, sub *deviceSub, index int, action string) {
	switch action {
	case "release":
		p.mu.Lock()
		held := sub.held[index]
		if held {
			delete(sub.held, index)
			if t := sub.holdTimers[index]; t != nil {
				t.Stop()
				delete(sub.holdTimers, index)
			}
		}
		p.mu.Unlock()
		if !held {
			return
		}
		_ = p.host.SetButtonAction(ctx, sub.mapping.DSUID, index, "release")
		_ = p.host.UpdateButton(ctx, sub.mapping.DSUID, index, 0)
		return

	case "hold":
		p.mu.Lock()
		// Cancel any prior watchdog and (re)arm. We always overwrite to
		// extend the deadline on repeated hold notifications.
		if t := sub.holdTimers[index]; t != nil {
			t.Stop()
		}
		sub.held[index] = true
		dsuid := sub.mapping.DSUID
		sub.holdTimers[index] = time.AfterFunc(buttonHoldWatchdog, func() {
			p.autoReleaseHold(sub, index, dsuid)
		})
		p.mu.Unlock()
		_ = p.host.UpdateButton(ctx, sub.mapping.DSUID, index, 1)
		_ = p.host.SetButtonAction(ctx, sub.mapping.DSUID, index, "hold")
		return

	default:
		// Discrete tip events: 1 → action → 0.
		_ = p.host.UpdateButton(ctx, sub.mapping.DSUID, index, 1)
		_ = p.host.SetButtonAction(ctx, sub.mapping.DSUID, index, action)
		_ = p.host.UpdateButton(ctx, sub.mapping.DSUID, index, 0)
	}
}

// autoReleaseHold fires when the watchdog expires without a real release.
// It clears the held state and pushes a synthetic release to dSS so that
// any in-progress dimming is stopped and the local controller observes the
// gesture's end.
func (p *Plugin) autoReleaseHold(sub *deviceSub, index int, dsuid string) {
	p.mu.Lock()
	if !sub.held[index] {
		p.mu.Unlock()
		return
	}
	delete(sub.held, index)
	delete(sub.holdTimers, index)
	p.mu.Unlock()
	logging.Warn("zigbee2mqtt_button_hold_watchdog", logging.Fields{
		"dsuid": dsuid,
		"index": index,
	})
	ctx := context.Background()
	_ = p.host.SetButtonAction(ctx, dsuid, index, "release")
	_ = p.host.UpdateButton(ctx, dsuid, index, 0)
}
