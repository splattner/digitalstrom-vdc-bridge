// Package httpapi provides the REST + WebSocket HTTP API for the vdcgo web UI.
package httpapi

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/logging"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

// Config configures the HTTP API server.
type Config struct {
	// Listen is the address to bind to, e.g. ":8090" or "0.0.0.0:8090".
	Listen string
	// DSUID is the vDC DSUID advertised by this daemon.
	DSUID string
	// Description is the human-readable vDC description.
	Description string
	// State is the shared device state store.
	State *vdcapi.StateStore
	// Config is the shared device config store.
	Config *vdcapi.ConfigStore
	// Scenes is the shared scene store.
	Scenes *vdcapi.SceneStore
	// SessionInfo is a live-updated hook returning vdsm session information.
	// May be nil; the /api/dss endpoint will return empty fields.
	SessionInfo func() DSSSessionInfo
	// Bridges is the bridge plugin registry, used by /api/plugins and /api/bridges.
	// May be nil; those endpoints will return empty results.
	Bridges *bridge.Registry

	// Runtime metadata exposed by /api/settings (optional).
	VdcAPIPort  int
	EnableDNSSD bool
	NonLocal    bool
	NoAuto      bool
	DataDir     string
	Version     string
}

// DSSSessionInfo carries runtime information about the active vDSM connection.
type DSSSessionInfo struct {
	Connected   bool      `json:"connected"`
	VdsmDSUID   string    `json:"vdsmDSUID,omitempty"`
	APIVersion  int       `json:"apiVersion,omitempty"`
	RemoteAddr  string    `json:"remoteAddr,omitempty"`
	ConnectedAt time.Time `json:"connectedAt,omitempty"`
	LastSeen    string    `json:"lastSeen,omitempty"`
}

// Server is the HTTP API server instance.
type Server struct {
	cfg      Config
	hub      *wsHub
	debugHub *wsHub
	router   chi.Router
}

// New creates a new Server. Call Run to start listening.
func New(cfg Config) *Server {
	s := &Server{
		cfg:      cfg,
		hub:      newWSHub(),
		debugHub: newWSHub(),
	}
	s.router = s.buildRouter()
	return s
}

// NotifyChange fans out a state-change notification to all WebSocket clients.
// Call this whenever a device state changes (from the push-notification pipeline).
func (s *Server) NotifyChange(dsuid string, changed map[string]any) {
	s.hub.broadcast(wsEvent{Type: "stateChange", DSUID: dsuid, Data: changed})
}

// Notify fans out an arbitrary event to all WebSocket clients. Used by the
// bridge registry to publish bridgeAdded / bridgeRemoved events.
func (s *Server) Notify(eventType string, data map[string]any) {
	dsuid, _ := data["dsuid"].(string)
	s.hub.broadcast(wsEvent{Type: eventType, DSUID: dsuid, Data: data})
}

// Handler returns the underlying http.Handler for use in tests.
func (s *Server) Handler() http.Handler {
	return s.router
}

// Run starts the HTTP listener and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	go s.hub.run(ctx)
	go s.debugHub.run(ctx)

	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return err
	}
	logging.Info("httpapi_listening", logging.Fields{"addr": ln.Addr().String()})

	httpSrv := &http.Server{
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // WebSocket streams; individual handlers set their own deadlines
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logging.Warn("httpapi_shutdown_error", logging.Fields{"error": err})
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// NotifyPbufTrace fans out a decoded protobuf trace frame to debug subscribers.
func (s *Server) NotifyPbufTrace(f vdcapi.PbufTraceFrame) {
	if s.debugHub == nil {
		return
	}
	s.debugHub.broadcast(wsEvent{
		Type: "pbuf",
		Data: map[string]any{
			"time":        f.Time,
			"direction":   f.Direction,
			"typeNum":     f.TypeNum,
			"typeName":    f.TypeName,
			"msgId":       f.MsgID,
			"hasMsgId":    f.HasMsgID,
			"deviceDSUID": f.DeviceDSUID,
			"decoded":     f.Decoded,
			"rawHex":      f.RawHex,
		},
	})
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/api/health", s.handleHealth)
	r.Get("/api/dss", s.handleDSS)
	r.Get("/api/devices", s.handleDevices)
	r.Get("/api/devices/{dsuid}", s.handleDevice)
	r.Get("/api/events", s.handleEvents)
	r.Get("/api/debug/pbuf", s.handleDebugPbuf)

	// Bridge / plugin endpoints (Sprint W3)
	r.Get("/api/plugins", s.handlePlugins)
	r.Post("/api/plugins", s.handleCreatePlugin)
	r.Delete("/api/plugins/{id}", s.handleDeletePlugin)
	r.Get("/api/plugins/{id}/config", s.handlePluginConfig)
	r.Put("/api/plugins/{id}/config", s.handlePluginConfigUpdate)
	r.Post("/api/plugins/{id}/probe", s.handlePluginProbe)
	r.Get("/api/plugins/{id}/discovered", s.handlePluginDiscovered)
	r.Get("/api/plugin-types", s.handlePluginTypes)
	r.Post("/api/plugin-types/{type}/probe", s.handlePluginTypeProbe)
	r.Get("/api/bridges", s.handleBridges)
	r.Post("/api/bridges", s.handleCreateBridge)
	r.Delete("/api/bridges/{dsuid}", s.handleDeleteBridge)

	// Settings endpoints
	r.Get("/api/settings", s.handleSettings)
	r.Post("/api/settings/forget-vdsm", s.handleForgetVdsm)
	r.Get("/api/settings/export", s.handleExportConfig)

	// Static files (web/dist embedded). Must come last.
	r.Handle("/*", staticHandler())

	return r
}

// corsMiddleware adds permissive CORS headers so the Vite dev server
// (which runs on a different port) can reach the API.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
