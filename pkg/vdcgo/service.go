package vdcgo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/bridge"
	"github.com/splattner/vdcgo/pkg/bridge/externaldevice"
	"github.com/splattner/vdcgo/pkg/bridge/homeassistant"
	mqttplugin "github.com/splattner/vdcgo/pkg/bridge/mqtt"
	"github.com/splattner/vdcgo/pkg/bridge/tasmota"
	"github.com/splattner/vdcgo/pkg/bridge/wled"
	"github.com/splattner/vdcgo/pkg/bridge/zigbee2mqtt"
	"github.com/splattner/vdcgo/pkg/discovery"
	"github.com/splattner/vdcgo/pkg/httpapi"
	"github.com/splattner/vdcgo/pkg/logging"
	mqttsvc "github.com/splattner/vdcgo/pkg/services/mqtt"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

// Version is the build version reported by /api/settings. Override at link
// time with -ldflags "-X github.com/splattner/vdcgo/pkg/vdcgo.Version=..." or
// from main before constructing a Service.
var Version = "dev"

// Config defines daemon/service behavior.
type Config struct {
	NonLocal     bool
	VdcAPIPort   int
	EnableVdcAPI bool
	EnableDNSSD  bool
	// UseAvahiDBus selects the Avahi D-Bus backend for DNS-SD advertisement
	// instead of opening raw multicast sockets. Requires the host D-Bus socket
	// to be accessible (host_dbus: true in a Home Assistant add-on).
	UseAvahiDBus bool
	DSUID        string
	Description  string
	Vendor       string
	Model        string
	NoAuto       bool
	// DataDir is the directory for persistent data (scenes, device config).
	// If empty, no data is persisted across restarts.
	DataDir string
	// HTTPListen is the address for the REST/WebSocket HTTP API, e.g. ":8090".
	// Empty string disables the HTTP API.
	HTTPListen string
	// PluginConfigs is the list of bridge plugin instances to start.
	// Each entry describes a plugin type and its configuration.
	PluginConfigs []bridge.PluginConfig
}

// Service runs the bridge and vDC API host.
type Service struct {
	cfg        Config
	announce   *discovery.Advertiser
	state      *vdcapi.StateStore
	scenes     *vdcapi.SceneStore
	config     *vdcapi.ConfigStore
	httpServer *httpapi.Server
	bridges    *bridge.Registry
	pbufServer *vdcapi.PbufServer
}

// NewService creates a configured service instance.
func NewService(cfg Config) (*Service, error) {
	if cfg.DSUID == "" {
		// Try to load a previously persisted vDC DSUID so it survives restarts.
		if cfg.DataDir != "" {
			if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
				return nil, fmt.Errorf("create data dir %q: %w", cfg.DataDir, err)
			}
			dsuidPath := filepath.Join(cfg.DataDir, "dsuid")
			if stored, err := os.ReadFile(dsuidPath); err == nil {
				candidate := strings.TrimSpace(string(stored))
				if IsValidDSUID(candidate) {
					cfg.DSUID = candidate
					logging.Info("dsuid_loaded", logging.Fields{"dsuid": cfg.DSUID})
				}
			}
		}
		if cfg.DSUID == "" {
			// No persisted DSUID — generate a fresh one.
			generated, err := GenerateDSUID()
			if err != nil {
				return nil, fmt.Errorf("generate dSUID: %w", err)
			}
			cfg.DSUID = generated
			// Persist immediately so the next restart reuses the same DSUID.
			if cfg.DataDir != "" {
				dsuidPath := filepath.Join(cfg.DataDir, "dsuid")
				if err := os.WriteFile(dsuidPath, []byte(cfg.DSUID+"\n"), 0o644); err != nil {
					return nil, fmt.Errorf("persist dSUID: %w", err)
				}
				logging.Info("dsuid_generated_and_persisted", logging.Fields{"dsuid": cfg.DSUID})
			} else {
				logging.Info("dsuid_generated_ephemeral", logging.Fields{
					"dsuid":   cfg.DSUID,
					"warning": "no --datadir set; dSUID will change on restart",
				})
			}
		}
	}
	if !IsValidDSUID(cfg.DSUID) {
		return nil, fmt.Errorf("invalid dSUID %q: expected 34 hex chars", cfg.DSUID)
	}
	if cfg.VdcAPIPort == 0 {
		cfg.VdcAPIPort = 8340
	}
	if cfg.Description == "" {
		cfg.Description = "vdcgo external"
	}
	if cfg.Vendor == "" {
		cfg.Vendor = "github.com/splattner"
	}
	if cfg.Model == "" {
		cfg.Model = "vdcgo"
	}
	state := vdcapi.NewStateStore()
	scenes := vdcapi.NewSceneStore()
	configStore := vdcapi.NewConfigStore()

	if cfg.DataDir != "" {
		// MkdirAll was already called above when persisting/loading the dSUID.
		// If DataDir was not needed for dSUID (explicit --dsuid given), create it now.
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir %q: %w", cfg.DataDir, err)
		}
		scenesPath := filepath.Join(cfg.DataDir, "scenes.json")
		configPath := filepath.Join(cfg.DataDir, "config.json")
		if err := scenes.LoadFromFile(scenesPath); err != nil {
			return nil, fmt.Errorf("load scenes: %w", err)
		}
		if err := configStore.LoadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		scenes.SetAutoSave(scenesPath)
		configStore.SetAutoSave(configPath)
		logging.Info("persistence_enabled", logging.Fields{"data_dir": cfg.DataDir})

		// Optional plugins.json — loaded only when not already supplied via Config.
		if len(cfg.PluginConfigs) == 0 {
			pluginsPath := filepath.Join(cfg.DataDir, "plugins.json")
			loaded, err := loadPluginConfigs(pluginsPath)
			if err != nil {
				return nil, fmt.Errorf("load plugins: %w", err)
			}
			if len(loaded) > 0 {
				cfg.PluginConfigs = loaded
				logging.Info("plugins_loaded", logging.Fields{"path": pluginsPath, "count": len(loaded)})
			}
		}

		// identity.json — persisted description/vendor/model overrides CLI flags.
		identityPath := filepath.Join(cfg.DataDir, "identity.json")
		if raw, err := os.ReadFile(identityPath); err == nil {
			var id struct {
				Description string `json:"description"`
				Vendor      string `json:"vendor"`
				Model       string `json:"model"`
			}
			if json.Unmarshal(raw, &id) == nil {
				if id.Description != "" {
					cfg.Description = id.Description
				}
				if id.Vendor != "" {
					cfg.Vendor = id.Vendor
				}
				if id.Model != "" {
					cfg.Model = id.Model
				}
				logging.Info("identity_loaded", logging.Fields{"path": identityPath})
			}
		}
	}

	// Build bridge registry (always created, even if no plugins are configured).
	mappings := bridge.NewMappingStore()
	if cfg.DataDir != "" {
		mappingsPath := filepath.Join(cfg.DataDir, "bridges.json")
		if err := mappings.LoadFromFile(mappingsPath); err != nil {
			return nil, fmt.Errorf("load bridge mappings: %w", err)
		}
		mappings.SetAutoSave(mappingsPath)
	}
	host := bridge.NewHost(state, mqttsvc.NewManager())
	bridgeRegistry := bridge.NewRegistry(host, mappings)

	// Register built-in plugin factories with full FactoryEntry metadata so the
	// HTTP API / web UI can render schema-driven config forms.
	bridgeRegistry.Register(externaldevice.PluginType, externaldevice.RegisterEntry())
	bridgeRegistry.Register(mqttplugin.PluginType, mqttplugin.RegisterEntry())
	bridgeRegistry.Register(homeassistant.PluginType, homeassistant.RegisterEntry())
	bridgeRegistry.Register(tasmota.PluginType, tasmota.RegisterEntry())
	bridgeRegistry.Register(wled.PluginType, wled.RegisterEntry())
	bridgeRegistry.Register(zigbee2mqtt.PluginType, zigbee2mqtt.RegisterEntry())

	// Persist plugin config changes (made via the HTTP API) to plugins.json.
	if cfg.DataDir != "" {
		pluginsPath := filepath.Join(cfg.DataDir, "plugins.json")
		bridgeRegistry.SetPersister(func(list []bridge.PluginConfig) error {
			return savePluginConfigs(pluginsPath, list)
		})
	}

	// Create the ring-buffer that collects per-plugin structured events and
	// wire it into the registry before any plugins are started.
	eventBuf := bridge.NewEventBuffer(500, 2000)
	bridgeRegistry.SetEventSink(eventBuf)

	// Create the ring-buffer that records per-device activity events (channel
	// changes, active-state transitions) with source attribution (vdsm vs plugin).
	activityBuf := bridge.NewActivityBuffer(200, 2000)
	bridgeRegistry.SetActivityBuffer(activityBuf)

	cmdr := bridge.NewCommander(bridgeRegistry, nil)
	cmdr.SetActivityBuffer(activityBuf)

	// Create the service skeleton so closures below can capture it.
	svc := &Service{cfg: cfg, state: state, scenes: scenes, config: configStore, bridges: bridgeRegistry}

	// Pre-create PbufServer so SessionInfo can read live session state.
	if cfg.EnableVdcAPI {
		svc.pbufServer = &vdcapi.PbufServer{
			ServerConfig: vdcapi.ServerConfig{
				Port:        cfg.VdcAPIPort,
				DSUID:       cfg.DSUID,
				Description: cfg.Description,
				State:       state,
				Commander:   cmdr,
				Scenes:      scenes,
				Config:      configStore,
				OnTrace: func(f vdcapi.PbufTraceFrame) {
					if svc.httpServer != nil {
						svc.httpServer.NotifyPbufTrace(f)
					}
				},
			},
		}
	}

	if cfg.HTTPListen != "" {
		svc.httpServer = httpapi.New(httpapi.Config{
			Listen:      cfg.HTTPListen,
			DSUID:       cfg.DSUID,
			Description: cfg.Description,
			Vendor:      cfg.Vendor,
			Model:       cfg.Model,
			State:       state,
			Config:      configStore,
			Scenes:      scenes,
			Bridges:        bridgeRegistry,
			EventBuffer:    eventBuf,
			ActivityBuffer: activityBuf,
			VdcAPIPort:  cfg.VdcAPIPort,
			EnableDNSSD: cfg.EnableDNSSD,
			NonLocal:    cfg.NonLocal,
			NoAuto:      cfg.NoAuto,
			DataDir:     cfg.DataDir,
			Version:     Version,
			SessionInfo: func() httpapi.DSSSessionInfo {
				if svc.pbufServer == nil {
					return httpapi.DSSSessionInfo{}
				}
				snap := svc.pbufServer.Session()
				return httpapi.DSSSessionInfo{
					Connected:   snap.Connected,
					VdsmDSUID:   snap.VdsmDSUID,
					APIVersion:  snap.APIVersion,
					RemoteAddr:  snap.RemoteAddr,
					ConnectedAt: snap.ConnectedAt,
				}
			},
		})

		// Forward bridge lifecycle events to WebSocket clients.
		bridgeRegistry.SetNotifier(func(eventType string, data map[string]any) {
			if svc.httpServer != nil {
				svc.httpServer.Notify(eventType, data)
			}
		})
	}

	return svc, nil
}

// Config returns the resolved runtime config.
func (s *Service) Config() Config {
	return s.cfg
}

// Run starts listening and serving until context cancellation.
func (s *Service) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if s.cfg.EnableDNSSD {
		adv, err := discovery.Start(discovery.Config{
			Instance:     s.cfg.Description,
			Port:         s.cfg.VdcAPIPort,
			DSUID:        s.cfg.DSUID,
			NoAuto:       s.cfg.NoAuto,
			UseAvahiDBus: s.cfg.UseAvahiDBus,
		})
		if err != nil {
			return err
		}
		s.announce = adv
		defer s.announce.Shutdown()
		logging.Info("dnssd_advertised", logging.Fields{
			"service_type": discovery.ServiceTypeDSVDC,
			"port":         s.cfg.VdcAPIPort,
			"dsuid":        s.cfg.DSUID,
		})
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	if s.cfg.EnableVdcAPI && s.pbufServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.pbufServer.Run(runCtx); err != nil {
				errCh <- err
			}
		}()
	}

	// Start bridge plugins and restore persisted mappings.
	if err := s.bridges.Start(runCtx, s.cfg.PluginConfigs); err != nil {
		logging.Warn("bridge_start_error", logging.Fields{"error": err.Error()})
	}

	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.httpServer.Run(runCtx); err != nil {
				errCh <- err
			}
		}()

		// Fan out state updates → WebSocket push notifications.
		subID, updates := s.state.Subscribe()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer s.state.Unsubscribe(subID)
			for {
				select {
				case u, ok := <-updates:
					if !ok {
						return
					}
					dsuid := vdcapi.DeviceDSUID(s.cfg.DSUID, u.Device, u.Device.Key)
					s.httpServer.NotifyChange(dsuid, map[string]any{"eventType": u.Type})
				case <-runCtx.Done():
					return
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		cancel()
		wg.Wait()
		s.bridges.Stop()
		return nil
	case err := <-errCh:
		cancel()
		wg.Wait()
		s.bridges.Stop()
		return err
	}
}
