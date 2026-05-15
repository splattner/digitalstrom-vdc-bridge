package bridge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/splattner/vdcgo/pkg/logging"
)

// Notifier is invoked by the Registry on lifecycle events that the rest of
// the system (typically the HTTP API push pipeline) wants to observe.
type Notifier func(eventType string, data map[string]any)

// Persister persists the full plugin config list to disk. The Registry calls
// it after AddPlugin / RemovePlugin / UpdatePlugin succeed so that changes
// survive a restart. May be nil if persistence is disabled.
type Persister func(configs []PluginConfig) error

// Registry manages plugin instances and the bridge mapping lifecycle.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]FactoryEntry // type → factory entry
	instances map[string]Plugin       // id → running plugin
	configs   map[string]PluginConfig // id → last-applied config
	mappings  *MappingStore
	host      Host
	ctx       context.Context
	notify    Notifier
	persist   Persister
	sink      EventSink
}

// SetNotifier installs a notifier callback. Safe to call before or after Start.
func (r *Registry) SetNotifier(n Notifier) {
	r.mu.Lock()
	r.notify = n
	r.mu.Unlock()
}

// SetEventSink installs the EventSink that receives PluginEvents emitted by
// plugins and registry lifecycle actions. Safe to call at any time.
func (r *Registry) SetEventSink(s EventSink) {
	r.mu.Lock()
	r.sink = s
	r.mu.Unlock()
}

// getSink returns the current EventSink under the read lock. Used as a closure
// passed to pluginHost so that plugins always see the latest sink.
func (r *Registry) getSink() EventSink {
	r.mu.RLock()
	s := r.sink
	r.mu.RUnlock()
	return s
}

// sinkEmit publishes a lifecycle PluginEvent to the EventSink (if set).
func (r *Registry) sinkEmit(pluginID string, level LogLevel, code, message string, fields map[string]any) {
	s := r.getSink()
	if s == nil {
		return
	}
	s.Publish(PluginEvent{
		PluginID: pluginID,
		Level:    level,
		Code:     code,
		Message:  message,
		Fields:   fields,
	})
}

// EmitPluginEvent publishes a PluginEvent on behalf of a plugin. Used by
// helpers (e.g. the Commander) that have no direct access to a per-plugin
// Host wrapper but still need to surface plugin-scoped log entries.
func (r *Registry) EmitPluginEvent(pluginID string, level LogLevel, code, message string, fields map[string]any) {
	r.sinkEmit(pluginID, level, code, message, fields)
}

func (r *Registry) emit(t string, data map[string]any) {
	r.mu.RLock()
	n := r.notify
	r.mu.RUnlock()
	if n != nil {
		n(t, data)
	}
}

// NewRegistry creates an empty Registry backed by the given host and mapping store.
func NewRegistry(host Host, mappings *MappingStore) *Registry {
	return &Registry{
		factories: make(map[string]FactoryEntry),
		instances: make(map[string]Plugin),
		configs:   make(map[string]PluginConfig),
		mappings:  mappings,
		host:      host,
	}
}

// RegisterFactory makes a plugin type available for config-driven instantiation.
// Equivalent to Register(typeName, FactoryEntry{Factory: f}). Kept for
// backwards compatibility with plugins that have not declared a schema.
func (r *Registry) RegisterFactory(typeName string, f Factory) {
	r.Register(typeName, FactoryEntry{Factory: f, DisplayName: typeName})
}

// Register makes a plugin type available with full metadata (display name,
// description, schema, optional probe).
func (r *Registry) Register(typeName string, entry FactoryEntry) {
	if entry.DisplayName == "" {
		entry.DisplayName = typeName
	}
	r.mu.Lock()
	r.factories[typeName] = entry
	r.mu.Unlock()
}

// FactoryTypes returns the list of registered plugin type names.
func (r *Registry) FactoryTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for t := range r.factories {
		out = append(out, t)
	}
	return out
}

// FactoryEntry returns the metadata for a registered plugin type.
func (r *Registry) FactoryEntry(typeName string) (FactoryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.factories[typeName]
	return e, ok
}

// SetPersister installs a callback to persist the current plugin config list.
// Called whenever the configured plugin set changes.
func (r *Registry) SetPersister(p Persister) {
	r.mu.Lock()
	r.persist = p
	r.mu.Unlock()
}

// Configs returns the currently loaded plugin configs (snapshot).
func (r *Registry) Configs() []PluginConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PluginConfig, 0, len(r.configs))
	for _, c := range r.configs {
		out = append(out, c)
	}
	return out
}

// Config returns the loaded config for the given plugin instance id.
func (r *Registry) Config(id string) (PluginConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.configs[id]
	return c, ok
}

// Start initialises all plugins from configs and restores persisted mappings.
// It must be called once before CreateBridge / RemoveBridge.
func (r *Registry) Start(ctx context.Context, configs []PluginConfig) error {
	r.mu.Lock()
	r.ctx = ctx
	r.mu.Unlock()

	for _, pc := range configs {
		if err := r.startPlugin(ctx, pc); err != nil {
			logging.Warn("bridge_plugin_start_error", logging.Fields{
				"id":    pc.ID,
				"type":  pc.Type,
				"error": err.Error(),
			})
			// Non-fatal: keep going so other plugins start.
		}
	}

	// Restore persisted mappings — announce each device back into the state store
	// and subscribe the owning plugin so it starts forwarding updates.
	for _, m := range r.mappings.List() {
		r.mu.RLock()
		p, ok := r.instances[m.PluginID]
		r.mu.RUnlock()
		if !ok {
			logging.Warn("bridge_orphan_mapping", logging.Fields{"plugin_id": m.PluginID, "dsuid": m.DSUID})
			continue
		}
		if err := r.host.AnnounceDevice(ctx, m); err != nil {
			logging.Warn("bridge_restore_announce_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
		}
		if err := p.Subscribe(ctx, m); err != nil {
			logging.Warn("bridge_restore_subscribe_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
		}
		logging.Info("bridge_mapping_restored", logging.Fields{"plugin_id": m.PluginID, "dsuid": m.DSUID, "name": m.Name})
	}
	return nil
}

// startPlugin instantiates and initialises a single plugin.
func (r *Registry) startPlugin(ctx context.Context, pc PluginConfig) error {
	r.mu.RLock()
	entry, ok := r.factories[pc.Type]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown plugin type %q", pc.Type)
	}

	resolvedCfg := applyEnvOverlay(pc.ID, pc.Config)

	p := entry.Factory(pc.ID)
	// Wrap the shared host with a per-plugin facade that auto-tags Log events.
	ph := &pluginHost{Host: r.host, pluginID: pc.ID, getSink: r.getSink}
	if err := p.Init(ctx, resolvedCfg, ph); err != nil {
		r.sinkEmit(pc.ID, LevelError, CodeConnectFailed, "plugin init failed", map[string]any{"error": err.Error(), "type": pc.Type})
		return err
	}

	r.mu.Lock()
	r.instances[pc.ID] = p
	r.configs[pc.ID] = pc
	r.mu.Unlock()

	logging.Info("bridge_plugin_started", logging.Fields{"id": pc.ID, "type": pc.Type})
	r.sinkEmit(pc.ID, LevelInfo, CodePluginStarted, "plugin started", map[string]any{"type": pc.Type})
	return nil
}

// AddPlugin starts a brand-new plugin instance and persists the config list.
// Fails if the id already exists or the type is unknown.
func (r *Registry) AddPlugin(ctx context.Context, pc PluginConfig) error {
	if pc.ID == "" {
		return fmt.Errorf("plugin id is required")
	}
	r.mu.RLock()
	_, exists := r.instances[pc.ID]
	r.mu.RUnlock()
	if exists {
		return fmt.Errorf("plugin %q already exists", pc.ID)
	}
	if err := r.startPlugin(ctx, pc); err != nil {
		return err
	}
	r.persistConfigs()
	r.emit("pluginAdded", map[string]any{"id": pc.ID, "type": pc.Type})
	r.sinkEmit(pc.ID, LevelInfo, CodePluginStarted, "plugin added", map[string]any{"type": pc.Type})
	return nil
}

// RemovePlugin stops a plugin and removes any of its bridge mappings.
func (r *Registry) RemovePlugin(ctx context.Context, id string) error {
	r.mu.Lock()
	p, ok := r.instances[id]
	delete(r.instances, id)
	delete(r.configs, id)
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("plugin %q not found", id)
	}

	// Tear down all bridge mappings owned by this plugin.
	for _, m := range r.mappings.ListForPlugin(id) {
		_ = p.Unsubscribe(ctx, m.DSUID)
		_ = r.host.RemoveDevice(ctx, m.DSUID)
		_, _ = r.mappings.Remove(m.DSUID)
	}

	if err := p.Close(); err != nil {
		logging.Warn("bridge_plugin_close_error", logging.Fields{"id": id, "error": err.Error()})
	}
	r.persistConfigs()
	r.emit("pluginRemoved", map[string]any{"id": id})
	r.sinkEmit(id, LevelInfo, CodePluginStopped, "plugin removed", nil)
	return nil
}

// UpdatePlugin replaces a plugin's config in place: stops the running
// instance, starts a fresh one with the new config, and re-subscribes any
// existing mappings.
func (r *Registry) UpdatePlugin(ctx context.Context, pc PluginConfig) error {
	r.mu.Lock()
	old, ok := r.instances[pc.ID]
	delete(r.instances, pc.ID)
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("plugin %q not found", pc.ID)
	}
	if err := old.Close(); err != nil {
		logging.Warn("bridge_plugin_close_error", logging.Fields{"id": pc.ID, "error": err.Error()})
	}
	if err := r.startPlugin(ctx, pc); err != nil {
		return err
	}
	// Re-subscribe persisted mappings on the fresh instance.
	r.mu.RLock()
	fresh := r.instances[pc.ID]
	r.mu.RUnlock()
	if fresh != nil {
		for _, m := range r.mappings.ListForPlugin(pc.ID) {
			if err := fresh.Subscribe(ctx, m); err != nil {
				logging.Warn("bridge_resubscribe_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
			}
		}
	}
	r.persistConfigs()
	r.emit("pluginUpdated", map[string]any{"id": pc.ID, "type": pc.Type})
	r.sinkEmit(pc.ID, LevelInfo, CodePluginRestarted, "plugin config updated and restarted", map[string]any{"type": pc.Type})
	return nil
}

// persistConfigs invokes the registered Persister with the current config list.
func (r *Registry) persistConfigs() {
	r.mu.RLock()
	p := r.persist
	list := make([]PluginConfig, 0, len(r.configs))
	for _, c := range r.configs {
		list = append(list, c)
	}
	r.mu.RUnlock()
	if p == nil {
		return
	}
	if err := p(list); err != nil {
		logging.Warn("plugins_persist_error", logging.Fields{"error": err.Error()})
	}
}

// RuntimeContext returns the lifetime context that was passed to Start.
func (r *Registry) RuntimeContext() context.Context {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ctx
}

// Instances returns a snapshot of all running plugin instances.
func (r *Registry) Instances() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, 0, len(r.instances))
	for _, p := range r.instances {
		out = append(out, p)
	}
	return out
}

// Plugin returns the plugin instance with the given id, or false if not found.
func (r *Registry) Plugin(id string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.instances[id]
	return p, ok
}

// Mappings returns the underlying MappingStore.
func (r *Registry) Mappings() *MappingStore {
	return r.mappings
}

// CreateBridge creates a new bridge mapping: derives the DSUID, persists the
// mapping, announces the device, and subscribes the plugin.
func (r *Registry) CreateBridge(ctx context.Context, pluginID, remoteEntityID, name, kind string) (Mapping, error) {
	r.mu.RLock()
	p, ok := r.instances[pluginID]
	r.mu.RUnlock()
	if !ok {
		return Mapping{}, fmt.Errorf("plugin %q not found", pluginID)
	}

	if _, exists := r.mappings.GetByRemote(pluginID, remoteEntityID); exists {
		return Mapping{}, fmt.Errorf("entity %q of plugin %q is already bridged", remoteEntityID, pluginID)
	}

	dsuid := r.host.DeriveDSUID(pluginID, remoteEntityID)
	m := Mapping{
		PluginID:       pluginID,
		RemoteEntityID: remoteEntityID,
		DSUID:          dsuid,
		Kind:           kind,
		Name:           name,
	}

	if _, err := r.mappings.Add(m); err != nil {
		return Mapping{}, fmt.Errorf("persist mapping: %w", err)
	}
	if err := r.host.AnnounceDevice(ctx, m); err != nil {
		_, _ = r.mappings.Remove(dsuid)
		return Mapping{}, fmt.Errorf("announce device: %w", err)
	}
	if err := p.Subscribe(ctx, m); err != nil {
		// Non-fatal — device is announced, just won't get live updates until Subscribe succeeds.
		logging.Warn("bridge_subscribe_error", logging.Fields{"dsuid": dsuid, "error": err.Error()})
	}

	logging.Info("bridge_created", logging.Fields{"plugin_id": pluginID, "remote_id": remoteEntityID, "dsuid": dsuid, "name": name})
	r.emit("bridgeAdded", map[string]any{
		"pluginId":       m.PluginID,
		"remoteEntityId": m.RemoteEntityID,
		"dsuid":          m.DSUID,
		"name":           m.Name,
		"kind":           m.Kind,
	})
	r.sinkEmit(pluginID, LevelInfo, CodeEntityAdded, "bridge mapping created", map[string]any{"dsuid": dsuid, "remoteId": remoteEntityID, "name": name, "kind": kind})
	return m, nil
}

// RemoveBridge removes a bridge mapping by DSUID.
func (r *Registry) RemoveBridge(ctx context.Context, dsuid string) error {
	m, ok := r.mappings.Get(dsuid)
	if !ok {
		return fmt.Errorf("bridge %q not found", dsuid)
	}

	r.mu.RLock()
	p, hasPlugin := r.instances[m.PluginID]
	r.mu.RUnlock()
	if hasPlugin {
		if err := p.Unsubscribe(ctx, dsuid); err != nil {
			logging.Warn("bridge_unsubscribe_error", logging.Fields{"dsuid": dsuid, "error": err.Error()})
		}
	}

	if err := r.host.RemoveDevice(ctx, dsuid); err != nil {
		logging.Warn("bridge_remove_device_error", logging.Fields{"dsuid": dsuid, "error": err.Error()})
	}

	if _, err := r.mappings.Remove(dsuid); err != nil {
		return fmt.Errorf("remove mapping: %w", err)
	}

	logging.Info("bridge_removed", logging.Fields{"dsuid": dsuid})
	r.emit("bridgeRemoved", map[string]any{
		"pluginId": m.PluginID,
		"dsuid":    dsuid,
		"name":     m.Name,
	})
	r.sinkEmit(m.PluginID, LevelInfo, CodeEntityRemoved, "bridge mapping removed", map[string]any{"dsuid": dsuid, "name": m.Name})
	return nil
}

// Stop shuts down all plugins gracefully.
func (r *Registry) Stop() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.instances {
		if err := p.Close(); err != nil {
			logging.Warn("bridge_plugin_close_error", logging.Fields{"id": p.ID(), "error": err.Error()})
		}
	}
}

// applyEnvOverlay merges environment variable overrides into cfg.
// For a plugin with id="ha-main", the config key "url" is overridden by
// VDCGO_HA_MAIN_URL (id normalised: uppercased, "-" → "_").
func applyEnvOverlay(pluginID string, cfg map[string]any) map[string]any {
	result := make(map[string]any, len(cfg))
	for k, v := range cfg {
		result[k] = v
	}
	prefix := "VDCGO_" + strings.ToUpper(strings.ReplaceAll(pluginID, "-", "_")) + "_"
	for _, env := range os.Environ() {
		eq := strings.IndexByte(env, '=')
		if eq <= 0 {
			continue
		}
		key, val := env[:eq], env[eq+1:]
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		cfgKey := strings.ToLower(strings.TrimPrefix(key, prefix))
		result[cfgKey] = val
		logging.Debug("bridge_env_override", logging.Fields{"plugin_id": pluginID, "key": cfgKey})
	}
	return result
}
