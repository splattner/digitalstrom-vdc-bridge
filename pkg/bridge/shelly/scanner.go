package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/splattner/vdcgo/pkg/logging"
)

const (
	shellyServiceType = "_shelly._tcp"
	scanSrc           = "vdcgo-shelly-scan"
)

// discoveredDevice is a Shelly Gen2+ device found via mDNS and enriched via
// its local RPC API. Identity is the device's canonical id (e.g.
// "shellyplus1pm-441793a66a4c") from Shelly.GetDeviceInfo — not the mDNS
// instance name, which is unstable: a device with a configured friendly name
// advertises a second, differently-named _shelly._tcp instance at the same
// address, and TXT records carry no id/MAC at all (only gen/app/ver).
type discoveredDevice struct {
	ID          string // canonical device id
	Name        string // configured device name, if any ("" if unset)
	Model       string // "app" field, e.g. "Plus1PM"
	Gen         int
	FW          string // "ver" field
	Addr        string // "host:port"
	AuthEnabled bool
	Components  []component
}

// deviceInfo mirrors the fields we use from Shelly.GetDeviceInfo.
type deviceInfo struct {
	ID          string  `json:"id"`
	Model       string  `json:"model"`
	Gen         int     `json:"gen"`
	App         string  `json:"app"`
	Ver         string  `json:"ver"`
	Name        *string `json:"name"`
	AuthEnabled bool    `json:"auth_en"`
}

// scanner continuously browses mDNS for Shelly Gen2+ devices, enriches each
// newly-seen address with its canonical device id and component list via the
// local RPC API, and calls onFound for each new or changed device. It runs
// until ctx is cancelled.
type scanner struct {
	mu         sync.RWMutex
	devices    map[string]discoveredDevice // device id -> device
	byAddr     map[string]string           // addr -> device id, once resolved
	inFlight   map[string]bool             // addr currently being enriched
	warnedGen1 map[string]bool             // addr -> already logged as skipped (avoid log spam)

	onFound func(discoveredDevice)
	// onStart is called each time a browse cycle successfully begins — used
	// by the plugin to report an honest "connected" status rather than one
	// hardcoded the moment Init returns.
	onStart func()
	onError func(error) // best-effort; nil is fine
}

func newScanner(onFound func(discoveredDevice), onStart func(), onError func(error)) *scanner {
	return &scanner{
		devices:    make(map[string]discoveredDevice),
		byAddr:     make(map[string]string),
		inFlight:   make(map[string]bool),
		warnedGen1: make(map[string]bool),
		onFound:    onFound,
		onStart:    onStart,
		onError:    onError,
	}
}

// Run blocks until ctx is cancelled, continuously scanning for Shelly devices.
func (s *scanner) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.browse(ctx); err != nil && ctx.Err() == nil {
			logging.Warn("shelly_mdns_error", logging.Fields{"error": err.Error()})
			if s.onError != nil {
				s.onError(err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			// brief pause before re-creating resolver on error
		}
	}
}

func (s *scanner) browse(ctx context.Context) error {
	resolver, err := zeroconf.NewResolver()
	if err != nil {
		return fmt.Errorf("create resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	if err := resolver.Browse(ctx, shellyServiceType, "local.", entries); err != nil {
		return fmt.Errorf("browse: %w", err)
	}
	if s.onStart != nil {
		s.onStart()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case entry, ok := <-entries:
			if !ok {
				return nil
			}
			s.handleEntry(ctx, entry)
		}
	}
}

// handleEntry filters non-Gen2+ advertisements and kicks off asynchronous
// enrichment for any address not already known or in flight. Enrichment is
// deliberately async — it makes network calls and must not block the mDNS
// entries channel this is called from.
func (s *scanner) handleEntry(ctx context.Context, entry *zeroconf.ServiceEntry) {
	if len(entry.AddrIPv4) == 0 {
		return
	}
	ip := entry.AddrIPv4[0]
	addr := fmt.Sprintf("%s:%d", ip.String(), entry.Port)

	gen := 0
	for _, txt := range entry.Text {
		if strings.HasPrefix(strings.ToLower(txt), "gen=") {
			gen, _ = strconv.Atoi(txt[4:])
		}
	}
	if gen < 2 {
		s.mu.Lock()
		alreadyWarned := s.warnedGen1[addr]
		s.warnedGen1[addr] = true
		s.mu.Unlock()
		if !alreadyWarned {
			logging.Info("shelly_skip_gen1", logging.Fields{"addr": addr, "instance": entry.Instance})
		}
		return
	}

	s.mu.Lock()
	_, known := s.byAddr[addr]
	inflight := s.inFlight[addr]
	if !known && !inflight {
		s.inFlight[addr] = true
	}
	s.mu.Unlock()

	if known || inflight {
		return
	}

	go s.enrich(ctx, addr)
}

// enrich resolves the canonical device id and component list for a
// newly-seen address via two RPC calls, then records the device and fires
// onFound if it is new or changed. Devices with authentication enabled are
// logged and skipped — Gen2+ auth is not yet supported (see plan Deferred).
func (s *scanner) enrich(ctx context.Context, addr string) {
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, addr)
		s.mu.Unlock()
	}()

	enrichCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	info, err := fetchDeviceInfo(enrichCtx, addr)
	if err != nil {
		logging.Warn("shelly_enrich_device_info_error", logging.Fields{"addr": addr, "error": err.Error()})
		if s.onError != nil {
			s.onError(fmt.Errorf("shelly %s: %w", addr, err))
		}
		return
	}
	if info.AuthEnabled {
		logging.Warn("shelly_auth_enabled_skip", logging.Fields{"addr": addr, "id": info.ID})
		if s.onError != nil {
			s.onError(fmt.Errorf("shelly %s (%s): authentication is enabled, unsupported — skipping", addr, info.ID))
		}
		return
	}

	status, err := fetchStatus(enrichCtx, addr, info.ID)
	if err != nil {
		logging.Warn("shelly_enrich_status_error", logging.Fields{"addr": addr, "error": err.Error()})
		if s.onError != nil {
			s.onError(fmt.Errorf("shelly %s: %w", addr, err))
		}
		return
	}

	name := ""
	if info.Name != nil {
		name = *info.Name
	}
	dev := discoveredDevice{
		ID:          info.ID,
		Name:        name,
		Model:       info.App,
		Gen:         info.Gen,
		FW:          info.Ver,
		Addr:        addr,
		AuthEnabled: info.AuthEnabled,
		Components:  parseComponents(status),
	}

	s.mu.Lock()
	existing, had := s.devices[dev.ID]
	s.devices[dev.ID] = dev
	s.byAddr[addr] = dev.ID
	s.mu.Unlock()

	if !had || existing.Addr != dev.Addr || !sameComponents(existing.Components, dev.Components) {
		logging.Info("shelly_device_found", logging.Fields{
			"id": dev.ID, "model": dev.Model, "addr": dev.Addr, "gen": dev.Gen,
		})
		if s.onFound != nil {
			s.onFound(dev)
		}
	}
}

// All returns a snapshot of all currently known devices.
func (s *scanner) All() []discoveredDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]discoveredDevice, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	return out
}

// Get returns the device with the given canonical id, if known.
func (s *scanner) Get(id string) (discoveredDevice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[id]
	return d, ok
}

// fetchDeviceInfo calls Shelly.GetDeviceInfo to resolve a device's canonical
// identity ahead of anything else — TXT records alone cannot provide it.
func fetchDeviceInfo(ctx context.Context, addr string) (deviceInfo, error) {
	result, err := callHTTP(ctx, addr, scanSrc, "Shelly.GetDeviceInfo", nil)
	if err != nil {
		return deviceInfo{}, err
	}
	var info deviceInfo
	if err := json.Unmarshal(result, &info); err != nil {
		return deviceInfo{}, fmt.Errorf("decode device info: %w", err)
	}
	return info, nil
}

// fetchStatus calls Shelly.GetStatus and returns it as a component-keyed map
// (e.g. "switch:0" -> {"output": true, ...}), skipping any top-level key that
// isn't a JSON object (there are none in practice, but this stays robust
// against unexpected future component shapes).
func fetchStatus(ctx context.Context, addr, src string) (map[string]map[string]any, error) {
	result, err := callHTTP(ctx, addr, src, "Shelly.GetStatus", nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(result, &raw); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	out := make(map[string]map[string]any, len(raw))
	for k, v := range raw {
		var sub map[string]any
		if err := json.Unmarshal(v, &sub); err != nil {
			continue
		}
		out[k] = sub
	}
	return out, nil
}
