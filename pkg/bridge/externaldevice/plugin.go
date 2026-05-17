// Package externaldevice implements a bridge plugin that hosts the vdcd
// external device API (TCP JSON/simple-text protocol).  External scripts and
// programs connect to the plugin's TCP listener, declare their devices via an
// "init" message, and then exchange channel / sensor / button / input events
// using the standard vdcd external device wire protocol.
//
// Each connected external device is exposed as a RemoteEntity whose ID equals
// the device's "uniqueid" from the init message.  Bridge mappings are created
// by the user in the web UI in the same way as for any other plugin type.
// Once a mapping exists, the plugin announces the device to the vDC state
// store via the Host interface and forwards all subsequent events.
package externaldevice

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/runtime"
	"github.com/splattner/vdcgo/pkg/server"
)

// PluginType is the type identifier used when registering the factory.
const PluginType = "externaldevice"

// Plugin implements bridge.Plugin for the external device API.
type Plugin struct {
	id   string
	mu   sync.RWMutex
	host bridge.Host

	srv    *server.Server
	cancel context.CancelFunc

	// devices holds all currently connected external devices by uniqueID.
	devices map[string]bridge.RemoteEntity

	// subs holds active subscriptions: uniqueID → Mapping.
	// Populated by Subscribe, cleared by Unsubscribe.
	subs map[string]bridge.Mapping

	// dsuidToUID is the reverse index of subs: DSUID → uniqueID.
	// Used by Unsubscribe which only receives the DSUID.
	dsuidToUID map[string]string

	listenAddr string
}

// Factory returns a bridge.Factory that creates Plugin instances.
func Factory() bridge.Factory {
	return func(id string) bridge.Plugin {
		return &Plugin{
			id:         id,
			devices:    make(map[string]bridge.RemoteEntity),
			subs:       make(map[string]bridge.Mapping),
			dsuidToUID: make(map[string]string),
		}
	}
}

// ID implements bridge.Plugin.
func (p *Plugin) ID() string { return p.id }

// Init implements bridge.Plugin.
//
// Supported config keys:
//   - "listen"   string  TCP port or address to listen on (default "8999").
//   - "nonlocal" bool    Accept connections from non-loopback addresses (default false).
func (p *Plugin) Init(ctx context.Context, cfg map[string]any, host bridge.Host) error {
	p.host = host

	listen, _ := cfg["listen"].(string)
	if strings.TrimSpace(listen) == "" {
		listen = "8999"
	}
	nonLocal, _ := cfg["nonlocal"].(bool)

	srv, err := server.New(server.Config{
		Listen:   listen,
		NonLocal: nonLocal,
		OnEvent:  p.onEvent,
	})
	if err != nil {
		return fmt.Errorf("externaldevice: create server: %w", err)
	}
	p.srv = srv

	p.mu.Lock()
	p.listenAddr = listen
	p.mu.Unlock()

	srvCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	go func() {
		if err := srv.Run(srvCtx); err != nil {
			host.Log(bridge.LevelError, "server_error",
				"external device server stopped unexpectedly",
				map[string]any{"error": err.Error()})
		}
	}()

	host.Log(bridge.LevelInfo, "server_started",
		"external device server listening",
		map[string]any{"listen": listen, "nonlocal": nonLocal})
	return nil
}

// Status implements bridge.Plugin.
func (p *Plugin) Status() string {
	p.mu.RLock()
	addr := p.listenAddr
	p.mu.RUnlock()
	if addr == "" {
		return "idle"
	}
	return "connected"
}

// Discover implements bridge.Plugin.
// Returns all external devices that have sent an "init" message and are
// still connected.  The bridge.Registry caller filters out entities that
// already have a Mapping.
func (p *Plugin) Discover(_ context.Context) ([]bridge.RemoteEntity, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]bridge.RemoteEntity, 0, len(p.devices))
	for _, e := range p.devices {
		out = append(out, e)
	}
	return out, nil
}

// Subscribe implements bridge.Plugin.
// Records the mapping so that future (and already-connected) device events
// are forwarded through the Host interface.
func (p *Plugin) Subscribe(ctx context.Context, m bridge.Mapping) error {
	p.mu.Lock()
	p.subs[m.RemoteEntityID] = m
	p.dsuidToUID[m.DSUID] = m.RemoteEntityID
	entity, connected := p.devices[m.RemoteEntityID]
	p.mu.Unlock()

	if connected {
		// Device already connected: announce it to the state store immediately.
		am := m
		if am.Kind == "" {
			am.Kind = entity.Kind
		}
		if err := p.host.AnnounceDevice(ctx, am); err != nil {
			return fmt.Errorf("externaldevice: announce existing device %q: %w", m.RemoteEntityID, err)
		}
	}
	return nil
}

// Unsubscribe implements bridge.Plugin.
// Removes the mapping so that subsequent events for the device are no longer
// forwarded.  The external TCP connection itself is unaffected.
func (p *Plugin) Unsubscribe(_ context.Context, dsuid string) error {
	p.mu.Lock()
	uid, ok := p.dsuidToUID[dsuid]
	if ok {
		delete(p.subs, uid)
		delete(p.dsuidToUID, dsuid)
	}
	p.mu.Unlock()
	return nil
}

// Apply implements bridge.Plugin.
// Forwards a command to the external device over its TCP connection.
func (p *Plugin) Apply(_ context.Context, m bridge.Mapping, cmd bridge.Command) error {
	if p.srv == nil {
		return fmt.Errorf("externaldevice: server not initialised")
	}
	switch cmd.Type {
	case "setChannel":
		return p.srv.SetChannelValue(m.RemoteEntityID, cmd.Channel, cmd.Value)
	default:
		return fmt.Errorf("externaldevice: unsupported command type %q", cmd.Type)
	}
}

// Close implements bridge.Plugin.
func (p *Plugin) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// Stats implements bridge.StatsProvider.
func (p *Plugin) Stats() bridge.PluginStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return bridge.PluginStats{
		Discovered: len(p.devices),
		Active:     len(p.subs),
	}
}

// onEvent is the server.Config.OnEvent callback. It receives all runtime
// events emitted by connected external devices and routes them through the
// Host interface for subscribed devices.
func (p *Plugin) onEvent(e runtime.Event) {
	switch e.Type {
	case runtime.EventInit:
		p.mu.Lock()
		p.devices[e.UniqueID] = bridge.RemoteEntity{
			ID:   e.UniqueID,
			Name: e.Name,
			Kind: e.Output,
			Attributes: map[string]any{
				"protocol": e.Output,
			},
		}
		m, subscribed := p.subs[e.UniqueID]
		p.mu.Unlock()

		if subscribed {
			am := m
			if am.Kind == "" {
				am.Kind = e.Output
			}
			if err := p.host.AnnounceDevice(context.Background(), am); err != nil {
				p.host.Log(bridge.LevelWarn, bridge.CodeApplyFailed,
					"failed to announce device on reconnect",
					map[string]any{"uniqueid": e.UniqueID, "error": err.Error()})
			}
		}

	case runtime.EventRemove:
		p.mu.Lock()
		delete(p.devices, e.UniqueID)
		m, subscribed := p.subs[e.UniqueID]
		p.mu.Unlock()

		if subscribed {
			if err := p.host.UpdateActive(context.Background(), m.DSUID, false); err != nil {
				p.host.Log(bridge.LevelWarn, bridge.CodeApplyFailed,
					"failed to mark device inactive on disconnect",
					map[string]any{"uniqueid": e.UniqueID, "error": err.Error()})
			}
		}

	case runtime.EventChannel:
		if m, ok := p.lookupSub(e.UniqueID); ok {
			_ = p.host.UpdateChannel(context.Background(), m.DSUID, e.Index, e.Value)
		}

	case runtime.EventButton:
		if m, ok := p.lookupSub(e.UniqueID); ok {
			_ = p.host.UpdateButton(context.Background(), m.DSUID, e.Index, e.Value)
		}

	case runtime.EventButtonAction:
		if m, ok := p.lookupSub(e.UniqueID); ok {
			_ = p.host.SetButtonAction(context.Background(), m.DSUID, e.Index, e.Action)
		}

	case runtime.EventInput:
		if m, ok := p.lookupSub(e.UniqueID); ok {
			_ = p.host.UpdateInput(context.Background(), m.DSUID, e.Index, e.Value)
		}

	case runtime.EventSensor:
		if m, ok := p.lookupSub(e.UniqueID); ok {
			_ = p.host.UpdateSensor(context.Background(), m.DSUID, e.Index, e.Value)
		}

	case runtime.EventActive:
		if m, ok := p.lookupSub(e.UniqueID); ok {
			_ = p.host.UpdateActive(context.Background(), m.DSUID, e.Active)
		}
	}
}

// lookupSub returns the Mapping for uniqueID under a read lock.
func (p *Plugin) lookupSub(uniqueID string) (bridge.Mapping, bool) {
	p.mu.RLock()
	m, ok := p.subs[uniqueID]
	p.mu.RUnlock()
	return m, ok
}
