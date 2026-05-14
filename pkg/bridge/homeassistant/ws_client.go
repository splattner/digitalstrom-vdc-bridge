// Package homeassistant implements a bridge.Plugin that connects to a Home
// Assistant instance over its WebSocket API.
package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/splattner/vdcgo/pkg/logging"
)

// haEntity is a single HA entity (light, sensor, …) as returned by get_states.
type haEntity struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	LastChanged string         `json:"last_changed,omitempty"`
	LastUpdated string         `json:"last_updated,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	Extra       map[string]any `json:"-"`
}

// stateChange is delivered for each HA state_changed event.
type stateChange struct {
	EntityID string    `json:"entity_id"`
	NewState *haEntity `json:"new_state"`
	OldState *haEntity `json:"old_state"`
}

// wsClient is a minimal Home Assistant WebSocket client.
//
// It handles auth, fetches initial states, and subscribes to state_changed
// events. On disconnect it reconnects with exponential backoff. State updates
// are delivered to the OnStateChange callback; the initial snapshot is
// delivered via OnSnapshot.
type wsClient struct {
	url   string
	token string

	// callbacks (set before Run)
	onSnapshot    func(map[string]haEntity)
	onStateChange func(stateChange)
	onStatus      func(string) // "connecting", "connected", "reconnecting", "auth_failed"
	onRegistries  func(haRegistries)

	// runtime
	mu        sync.RWMutex
	conn      *websocket.Conn
	connected atomic.Bool
	nextID    atomic.Uint64

	// pending result channels keyed by message id
	pendingMu sync.Mutex
	pending   map[uint64]chan json.RawMessage
}

func newWSClient(url, token string) *wsClient {
	c := &wsClient{
		url:     url,
		token:   token,
		pending: make(map[uint64]chan json.RawMessage),
	}
	c.nextID.Store(0)
	return c
}

// Connected reports whether the client is currently connected and authed.
func (c *wsClient) Connected() bool { return c.connected.Load() }

// Run blocks until ctx is cancelled, reconnecting on transient failures.
func (c *wsClient) Run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		c.setStatus("connecting")
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		c.connected.Store(false)

		if err != nil {
			logging.Warn("ha_ws_disconnect", logging.Fields{"url": c.url, "error": err.Error()})
			// auth_failed is permanent — back off long but keep trying so a token rotation recovers.
			if err.Error() == "auth_failed" {
				c.setStatus("auth_failed")
				backoff = maxBackoff
			} else {
				c.setStatus("reconnecting")
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce performs one connection lifecycle: dial, auth, subscribe, then read
// until error or context cancellation.
func (c *wsClient) runOnce(ctx context.Context) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, _, err := websocket.Dial(connCtx, c.url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	conn.SetReadLimit(8 << 20) // 8 MB — HA snapshots can be large
	defer conn.CloseNow()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Auth handshake.
	if err := c.authenticate(connCtx, conn); err != nil {
		return err
	}

	c.connected.Store(true)
	// Reset backoff after a successful auth on the next iteration is the caller's job;
	// here we just signal we're up.
	c.setStatus("connected")
	logging.Info("ha_ws_connected", logging.Fields{"url": c.url})

	// Subscribe to state_changed events first so we don't miss updates that
	// occur between get_states and subscribe.
	subID := c.nextMsgID()
	if err := c.send(connCtx, conn, map[string]any{
		"id":         subID,
		"type":       "subscribe_events",
		"event_type": "state_changed",
	}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	// get_states for the initial snapshot.
	getID := c.nextMsgID()
	resultCh := c.registerPending(getID)
	if err := c.send(connCtx, conn, map[string]any{
		"id":   getID,
		"type": "get_states",
	}); err != nil {
		c.cancelPending(getID)
		return fmt.Errorf("get_states: %w", err)
	}

	// Read loop dispatches results & events.
	readErrCh := make(chan error, 1)
	go func() { readErrCh <- c.readLoop(connCtx, conn) }()

	// Wait for the snapshot result.
	select {
	case <-connCtx.Done():
		return connCtx.Err()
	case raw := <-resultCh:
		var result struct {
			Success bool        `json:"success"`
			Result  []haEntity  `json:"result"`
			Error   interface{} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("decode get_states: %w", err)
		}
		if !result.Success {
			return fmt.Errorf("get_states failed: %v", result.Error)
		}
		snap := make(map[string]haEntity, len(result.Result))
		for _, e := range result.Result {
			snap[e.EntityID] = e
		}
		if c.onSnapshot != nil {
			c.onSnapshot(snap)
		}
	case err := <-readErrCh:
		return err
	}

	// Fetch HA registries (areas / devices / entities) so the plugin can
	// surface device + area names alongside discovered entities. Best effort:
	// if HA rejects the calls (e.g. token without admin) we just skip.
	if c.onRegistries != nil {
		if regs, err := c.fetchRegistries(connCtx, conn); err == nil {
			c.onRegistries(regs)
		} else {
			logging.Warn("ha_registries_failed", logging.Fields{"error": err.Error()})
		}
	}

	// Block until read loop ends.
	return <-readErrCh
}

// authenticate performs the auth_required → auth → auth_ok handshake.
func (c *wsClient) authenticate(ctx context.Context, conn *websocket.Conn) error {
	// Read auth_required.
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("read auth_required: %w", err)
	}
	var req struct {
		Type      string `json:"type"`
		HaVersion string `json:"ha_version"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("decode auth_required: %w", err)
	}
	if req.Type != "auth_required" {
		return fmt.Errorf("expected auth_required, got %q", req.Type)
	}

	// Send auth.
	if err := c.send(ctx, conn, map[string]any{
		"type":         "auth",
		"access_token": c.token,
	}); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	// Read auth_ok / auth_invalid.
	_, raw, err = conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("read auth result: %w", err)
	}
	var res struct {
		Type    string `json:"type"`
		Message string `json:"message,omitempty"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("decode auth result: %w", err)
	}
	switch res.Type {
	case "auth_ok":
		return nil
	case "auth_invalid":
		logging.Warn("ha_auth_invalid", logging.Fields{"message": res.Message})
		return fmt.Errorf("auth_failed")
	default:
		return fmt.Errorf("unexpected auth response %q", res.Type)
	}
}

// readLoop reads server messages and dispatches them to event/result handlers.
func (c *wsClient) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var head struct {
			ID    uint64          `json:"id"`
			Type  string          `json:"type"`
			Event json.RawMessage `json:"event,omitempty"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			logging.Warn("ha_ws_decode_error", logging.Fields{"error": err.Error()})
			continue
		}
		switch head.Type {
		case "result":
			if ch := c.takePending(head.ID); ch != nil {
				ch <- raw
			}
		case "event":
			c.dispatchEvent(head.Event)
		case "pong":
			// ignore
		default:
			// ignore unknown
		}
	}
}

func (c *wsClient) dispatchEvent(eventRaw json.RawMessage) {
	if c.onStateChange == nil || len(eventRaw) == 0 {
		return
	}
	var ev struct {
		EventType string      `json:"event_type"`
		Data      stateChange `json:"data"`
	}
	if err := json.Unmarshal(eventRaw, &ev); err != nil {
		return
	}
	if ev.EventType != "state_changed" {
		return
	}
	c.onStateChange(ev.Data)
}

// callService sends a call_service command and waits for the result.
func (c *wsClient) callService(ctx context.Context, domain, service string, target, data map[string]any) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil || !c.Connected() {
		return fmt.Errorf("not connected")
	}
	id := c.nextMsgID()
	resultCh := c.registerPending(id)
	defer c.cancelPending(id)

	msg := map[string]any{
		"id":      id,
		"type":    "call_service",
		"domain":  domain,
		"service": service,
	}
	if len(target) > 0 {
		msg["target"] = target
	}
	if len(data) > 0 {
		msg["service_data"] = data
	}

	if err := c.send(ctx, conn, msg); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("call_service timeout")
	case raw := <-resultCh:
		var res struct {
			Success bool `json:"success"`
			Error   any  `json:"error,omitempty"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return err
		}
		if !res.Success {
			return fmt.Errorf("call_service failed: %v", res.Error)
		}
		return nil
	}
}

func (c *wsClient) send(ctx context.Context, conn *websocket.Conn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func (c *wsClient) nextMsgID() uint64 {
	return c.nextID.Add(1)
}

func (c *wsClient) registerPending(id uint64) chan json.RawMessage {
	ch := make(chan json.RawMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
	return ch
}

func (c *wsClient) takePending(id uint64) chan json.RawMessage {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	return ch
}

func (c *wsClient) cancelPending(id uint64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *wsClient) setStatus(s string) {
	if c.onStatus != nil {
		c.onStatus(s)
	}
}
