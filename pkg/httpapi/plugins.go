package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/splattner/vdcgo/pkg/bridge"
)

// pluginTypeInfo is the JSON shape returned by GET /api/plugin-types.
type pluginTypeInfo struct {
	Type        string              `json:"type"`
	DisplayName string              `json:"displayName"`
	Description string              `json:"description"`
	Schema      bridge.ConfigSchema `json:"schema"`
	HasProbe    bool                `json:"hasProbe"`
}

// handlePluginTypes lists all plugin factory types known to the registry.
func (s *Server) handlePluginTypes(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusOK, []pluginTypeInfo{})
		return
	}
	types := s.cfg.Bridges.FactoryTypes()
	sort.Strings(types)
	out := make([]pluginTypeInfo, 0, len(types))
	for _, t := range types {
		entry, ok := s.cfg.Bridges.FactoryEntry(t)
		if !ok {
			continue
		}
		out = append(out, pluginTypeInfo{
			Type:        t,
			DisplayName: entry.DisplayName,
			Description: entry.Description,
			Schema:      entry.Schema,
			HasProbe:    entry.Probe != nil,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// pluginConfigResponse is the JSON shape returned by GET /api/plugins/{id}/config.
//
// Secret fields (e.g. password) are stripped from the config. The `secrets`
// list reports which top-level keys were present-but-redacted so the UI can
// display a "(set)" hint and offer to clear them.
type pluginConfigResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Config  map[string]any `json:"config"`
	Secrets []string       `json:"secrets"`
}

// handlePluginConfig returns the current config for a plugin instance.
func (s *Server) handlePluginConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	pc, ok := s.cfg.Bridges.Config(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	entry, _ := s.cfg.Bridges.FactoryEntry(pc.Type)
	cleaned, secrets := scrubSecrets(pc.Config, entry.Schema)
	writeJSON(w, http.StatusOK, pluginConfigResponse{
		ID:      pc.ID,
		Type:    pc.Type,
		Config:  cleaned,
		Secrets: secrets,
	})
}

// updatePluginRequest is the body of PUT /api/plugins/{id}/config.
type updatePluginRequest struct {
	Config map[string]any `json:"config"`
}

// handlePluginConfigUpdate replaces a plugin's config and restarts the instance.
//
// Secret fields not present in the request body are merged from the existing
// config so the UI never has to round-trip secrets it never received.
func (s *Server) handlePluginConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	existing, ok := s.cfg.Bridges.Config(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	var req updatePluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	entry, _ := s.cfg.Bridges.FactoryEntry(existing.Type)
	merged := mergeSecrets(req.Config, existing.Config, entry.Schema)

	newCfg := bridge.PluginConfig{ID: id, Type: existing.Type, Config: merged}
	if err := s.cfg.Bridges.UpdatePlugin(r.Context(), newCfg); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	cleaned, secrets := scrubSecrets(merged, entry.Schema)
	writeJSON(w, http.StatusOK, pluginConfigResponse{
		ID: id, Type: existing.Type, Config: cleaned, Secrets: secrets,
	})
}

// createPluginRequest is the body of POST /api/plugins.
type createPluginRequest struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// handleCreatePlugin starts a brand-new plugin instance.
func (s *Server) handleCreatePlugin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	var req createPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.ID == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id and type are required"})
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	if _, ok := s.cfg.Bridges.FactoryEntry(req.Type); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown plugin type"})
		return
	}
	pc := bridge.PluginConfig{ID: req.ID, Type: req.Type, Config: req.Config}
	if err := s.cfg.Bridges.AddPlugin(r.Context(), pc); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	entry, _ := s.cfg.Bridges.FactoryEntry(req.Type)
	cleaned, secrets := scrubSecrets(req.Config, entry.Schema)
	writeJSON(w, http.StatusCreated, pluginConfigResponse{
		ID: req.ID, Type: req.Type, Config: cleaned, Secrets: secrets,
	})
}

// handleDeletePlugin stops and removes a plugin instance.
func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.cfg.Bridges.RemovePlugin(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// probeRequest is the body of POST /api/plugins/{id}/probe and
// POST /api/plugin-types/{type}/probe — both accept an optional config to
// override stored secrets/values for the test.
type probeRequest struct {
	Config map[string]any `json:"config"`
}

// probeResponse is returned by both probe endpoints.
type probeResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// handlePluginProbe runs the type's Probe function against an instance's
// (optionally overridden) config.
func (s *Server) handlePluginProbe(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	pc, ok := s.cfg.Bridges.Config(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}
	entry, _ := s.cfg.Bridges.FactoryEntry(pc.Type)
	if entry.Probe == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "plugin type does not support probing"})
		return
	}
	var req probeRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	cfg := pc.Config
	if req.Config != nil {
		cfg = mergeSecrets(req.Config, pc.Config, entry.Schema)
	}
	if err := entry.Probe(cfg); err != nil {
		writeJSON(w, http.StatusOK, probeResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, probeResponse{OK: true})
}

// handlePluginTypeProbe runs Probe with a config supplied by the caller (no
// existing instance), useful before creating a new plugin.
func (s *Server) handlePluginTypeProbe(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bridges == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "bridges not configured"})
		return
	}
	t := chi.URLParam(r, "type")
	entry, ok := s.cfg.Bridges.FactoryEntry(t)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin type not found"})
		return
	}
	if entry.Probe == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "plugin type does not support probing"})
		return
	}
	var req probeRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	cfg := req.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	if err := entry.Probe(cfg); err != nil {
		writeJSON(w, http.StatusOK, probeResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, probeResponse{OK: true})
}

// scrubSecrets returns a deep-copy of cfg with all secret fields removed,
// plus a list of secret keys that were actually present (dot-notation for
// nested objects, e.g. "will.payload" — not currently used for nested but
// kept future-proof).
func scrubSecrets(cfg map[string]any, schema bridge.ConfigSchema) (map[string]any, []string) {
	cleaned := deepCopy(cfg)
	secrets := []string{}
	stripSecrets(cleaned, schema.Fields, "", &secrets)
	return cleaned, secrets
}

func stripSecrets(target map[string]any, fields []bridge.ConfigField, prefix string, secrets *[]string) {
	for _, f := range fields {
		key := f.Key
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		if bridge.IsSecretField(f) {
			if v, ok := target[key]; ok {
				if s, isStr := v.(string); isStr && s != "" {
					*secrets = append(*secrets, full)
				}
				delete(target, key)
			}
		}
		if f.Type == "object" && len(f.Children) > 0 {
			if child, ok := target[key].(map[string]any); ok {
				stripSecrets(child, f.Children, full, secrets)
			}
		}
	}
}

// mergeSecrets fills in secret fields missing from incoming using the values
// from existing — this lets the UI submit a config without ever having
// received the original secrets.
func mergeSecrets(incoming, existing map[string]any, schema bridge.ConfigSchema) map[string]any {
	out := deepCopy(incoming)
	mergeSecretsRec(out, existing, schema.Fields)
	return out
}

func mergeSecretsRec(target, existing map[string]any, fields []bridge.ConfigField) {
	for _, f := range fields {
		key := f.Key
		if bridge.IsSecretField(f) {
			if _, present := target[key]; !present {
				if v, ok := existing[key]; ok {
					target[key] = v
				}
			}
		}
		if f.Type == "object" && len(f.Children) > 0 {
			tChild, _ := target[key].(map[string]any)
			eChild, _ := existing[key].(map[string]any)
			if tChild != nil && eChild != nil {
				mergeSecretsRec(tChild, eChild, f.Children)
			}
		}
	}
}

// deepCopy clones a JSON-shaped map so the original is never mutated.
func deepCopy(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch vv := v.(type) {
		case map[string]any:
			out[k] = deepCopy(vv)
		case []any:
			cp := make([]any, len(vv))
			for i, item := range vv {
				if m, ok := item.(map[string]any); ok {
					cp[i] = deepCopy(m)
				} else {
					cp[i] = item
				}
			}
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}
