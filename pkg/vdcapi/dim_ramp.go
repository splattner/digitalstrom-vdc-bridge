package vdcapi

import (
	"context"
	"sync"
	"time"
)

// dimRampTickInterval and dimRampFullSweepDuration govern the smooth ramp
// started by dimChannel notifications. dimRampFullSweepDuration is the time
// a ramp would take to sweep the full 0–100 range; package vars (not
// constants) so tests can override them for fast, deterministic runs — same
// pattern as pkg/server's button timing vars.
//
// The real dS wire protocol encodes dimTimeUp/dimTimeDown as an index into a
// nonlinear hardware timing table (not a literal duration), which isn't
// available here. Using one fixed, reasonable sweep duration for every
// device is a deliberate simplification: it replaces the previous behavior
// (an instant snap to 0 or 100 on every dimChannel notification) with an
// actual ramp, without pretending to reproduce the exact dS timing curve.
var (
	dimRampTickInterval      = 100 * time.Millisecond
	dimRampFullSweepDuration = 3 * time.Second
)

// dimRampManager runs one smooth ramp goroutine per addressed device on
// behalf of dimChannel notifications (mode -1/+1 starts a ramp toward 0/100;
// mode 0 stops it). Safe for concurrent use.
type dimRampManager struct {
	mu     sync.Mutex
	active map[string]dimRampHandle // keyed by device uniqueID
}

// dimRampHandle identifies one running ramp goroutine. ctx is kept alongside
// cancel purely so a finishing goroutine can tell whether it is still the
// current ramp for its uniqueID before removing the map entry — cancel
// funcs themselves aren't comparable, but the ctx pointer is.
type dimRampHandle struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDimRampManager creates an empty dimRampManager.
func NewDimRampManager() *dimRampManager {
	return &dimRampManager{active: make(map[string]dimRampHandle)}
}

// start begins a ramp for uniqueID from startValue toward 0 (direction < 0)
// or 100 (direction > 0). apply is called with each intermediate value,
// roughly every dimRampTickInterval, until stop is called or the bound is
// reached. Starting a new ramp for a uniqueID that already has one running
// cancels the previous one first. apply must not block.
func (r *dimRampManager) start(uniqueID string, direction int, startValue float64, apply func(value float64)) {
	if direction == 0 {
		return
	}
	target := 0.0
	if direction > 0 {
		target = 100.0
	}
	if startValue == target {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if prev, ok := r.active[uniqueID]; ok {
		prev.cancel()
	}
	r.active[uniqueID] = dimRampHandle{ctx: ctx, cancel: cancel}
	r.mu.Unlock()

	step := 100.0 * (dimRampTickInterval.Seconds() / dimRampFullSweepDuration.Seconds())
	if target < startValue {
		step = -step
	}

	go func() {
		ticker := time.NewTicker(dimRampTickInterval)
		defer ticker.Stop()
		value := startValue
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				value += step
				done := (step > 0 && value >= target) || (step < 0 && value <= target)
				if done {
					value = target
				}
				apply(value)
				if done {
					r.finish(uniqueID, ctx)
					return
				}
			}
		}
	}()
}

// finish removes uniqueID's map entry, but only if it still points at ctx —
// a stop() or a newer start() for the same uniqueID may have already
// replaced or removed it, and this must not clobber that.
func (r *dimRampManager) finish(uniqueID string, ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.active[uniqueID]; ok && current.ctx == ctx {
		delete(r.active, uniqueID)
	}
}

// stop cancels the active ramp for uniqueID, if any.
func (r *dimRampManager) stop(uniqueID string) {
	r.mu.Lock()
	handle, ok := r.active[uniqueID]
	if ok {
		delete(r.active, uniqueID)
	}
	r.mu.Unlock()
	if ok {
		handle.cancel()
	}
}

// stopAll cancels every active ramp. Intended for server shutdown.
func (r *dimRampManager) stopAll() {
	r.mu.Lock()
	all := r.active
	r.active = make(map[string]dimRampHandle)
	r.mu.Unlock()
	for _, handle := range all {
		handle.cancel()
	}
}
