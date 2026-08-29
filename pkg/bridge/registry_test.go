package bridge

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splattner/vdcgo/pkg/services/mqtt"
)

// ── fakeHost: records every call, backed by an in-memory map instead of a
// real vdcapi.StateStore, so Registry can be tested without pulling in the
// vdcapi package. ──────────────────────────────────────────────────────────

type fakeHost struct {
	mu sync.Mutex

	announced map[string]Mapping // dsuid -> mapping, present while "existing"
	removed   []string
	active    map[string]bool

	announceErr map[string]error // dsuid -> error to return from AnnounceDevice
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		announced: make(map[string]Mapping),
		active:    make(map[string]bool),
	}
}

func (h *fakeHost) DeriveDSUID(pluginID, remoteEntityID string) string {
	return fmt.Sprintf("DSUID-%s-%s", pluginID, remoteEntityID)
}

func (h *fakeHost) AnnounceDevice(_ context.Context, m Mapping) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.announceErr[m.DSUID]; err != nil {
		return err
	}
	h.announced[m.DSUID] = m
	return nil
}

func (h *fakeHost) RemoveDevice(_ context.Context, dsuid string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.announced, dsuid)
	h.removed = append(h.removed, dsuid)
	return nil
}

func (h *fakeHost) HasDevice(dsuid string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.announced[dsuid]
	return ok
}

func (h *fakeHost) isAnnounced(dsuid string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.announced[dsuid]
	return ok
}

func (h *fakeHost) UpdateChannel(context.Context, string, int, float64) error         { return nil }
func (h *fakeHost) UpdateButton(context.Context, string, int, float64) error          { return nil }
func (h *fakeHost) SetButtonAction(context.Context, string, int, string) error        { return nil }
func (h *fakeHost) UpdateSensor(context.Context, string, int, float64) error          { return nil }
func (h *fakeHost) SetSensorDescriptor(context.Context, string, int, SensorDescriptor) error {
	return nil
}
func (h *fakeHost) SetButtonDescriptor(context.Context, string, int, ButtonDescriptor) error {
	return nil
}
func (h *fakeHost) AnnounceRichDevice(ctx context.Context, m Mapping, _ DeviceDescriptor) error {
	return h.AnnounceDevice(ctx, m)
}
func (h *fakeHost) UpdateInput(context.Context, string, int, float64) error { return nil }
func (h *fakeHost) SetBinaryInputDescriptor(context.Context, string, int, BinaryInputDescriptor) error {
	return nil
}
func (h *fakeHost) UpdateDeviceMeta(context.Context, string, string, string, string) error {
	return nil
}
func (h *fakeHost) ReAnnounce(context.Context, string) error { return nil }
func (h *fakeHost) UpdateActive(_ context.Context, dsuid string, active bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active[dsuid] = active
	return nil
}
func (h *fakeHost) MQTT() *mqtt.Manager                          { return nil }
func (h *fakeHost) Log(LogLevel, string, string, map[string]any) {}
func (h *fakeHost) NotifyDiscoveryChanged()                      {}

// ── fakePlugin: a scriptable Plugin implementation. ─────────────────────────

type fakePlugin struct {
	mu sync.Mutex

	id string

	initErr      error
	discoverList []RemoteEntity
	discoverErr  error
	subscribeErr error
	applyErr     error
	// failInitTimes, when > 0, makes Init fail this many times (returning
	// errFakeInitTransient) before succeeding on every call after —
	// independent of initErr, which means "always fail, unbounded". Used to
	// drive the supervisor's retry-then-recover path deterministically.
	failInitTimes int

	initCalls       int
	lastInitCfg     map[string]any
	lastInitHost    Host
	subscribed      map[string]Mapping // dsuid -> mapping
	unsubscribed    []string
	applyCalls      []Command
	closed          bool
	status          string
}

var errFakeInitTransient = errors.New("fake transient init failure")

func newFakePlugin(id string) *fakePlugin {
	return &fakePlugin{id: id, subscribed: make(map[string]Mapping), status: "connected"}
}

func (p *fakePlugin) ID() string { return p.id }

func (p *fakePlugin) Init(_ context.Context, cfg map[string]any, host Host) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initCalls++
	p.lastInitCfg = cfg
	p.lastInitHost = host
	if p.initErr != nil {
		return p.initErr
	}
	if p.failInitTimes > 0 {
		p.failInitTimes--
		return errFakeInitTransient
	}
	return nil
}

func (p *fakePlugin) Status() string { return p.status }

func (p *fakePlugin) Discover(context.Context) ([]RemoteEntity, error) {
	return p.discoverList, p.discoverErr
}

func (p *fakePlugin) Subscribe(_ context.Context, m Mapping) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.subscribeErr != nil {
		return p.subscribeErr
	}
	p.subscribed[m.DSUID] = m
	return nil
}

func (p *fakePlugin) Unsubscribe(_ context.Context, dsuid string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subscribed, dsuid)
	p.unsubscribed = append(p.unsubscribed, dsuid)
	return nil
}

func (p *fakePlugin) Apply(_ context.Context, _ Mapping, cmd Command) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.applyCalls = append(p.applyCalls, cmd)
	return p.applyErr
}

func (p *fakePlugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

func (p *fakePlugin) isSubscribed(dsuid string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.subscribed[dsuid]
	return ok
}

func (p *fakePlugin) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// ── helpers ──────────────────────────────────────────────────────────────────

// newTestRegistry returns a Registry wired to a fakeHost, with a "fake"
// factory type registered that hands out fresh *fakePlugin instances (one
// per AddPlugin/startPlugin call, keyed by id via the closure below).
func newTestRegistry() (*Registry, *fakeHost, map[string]*fakePlugin) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	instances := make(map[string]*fakePlugin)
	reg.RegisterFactory("fake", func(id string) Plugin {
		p := newFakePlugin(id)
		instances[id] = p
		return p
	})
	return reg, host, instances
}

// ── Registry: factory registration ──────────────────────────────────────────

func TestRegistryRegisterAndFactoryTypes(t *testing.T) {
	reg, _, _ := newTestRegistry()
	reg.Register("other", FactoryEntry{
		DisplayName: "Other Plugin",
		Description: "desc",
		Factory:     func(id string) Plugin { return newFakePlugin(id) },
	})

	types := reg.FactoryTypes()
	if len(types) != 2 {
		t.Fatalf("expected 2 registered types, got %v", types)
	}

	entry, ok := reg.FactoryEntry("other")
	if !ok {
		t.Fatal("expected FactoryEntry(other) to be found")
	}
	if entry.DisplayName != "Other Plugin" {
		t.Fatalf("expected DisplayName preserved, got %q", entry.DisplayName)
	}

	// RegisterFactory (no metadata) defaults DisplayName to the type name.
	fakeEntry, ok := reg.FactoryEntry("fake")
	if !ok || fakeEntry.DisplayName != "fake" {
		t.Fatalf("expected RegisterFactory to default DisplayName to type name, got %+v ok=%t", fakeEntry, ok)
	}

	if _, ok := reg.FactoryEntry("nope"); ok {
		t.Fatal("expected FactoryEntry(nope) to report not found")
	}
}

// ── Registry.Start ───────────────────────────────────────────────────────────

func TestRegistryStartInstantiatesEnabledPlugins(t *testing.T) {
	reg, _, instances := newTestRegistry()
	err := reg.Start(context.Background(), []PluginConfig{
		{ID: "p1", Type: "fake", Config: map[string]any{"k": "v"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	p1, ok := reg.Plugin("p1")
	if !ok {
		t.Fatal("expected plugin p1 to be running after Start")
	}
	if instances["p1"].initCalls != 1 {
		t.Fatalf("expected exactly one Init call, got %d", instances["p1"].initCalls)
	}
	if instances["p1"].lastInitCfg["k"] != "v" {
		t.Fatalf("expected config passed through to Init, got %+v", instances["p1"].lastInitCfg)
	}
	if p1.ID() != "p1" {
		t.Fatalf("expected running plugin id=p1, got %s", p1.ID())
	}
}

func TestRegistryStartSkipsDisabledPlugins(t *testing.T) {
	reg, _, instances := newTestRegistry()
	err := reg.Start(context.Background(), []PluginConfig{
		{ID: "p1", Type: "fake", Disabled: true},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected disabled plugin to not be started")
	}
	if _, ok := instances["p1"]; ok {
		t.Fatal("expected disabled plugin's factory to never be called")
	}
	// Config is still remembered so it can be listed / re-enabled later.
	if _, ok := reg.Config("p1"); !ok {
		t.Fatal("expected disabled plugin's config to be retained")
	}
	if reg.IsEnabled("p1") {
		t.Fatal("expected IsEnabled(p1) to be false")
	}
}

func TestRegistryStartContinuesAfterOnePluginFailsInit(t *testing.T) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	reg.RegisterFactory("bad", func(id string) Plugin {
		p := newFakePlugin(id)
		p.initErr = fmt.Errorf("boom")
		return p
	})
	reg.RegisterFactory("fake", func(id string) Plugin { return newFakePlugin(id) })

	err := reg.Start(context.Background(), []PluginConfig{
		{ID: "broken", Type: "bad"},
		{ID: "ok", Type: "fake"},
	})
	if err != nil {
		t.Fatalf("Start should not fail outright when one plugin errors, got %v", err)
	}
	if _, ok := reg.Plugin("broken"); ok {
		t.Fatal("expected failed plugin to not be registered as running")
	}
	if _, ok := reg.Plugin("ok"); !ok {
		t.Fatal("expected the other plugin to still start")
	}
}

func TestRegistryStartRestoresPersistedMappings(t *testing.T) {
	reg, host, instances := newTestRegistry()
	m := Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1", Kind: "light", Name: "Lamp"}
	if _, err := reg.Mappings().Add(m); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	if err := reg.Start(context.Background(), []PluginConfig{{ID: "p1", Type: "fake"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !host.isAnnounced("D1") {
		t.Fatal("expected restored mapping to be announced to the host")
	}
	if !instances["p1"].isSubscribed("D1") {
		t.Fatal("expected restored mapping's plugin to be subscribed")
	}
}

func TestRegistryStartAnnouncesOrphanMappingForDisabledPlugin(t *testing.T) {
	reg, host, _ := newTestRegistry()
	m := Mapping{PluginID: "p1", RemoteEntityID: "e1", DSUID: "D1"}
	if _, err := reg.Mappings().Add(m); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	if err := reg.Start(context.Background(), []PluginConfig{{ID: "p1", Type: "fake", Disabled: true}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !host.isAnnounced("D1") {
		t.Fatal("expected mapping belonging to a disabled plugin to still be announced (device exists, just no live updates)")
	}
	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("disabled plugin must not be running")
	}
}

func TestRegistryStartSkipsMappingForUnknownPlugin(t *testing.T) {
	reg, host, _ := newTestRegistry()
	m := Mapping{PluginID: "ghost", RemoteEntityID: "e1", DSUID: "D1"}
	if _, err := reg.Mappings().Add(m); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	// No config at all for "ghost" — must not panic, must not announce.
	if err := reg.Start(context.Background(), nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if host.isAnnounced("D1") {
		t.Fatal("expected orphan mapping (no plugin, no config) to not be announced")
	}
}

// ── Registry.AddPlugin / RemovePlugin ────────────────────────────────────────

func TestRegistryAddPluginRejectsDuplicateID(t *testing.T) {
	reg, _, _ := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("first AddPlugin: %v", err)
	}
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err == nil {
		t.Fatal("expected error adding a plugin with a duplicate id")
	}
}

func TestRegistryAddPluginRejectsUnknownType(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "no-such-type"}); err == nil {
		t.Fatal("expected error adding a plugin of an unregistered type")
	}
	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected no plugin instance to exist after a failed AddPlugin")
	}
}

func TestRegistryAddPluginRejectsEmptyID(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if err := reg.AddPlugin(context.Background(), PluginConfig{Type: "fake"}); err == nil {
		t.Fatal("expected error adding a plugin with an empty id")
	}
}

func TestRegistryAddPluginPersistsConfig(t *testing.T) {
	reg, _, _ := newTestRegistry()
	var persisted []PluginConfig
	reg.SetPersister(func(configs []PluginConfig) error {
		persisted = configs
		return nil
	})
	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if len(persisted) != 1 || persisted[0].ID != "p1" {
		t.Fatalf("expected persister to be called with the new config, got %+v", persisted)
	}
}

func TestRegistryRemovePluginTearsDownOwnedMappings(t *testing.T) {
	reg, host, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light"); err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}
	dsuid := "DSUID-p1-e1"
	if !host.isAnnounced(dsuid) {
		t.Fatal("expected device to be announced after CreateBridge")
	}

	if err := reg.RemovePlugin(ctx, "p1"); err != nil {
		t.Fatalf("RemovePlugin: %v", err)
	}

	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected plugin instance to be gone after RemovePlugin")
	}
	if !instances["p1"].isClosed() {
		t.Fatal("expected plugin Close() to be called")
	}
	if host.isAnnounced(dsuid) {
		t.Fatal("expected owned device to be removed from the host")
	}
	if _, ok := reg.Mappings().Get(dsuid); ok {
		t.Fatal("expected owned mapping to be removed from the mapping store")
	}
}

func TestRegistryRemovePluginUnknownIDErrors(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if err := reg.RemovePlugin(context.Background(), "nope"); err == nil {
		t.Fatal("expected error removing an unknown plugin")
	}
}

// ── Registry.UpdatePlugin / RestartPlugin ───────────────────────────────────

func TestRegistryUpdatePluginReplacesInstanceAndResubscribes(t *testing.T) {
	reg, _, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake", Config: map[string]any{"v": 1}}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light"); err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}
	oldInstance := instances["p1"]

	if err := reg.UpdatePlugin(ctx, PluginConfig{ID: "p1", Type: "fake", Config: map[string]any{"v": 2}}); err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}

	if !oldInstance.isClosed() {
		t.Fatal("expected old instance to be closed")
	}
	newInstance := instances["p1"]
	if newInstance == oldInstance {
		t.Fatal("expected a fresh plugin instance after UpdatePlugin")
	}
	if newInstance.lastInitCfg["v"] != 2 {
		t.Fatalf("expected new instance to receive updated config, got %+v", newInstance.lastInitCfg)
	}
	if !newInstance.isSubscribed("DSUID-p1-e1") {
		t.Fatal("expected existing bridge mapping to be resubscribed on the new instance")
	}
}

func TestRegistryUpdatePluginUnknownIDErrors(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if err := reg.UpdatePlugin(context.Background(), PluginConfig{ID: "nope", Type: "fake"}); err == nil {
		t.Fatal("expected error updating an unknown plugin")
	}
}

func TestRegistryRestartPluginRefusesDisabled(t *testing.T) {
	reg, _, _ := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if err := reg.SetEnabled(ctx, "p1", false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if err := reg.RestartPlugin(ctx, "p1"); err == nil {
		t.Fatal("expected RestartPlugin to refuse a disabled plugin")
	}
}

func TestRegistryRestartPluginUnknownIDErrors(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if err := reg.RestartPlugin(context.Background(), "nope"); err == nil {
		t.Fatal("expected error restarting an unknown plugin")
	}
}

func TestRegistryRestartPluginFreshInstanceResubscribes(t *testing.T) {
	reg, _, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light"); err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}
	old := instances["p1"]

	if err := reg.RestartPlugin(ctx, "p1"); err != nil {
		t.Fatalf("RestartPlugin: %v", err)
	}
	if !old.isClosed() {
		t.Fatal("expected old instance closed on restart")
	}
	if !instances["p1"].isSubscribed("DSUID-p1-e1") {
		t.Fatal("expected mapping resubscribed on the restarted instance")
	}
}

// ── Registry.SetEnabled / IsEnabled ─────────────────────────────────────────

func TestRegistrySetEnabledDisableThenEnable(t *testing.T) {
	reg, _, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light"); err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}
	firstInstance := instances["p1"]

	if err := reg.SetEnabled(ctx, "p1", false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if !firstInstance.isClosed() {
		t.Fatal("expected instance closed on disable")
	}
	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected no running instance while disabled")
	}
	if reg.IsEnabled("p1") {
		t.Fatal("expected IsEnabled(p1) false after disable")
	}
	// Mapping must survive the disable.
	if _, ok := reg.Mappings().Get("DSUID-p1-e1"); !ok {
		t.Fatal("expected bridge mapping to survive disabling its plugin")
	}

	if err := reg.SetEnabled(ctx, "p1", true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if !reg.IsEnabled("p1") {
		t.Fatal("expected IsEnabled(p1) true after re-enable")
	}
	fresh, ok := reg.Plugin("p1")
	if !ok {
		t.Fatal("expected a running instance after re-enable")
	}
	if !instances["p1"].isSubscribed("DSUID-p1-e1") || fresh == firstInstance {
		t.Fatal("expected a fresh, resubscribed instance after re-enable")
	}
}

func TestRegistrySetEnabledNoopWhenAlreadyInState(t *testing.T) {
	reg, _, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	calls := instances["p1"].initCalls
	if err := reg.SetEnabled(ctx, "p1", true); err != nil {
		t.Fatalf("SetEnabled(true) on already-enabled plugin: %v", err)
	}
	if instances["p1"].initCalls != calls {
		t.Fatal("expected no-op SetEnabled to not restart the plugin")
	}
}

func TestRegistryIsEnabledDefaultsTrueForUnknownID(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if !reg.IsEnabled("never-heard-of-it") {
		t.Fatal("expected IsEnabled to default to true for an unknown id")
	}
}

// ── Registry.CreateBridge / RemoveBridge ────────────────────────────────────

func TestRegistryCreateBridgeUnknownPluginErrors(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if _, err := reg.CreateBridge(context.Background(), "nope", "e1", "Lamp", "light"); err == nil {
		t.Fatal("expected error creating a bridge for an unregistered plugin")
	}
}

func TestRegistryCreateBridgeRejectsDuplicateRemoteEntity(t *testing.T) {
	reg, _, _ := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light"); err != nil {
		t.Fatalf("first CreateBridge: %v", err)
	}
	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp Again", "light"); err == nil {
		t.Fatal("expected error re-bridging an already-bridged remote entity")
	}
}

func TestRegistryCreateBridgeSurvivesSubscribeFailure(t *testing.T) {
	reg, host, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	instances["p1"].subscribeErr = fmt.Errorf("boom")

	m, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light")
	if err != nil {
		t.Fatalf("expected CreateBridge to succeed even if Subscribe fails, got %v", err)
	}
	if !host.isAnnounced(m.DSUID) {
		t.Fatal("expected device to still be announced despite Subscribe failure")
	}
	if _, ok := reg.Mappings().Get(m.DSUID); !ok {
		t.Fatal("expected mapping to still be persisted despite Subscribe failure")
	}
}

func TestRegistryCreateBridgeRollsBackMappingIfAnnounceFails(t *testing.T) {
	reg, host, _ := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	dsuid := host.DeriveDSUID("p1", "e1")
	host.mu.Lock()
	if host.announceErr == nil {
		host.announceErr = map[string]error{}
	}
	host.announceErr[dsuid] = fmt.Errorf("announce failed")
	host.mu.Unlock()

	if _, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light"); err == nil {
		t.Fatal("expected CreateBridge to fail when AnnounceDevice fails")
	}
	if _, ok := reg.Mappings().Get(dsuid); ok {
		t.Fatal("expected mapping to be rolled back after a failed announce")
	}
}

func TestRegistryRemoveBridgeUnknownDSUIDErrors(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if err := reg.RemoveBridge(context.Background(), "nope"); err == nil {
		t.Fatal("expected error removing an unknown bridge")
	}
}

func TestRegistryRemoveBridgeFullTeardown(t *testing.T) {
	reg, host, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	m, err := reg.CreateBridge(ctx, "p1", "e1", "Lamp", "light")
	if err != nil {
		t.Fatalf("CreateBridge: %v", err)
	}
	if !instances["p1"].isSubscribed(m.DSUID) {
		t.Fatal("expected subscribe to have happened")
	}

	if err := reg.RemoveBridge(ctx, m.DSUID); err != nil {
		t.Fatalf("RemoveBridge: %v", err)
	}
	if instances["p1"].isSubscribed(m.DSUID) {
		t.Fatal("expected plugin to be unsubscribed")
	}
	if host.isAnnounced(m.DSUID) {
		t.Fatal("expected device removed from host")
	}
	if _, ok := reg.Mappings().Get(m.DSUID); ok {
		t.Fatal("expected mapping removed from store")
	}
}

// ── Registry.Stop ────────────────────────────────────────────────────────────

func TestRegistryStopClosesAllInstances(t *testing.T) {
	reg, _, instances := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin p1: %v", err)
	}
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p2", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin p2: %v", err)
	}

	reg.Stop()

	if !instances["p1"].isClosed() || !instances["p2"].isClosed() {
		t.Fatal("expected Stop to close every running plugin instance")
	}
}

// ── applyEnvOverlay ──────────────────────────────────────────────────────────

func TestApplyEnvOverlay(t *testing.T) {
	t.Setenv("VDCGO_MY_BROKER_URL", "ws://override")
	t.Setenv("VDCGO_MY_BROKER_TOKEN", "secret")
	t.Setenv("VDCGO_OTHER_PLUGIN_URL", "should-not-apply")

	cfg := map[string]any{"url": "ws://original", "keep": "me"}
	out := applyEnvOverlay("my-broker", cfg)

	if out["url"] != "ws://override" {
		t.Fatalf("expected url overridden, got %+v", out)
	}
	if out["token"] != "secret" {
		t.Fatalf("expected token injected from env, got %+v", out)
	}
	if out["keep"] != "me" {
		t.Fatalf("expected unrelated key preserved, got %+v", out)
	}
	// Original map must not be mutated.
	if cfg["url"] != "ws://original" {
		t.Fatalf("expected original config map to be left untouched, got %+v", cfg)
	}
}

func TestApplyEnvOverlayNoMatchingEnv(t *testing.T) {
	cfg := map[string]any{"url": "ws://original"}
	out := applyEnvOverlay("plugin-with-no-env-overrides", cfg)
	if out["url"] != "ws://original" {
		t.Fatalf("expected config unchanged when no matching env vars exist, got %+v", out)
	}
}

// ── Registry: remaining accessors ───────────────────────────────────────────

func TestRegistrySetEventSinkForwardsLifecycleEvents(t *testing.T) {
	reg, _, _ := newTestRegistry()
	sink := NewEventBuffer(10, 100)
	reg.SetEventSink(sink)

	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}

	events := sink.Snapshot("p1", 0, "", 0)
	if len(events) == 0 {
		t.Fatal("expected AddPlugin to publish at least one lifecycle event to the sink")
	}
}

func TestRegistryEmitPluginEventForwardsToSink(t *testing.T) {
	reg, _, _ := newTestRegistry()
	sink := NewEventBuffer(10, 100)
	reg.SetEventSink(sink)

	reg.EmitPluginEvent("p1", LevelWarn, "manual_code", "manual message", nil)

	events := sink.Snapshot("p1", 0, "", 0)
	if len(events) != 1 || events[0].Code != "manual_code" {
		t.Fatalf("expected EmitPluginEvent to reach the sink, got %+v", events)
	}
}

func TestRegistryConfigsReturnsSnapshot(t *testing.T) {
	reg, _, _ := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p2", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	configs := reg.Configs()
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d: %+v", len(configs), configs)
	}
}

func TestRegistryInstancesReturnsRunningPlugins(t *testing.T) {
	reg, _, _ := newTestRegistry()
	ctx := context.Background()
	if err := reg.AddPlugin(ctx, PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}
	instances := reg.Instances()
	if len(instances) != 1 || instances[0].ID() != "p1" {
		t.Fatalf("expected Instances() to report the running plugin, got %+v", instances)
	}
}

func TestRegistryRuntimeContextReflectsStart(t *testing.T) {
	reg, _, _ := newTestRegistry()
	if reg.RuntimeContext() != nil {
		t.Fatal("expected RuntimeContext() to be nil before Start")
	}
	ctx := context.Background()
	if err := reg.Start(ctx, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if reg.RuntimeContext() != ctx {
		t.Fatal("expected RuntimeContext() to return the ctx passed to Start")
	}
}

func TestRegistrySetActivityBufferUsedByPluginHost(t *testing.T) {
	reg, _, instances := newTestRegistry()
	ab := NewActivityBuffer(10, 100)
	reg.SetActivityBuffer(ab)

	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}

	// The Host each plugin receives at Init is the Registry's pluginHost
	// wrapper, which records channel updates to the ActivityBuffer.
	ph := instances["p1"].lastInitHost
	if err := ph.UpdateChannel(context.Background(), "D1", 0, 42); err != nil {
		t.Fatalf("UpdateChannel: %v", err)
	}

	activity := ab.Snapshot("D1", 0, 0)
	if len(activity) != 1 || activity[0].PluginID != "p1" || activity[0].Value == nil || *activity[0].Value != 42 {
		t.Fatalf("expected UpdateChannel to publish a device activity entry tagged with plugin p1, got %+v", activity)
	}
}

// ── Registry: config retained across a failed Init ──────────────────────────

func TestRegistryConfigSurvivesInitFailureAndIsNotRunning(t *testing.T) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	reg.RegisterFactory("bad", func(id string) Plugin {
		p := newFakePlugin(id)
		p.initErr = errors.New("boom")
		return p
	})

	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "bad"}); err == nil {
		t.Fatal("expected AddPlugin to report the Init failure")
	}
	if _, ok := reg.Config("p1"); !ok {
		t.Fatal("expected config to be retained despite the Init failure")
	}
	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected no running instance for a plugin whose Init failed")
	}
}

func TestRegistryRemovePluginWorksAfterInitFailure(t *testing.T) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	reg.RegisterFactory("bad", func(id string) Plugin {
		p := newFakePlugin(id)
		p.initErr = errors.New("boom")
		return p
	})
	_ = reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "bad"})

	if err := reg.RemovePlugin(context.Background(), "p1"); err != nil {
		t.Fatalf("expected RemovePlugin to succeed for a plugin with no running instance, got %v", err)
	}
	if _, ok := reg.Config("p1"); ok {
		t.Fatal("expected config to be gone after RemovePlugin")
	}
}

func TestRegistryUpdatePluginWorksAfterInitFailure(t *testing.T) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	var attempt int
	reg.RegisterFactory("flaky", func(id string) Plugin {
		attempt++
		p := newFakePlugin(id)
		if attempt == 1 {
			p.initErr = errors.New("boom")
		}
		return p
	})
	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "flaky"}); err == nil {
		t.Fatal("expected AddPlugin's initial attempt to fail")
	}

	// UpdatePlugin (e.g. the user fixing a bad token via the UI) must work
	// even though there's no running instance yet — only a retained config.
	if err := reg.UpdatePlugin(context.Background(), PluginConfig{ID: "p1", Type: "flaky"}); err != nil {
		t.Fatalf("expected UpdatePlugin to succeed after a failed Init, got %v", err)
	}
	if _, ok := reg.Plugin("p1"); !ok {
		t.Fatal("expected a running instance after UpdatePlugin's retry succeeds")
	}
}

// ── Registry: supervisor auto-retry ─────────────────────────────────────────

func TestRegistrySupervisorRetriesAndRecoversFailedInit(t *testing.T) {
	// t.Cleanup runs LIFO: register the package-var restore first so it
	// executes last, after the "stop the supervisor" cleanup below has
	// already cancelled its context — otherwise the still-running
	// supervisor goroutine races with restoring these vars.
	prevInterval, prevMax := supervisorInterval, supervisorBackoffMax
	supervisorInterval = 5 * time.Millisecond
	supervisorBackoffMax = 20 * time.Millisecond
	t.Cleanup(func() { supervisorInterval, supervisorBackoffMax = prevInterval, prevMax })

	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	var attempt int32
	reg.RegisterFactory("flaky", func(id string) Plugin {
		n := atomic.AddInt32(&attempt, 1)
		p := newFakePlugin(id)
		if n == 1 {
			p.initErr = errors.New("boom")
		}
		return p
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Start() with a config that fails its first attempt — the supervisor
	// (started by Start) must pick it up and retry on its own, with no
	// manual RestartPlugin/UpdatePlugin call.
	if err := reg.Start(ctx, []PluginConfig{{ID: "p1", Type: "flaky"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected the plugin to not be running immediately after the failed first attempt")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := reg.Plugin("p1"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the supervisor to auto-retry and recover the plugin")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestRegistrySupervisorSkipsDisabledPlugins(t *testing.T) {
	// See the LIFO-ordering note in TestRegistrySupervisorRetriesAndRecoversFailedInit.
	prevInterval := supervisorInterval
	supervisorInterval = 5 * time.Millisecond
	t.Cleanup(func() { supervisorInterval = prevInterval })

	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	var calls int32
	reg.RegisterFactory("counting", func(id string) Plugin {
		atomic.AddInt32(&calls, 1)
		return newFakePlugin(id)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := reg.Start(ctx, []PluginConfig{{ID: "p1", Type: "counting", Disabled: true}}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(30 * time.Millisecond) // give the supervisor a few ticks
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected the supervisor to never instantiate a disabled plugin, got %d factory calls", got)
	}
}

// ── Registry: discovery-change push notification ────────────────────────────

func TestRegistryNotifyDiscoveryChangedReachesNotifier(t *testing.T) {
	reg, _, instances := newTestRegistry()

	var mu sync.Mutex
	var gotType string
	var gotData map[string]any
	reg.SetNotifier(func(eventType string, data map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		gotType = eventType
		gotData = data
	})

	if err := reg.AddPlugin(context.Background(), PluginConfig{ID: "p1", Type: "fake"}); err != nil {
		t.Fatalf("AddPlugin: %v", err)
	}

	// The Host each plugin receives at Init is the Registry's pluginHost
	// wrapper, which forwards NotifyDiscoveryChanged to the Registry.
	ph := instances["p1"].lastInitHost
	ph.NotifyDiscoveryChanged()

	mu.Lock()
	defer mu.Unlock()
	if gotType != "discoveryChanged" {
		t.Fatalf("expected a discoveryChanged event, got type %q", gotType)
	}
	if gotData["pluginId"] != "p1" {
		t.Fatalf("expected pluginId p1 in event data, got %+v", gotData)
	}
}

// TestRegistryStartAnnouncesMappingsOfPluginThatFailedInit covers the bug
// behind "device is gone from the Devices page but still shows as bridged on
// Discovered": when a plugin's Init fails at startup (e.g. a Tasmota plugin
// whose MQTT broker plugin has not been registered yet), its persisted
// mappings must still be announced into the vDC state store. Otherwise the
// device silently does not exist at all — the dSS is told to vanish it during
// the next vDSM handshake, while the mapping stays on disk and keeps showing
// as bridged, and no later recovery path ever announces it again.
func TestRegistryStartAnnouncesMappingsOfPluginThatFailedInit(t *testing.T) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	reg.RegisterFactory("brokenfake", func(id string) Plugin {
		p := newFakePlugin(id)
		p.initErr = errors.New("broker \"mqtt1\" not registered")
		return p
	})

	m := Mapping{PluginID: "p1", RemoteEntityID: "relay1", DSUID: "DSUID-p1-relay1", Kind: "light", Name: "Relay"}
	if _, err := reg.Mappings().Add(m); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := reg.Start(ctx, []PluginConfig{{ID: "p1", Type: "brokenfake"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, ok := reg.Plugin("p1"); ok {
		t.Fatal("expected the plugin to have failed Init")
	}
	if !host.isAnnounced(m.DSUID) {
		t.Fatal("mapping of a plugin whose Init failed was not announced — the device does not exist in the vDC state store, yet the mapping still reports it as bridged")
	}
}

// TestRegistrySupervisorAnnouncesRecoveredMappings covers the second half of
// the same failure: once the supervisor gets the plugin running, a mapping
// that is not currently in the state store must be announced, not just
// subscribed. Subscribe alone only wires up live updates, and every state
// update for an unknown device is silently dropped.
func TestRegistrySupervisorAnnouncesRecoveredMappings(t *testing.T) {
	host := newFakeHost()
	reg := NewRegistry(host, NewMappingStore())
	reg.RegisterFactory("fake", func(id string) Plugin { return newFakePlugin(id) })

	m := Mapping{PluginID: "p1", RemoteEntityID: "relay1", DSUID: "DSUID-p1-relay1", Kind: "light", Name: "Relay"}
	if _, err := reg.Mappings().Add(m); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := reg.Start(ctx, []PluginConfig{{ID: "p1", Type: "fake"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Simulate the dSS asking us to forget the device (a vDSM "remove"),
	// which drops it from the state store but leaves the mapping in place.
	if err := host.RemoveDevice(ctx, m.DSUID); err != nil {
		t.Fatalf("RemoveDevice: %v", err)
	}
	if host.isAnnounced(m.DSUID) {
		t.Fatal("precondition: device should be gone from the state store")
	}

	// Restarting the plugin must bring the device back.
	if err := reg.RestartPlugin(ctx, "p1"); err != nil {
		t.Fatalf("RestartPlugin: %v", err)
	}
	if !host.isAnnounced(m.DSUID) {
		t.Fatal("restarting the plugin did not re-announce a mapping missing from the state store")
	}
}
