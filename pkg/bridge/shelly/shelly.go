// Package shelly implements a bridge.Plugin that discovers Shelly Gen2+
// devices via mDNS (_shelly._tcp) and integrates their components (relays,
// dimmers) as vDC devices.
//
// A single physical Shelly device can carry several bridgeable components
// (e.g. a relay plus, later, a power meter and inputs), so it fans out into
// one bridge.RemoteEntity per component rather than one entity per device —
// the same split Zigbee2MQTT uses for multi-function hardware. All
// components of one device share a single WebSocket connection.
//
// Live state updates arrive via each device's local RPC WebSocket (/rpc);
// commands are sent as HTTP POST JSON-RPC requests to the same endpoint.
package shelly

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/logging"
)

// PluginType is the registered Factory key for Shelly plugins.
const PluginType = "shelly"

// Factory returns a bridge.Factory that produces Shelly plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{
			id:         id,
			subscribed: make(map[string]*deviceSub),
			clients:    make(map[string]*sharedClient),
		}
	}
}

// deviceSub is the per-mapping runtime state for a subscribed entity.
type deviceSub struct {
	mapping  bridge.Mapping
	deviceID string
	// identity is this entity's identifying component, parsed from
	// mapping.RemoteEntityID — used to find spec in the device's Entities
	// once known.
	identity component
	// spec is resolved once activate() finds a matching entitySpec for
	// identity; zero value until then.
	spec entitySpec
	// activated is true once this subscription has an active shared client
	// wired up and spec resolved. False while waiting for the device to be
	// seen via mDNS, or after its client was torn down for an address
	// change and is awaiting re-activation.
	activated bool
}

// sharedClient is a deviceClient plus a reference count of the
// subscriptions currently using it — several bridged components on the same
// physical device share one connection.
type sharedClient struct {
	client *deviceClient
	refs   int
}

// Plugin is the bridge.Plugin implementation for Shelly.
type Plugin struct {
	id      string
	host    bridge.Host
	scanner *scanner

	statusVal atomic.Value // string

	// ctx/cancel are derived from the ctx passed to Init, once, and never
	// touched again after that — safe to read from any method without a
	// lock, since the Registry only ever calls Subscribe/etc. after Init has
	// returned. Used to scope every background goroutine this plugin starts
	// (the scanner and every shared device client) to this plugin instance
	// specifically, rather than the Registry's shared, only-cancelled-at
	// process-shutdown runtime ctx — so Close() can actually stop them.
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.RWMutex
	// subscribed maps DSUID -> subscription.
	subscribed map[string]*deviceSub
	// clients maps device id -> shared client, one per physical device with
	// at least one active subscription.
	clients map[string]*sharedClient
}

// ID returns the configured instance id.
func (p *Plugin) ID() string { return p.id }

// Status returns the plugin connectivity state, reflecting whether the mDNS
// scanner has actually managed to start a browse cycle rather than simply
// whether Init has run.
func (p *Plugin) Status() string {
	if v := p.statusVal.Load(); v != nil {
		return v.(string)
	}
	return "not_initialized"
}

// Stats reports the discovered (bridgeable entities across every seen
// device) and active (subscribed) counts.
func (p *Plugin) Stats() bridge.PluginStats {
	discovered := 0
	for _, d := range p.scanner.All() {
		discovered += len(d.Entities)
	}
	p.mu.RLock()
	active := len(p.subscribed)
	p.mu.RUnlock()
	return bridge.PluginStats{Discovered: discovered, Active: active}
}

// Init starts the mDNS scanner and wires callbacks.
func (p *Plugin) Init(ctx context.Context, _ map[string]any, host bridge.Host) error {
	p.host = host
	p.statusVal.Store("starting")
	runCtx, cancel := context.WithCancel(ctx)
	p.ctx = runCtx
	p.cancel = cancel
	p.scanner = newScanner(
		p.onDeviceFound,
		func() { p.statusVal.Store("connected") },
		func(err error) {
			p.statusVal.Store("degraded")
			host.Log(bridge.LevelWarn, bridge.CodeEntityError, "Shelly discovery error",
				map[string]any{"error": err.Error()})
		},
	)
	go p.scanner.Run(runCtx)
	logging.Info("shelly_plugin_started", logging.Fields{"id": p.id})
	return nil
}

// Discover returns all bridgeable entities of every Shelly device currently
// visible on the network.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	devs := p.scanner.All()
	out := make([]bridge.RemoteEntity, 0, len(devs))
	for _, d := range devs {
		multi := len(d.Entities) > 1
		for _, e := range d.Entities {
			name := displayName(d)
			if multi {
				name = fmt.Sprintf("%s · %s", name, entitySuffix(e))
			}
			out = append(out, bridge.RemoteEntity{
				ID:   entityID(d.ID, e.Component),
				Name: name,
				Kind: e.Kind,
				Attributes: map[string]any{
					"device_id": d.ID,
					"addr":      d.Addr,
					"model":     d.Model,
					"gen":       d.Gen,
					"fw":        d.FW,
					"component": e.Component.key(),
				},
			})
		}
	}
	return out, nil
}

// entitySuffix names the part of a multi-entity device's display name that
// distinguishes it from its siblings.
func entitySuffix(e entitySpec) string {
	switch e.Kind {
	case "sensor", "binary":
		return "sensors"
	case "button":
		return fmt.Sprintf("button %d", e.Component.Index)
	default:
		return e.Component.key()
	}
}

func displayName(d discoveredDevice) string {
	if d.Name != "" {
		return d.Name
	}
	if d.Model != "" {
		return d.Model
	}
	return d.ID
}

// Subscribe registers a mapping and activates it immediately if the device
// is already known, or leaves it pending until onDeviceFound sees it.
func (p *Plugin) Subscribe(_ context.Context, m bridge.Mapping) error {
	devID, c, ok := parseEntityID(m.RemoteEntityID)
	if !ok {
		return fmt.Errorf("shelly: invalid remote entity id %q", m.RemoteEntityID)
	}
	sub := &deviceSub{mapping: m, deviceID: devID, identity: c}

	p.mu.Lock()
	p.subscribed[m.DSUID] = sub
	p.mu.Unlock()

	dev, known := p.scanner.Get(devID)
	if !known {
		logging.Info("shelly_subscribe_pending", logging.Fields{"dsuid": m.DSUID, "device_id": devID})
		p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "subscription registered, waiting for device",
			map[string]any{"dsuid": m.DSUID, "device_id": devID})
		return nil
	}
	p.activate(sub, dev)
	return nil
}

// Unsubscribe stops forwarding state for the given device and releases the
// shared client for its device once no subscription references it anymore.
func (p *Plugin) Unsubscribe(_ context.Context, dsuid string) error {
	p.mu.Lock()
	sub, ok := p.subscribed[dsuid]
	if !ok {
		p.mu.Unlock()
		return nil
	}
	delete(p.subscribed, dsuid)

	var toStop *deviceClient
	if sc, exists := p.clients[sub.deviceID]; exists {
		sc.refs--
		if sc.refs <= 0 {
			toStop = sc.client
			delete(p.clients, sub.deviceID)
		}
	}
	p.mu.Unlock()

	if toStop != nil {
		toStop.stop()
	}
	return nil
}

// Apply translates a vDC command into a Shelly RPC call.
func (p *Plugin) Apply(ctx context.Context, m bridge.Mapping, cmd bridge.Command) error {
	p.mu.RLock()
	sub, ok := p.subscribed[m.DSUID]
	var client *deviceClient
	if ok {
		if sc, exists := p.clients[sub.deviceID]; exists {
			client = sc.client
		}
	}
	p.mu.RUnlock()

	if !ok || client == nil {
		return fmt.Errorf("shelly: device %q not connected", m.RemoteEntityID)
	}

	if cmd.Type != "setChannel" || cmd.Channel != 0 {
		return nil // only channel 0 (on/off, brightness) is exposed so far
	}

	switch sub.identity.Kind {
	case "switch":
		return client.setSwitch(ctx, sub.identity.Index, cmd.Value > 0)
	case "light":
		if cmd.Value <= 0 {
			return client.setLight(ctx, sub.identity.Index, false, nil)
		}
		bri := clampF(cmd.Value, 0, 100)
		return client.setLight(ctx, sub.identity.Index, true, &bri)
	}
	return nil
}

// Close shuts down every shared device client (the scanner stops when ctx is
// cancelled).
func (p *Plugin) Close() error {
	if p.cancel != nil {
		p.cancel()
	}

	p.mu.Lock()
	clients := make([]*deviceClient, 0, len(p.clients))
	for _, sc := range p.clients {
		clients = append(clients, sc.client)
	}
	p.clients = make(map[string]*sharedClient)
	p.subscribed = make(map[string]*deviceSub)
	p.mu.Unlock()

	for _, c := range clients {
		c.stop()
	}
	return nil
}

// ── internal helpers ────────────────────────────────────────────────────────

// onDeviceFound is called by the scanner when a new or changed Shelly device
// appears. It activates any subscriptions pending on that device, and
// restarts the shared client (re-activating everything on it) if the
// device's address changed, e.g. a new DHCP lease.
func (p *Plugin) onDeviceFound(dev discoveredDevice) {
	p.host.Log(bridge.LevelInfo, bridge.CodeEntityAdded, "Shelly device found via mDNS",
		map[string]any{"id": dev.ID, "model": dev.Model, "addr": dev.Addr, "gen": dev.Gen})
	p.host.NotifyDiscoveryChanged()

	p.mu.Lock()
	var staleClient *deviceClient
	if sc, exists := p.clients[dev.ID]; exists && sc.client.addr != dev.Addr {
		staleClient = sc.client
		delete(p.clients, dev.ID)
		for _, sub := range p.subscribed {
			if sub.deviceID == dev.ID {
				sub.activated = false
			}
		}
	}
	var pending []*deviceSub
	for _, sub := range p.subscribed {
		if sub.deviceID == dev.ID && !sub.activated {
			pending = append(pending, sub)
		}
	}
	p.mu.Unlock()

	if staleClient != nil {
		// Stop outside the lock — the client's callbacks re-enter plugin code.
		staleClient.stop()
	}

	for _, sub := range pending {
		p.activate(sub, dev)
	}
}

// activate wires a subscription to the (possibly newly created) shared
// client for its device, resolves its entitySpec, pushes sensor/binary
// descriptors if it needs them, then pushes whatever state is already
// cached so the entity reflects reality immediately rather than waiting for
// the next push.
func (p *Plugin) activate(sub *deviceSub, dev discoveredDevice) {
	p.mu.Lock()
	if sub.activated {
		p.mu.Unlock()
		return
	}
	spec, found := entityForComponent(dev.Entities, sub.identity)
	if !found {
		p.mu.Unlock()
		logging.Warn("shelly_activate_no_entity", logging.Fields{
			"dsuid": sub.mapping.DSUID, "device_id": dev.ID, "component": sub.identity.key(),
		})
		return
	}
	sub.spec = spec
	sub.activated = true
	sc, exists := p.clients[dev.ID]
	if !exists {
		c := newDeviceClient(dev.Addr, dev.ID, "vdcgo-"+p.id)
		c.onStatus = func(status map[string]map[string]any) { p.handleStatus(dev.ID, status) }
		c.onConn = func(s string) { p.handleConnState(dev.ID, s) }
		c.onEvent = func(events []shellyEvent) { p.handleEvents(dev.ID, events) }
		sc = &sharedClient{client: c}
		p.clients[dev.ID] = sc
	}
	sc.refs++
	p.mu.Unlock()

	if !exists {
		sc.client.start(p.ctx)
	}

	if spec.Kind == "sensor" || spec.Kind == "binary" {
		p.pushDescriptors(sub)
	}
	p.applySpec(sub, sc.client.status.snapshot())

	logging.Info("shelly_subscribed", logging.Fields{
		"dsuid": sub.mapping.DSUID, "device_id": dev.ID, "component": sub.identity.key(),
	})
	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "device subscribed",
		map[string]any{"dsuid": sub.mapping.DSUID, "device_id": dev.ID, "component": sub.identity.key()})
}

// pushDescriptors publishes SetSensorDescriptor/SetBinaryInputDescriptor for
// a sensor/binary entity's features, then triggers a ReAnnounce so the vDSM
// re-queries the device now that descriptors are available — mirroring
// Zigbee2MQTT's activate() (pkg/bridge/zigbee2mqtt/zigbee2mqtt.go).
func (p *Plugin) pushDescriptors(sub *deviceSub) {
	ctx := context.Background()
	for _, sf := range sub.spec.SensorFeatures {
		if err := p.host.SetSensorDescriptor(ctx, sub.mapping.DSUID, sf.Index, bridge.SensorDescriptor{
			Type: sf.Meta.Type, Usage: sf.Meta.Usage, Name: sf.Meta.Name,
			Min: sf.Meta.Min, Max: sf.Meta.Max, Resolution: sf.Meta.Resolution,
			SIUnit: sf.Meta.SIUnit, Symbol: sf.Meta.Symbol,
		}); err != nil {
			logging.Warn("shelly_set_sensor_descriptor_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
		}
	}
	for _, bf := range sub.spec.BinaryFeatures {
		// Shelly's input:N carries no semantic meaning of its own (it's a dry
		// contact wired to whatever the installer connected) — Function 0
		// ("none") is the only honest default; Zigbee2MQTT can do better here
		// because its exposes name the actual sensor type (occupancy, contact, ...).
		if err := p.host.SetBinaryInputDescriptor(ctx, sub.mapping.DSUID, bf.Index, bridge.BinaryInputDescriptor{
			Name: fmt.Sprintf("input %d", bf.Source.Index), Function: 0,
		}); err != nil {
			logging.Warn("shelly_set_binary_descriptor_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
		}
	}
	if err := p.host.ReAnnounce(ctx, sub.mapping.DSUID); err != nil {
		logging.Warn("shelly_reannounce_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
	}
}

// handleStatus is called by a shared client whenever its device's status
// changes; it fans the update out to every activated subscription on that
// device.
func (p *Plugin) handleStatus(deviceID string, status map[string]map[string]any) {
	p.mu.RLock()
	var subs []*deviceSub
	for _, sub := range p.subscribed {
		if sub.deviceID == deviceID && sub.activated {
			subs = append(subs, sub)
		}
	}
	p.mu.RUnlock()

	for _, sub := range subs {
		p.applySpec(sub, status)
	}
}

// applySpec pushes an activated subscription's current values to the host,
// based on its entitySpec's kind. Button entities have no status fields —
// they're driven by NotifyEvent via handleEvents instead — so this is a
// no-op for them (and, notably, doesn't call UpdateActive for them either;
// device online/offline already covers that uniformly via handleConnState).
func (p *Plugin) applySpec(sub *deviceSub, status map[string]map[string]any) {
	ctx := context.Background()
	switch sub.spec.Kind {
	case "light":
		fields := status[sub.identity.key()]
		if v, ok := switchChannelValue(fields); ok {
			if err := p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, v); err != nil {
				logging.Warn("shelly_update_channel_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
			}
		}
	case "dimmer":
		fields := status[sub.identity.key()]
		if v, ok := lightChannelValue(fields); ok {
			if err := p.host.UpdateChannel(ctx, sub.mapping.DSUID, 0, v); err != nil {
				logging.Warn("shelly_update_channel_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
			}
		}
	case "sensor", "binary":
		for _, sf := range sub.spec.SensorFeatures {
			v, ok := numberFieldDotted(status[sf.Source.key()], sf.Field)
			if !ok {
				continue
			}
			if sf.Field == "aenergy.total" {
				v /= 1000 // Wh -> kWh
			}
			if err := p.host.UpdateSensor(ctx, sub.mapping.DSUID, sf.Index, v); err != nil {
				logging.Warn("shelly_update_sensor_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
			}
		}
		for _, bf := range sub.spec.BinaryFeatures {
			v, ok := boolField(status[bf.Source.key()], bf.Field)
			if !ok {
				continue
			}
			iv := 0.0
			if v {
				iv = 1.0
			}
			if err := p.host.UpdateInput(ctx, sub.mapping.DSUID, bf.Index, iv); err != nil {
				logging.Warn("shelly_update_input_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
			}
		}
	case "button":
		return
	default:
		return
	}
	if err := p.host.UpdateActive(ctx, sub.mapping.DSUID, true); err != nil {
		logging.Warn("shelly_update_active_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
	}
}

// handleEvents is called by a shared client for every NotifyEvent frame; it
// dispatches input button pushes to whichever button subscription owns the
// event's component.
func (p *Plugin) handleEvents(deviceID string, events []shellyEvent) {
	p.mu.RLock()
	var subs []*deviceSub
	for _, sub := range p.subscribed {
		if sub.deviceID == deviceID && sub.activated && sub.spec.Kind == "button" {
			subs = append(subs, sub)
		}
	}
	p.mu.RUnlock()

	for _, ev := range events {
		action := mapInputEvent(ev.Event)
		if action == "" {
			continue
		}
		for _, sub := range subs {
			if sub.identity.key() == ev.Component {
				p.dispatchButtonAction(sub, action)
			}
		}
	}
}

// mapInputEvent maps a Shelly Gen2+ input event name to a dS button click
// type (Host.SetButtonAction's vocabulary: "tip", "tip2", "tip3", "tip4",
// "hold"). Unlike Zigbee2MQTT, Shelly reports "long_push" as a single
// discrete event rather than a hold/release pair, so there is no separate
// release to track.
func mapInputEvent(event string) string {
	switch event {
	case "single_push":
		return "tip"
	case "double_push":
		return "tip2"
	case "triple_push":
		return "tip3"
	case "long_push":
		return "hold"
	default:
		return ""
	}
}

// dispatchButtonAction pulses UpdateButton 1->0 around a SetButtonAction
// call, the same discrete-event pattern Zigbee2MQTT uses for its own tip
// events (pkg/bridge/zigbee2mqtt/zigbee2mqtt.go's dispatchButtonAction).
func (p *Plugin) dispatchButtonAction(sub *deviceSub, action string) {
	ctx := context.Background()
	if err := p.host.UpdateButton(ctx, sub.mapping.DSUID, 0, 1); err != nil {
		logging.Warn("shelly_update_button_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
	}
	if err := p.host.SetButtonAction(ctx, sub.mapping.DSUID, 0, action); err != nil {
		logging.Warn("shelly_set_button_action_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
	}
	if err := p.host.UpdateButton(ctx, sub.mapping.DSUID, 0, 0); err != nil {
		logging.Warn("shelly_update_button_error", logging.Fields{"dsuid": sub.mapping.DSUID, "error": err.Error()})
	}
}

// handleConnState is called by a shared client whenever its WebSocket
// connection state changes; it marks every subscription on that device
// active/inactive accordingly.
func (p *Plugin) handleConnState(deviceID, state string) {
	p.mu.RLock()
	var dsuids []string
	for dsuid, sub := range p.subscribed {
		if sub.deviceID == deviceID {
			dsuids = append(dsuids, dsuid)
		}
	}
	p.mu.RUnlock()

	active := state == "connected"
	for _, dsuid := range dsuids {
		if err := p.host.UpdateActive(context.Background(), dsuid, active); err != nil {
			logging.Warn("shelly_update_active_error", logging.Fields{"dsuid": dsuid, "error": err.Error()})
		}
	}
	switch state {
	case "connected":
		p.host.Log(bridge.LevelInfo, bridge.CodeConnectOK, "Shelly device connected", map[string]any{"device_id": deviceID})
	case "reconnecting":
		p.host.Log(bridge.LevelWarn, bridge.CodeConnectFailed, "Shelly device disconnected", map[string]any{"device_id": deviceID})
	}
}
