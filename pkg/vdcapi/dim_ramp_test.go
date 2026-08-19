package vdcapi

import (
	"sync"
	"testing"
	"time"
)

// withFastRamp overrides the ramp timing vars for the duration of a test and
// restores them afterward.
func withFastRamp(t *testing.T, tick, sweep time.Duration) {
	t.Helper()
	prevTick, prevSweep := dimRampTickInterval, dimRampFullSweepDuration
	dimRampTickInterval, dimRampFullSweepDuration = tick, sweep
	t.Cleanup(func() {
		dimRampTickInterval, dimRampFullSweepDuration = prevTick, prevSweep
	})
}

// rampRecorder collects apply() calls in order, safe for concurrent use.
type rampRecorder struct {
	mu     sync.Mutex
	values []float64
}

func (r *rampRecorder) record(v float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = append(r.values, v)
}

func (r *rampRecorder) snapshot() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]float64, len(r.values))
	copy(out, r.values)
	return out
}

func (r *rampRecorder) waitForLen(t *testing.T, n int, timeout time.Duration) []float64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		v := r.snapshot()
		if len(v) >= n {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d apply() calls, got %d: %v", n, len(v), v)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDimRampManagerRampsUpToTarget(t *testing.T) {
	withFastRamp(t, time.Millisecond, 10*time.Millisecond)
	r := NewDimRampManager()
	rec := &rampRecorder{}

	r.start("dev1", 1, 0, rec.record)

	// 10ms sweep at 1ms ticks ≈ 10 steps of 10 each, landing exactly on 100.
	waitForFloatEquals(t, rec, 100, time.Second)

	values := rec.snapshot()
	if len(values) == 0 {
		t.Fatal("expected at least one apply() call")
	}
	last := values[len(values)-1]
	if last != 100 {
		t.Fatalf("expected ramp to land exactly on 100, got %v (all: %v)", last, values)
	}
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			t.Fatalf("expected monotonically increasing values, got %v", values)
		}
	}
}

func TestDimRampManagerRampsDownToTarget(t *testing.T) {
	withFastRamp(t, time.Millisecond, 10*time.Millisecond)
	r := NewDimRampManager()
	rec := &rampRecorder{}

	r.start("dev1", -1, 100, rec.record)

	waitForFloatEquals(t, rec, 0, time.Second)
	values := rec.snapshot()
	for i := 1; i < len(values); i++ {
		if values[i] > values[i-1] {
			t.Fatalf("expected monotonically decreasing values, got %v", values)
		}
	}
}

func TestDimRampManagerStopHaltsRamp(t *testing.T) {
	// A slow sweep relative to the tick so we can reliably stop it midway.
	withFastRamp(t, 2*time.Millisecond, 2*time.Second)
	r := NewDimRampManager()
	rec := &rampRecorder{}

	r.start("dev1", 1, 0, rec.record)
	rec.waitForLen(t, 2, time.Second) // let a couple of ticks land
	r.stop("dev1")

	countAtStop := len(rec.snapshot())
	time.Sleep(50 * time.Millisecond) // long enough for several more ticks if not actually stopped
	countAfter := len(rec.snapshot())
	if countAfter != countAtStop {
		t.Fatalf("expected no further apply() calls after stop, got %d before / %d after", countAtStop, countAfter)
	}
	last := rec.snapshot()[countAfter-1]
	if last <= 0 || last >= 100 {
		t.Fatalf("expected ramp to be stopped mid-range, landed at %v", last)
	}
}

func TestDimRampManagerRestartReplacesPreviousRamp(t *testing.T) {
	withFastRamp(t, time.Millisecond, 20*time.Millisecond)
	r := NewDimRampManager()
	up := &rampRecorder{}
	down := &rampRecorder{}

	r.start("dev1", 1, 0, up.record)
	time.Sleep(3 * time.Millisecond) // let the up-ramp make some progress
	r.start("dev1", -1, 50, down.record)

	waitForFloatEquals(t, down, 0, time.Second)

	// The up-ramp must not have kept applying after being superseded.
	upCountAtSwitch := len(up.snapshot())
	time.Sleep(30 * time.Millisecond)
	if len(up.snapshot()) != upCountAtSwitch {
		t.Fatalf("expected superseded ramp to stop applying, count grew from %d to %d", upCountAtSwitch, len(up.snapshot()))
	}
}

func TestDimRampManagerNoopWhenAlreadyAtBound(t *testing.T) {
	withFastRamp(t, time.Millisecond, 10*time.Millisecond)
	r := NewDimRampManager()
	rec := &rampRecorder{}

	r.start("dev1", 1, 100, rec.record) // already at the target
	time.Sleep(30 * time.Millisecond)
	if len(rec.snapshot()) != 0 {
		t.Fatalf("expected no apply() calls when already at bound, got %v", rec.snapshot())
	}
}

func TestDimRampManagerStopWithoutActiveRampIsNoop(t *testing.T) {
	r := NewDimRampManager()
	r.stop("never-started") // must not panic
}

// waitForFloatEquals polls rec until its most recent recorded value equals
// want or the timeout elapses.
func waitForFloatEquals(t *testing.T, rec *rampRecorder, want float64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		v := rec.snapshot()
		if len(v) > 0 && v[len(v)-1] == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for last value to reach %v, got %v", want, v)
		}
		time.Sleep(time.Millisecond)
	}
}
