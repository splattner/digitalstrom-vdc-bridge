package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/splattner/vdcgo/pkg/logging"
)

// handleDebugPbuf streams decoded protobuf trace frames to WebSocket clients.
func (s *Server) handleDebugPbuf(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		logging.Warn("httpapi_debug_ws_upgrade_failed", logging.Fields{"error": err})
		return
	}
	defer conn.CloseNow() //nolint:errcheck

	ch := s.debugHub.subscribe()
	defer s.debugHub.unsubscribe(ch)

	ctx := conn.CloseRead(r.Context())

	hello, _ := json.Marshal(wsEvent{Type: "hello", Data: map[string]any{"stream": "pbuf", "time": time.Now().UTC().Format(time.RFC3339)}})
	if err := conn.Write(ctx, websocket.MessageText, hello); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Write(writeCtx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
