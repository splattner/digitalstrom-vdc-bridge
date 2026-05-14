package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/splattner/vdcgo/pkg/logging"
)

// wsEvent is the message sent to WebSocket clients.
type wsEvent struct {
	Type  string         `json:"type"`
	DSUID string         `json:"dsuid,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
}

// wsHub manages all connected WebSocket clients and fans out events.
type wsHub struct {
	mu      sync.Mutex
	clients map[chan wsEvent]struct{}
	bcast   chan wsEvent
}

func newWSHub() *wsHub {
	return &wsHub{
		clients: make(map[chan wsEvent]struct{}),
		bcast:   make(chan wsEvent, 64),
	}
}

func (h *wsHub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-h.bcast:
			h.mu.Lock()
			for ch := range h.clients {
				select {
				case ch <- ev:
				default:
					// slow client: drop
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *wsHub) broadcast(ev wsEvent) {
	select {
	case h.bcast <- ev:
	default:
		logging.Warn("httpapi_ws_broadcast_dropped", logging.Fields{"event_type": ev.Type})
	}
}

func (h *wsHub) subscribe() chan wsEvent {
	ch := make(chan wsEvent, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *wsHub) unsubscribe(ch chan wsEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// handleEvents upgrades the connection to a WebSocket and streams state-change
// events until the client disconnects or the server shuts down.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS is handled by middleware
	})
	if err != nil {
		logging.Warn("httpapi_ws_upgrade_failed", logging.Fields{"error": err})
		return
	}
	defer conn.CloseNow() //nolint:errcheck

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	ctx := conn.CloseRead(r.Context())

	// Send an initial "hello" so the client can confirm the connection.
	hello, _ := json.Marshal(wsEvent{Type: "hello", Data: map[string]any{"time": time.Now().UTC().Format(time.RFC3339)}})
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
