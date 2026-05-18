package bridge

import (
	"sync"
	"sync/atomic"
	"time"
)

// ActivitySource identifies who initiated the device activity event.
type ActivitySource string

const (
	// ActivitySourceVDSM means the event was a command sent by the vDSM/dSS.
	ActivitySourceVDSM ActivitySource = "vdsm"
	// ActivitySourcePlugin means the event was a state update pushed by the plugin.
	ActivitySourcePlugin ActivitySource = "plugin"
)

// DeviceActivity records a single observable event on a bridged device:
// a channel value change, scene call, or active-state transition.
type DeviceActivity struct {
	Seq      uint64         `json:"seq"`
	Time     time.Time      `json:"time"`
	DSUID    string         `json:"dsuid"`
	Source   ActivitySource `json:"source"`
	PluginID string         `json:"pluginId,omitempty"`
	// Type is one of "channel", "scene", or "active".
	Type   string   `json:"type"`
	Index  int      `json:"index,omitempty"`   // channel index (type=="channel")
	Value  *float64 `json:"value,omitempty"`   // channel value (type=="channel")
	Scene  int      `json:"scene,omitempty"`   // scene number (type=="scene")
	Active *bool    `json:"active,omitempty"`  // new state (type=="active")
}

// ActivityBuffer is a thread-safe in-memory ring buffer for DeviceActivity events.
// It stores up to perCap events per device DSUID and up to globalCap events in a
// global log. Subscribers receive a live channel of every published event.
type ActivityBuffer struct {
	mu        sync.RWMutex
	byDSUID   map[string][]DeviceActivity
	global    []DeviceActivity
	perCap    int
	globalCap int

	counter atomic.Uint64

	subMu sync.Mutex
	subs  map[chan DeviceActivity]struct{}
}

// NewActivityBuffer creates an ActivityBuffer with the given per-device and global
// ring-buffer capacities.
func NewActivityBuffer(perCap, globalCap int) *ActivityBuffer {
	return &ActivityBuffer{
		byDSUID:   make(map[string][]DeviceActivity),
		perCap:    perCap,
		globalCap: globalCap,
		subs:      make(map[chan DeviceActivity]struct{}),
	}
}

// Publish assigns a sequence number, appends to ring buffers, and fans out
// synchronously (non-blocking) to all live subscribers.
func (b *ActivityBuffer) Publish(ev DeviceActivity) {
	ev.Seq = b.counter.Add(1)
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}

	b.mu.Lock()
	slice := b.byDSUID[ev.DSUID]
	slice = append(slice, ev)
	if len(slice) > b.perCap {
		slice = slice[len(slice)-b.perCap:]
	}
	b.byDSUID[ev.DSUID] = slice

	b.global = append(b.global, ev)
	if len(b.global) > b.globalCap {
		b.global = b.global[len(b.global)-b.globalCap:]
	}
	b.mu.Unlock()

	b.subMu.Lock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // slow subscriber: drop rather than block
		}
	}
	b.subMu.Unlock()
}

// Snapshot returns stored events for the given DSUID (empty = global log),
// filtered by sinceSeq and capped to limit (0 = no limit).
func (b *ActivityBuffer) Snapshot(dsuid string, sinceSeq uint64, limit int) []DeviceActivity {
	b.mu.RLock()
	var src []DeviceActivity
	if dsuid == "" {
		src = b.global
	} else {
		src = b.byDSUID[dsuid]
	}
	out := make([]DeviceActivity, 0, len(src))
	for _, ev := range src {
		if ev.Seq <= sinceSeq {
			continue
		}
		out = append(out, ev)
	}
	b.mu.RUnlock()

	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Subscribe returns a channel that receives every subsequently published
// DeviceActivity. Call the returned cancel func to unsubscribe and close
// the channel.
func (b *ActivityBuffer) Subscribe() (<-chan DeviceActivity, func()) {
	ch := make(chan DeviceActivity, 64)
	b.subMu.Lock()
	b.subs[ch] = struct{}{}
	b.subMu.Unlock()
	return ch, func() {
		b.subMu.Lock()
		delete(b.subs, ch)
		b.subMu.Unlock()
	}
}
