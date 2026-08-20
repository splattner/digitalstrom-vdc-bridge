package shelly

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

// deviceClient maintains a persistent connection to a single Shelly Gen2+
// device: a WebSocket to ws://<addr>/rpc for live NotifyStatus/NotifyFullStatus
// push, with HTTP POST /rpc used both for commands and as a fallback/heartbeat
// poll when the socket is down. One deviceClient is shared across every
// bridged component (switch, light, ...) on the same physical device — see
// Plugin.activate.
type deviceClient struct {
	addr  string // "host:port"
	devID string
	src   string // our RPC "src" identifier, unique per plugin instance

	// callbacks (set before start)
	onStatus func(map[string]map[string]any) // called after every status merge
	onConn   func(string)                    // "connecting", "connected", "reconnecting"
	onEvent  func([]shellyEvent)             // called for every NotifyEvent (button pushes)

	status *deviceStatus

	connected atomic.Bool
	statusVal atomic.Value // string
	reqID     atomic.Int32
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// shellyEvent is one entry of a Shelly Gen2+ NotifyEvent's "events" array —
// used for input button pushes ("single_push", "double_push", "long_push").
// NotifyEvent is pure live push with no retained/replay semantics (unlike
// MQTT), so unlike Zigbee2MQTT's retained-message handling there is no risk
// of a stale event firing on reconnect — nothing to suppress here.
type shellyEvent struct {
	Component string `json:"component"`
	Event     string `json:"event"`
}

type notifyEventParams struct {
	Events []shellyEvent `json:"events"`
}

func newDeviceClient(addr, devID, src string) *deviceClient {
	return &deviceClient{
		addr:   addr,
		devID:  devID,
		src:    src,
		status: newDeviceStatus(),
	}
}

func (c *deviceClient) Connected() bool { return c.connected.Load() }

func (c *deviceClient) setConnState(s string) {
	c.statusVal.Store(s)
	if c.onConn != nil {
		c.onConn(s)
	}
}

// start launches the connection loop in a background goroutine.
// Call stop() to shut it down.
func (c *deviceClient) start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.run(ctx)
	}()
}

func (c *deviceClient) stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

// run keeps the device connected, reconnecting with exponential backoff.
func (c *deviceClient) run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		c.setConnState("connecting")
		err := c.runWS(ctx)
		if ctx.Err() != nil {
			return
		}
		c.connected.Store(false)
		if err != nil {
			logging.Warn("shelly_ws_disconnect", logging.Fields{
				"addr": c.addr, "device_id": c.devID, "error": err.Error(),
			})
		}
		c.setConnState("reconnecting")

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

// runWS performs one WebSocket lifecycle: connect, kick off the notification
// stream, then receive pushes until the connection drops.
func (c *deviceClient) runWS(ctx context.Context) error {
	wsURL := fmt.Sprintf("ws://%s/rpc", c.addr)
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, _, err := websocket.Dial(connCtx, wsURL, nil)
	if err != nil {
		// WS unavailable — do a single HTTP poll and return so the caller retries.
		if pollErr := c.pollOnce(ctx); pollErr != nil {
			return fmt.Errorf("ws dial: %w; poll fallback: %v", err, pollErr)
		}
		return err
	}
	conn.SetReadLimit(256 << 10) // 256 KB
	defer conn.CloseNow()

	// Sending any request frame with a valid src is what starts the
	// NotifyStatus/NotifyEvent push stream on a Shelly Gen2+ device; its
	// response also seeds our status cache with a full snapshot.
	if err := c.sendRequest(connCtx, conn, "Shelly.GetStatus", nil); err != nil {
		return fmt.Errorf("initial request: %w", err)
	}

	c.connected.Store(true)
	c.setConnState("connected")
	logging.Info("shelly_ws_connected", logging.Fields{"addr": c.addr, "device_id": c.devID})

	// Frames are read on their own goroutine and fed through a channel so the
	// heartbeat ticker below is evaluated even while conn.Read is blocked
	// waiting on the next frame. A plain `select` with a `default:` branch
	// wrapped around a blocking Read would starve the ticker whenever the
	// device goes silent — exactly the case the heartbeat exists to cover.
	frames := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		for {
			_, msg, err := conn.Read(connCtx)
			if err != nil {
				readErr <- err
				return
			}
			select {
			case frames <- msg:
			case <-connCtx.Done():
				return
			}
		}
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-connCtx.Done():
			return connCtx.Err()
		case err := <-readErr:
			return fmt.Errorf("ws read: %w", err)
		case <-heartbeat.C:
			// Quietly re-fetch state so active/inactive status stays fresh
			// even if the device has gone silent between pushes.
			_ = c.pollOnce(ctx)
		case msg := <-frames:
			c.handleFrame(msg)
		}
	}
}

func (c *deviceClient) sendRequest(ctx context.Context, conn *websocket.Conn, method string, params any) error {
	id := int(c.reqID.Add(1))
	body, err := json.Marshal(rpcRequest{ID: id, Src: c.src, Method: method, Params: params})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, body)
}

func (c *deviceClient) handleFrame(msg []byte) {
	var f rpcFrame
	if err := json.Unmarshal(msg, &f); err != nil {
		return // ignore unparseable frames
	}
	if f.Error != nil {
		logging.Warn("shelly_rpc_error", logging.Fields{
			"device_id": c.devID, "code": f.Error.Code, "message": f.Error.Message,
		})
		return
	}

	if f.Method == "NotifyEvent" {
		var params notifyEventParams
		if err := json.Unmarshal(f.Params, &params); err != nil {
			return
		}
		if c.onEvent != nil {
			c.onEvent(params.Events)
		}
		return
	}

	var payload map[string]any
	switch {
	case f.Method == "NotifyStatus" || f.Method == "NotifyFullStatus":
		if err := json.Unmarshal(f.Params, &payload); err != nil {
			return
		}
	case f.ID != nil && len(f.Result) > 0:
		// Response to the initial Shelly.GetStatus request sent when the
		// socket opened — treat it as a full status snapshot.
		if err := json.Unmarshal(f.Result, &payload); err != nil {
			return
		}
	default:
		return
	}

	c.status.merge(payload)
	if c.onStatus != nil {
		c.onStatus(c.status.snapshot())
	}
}

// pollOnce fetches the full status via HTTP RPC — used as a fallback when the
// WebSocket is unavailable and as a heartbeat while it is connected.
func (c *deviceClient) pollOnce(ctx context.Context) error {
	result, err := callHTTP(ctx, c.addr, c.src, "Shelly.GetStatus", nil)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		return fmt.Errorf("decode status: %w", err)
	}
	c.status.merge(payload)
	if c.onStatus != nil {
		c.onStatus(c.status.snapshot())
	}
	return nil
}

// setSwitch calls Switch.Set over HTTP RPC.
func (c *deviceClient) setSwitch(ctx context.Context, index int, on bool) error {
	_, err := callHTTP(ctx, c.addr, c.src, "Switch.Set", map[string]any{"id": index, "on": on})
	return err
}

// setLight calls Light.Set over HTTP RPC. brightness is omitted when nil
// (used for "turn off" — Shelly implicitly keeps the last brightness).
func (c *deviceClient) setLight(ctx context.Context, index int, on bool, brightness *float64) error {
	params := map[string]any{"id": index, "on": on}
	if brightness != nil {
		params["brightness"] = *brightness
	}
	_, err := callHTTP(ctx, c.addr, c.src, "Light.Set", params)
	return err
}
