package httpapi

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Hardcoded vDC identity values surfaced on the Settings page.
const (
	settingsFirmwareVersion = "0.1.0"
)

// SettingsInfo is the payload returned by GET /api/settings.
type SettingsInfo struct {
	// Identity
	VdcDSUID        string `json:"vdcDSUID"`
	Description     string `json:"description"`
	Vendor          string `json:"vendor"`
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmwareVersion"`

	// Runtime
	VdcAPIPort   int    `json:"vdcAPIPort"`
	HTTPListen   string `json:"httpListen"`
	EnableDNSSD  bool   `json:"enableDNSSD"`
	NonLocal     bool   `json:"nonLocal"`
	NoAuto       bool   `json:"noAuto"`
	DataDir      string `json:"dataDir"`
	AuthEnabled  bool   `json:"authEnabled"`
	BuildVersion string `json:"buildVersion"`
	GoVersion    string `json:"goVersion"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`

	// vDSM session (mirrors /api/dss for convenience)
	Session DSSSessionInfo `json:"session"`
}

// handleSettings returns vDC identity, runtime and current vDSM session info.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	version := s.cfg.Version
	if version == "" {
		version = "dev"
	}
	session := DSSSessionInfo{}
	if s.cfg.SessionInfo != nil {
		session = s.cfg.SessionInfo()
	}
	s.identityMu.RLock()
	desc := s.identityDesc
	vendor := s.identityVendor
	model := s.identityModel
	s.identityMu.RUnlock()
	writeJSON(w, http.StatusOK, SettingsInfo{
		VdcDSUID:        s.cfg.DSUID,
		Description:     desc,
		Vendor:          vendor,
		Model:           model,
		FirmwareVersion: settingsFirmwareVersion,
		VdcAPIPort:      s.cfg.VdcAPIPort,
		HTTPListen:      s.cfg.Listen,
		EnableDNSSD:     s.cfg.EnableDNSSD,
		NonLocal:        s.cfg.NonLocal,
		NoAuto:          s.cfg.NoAuto,
		DataDir:         s.cfg.DataDir,
		AuthEnabled:     s.cfg.AuthPassword != "",
		BuildVersion:    version,
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		Session:         session,
	})
}

// patchIdentityRequest is the body of PATCH /api/settings/identity.
type patchIdentityRequest struct {
	Description *string `json:"description"`
	Vendor      *string `json:"vendor"`
	Model       *string `json:"model"`
}

// handlePatchIdentity updates the mutable description/vendor/model and
// persists the new values to {DataDir}/identity.json when DataDir is set.
func (s *Server) handlePatchIdentity(w http.ResponseWriter, r *http.Request) {
	var req patchIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	s.identityMu.Lock()
	if req.Description != nil {
		s.identityDesc = *req.Description
	}
	if req.Vendor != nil {
		s.identityVendor = *req.Vendor
	}
	if req.Model != nil {
		s.identityModel = *req.Model
	}
	desc := s.identityDesc
	vendor := s.identityVendor
	model := s.identityModel
	s.identityMu.Unlock()

	if s.cfg.DataDir != "" {
		identityPath := filepath.Join(s.cfg.DataDir, "identity.json")
		payload, _ := json.MarshalIndent(map[string]string{
			"description": desc,
			"vendor":      vendor,
			"model":       model,
		}, "", "  ")
		if err := os.WriteFile(identityPath, payload, 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "persist failed: " + err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"description": desc,
		"vendor":      vendor,
		"model":       model,
	})
}

// handleForgetVdsm clears the set of DSUIDs previously announced to a vDSM, so
// the next hello/announce cycle re-announces all currently known devices.
func (s *Server) handleForgetVdsm(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "config store not available"})
		return
	}
	cleared := s.cfg.Config.ClearAnnouncedDSUIDs()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"cleared": cleared,
	})
}

// handleExportConfig streams a tar.gz of persistent state files (config.json,
// scenes.json, bridges.json, plugins.json, dsuid) from DataDir.
func (s *Server) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DataDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no data directory configured"})
		return
	}
	candidates := []string{"config.json", "scenes.json", "bridges.json", "plugins.json", "dsuid"}
	stamp := time.Now().UTC().Format("20060102-150405")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="vdcgo-config-%s.tar.gz"`, stamp))

	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for _, name := range candidates {
		p := filepath.Join(s.cfg.DataDir, name)
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		hdr := &tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len(data)),
			ModTime: fi.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return
		}
		if _, err := tw.Write(data); err != nil {
			return
		}
	}
}
