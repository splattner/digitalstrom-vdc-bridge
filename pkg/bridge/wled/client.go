package wled

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/splattner/vdcgo/pkg/logging"
)

// deviceClient manages the connection to a single WLED device.
// It connects to the WebSocket for live push and falls back to polling
// when the WebSocket is unavailable. HTTP POST is used for Apply commands.
type deviceClient struct {
	addr string // "host:port", e.g. "192.168.1.50:80"
	mac  string

	// callbacks (set before start)
	onState  func(wledSI) // called on every state+info update
	onStatus func(string) // "connecting", "connected", "reconnecting", "offline"

	// runtime
	connected atomic.Bool
	statusVal atomic.Value // string
	cancel    context.CancelFunc

	wg sync.WaitGroup
}

func newDeviceClient(addr, mac string) *deviceClient {
	return &deviceClient{addr: addr, mac: mac}
}

func (c *deviceClient) Connected() bool { return c.connected.Load() }

func (c *deviceClient) Status() string {
	if v := c.statusVal.Load(); v != nil {
		return v.(string)
	}
	return "starting"
}

func (c *deviceClient) setStatus(s string) {
	c.statusVal.Store(s)
	if c.onStatus != nil {
		c.onStatus(s)
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

// run keeps the device connected; attempts WS first, falls back to polling.
func (c *deviceClient) run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		c.setStatus("connecting")
		err := c.runWS(ctx)
		if ctx.Err() != nil {
			return
		}
		c.connected.Store(false)
		if err != nil {
			logging.Warn("wled_ws_disconnect", logging.Fields{
				"addr":  c.addr,
				"mac":   c.mac,
				"error": err.Error(),
			})
			c.setStatus("reconnecting")
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

// runWS performs one WebSocket lifecycle: connect → recv initial state → recv pushes.
func (c *deviceClient) runWS(ctx context.Context) error {
	wsURL := fmt.Sprintf("ws://%s/ws", c.addr)
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

	c.connected.Store(true)
	c.setStatus("connected")
	logging.Info("wled_ws_connected", logging.Fields{"addr": c.addr, "mac": c.mac})

	// Also start a slow poll ticker (30 s) as a heartbeat in case the device
	// goes silent between state changes (e.g. manually powered on/off via switch).
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-connCtx.Done():
			return connCtx.Err()
		case <-heartbeat.C:
			// Quietly re-fetch state so active/inactive status stays fresh.
			_ = c.pollOnce(ctx)
		default:
		}

		_, msg, err := conn.Read(connCtx)
		if err != nil {
			return fmt.Errorf("ws read: %w", err)
		}
		var si wledSI
		if jsonErr := json.Unmarshal(msg, &si); jsonErr != nil {
			// Ignore unparseable frames (e.g. live LED stream).
			continue
		}
		// Filter out frames that lack both MAC and state (e.g. live pixel data).
		if si.Info.MAC == "" && !si.State.On && si.State.Bri == 0 {
			continue
		}
		if c.onState != nil {
			c.onState(si)
		}
	}
}

// pollOnce fetches /json/si via HTTP and calls onState.
func (c *deviceClient) pollOnce(ctx context.Context) error {
	url := fmt.Sprintf("http://%s/json/si", c.addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return err
	}
	var si wledSI
	if err := json.Unmarshal(body, &si); err != nil {
		return fmt.Errorf("decode /json/si: %w", err)
	}
	if c.onState != nil {
		c.onState(si)
	}
	return nil
}

// postState sends a partial state JSON to the device via HTTP POST /json/state.
// This is used for Apply commands; WS send would also work but HTTP is
// fire-and-forget and doesn't require the WS to be open.
func (c *deviceClient) postState(ctx context.Context, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	url := fmt.Sprintf("http://%s/json/state", c.addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post state: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("post state: HTTP %d", resp.StatusCode)
	}
	return nil
}
