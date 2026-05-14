package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

// writeJSON encodes v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleHealth returns a simple liveness response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": "dev",
	})
}

// handleDSS returns information about the active vDSM session.
func (s *Server) handleDSS(w http.ResponseWriter, r *http.Request) {
	info := DSSSessionInfo{}
	if s.cfg.SessionInfo != nil {
		info = s.cfg.SessionInfo()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vdcDSUID": s.cfg.DSUID,
		"session":  info,
	})
}

// handleDevices returns all announced devices.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	snapshot := s.cfg.State.Snapshot()
	root, _, devices := vdcapi.BuildPropertyTree(s.cfg.DSUID, s.cfg.Description, snapshot, s.cfg.Scenes, s.cfg.Config)
	_ = root
	writeJSON(w, http.StatusOK, devices)
}

// handleDevice returns a single announced device by DSUID.
func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	dsuid := chi.URLParam(r, "dsuid")
	snapshot := s.cfg.State.Snapshot()
	_, _, devices := vdcapi.BuildPropertyTree(s.cfg.DSUID, s.cfg.Description, snapshot, s.cfg.Scenes, s.cfg.Config)
	dev, ok := devices[dsuid]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "device not found"})
		return
	}
	writeJSON(w, http.StatusOK, dev)
}
