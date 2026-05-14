package wled

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/splattner/vdcgo/pkg/logging"
)

const wledServiceType = "_wled._tcp"

// discoveredDevice is a WLED device found via mDNS.
type discoveredDevice struct {
	MAC  string // normalised lowercase no-colons, from TXT "mac=..."
	Name string // mDNS instance name (friendly name from WLED)
	Addr string // "host:port"
	Ver  string // from TXT "ver=..."
	IP   net.IP
	Port int
}

// scanner continuously browses mDNS for WLED devices and calls onFound for
// each new or updated device. It runs until ctx is cancelled.
type scanner struct {
	mu      sync.RWMutex
	devices map[string]discoveredDevice // mac → device
	onFound func(discoveredDevice)
	onLost  func(mac string) // called when a previously-seen device is no longer responding (best-effort, not reliable with zeroconf)
}

func newScanner(onFound func(discoveredDevice), onLost func(string)) *scanner {
	return &scanner{
		devices: make(map[string]discoveredDevice),
		onFound: onFound,
		onLost:  onLost,
	}
}

// Run blocks until ctx is cancelled, continuously scanning for WLED devices.
func (s *scanner) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.browse(ctx); err != nil && ctx.Err() == nil {
			logging.Warn("wled_mdns_error", logging.Fields{"error": err.Error()})
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
	if err := resolver.Browse(ctx, wledServiceType, "local.", entries); err != nil {
		return fmt.Errorf("browse: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case entry, ok := <-entries:
			if !ok {
				return nil
			}
			s.handleEntry(entry)
		}
	}
}

func (s *scanner) handleEntry(entry *zeroconf.ServiceEntry) {
	if len(entry.AddrIPv4) == 0 {
		return
	}
	ip := entry.AddrIPv4[0]
	addr := fmt.Sprintf("%s:%d", ip.String(), entry.Port)

	// Parse TXT records: "mac=aabbccddeeff", "ver=0.14.0"
	var mac, ver string
	for _, txt := range entry.Text {
		if strings.HasPrefix(strings.ToLower(txt), "mac=") {
			mac = normalisedMAC(txt[4:])
		} else if strings.HasPrefix(strings.ToLower(txt), "ver=") {
			ver = txt[4:]
		}
	}
	if mac == "" {
		// No MAC in TXT records — use the IP as fallback identity (less stable).
		mac = strings.ReplaceAll(ip.String(), ".", "_")
	}

	dev := discoveredDevice{
		MAC:  mac,
		Name: entry.Instance,
		Addr: addr,
		IP:   ip,
		Port: entry.Port,
		Ver:  ver,
	}

	s.mu.Lock()
	existing, had := s.devices[mac]
	s.devices[mac] = dev
	s.mu.Unlock()

	// Only fire callback when the device is new or its address changed.
	if !had || existing.Addr != addr || existing.Name != dev.Name {
		logging.Info("wled_device_found", logging.Fields{
			"mac":  mac,
			"name": dev.Name,
			"addr": addr,
			"ver":  ver,
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

// Get returns the device with the given MAC, if known.
func (s *scanner) Get(mac string) (discoveredDevice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[mac]
	return d, ok
}

// normalisedMAC strips colons/hyphens and lowercases a MAC address string.
func normalisedMAC(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	return strings.TrimSpace(s)
}
