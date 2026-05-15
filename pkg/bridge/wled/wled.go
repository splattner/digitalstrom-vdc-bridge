// Package wled implements a bridge.Plugin that discovers WLED LED controllers
// via mDNS (_wled._tcp) and integrates them as vDC light devices.
//
// Each WLED device is exposed as a single colorlight (or dimmer/light,
// depending on hardware capabilities). Live state updates arrive via the WLED
// WebSocket (/ws); commands are sent as HTTP POST to /json/state.
package wled

import (
	"context"
	"fmt"
	"sync"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/logging"
)

// PluginType is the registered Factory key for WLED plugins.
const PluginType = "wled"

// Factory returns a bridge.Factory that produces WLED plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{
			id:         id,
			subscribed: make(map[string]subscription),
			byMAC:      make(map[string]string),
		}
	}
}

// subscription tracks an active bridge mapping and its per-device WS client.
type subscription struct {
	mapping bridge.Mapping
	client  *deviceClient
	// cached color state for combining hue+sat into a single RGB write
	colorState colorChannels
}

// colorChannels caches the most recent per-channel HSV values so that
// single-axis color updates can be recombined with the unchanged axis.
type colorChannels struct {
	hue, sat float64
	hueSet   bool
	satSet   bool
}

// Plugin is the bridge.Plugin implementation for WLED.
type Plugin struct {
	id      string
	host    bridge.Host
	scanner *scanner

	mu sync.RWMutex
	// subscribed maps DSUID → subscription (active forward)
	subscribed map[string]subscription
	// byMAC maps mac → DSUID for fast dispatch from scanner/client callbacks
	byMAC map[string]string
}

// ID returns the configured instance id.
func (p *Plugin) ID() string { return p.id }

// Status returns a short status string.
func (p *Plugin) Status() string {
	p.mu.RLock()
	n := len(p.subscribed)
	p.mu.RUnlock()
	return fmt.Sprintf("tracking %d device(s)", n)
}

// Init starts the mDNS scanner and wires callbacks.
func (p *Plugin) Init(ctx context.Context, _ map[string]any, host bridge.Host) error {
	p.host = host
	p.scanner = newScanner(
		func(dev discoveredDevice) { p.onDeviceFound(ctx, dev) },
		nil,
		func(err error) {
			host.Log(bridge.LevelWarn, bridge.CodeEntityError, "WLED mDNS browse error",
				map[string]any{"error": err.Error()})
		},
	)
	go p.scanner.Run(ctx)
	logging.Info("wled_plugin_started", logging.Fields{"id": p.id})
	return nil
}

// Discover returns all WLED devices currently visible on the network.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	devs := p.scanner.All()
	out := make([]bridge.RemoteEntity, 0, len(devs))
	for _, d := range devs {
		out = append(out, bridge.RemoteEntity{
			ID:   d.MAC,
			Name: d.Name,
			Kind: "colorlight", // overridden after /json/info; good default
			Attributes: map[string]any{
				"mac":  d.MAC,
				"addr": d.Addr,
				"ver":  d.Ver,
			},
		})
	}
	return out, nil
}

// Subscribe registers a mapping and starts the per-device WS client.
func (p *Plugin) Subscribe(ctx context.Context, m bridge.Mapping) error {
	mac := m.RemoteEntityID

	dev, ok := p.scanner.Get(mac)
	if !ok {
		// Device not yet seen via mDNS. Register the subscription anyway; it
		// will be activated when the scanner finds the device later.
		p.mu.Lock()
		p.subscribed[m.DSUID] = subscription{mapping: m}
		p.byMAC[mac] = m.DSUID
		p.mu.Unlock()
		logging.Info("wled_subscribe_pending", logging.Fields{"dsuid": m.DSUID, "mac": mac})
		p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "subscription registered, waiting for device",
			map[string]any{"dsuid": m.DSUID, "mac": mac})
		return nil
	}
	return p.startDeviceClient(ctx, m, dev)
}

// Unsubscribe stops the per-device WS client and removes the mapping.
func (p *Plugin) Unsubscribe(_ context.Context, dsuid string) error {
	p.mu.Lock()
	sub, ok := p.subscribed[dsuid]
	if ok {
		delete(p.byMAC, sub.mapping.RemoteEntityID)
		delete(p.subscribed, dsuid)
	}
	p.mu.Unlock()

	if ok && sub.client != nil {
		sub.client.stop()
	}
	return nil
}

// Apply translates a vDC command to a WLED HTTP state update.
func (p *Plugin) Apply(ctx context.Context, m bridge.Mapping, cmd bridge.Command) error {
	p.mu.RLock()
	sub, ok := p.subscribed[m.DSUID]
	p.mu.RUnlock()

	if !ok || sub.client == nil {
		return fmt.Errorf("wled: device %q not connected", m.RemoteEntityID)
	}

	payload, err := p.buildPayload(m, cmd)
	if err != nil || payload == nil {
		return err
	}
	return sub.client.postState(ctx, payload)
}

// Close shuts down all per-device clients (scanner stops when ctx is cancelled).
func (p *Plugin) Close() error {
	p.mu.Lock()
	subs := make([]subscription, 0, len(p.subscribed))
	for _, sub := range p.subscribed {
		subs = append(subs, sub)
	}
	p.mu.Unlock()

	for _, sub := range subs {
		if sub.client != nil {
			sub.client.stop()
		}
	}
	return nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// onDeviceFound is called by the scanner when a new or updated WLED device appears.
func (p *Plugin) onDeviceFound(ctx context.Context, dev discoveredDevice) {
	p.host.Log(bridge.LevelInfo, bridge.CodeEntityAdded, "WLED device found via mDNS",
		map[string]any{"mac": dev.MAC, "name": dev.Name, "addr": dev.Addr, "ver": dev.Ver})

	p.mu.Lock()
	dsuid, hasSub := p.byMAC[dev.MAC]
	var sub subscription
	if hasSub {
		sub = p.subscribed[dsuid]
	}
	p.mu.Unlock()

	if !hasSub {
		return // not subscribed yet — nothing to activate
	}

	// If the device address changed (e.g. new DHCP lease) we restart the client.
	if sub.client != nil {
		if sub.client.addr == dev.Addr {
			return // already connected to the right address
		}
		sub.client.stop()
	}

	if err := p.startDeviceClient(ctx, sub.mapping, dev); err != nil {
		logging.Warn("wled_start_client_error", logging.Fields{
			"mac":   dev.MAC,
			"addr":  dev.Addr,
			"error": err.Error(),
		})
	}
}

// startDeviceClient creates and starts a deviceClient for the given mapping+device.
func (p *Plugin) startDeviceClient(ctx context.Context, m bridge.Mapping, dev discoveredDevice) error {
	c := newDeviceClient(dev.Addr, dev.MAC)

	c.onState = func(si wledSI) {
		p.handleState(m.DSUID, m, si)
	}
	c.onStatus = func(s string) {
		active := s == "connected"
		if err := p.host.UpdateActive(context.Background(), m.DSUID, active); err != nil {
			logging.Warn("wled_update_active_error", logging.Fields{
				"dsuid": m.DSUID,
				"error": err.Error(),
			})
		}
		if s == "connected" {
			p.host.Log(bridge.LevelInfo, bridge.CodeConnectOK, "WLED device connected",
				map[string]any{"dsuid": m.DSUID, "mac": dev.MAC, "addr": dev.Addr})
		} else if s == "disconnected" {
			p.host.Log(bridge.LevelWarn, bridge.CodeConnectFailed, "WLED device disconnected",
				map[string]any{"dsuid": m.DSUID, "mac": dev.MAC, "addr": dev.Addr})
		}
	}

	p.mu.Lock()
	sub := p.subscribed[m.DSUID]
	sub.client = c
	p.subscribed[m.DSUID] = sub
	p.byMAC[dev.MAC] = m.DSUID
	p.mu.Unlock()

	c.start(ctx)

	// Fetch an initial state synchronously so the device appears in the UI
	// as soon as possible (the WS first-message also does this, but may lag).
	go func() {
		if err := c.pollOnce(ctx); err != nil {
			logging.Warn("wled_initial_poll_error", logging.Fields{
				"mac":   dev.MAC,
				"error": err.Error(),
			})
		}
	}()

	logging.Info("wled_client_started", logging.Fields{
		"dsuid": m.DSUID,
		"mac":   dev.MAC,
		"addr":  dev.Addr,
	})
	p.host.Log(bridge.LevelInfo, bridge.CodeSubscribeOK, "device subscribed",
		map[string]any{"dsuid": m.DSUID, "mac": dev.MAC, "addr": dev.Addr})
	return nil
}

// handleState is called whenever the device client receives a state update.
func (p *Plugin) handleState(dsuid string, m bridge.Mapping, si wledSI) {
	ctx := context.Background()
	state := si.State
	info := si.Info

	// If info carries a name and it differs from mapping, we could update it —
	// for now just use it to refine the kind after first real info response.
	lc := info.Leds.LC
	if lc == 0 {
		lc = lcBitRGB // assume RGB if info is absent (e.g. partial WS frame)
	}

	// Channel 0: brightness
	bri := briToVDC(state.Bri, state.On)
	if err := p.host.UpdateChannel(ctx, dsuid, 0, bri); err != nil {
		logging.Warn("wled_update_ch0", logging.Fields{"dsuid": dsuid, "error": err.Error()})
	}

	if lc&lcBitRGB != 0 {
		// Colorlight: extract hue + sat from primary segment colour.
		r, g, b := primaryRGB(state)
		hue, sat := rgbToHueSat(r, g, b)
		if err := p.host.UpdateChannel(ctx, dsuid, 1, hue); err != nil {
			logging.Warn("wled_update_ch1", logging.Fields{"dsuid": dsuid, "error": err.Error()})
		}
		if err := p.host.UpdateChannel(ctx, dsuid, 2, sat); err != nil {
			logging.Warn("wled_update_ch2", logging.Fields{"dsuid": dsuid, "error": err.Error()})
		}
	}

	if lc&lcBitCCT != 0 && len(state.Seg) > 0 {
		mired := cctToMired(state.Seg[0].CCT)
		if err := p.host.UpdateChannel(ctx, dsuid, 3, mired); err != nil {
			logging.Warn("wled_update_ch3", logging.Fields{"dsuid": dsuid, "error": err.Error()})
		}
	}

	// Mark device active whenever we receive a valid state push.
	if err := p.host.UpdateActive(ctx, dsuid, true); err != nil {
		logging.Warn("wled_update_active", logging.Fields{"dsuid": dsuid, "error": err.Error()})
	}

	_ = m // retained for future use (e.g. name sync)
}

// buildPayload converts a vDC Command into a WLED JSON state payload.
// Returns (nil, nil) for commands that require no action (e.g. read-only).
func (p *Plugin) buildPayload(m bridge.Mapping, cmd bridge.Command) (map[string]any, error) {
	switch cmd.Type {
	case "setActive":
		return map[string]any{"on": cmd.Active}, nil

	case "setChannel":
		switch cmd.Channel {
		case 0: // brightness
			if cmd.Value <= 0 {
				return map[string]any{"on": false}, nil
			}
			return map[string]any{
				"on":  true,
				"bri": clampI(vdcToBri(cmd.Value), 1, 255),
			}, nil

		case 1, 2: // hue or saturation — combine with cached counterpart
			p.mu.Lock()
			sub := p.subscribed[m.DSUID]
			cs := sub.colorState
			if cmd.Channel == 1 {
				cs.hue = clampF(cmd.Value, 0, 360)
				cs.hueSet = true
			} else {
				cs.sat = clampF(cmd.Value, 0, 100)
				cs.satSet = true
			}
			sub.colorState = cs
			p.subscribed[m.DSUID] = sub
			hue, sat := cs.hue, cs.sat
			p.mu.Unlock()

			r, g, b := hueSatToRGB(hue, sat)
			return map[string]any{
				"on": true,
				"seg": []any{map[string]any{
					"col": []any{[]int{r, g, b}},
				}},
			}, nil

		case 3: // color temperature in mired
			cct := miredToCCT(cmd.Value)
			return map[string]any{
				"on": true,
				"seg": []any{map[string]any{
					"cct": cct,
				}},
			}, nil
		}

	case "callScene":
		if cmd.Scene == 0 {
			return map[string]any{"on": false}, nil
		}
		return map[string]any{"on": true}, nil
	}

	return nil, nil
}
