package shelly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient is shared by every HTTP RPC call this package makes. A fixed
// timeout is essential here — the caller's ctx is not always short-lived
// (e.g. the plugin lifetime ctx used for a heartbeat poll), so without one an
// unreachable device would hang the calling goroutine indefinitely.
var httpClient = &http.Client{Timeout: 5 * time.Second}

// rpcRequest is a Shelly Gen2+ JSON-RPC request frame.
type rpcRequest struct {
	ID     int    `json:"id"`
	Src    string `json:"src"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// rpcError is the error object of a JSON-RPC response frame.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcFrame is a generic decode target for any frame received from a Shelly
// Gen2+ device: a response (ID set, Result or Error set) or a notification
// (Method set, ID absent).
type rpcFrame struct {
	ID     *int            `json:"id,omitempty"`
	Src    string          `json:"src,omitempty"`
	Dst    string          `json:"dst,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

// callHTTP performs one JSON-RPC request over HTTP POST /rpc — the transport
// used for commands (Switch.Set, Light.Set) and as a fallback/heartbeat poll
// when a device's WebSocket connection is down.
func callHTTP(ctx context.Context, addr, src, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{ID: 1, Src: src, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s/rpc", addr), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post rpc: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rpc HTTP %d", resp.StatusCode)
	}
	var f rpcFrame
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode rpc response: %w", err)
	}
	if f.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", f.Error.Code, f.Error.Message)
	}
	return f.Result, nil
}
