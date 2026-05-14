package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/splattner/vdcgo/pkg/bridge"
)

// pluginInfo is the JSON shape returned by GET /api/plugins.
type pluginInfo struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// handlePlugins lists all running plugin instances with their status.
func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusOK, []pluginInfo{})
		return
	}
	plugins := s.cfg.Bridges.Instances()
	out := make([]pluginInfo, len(plugins))
	for i, p := range plugins {
		info := pluginInfo{ID: p.ID(), Status: p.Status()}
		if cfg, ok := s.cfg.Bridges.Config(p.ID()); ok {
			info.Type = cfg.Type
		}
		out[i] = info
	}
	writeJSON(w, http.StatusOK, out)
}

// discoveredEntity extends RemoteEntity with a flag for already-mapped entities.
type discoveredEntity struct {
	bridge.RemoteEntity
	Mapped bool `json:"mapped"`
}

// handlePluginDiscovered returns all entities discoverable by a specific plugin,
// marking those that already have a Mapping.
func (s *Server) handlePluginDiscovered(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bridges not configured"})
		return
	}
	p, ok := s.cfg.Bridges.Plugin(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	entities, err := p.Discover(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	ms := s.cfg.Bridges.Mappings()
	out := make([]discoveredEntity, len(entities))
	for i, e := range entities {
		_, mapped := ms.GetByRemote(id, e.ID)
		out[i] = discoveredEntity{RemoteEntity: e, Mapped: mapped}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleBridges lists all current bridge mappings.
func (s *Server) handleBridges(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusOK, []bridge.Mapping{})
		return
	}
	writeJSON(w, http.StatusOK, s.cfg.Bridges.Mappings().List())
}

// createBridgeRequest is the POST /api/bridges request body.
type createBridgeRequest struct {
	PluginID       string `json:"pluginId"`
	RemoteEntityID string `json:"remoteEntityId"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
}

// handleCreateBridge creates a new bridge mapping.
func (s *Server) handleCreateBridge(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	var req createBridgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.PluginID == "" || req.RemoteEntityID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "pluginId and remoteEntityId are required"})
		return
	}
	if req.Kind == "" {
		req.Kind = "light"
	}

	m, err := s.cfg.Bridges.CreateBridge(r.Context(), req.PluginID, req.RemoteEntityID, req.Name, req.Kind)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// handleDeleteBridge removes a bridge mapping by DSUID.
func (s *Server) handleDeleteBridge(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	dsuid := chi.URLParam(r, "dsuid")
	if err := s.cfg.Bridges.RemoveBridge(r.Context(), dsuid); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
