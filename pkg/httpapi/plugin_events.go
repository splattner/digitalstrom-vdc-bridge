package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/splattner/vdcgo/pkg/bridge"
)

// handlePluginEventsGlobal returns a snapshot of the global event ring buffer.
//
// GET /api/plugin-events?since=<seq>&level=<level>&limit=<n>
func (s *Server) handlePluginEventsGlobal(w http.ResponseWriter, r *http.Request) {
	if s.cfg.EventBuffer == nil {
		writeJSON(w, http.StatusOK, []bridge.PluginEvent{})
		return
	}
	since, level, limit := parseEventQueryParams(r)
	evs := s.cfg.EventBuffer.Snapshot("", since, level, limit)
	if evs == nil {
		evs = []bridge.PluginEvent{}
	}
	writeJSON(w, http.StatusOK, evs)
}

// handlePluginEvents returns a per-plugin event snapshot.
//
// GET /api/plugins/{id}/events?since=<seq>&level=<level>&limit=<n>
func (s *Server) handlePluginEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.cfg.EventBuffer == nil {
		writeJSON(w, http.StatusOK, []bridge.PluginEvent{})
		return
	}
	since, level, limit := parseEventQueryParams(r)
	evs := s.cfg.EventBuffer.Snapshot(id, since, level, limit)
	if evs == nil {
		evs = []bridge.PluginEvent{}
	}
	writeJSON(w, http.StatusOK, evs)
}

// handleClearPluginEvents clears the event buffer for a specific plugin.
//
// DELETE /api/plugins/{id}/events
func (s *Server) handleClearPluginEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.cfg.EventBuffer == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.cfg.EventBuffer.ClearPlugin(id)
	w.WriteHeader(http.StatusNoContent)
}

// parseEventQueryParams extracts the common query parameters used by the
// event snapshot endpoints.
func parseEventQueryParams(r *http.Request) (sinceSeq uint64, level bridge.LogLevel, limit int) {
	q := r.URL.Query()
	if v := q.Get("since"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			sinceSeq = n
		}
	}
	if v := q.Get("level"); v != "" {
		level = bridge.LogLevel(v)
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	return sinceSeq, level, limit
}
