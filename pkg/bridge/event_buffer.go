package bridge

import (
	"sync"
	"sync/atomic"
	"time"
)

// EventSink receives structured plugin events. Implemented by EventBuffer.
type EventSink interface {
	Publish(ev PluginEvent)
}

// EventBuffer is a thread-safe in-memory ring buffer for PluginEvents.
//
// It stores up to perCap events per plugin and up to globalCap events in a
// global log. Subscribers receive a live channel of every published event.
// No background goroutine is required — fanout is synchronous and non-blocking.
type EventBuffer struct {
	mu        sync.RWMutex
	byPlugin  map[string][]PluginEvent
	global    []PluginEvent
	perCap    int
	globalCap int

	counter atomic.Uint64

	subMu sync.Mutex
	subs  map[chan PluginEvent]struct{}
}

// NewEventBuffer creates an EventBuffer with the given per-plugin and global
// ring-buffer capacities.
func NewEventBuffer(perCap, globalCap int) *EventBuffer {
	return &EventBuffer{
		byPlugin:  make(map[string][]PluginEvent),
		perCap:    perCap,
		globalCap: globalCap,
		subs:      make(map[chan PluginEvent]struct{}),
	}
}

// Publish assigns a sequence number, timestamps, appends to the ring buffers
// and fans out synchronously (non-blocking) to all live subscribers.
func (b *EventBuffer) Publish(ev PluginEvent) {
	ev.Seq = b.counter.Add(1)
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}

	b.mu.Lock()
	// append to per-plugin ring
	slice := b.byPlugin[ev.PluginID]
	slice = append(slice, ev)
	if len(slice) > b.perCap {
		slice = slice[len(slice)-b.perCap:]
	}
	b.byPlugin[ev.PluginID] = slice

	// append to global ring
	b.global = append(b.global, ev)
	if len(b.global) > b.globalCap {
		b.global = b.global[len(b.global)-b.globalCap:]
	}
	b.mu.Unlock()

	// synchronous non-blocking fanout — we hold subMu briefly
	b.subMu.Lock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // slow subscriber: drop rather than block plugin code
		}
	}
	b.subMu.Unlock()
}

// Snapshot returns a copy of stored events matching the given criteria.
//   - pluginID: if non-empty, restrict to that plugin; otherwise return global log.
//   - sinceSeq: only return events with Seq > sinceSeq (use 0 for all).
//   - level: if non-empty, only return events at exactly that level.
//   - limit: if > 0, return at most the last limit matching events.
func (b *EventBuffer) Snapshot(pluginID string, sinceSeq uint64, level LogLevel, limit int) []PluginEvent {
	b.mu.RLock()
	var src []PluginEvent
	if pluginID == "" {
		src = b.global
	} else {
		src = b.byPlugin[pluginID]
	}

	out := make([]PluginEvent, 0, len(src))
	for _, ev := range src {
		if ev.Seq <= sinceSeq {
			continue
		}
		if level != "" && ev.Level != level {
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

// PluginIDs returns the list of plugin IDs that have at least one event.
func (b *EventBuffer) PluginIDs() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ids := make([]string, 0, len(b.byPlugin))
	for id := range b.byPlugin {
		ids = append(ids, id)
	}
	return ids
}

// ClearPlugin removes all stored events for the given plugin.
func (b *EventBuffer) ClearPlugin(pluginID string) {
	b.mu.Lock()
	delete(b.byPlugin, pluginID)
	b.mu.Unlock()
}

// Subscribe returns a channel that receives all future PluginEvents and a
// cancel function that must be called to unsubscribe.
// The channel is buffered (64); if the consumer is too slow events are dropped.
func (b *EventBuffer) Subscribe() (<-chan PluginEvent, func()) {
	ch := make(chan PluginEvent, 64)
	b.subMu.Lock()
	b.subs[ch] = struct{}{}
	b.subMu.Unlock()

	cancel := func() {
		b.subMu.Lock()
		delete(b.subs, ch)
		b.subMu.Unlock()
	}
	return ch, cancel
}
