package bridge

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/splattner/vdcgo/pkg/logging"
)

// supervisorInterval and supervisorBackoffMax govern the background
// supervisor loop that retries a plugin whose Init failed (e.g. Init failed
// at daemon startup, or a manual RestartPlugin still failed). Package vars,
// not constants, so tests can override them for fast, deterministic runs —
// same pattern as pkg/vdcapi's dimRampTickInterval.
var (
	supervisorInterval  = 30 * time.Second
	supervisorBackoffMax = 5 * time.Minute
)

// retryState tracks a single plugin's automatic-retry backoff.
type retryState struct {
	nextAt time.Time
	delay  time.Duration
}

// Notifier is invoked by the Registry on lifecycle events that the rest of
// the system (typically the HTTP API push pipeline) wants to observe.
type Notifier func(eventType string, data map[string]any)

// Persister persists the full plugin config list to disk. The Registry calls
// it after AddPlugin / RemovePlugin / UpdatePlugin succeed so that changes
// survive a restart. May be nil if persistence is disabled.
type Persister func(configs []PluginConfig) error

// Registry manages plugin instances and the bridge mapping lifecycle.
type Registry struct {
	mu             sync.RWMutex
	factories      map[string]FactoryEntry // type → factory entry
	instances      map[string]Plugin       // id → running plugin
	configs        map[string]PluginConfig // id → last-applied config
	mappings       *MappingStore
	host           Host
	ctx            context.Context
	notify         Notifier
	persist        Persister
	sink           EventSink
	activityBuffer *ActivityBuffer
	// retryBackoff tracks automatic-retry state for plugins whose most
	// recent startPlugin attempt failed (see the supervisor loop started by
	// Start). Cleared on the next successful startPlugin for that id.
	retryBackoff map[string]retryState
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

// SetActivityBuffer installs the ActivityBuffer used to record per-device activity.
// Safe to call at any time; plugins created before this call will see it once started.
func (r *Registry) SetActivityBuffer(ab *ActivityBuffer) {
	r.mu.Lock()
	r.activityBuffer = ab
	r.mu.Unlock()
}

// getActivityBuffer returns the current ActivityBuffer under the read lock.
func (r *Registry) getActivityBuffer() *ActivityBuffer {
	r.mu.RLock()
	ab := r.activityBuffer
	r.mu.RUnlock()
	return ab
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

// notifyDiscoveryChanged emits a "discoveryChanged" event for pluginID via
// the registered Notifier (see SetNotifier) — the HTTP API forwards it to
// WebSocket clients so the web UI's Discovered page can refresh that
// plugin's list immediately instead of waiting for its poll interval.
func (r *Registry) notifyDiscoveryChanged(pluginID string) {
	r.emit("discoveryChanged", map[string]any{"pluginId": pluginID})
}

// NewRegistry creates an empty Registry backed by the given host and mapping store.
func NewRegistry(host Host, mappings *MappingStore) *Registry {
	return &Registry{
		factories:    make(map[string]FactoryEntry),
		instances:    make(map[string]Plugin),
		configs:      make(map[string]PluginConfig),
		mappings:     mappings,
		host:         host,
		retryBackoff: make(map[string]retryState),
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

	r.startSupervisor(ctx)

	for _, pc := range configs {
		if pc.Disabled {
			// Remember the config so it can be re-enabled later, but do not
			// instantiate the plugin and do not subscribe its mappings.
			r.mu.Lock()
			r.configs[pc.ID] = pc
			r.mu.Unlock()
			r.sinkEmit(pc.ID, LevelInfo, CodePluginStopped, "plugin disabled, not started", map[string]any{"type": pc.Type})
			continue
		}
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
		p, running := r.instances[m.PluginID]
		_, hasCfg := r.configs[m.PluginID]
		r.mu.RUnlock()
		if !running && !hasCfg {
			// No plugin config owns this mapping at all — nothing can ever
			// drive it, so announcing a permanently dead device would be
			// worse than leaving it out.
			logging.Warn("bridge_orphan_mapping", logging.Fields{"plugin_id": m.PluginID, "dsuid": m.DSUID})
			continue
		}
		// Announce regardless of whether the plugin is running. A plugin that
		// is disabled, or whose Init failed and is waiting on a supervisor
		// retry, must still have its device exist (as inactive) — otherwise
		// the device silently does not exist at all, the dSS is told to
		// vanish it on the next vDSM handshake, and nothing ever announces it
		// again even after the plugin recovers, while the mapping keeps
		// reporting it as bridged.
		r.ensureAnnounced(ctx, m)
		if !running {
			r.markInactive(ctx, m)
			continue
		}
		if err := p.Subscribe(ctx, m); err != nil {
			logging.Warn("bridge_restore_subscribe_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
		}
		logging.Info("bridge_mapping_restored", logging.Fields{"plugin_id": m.PluginID, "dsuid": m.DSUID, "name": m.Name})
	}
	return nil
}

// ensureAnnounced announces m's device unless it is already present in the
// vDC state store. Announcing is not free at the protocol level — the pbuf
// server turns it into a vanish/announce pair so the dSS re-queries every
// property — so this deliberately does nothing for a device that already
// exists.
func (r *Registry) ensureAnnounced(ctx context.Context, m Mapping) {
	if r.host.HasDevice(m.DSUID) {
		return
	}
	if err := r.host.AnnounceDevice(ctx, m); err != nil {
		logging.Warn("bridge_restore_announce_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
		return
	}
	logging.Info("bridge_mapping_announced", logging.Fields{"plugin_id": m.PluginID, "dsuid": m.DSUID, "name": m.Name})
}

// markInactive flags a just-announced device as offline, for a mapping whose
// plugin is not currently running. The device exists (so it survives the vDSM
// handshake's stale-device reconciliation and can recover) but is honestly
// reported as unreachable rather than silently stuck at its last value.
func (r *Registry) markInactive(ctx context.Context, m Mapping) {
	if err := r.host.UpdateActive(ctx, m.DSUID, false); err != nil {
		logging.Warn("bridge_restore_inactive_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
	}
}

// restoreMappings re-establishes every mapping owned by pluginID against a
// freshly started instance: the device is announced if it is missing from the
// state store, then subscribed. Subscribe alone is not enough — every state
// update for a device the store does not know about is silently dropped, so a
// mapping that was never announced would stay invisible forever.
func (r *Registry) restoreMappings(ctx context.Context, pluginID string, p Plugin) {
	for _, m := range r.mappings.ListForPlugin(pluginID) {
		r.ensureAnnounced(ctx, m)
		if err := p.Subscribe(ctx, m); err != nil {
			logging.Warn("bridge_resubscribe_error", logging.Fields{"dsuid": m.DSUID, "error": err.Error()})
		}
	}
}

// startPlugin instantiates and initialises a single plugin.
func (r *Registry) startPlugin(ctx context.Context, pc PluginConfig) error {
	// Retain the config regardless of what happens below — a plugin that
	// fails to start must still be visible/restartable (via the manual
	// Restart API, or the supervisor loop) rather than vanishing until the
	// whole daemon restarts.
	r.mu.Lock()
	r.configs[pc.ID] = pc
	r.mu.Unlock()

	r.mu.RLock()
	entry, ok := r.factories[pc.Type]
	r.mu.RUnlock()
	if !ok {
		err := fmt.Errorf("unknown plugin type %q", pc.Type)
		r.recordStartFailure(pc.ID)
		return err
	}

	resolvedCfg := applyEnvOverlay(pc.ID, pc.Config)

	p := entry.Factory(pc.ID)
	// Wrap the shared host with a per-plugin facade that auto-tags Log events
	// and records device activity to the ActivityBuffer.
	ph := &pluginHost{Host: r.host, pluginID: pc.ID, getSink: r.getSink, getActivityBuffer: r.getActivityBuffer, notifyDiscovery: r.notifyDiscoveryChanged}
	if err := p.Init(ctx, resolvedCfg, ph); err != nil {
		r.sinkEmit(pc.ID, LevelError, CodeConnectFailed, "plugin init failed", map[string]any{"error": err.Error(), "type": pc.Type})
		r.recordStartFailure(pc.ID)
		return err
	}

	r.mu.Lock()
	r.instances[pc.ID] = p
	delete(r.retryBackoff, pc.ID)
	r.mu.Unlock()

	logging.Info("bridge_plugin_started", logging.Fields{"id": pc.ID, "type": pc.Type})
	r.sinkEmit(pc.ID, LevelInfo, CodePluginStarted, "plugin started", map[string]any{"type": pc.Type})
	return nil
}

// recordStartFailure bumps id's automatic-retry backoff (doubling each
// consecutive failure, capped at supervisorBackoffMax) so the supervisor
// loop doesn't hammer a plugin that's failing every attempt.
func (r *Registry) recordStartFailure(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delay := r.retryBackoff[id].delay * 2
	if delay < supervisorInterval {
		delay = supervisorInterval
	}
	if delay > supervisorBackoffMax {
		delay = supervisorBackoffMax
	}
	r.retryBackoff[id] = retryState{nextAt: time.Now().Add(delay), delay: delay}
}

// startSupervisor runs superviseOnce every supervisorInterval until ctx is
// cancelled. Started once from Start.
func (r *Registry) startSupervisor(ctx context.Context) {
	// Read the package var synchronously here, not inside the goroutine
	// below: the goroutine's own first line running is not ordered against
	// this function returning, so a test that mutates supervisorInterval
	// right after calling Start could otherwise race with this goroutine's
	// delayed startup reading it.
	interval := supervisorInterval
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.superviseOnce(ctx)
			}
		}
	}()
}

// superviseOnce retries startPlugin for every enabled config that has no
// currently-running instance and whose backoff (if any) has elapsed — the
// case a plugin whose Init failed (at daemon startup, or a manual restart
// that still failed) would otherwise sit dead until a human intervenes
// again. A plugin that starts successfully is resubscribed to its existing
// bridge mappings, mirroring RestartPlugin's own resubscribe step.
func (r *Registry) superviseOnce(ctx context.Context) {
	now := time.Now()
	r.mu.RLock()
	var due []PluginConfig
	for id, pc := range r.configs {
		if pc.Disabled {
			continue
		}
		if _, running := r.instances[id]; running {
			continue
		}
		if st, ok := r.retryBackoff[id]; ok && now.Before(st.nextAt) {
			continue
		}
		due = append(due, pc)
	}
	r.mu.RUnlock()

	for _, pc := range due {
		if err := r.startPlugin(ctx, pc); err != nil {
			continue
		}
		r.sinkEmit(pc.ID, LevelInfo, CodePluginStarted,
			"automatically restarted after a previous failure", map[string]any{"type": pc.Type})

		r.mu.RLock()
		fresh := r.instances[pc.ID]
		r.mu.RUnlock()
		if fresh == nil {
			continue
		}
		r.restoreMappings(ctx, pc.ID, fresh)
	}
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
	// Plugins spawn long-lived goroutines tied to the ctx passed to Init,
	// so use the registry's runtime ctx — the caller's ctx is typically
	// the short-lived HTTP request ctx and would cancel on response send.
	if err := r.startPlugin(r.runtimeCtx(ctx), pc); err != nil {
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
	p, hasInstance := r.instances[id]
	_, hasConfig := r.configs[id]
	delete(r.instances, id)
	delete(r.configs, id)
	delete(r.retryBackoff, id)
	r.mu.Unlock()
	// A plugin can have a config but no running instance — e.g. its Init
	// failed and it's still waiting on the supervisor's next retry. That
	// must still be removable, not just one that's currently running.
	if !hasInstance && !hasConfig {
		return fmt.Errorf("plugin %q not found", id)
	}

	// Tear down all bridge mappings owned by this plugin.
	for _, m := range r.mappings.ListForPlugin(id) {
		if hasInstance {
			_ = p.Unsubscribe(ctx, m.DSUID)
		}
		_ = r.host.RemoveDevice(ctx, m.DSUID)
		_, _ = r.mappings.Remove(m.DSUID)
	}

	if hasInstance {
		if err := p.Close(); err != nil {
			logging.Warn("bridge_plugin_close_error", logging.Fields{"id": id, "error": err.Error()})
		}
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
	old, hasInstance := r.instances[pc.ID]
	_, hasConfig := r.configs[pc.ID]
	delete(r.instances, pc.ID)
	r.mu.Unlock()
	// A plugin can have a config but no running instance if its Init failed
	// (e.g. a bad token) — updating its config to fix that must still work,
	// not just editing a plugin that's currently running.
	if !hasInstance && !hasConfig {
		return fmt.Errorf("plugin %q not found", pc.ID)
	}
	if hasInstance {
		if err := old.Close(); err != nil {
			logging.Warn("bridge_plugin_close_error", logging.Fields{"id": pc.ID, "error": err.Error()})
		}
	}
	// See AddPlugin: use the long-lived runtime ctx for the new instance.
	runCtx := r.runtimeCtx(ctx)
	if err := r.startPlugin(runCtx, pc); err != nil {
		return err
	}
	// Re-subscribe persisted mappings on the fresh instance.
	r.mu.RLock()
	fresh := r.instances[pc.ID]
	r.mu.RUnlock()
	if fresh != nil {
		r.restoreMappings(runCtx, pc.ID, fresh)
	}
	r.persistConfigs()
	r.emit("pluginUpdated", map[string]any{"id": pc.ID, "type": pc.Type})
	r.sinkEmit(pc.ID, LevelInfo, CodePluginRestarted, "plugin config updated and restarted", map[string]any{"type": pc.Type})
	return nil
}

// RestartPlugin stops the running instance (if any) and starts a fresh one
// with the persisted config, then re-subscribes existing mappings. Refuses
// to act on a disabled plugin (call SetEnabled first).
func (r *Registry) RestartPlugin(ctx context.Context, id string) error {
	r.mu.RLock()
	pc, hasCfg := r.configs[id]
	r.mu.RUnlock()
	if !hasCfg {
		return fmt.Errorf("plugin %q not found", id)
	}
	if pc.Disabled {
		return fmt.Errorf("plugin %q is disabled", id)
	}
	r.mu.Lock()
	old, hadInstance := r.instances[id]
	delete(r.instances, id)
	r.mu.Unlock()
	if hadInstance {
		if err := old.Close(); err != nil {
			logging.Warn("bridge_plugin_close_error", logging.Fields{"id": id, "error": err.Error()})
		}
	}
	runCtx := r.runtimeCtx(ctx)
	if err := r.startPlugin(runCtx, pc); err != nil {
		return err
	}
	r.mu.RLock()
	fresh := r.instances[id]
	r.mu.RUnlock()
	if fresh != nil {
		r.restoreMappings(runCtx, id, fresh)
	}
	r.emit("pluginUpdated", map[string]any{"id": id, "type": pc.Type})
	r.sinkEmit(id, LevelInfo, CodePluginRestarted, "plugin restarted", map[string]any{"type": pc.Type})
	return nil
}

// SetEnabled toggles a plugin's enabled flag. Disabling stops the running
// instance but keeps the config and any bridge mappings on disk; enabling
// starts the plugin with the persisted config and re-subscribes mappings.
func (r *Registry) SetEnabled(ctx context.Context, id string, enabled bool) error {
	r.mu.Lock()
	pc, hasCfg := r.configs[id]
	r.mu.Unlock()
	if !hasCfg {
		return fmt.Errorf("plugin %q not found", id)
	}
	wasDisabled := pc.Disabled
	if wasDisabled == !enabled {
		return nil // no-op
	}
	pc.Disabled = !enabled
	if enabled {
		runCtx := r.runtimeCtx(ctx)
		if err := r.startPlugin(runCtx, pc); err != nil {
			return err
		}
		r.mu.RLock()
		fresh := r.instances[id]
		r.mu.RUnlock()
		if fresh != nil {
			r.restoreMappings(runCtx, id, fresh)
		}
		r.emit("pluginUpdated", map[string]any{"id": id, "type": pc.Type})
		r.sinkEmit(id, LevelInfo, CodePluginStarted, "plugin enabled", map[string]any{"type": pc.Type})
	} else {
		r.mu.Lock()
		old, hadInstance := r.instances[id]
		delete(r.instances, id)
		r.configs[id] = pc
		r.mu.Unlock()
		if hadInstance {
			for _, m := range r.mappings.ListForPlugin(id) {
				_ = old.Unsubscribe(ctx, m.DSUID)
			}
			if err := old.Close(); err != nil {
				logging.Warn("bridge_plugin_close_error", logging.Fields{"id": id, "error": err.Error()})
			}
		}
		r.emit("pluginUpdated", map[string]any{"id": id, "type": pc.Type})
		r.sinkEmit(id, LevelInfo, CodePluginStopped, "plugin disabled", map[string]any{"type": pc.Type})
	}
	// Persist the toggled state.
	r.mu.Lock()
	r.configs[id] = pc
	r.mu.Unlock()
	r.persistConfigs()
	return nil
}

// IsEnabled reports whether the plugin with the given id is configured to run.
// Returns true for unknown ids (caller should validate existence separately).
func (r *Registry) IsEnabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.configs[id]
	if !ok {
		return true
	}
	return !cfg.Disabled
}

// persistConfigs invokes the registered Persister with the current config list.
// The list is sorted by id: r.configs is a map, and an unsorted iteration
// would rewrite plugins.json in a different order on every save. Since Start
// instantiates plugins in file order, that turned a plugin's startup
// dependency (a Tasmota/Z2M plugin needs its MQTT broker plugin registered
// first) into a coin flip on each restart.
func (r *Registry) persistConfigs() {
	r.mu.RLock()
	p := r.persist
	list := make([]PluginConfig, 0, len(r.configs))
	for _, c := range r.configs {
		list = append(list, c)
	}
	r.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
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

// runtimeCtx returns the registry's long-lived ctx if Start has been called,
// otherwise falls back to the supplied ctx (e.g. tests that drive the
// registry without Start).
func (r *Registry) runtimeCtx(fallback context.Context) context.Context {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.ctx != nil {
		return r.ctx
	}
	return fallback
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
