package vdcapi

import (
	"sync"
	"testing"
	"time"
)

// mockCommander implements Commander for tests. All fields are guarded by mu
// since dimChannel ramps call SetLightLevel from a background goroutine —
// tests must read state via snapshot(), not the fields directly, to stay
// race-detector clean.
type mockCommander struct {
	mu           sync.Mutex
	called       bool
	uniqueID     string
	value        float64
	err          error
	colorCalled  bool
	colorUID     string
	colorChannel int
	colorValue   float64
}

func (m *mockCommander) SetLightLevel(uniqueID string, value float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	m.uniqueID = uniqueID
	m.value = value
	return m.err
}

func (m *mockCommander) SetChannelValue(uniqueID string, channelIndex int, value float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if channelIndex == 0 {
		m.called = true
		m.uniqueID = uniqueID
		m.value = value
	} else {
		m.colorCalled = true
		m.colorUID = uniqueID
		m.colorChannel = channelIndex
		m.colorValue = value
	}
	return m.err
}

// reset clears call-tracking state between subtests.
func (m *mockCommander) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = false
	m.uniqueID = ""
	m.value = 0
	m.colorCalled = false
	m.colorUID = ""
	m.colorChannel = 0
	m.colorValue = 0
}

// snapshot returns a race-safe copy of the call-tracking fields.
func (m *mockCommander) snapshot() (called bool, uniqueID string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called, m.uniqueID, m.value
}

// mockSceneCommander wraps mockCommander and adds a scriptable CallScene, so
// tests can exercise the native-scene-first path in applySceneToTargets
// without every existing mockCommander-based test picking it up implicitly.
type mockSceneCommander struct {
	*mockCommander

	mu         sync.Mutex
	sceneCalls []mockSceneCall
	sceneErr   error // returned by CallScene; nil = success
}

type mockSceneCall struct {
	uniqueID string
	scene    int
}

func newMockSceneCommander() *mockSceneCommander {
	return &mockSceneCommander{mockCommander: &mockCommander{}}
}

func (m *mockSceneCommander) CallScene(uniqueID string, scene int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sceneCalls = append(m.sceneCalls, mockSceneCall{uniqueID, scene})
	return m.sceneErr
}

func (m *mockSceneCommander) sceneCallSnapshot() []mockSceneCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockSceneCall, len(m.sceneCalls))
	copy(out, m.sceneCalls)
	return out
}

// waitForCommanderCallMatching polls mc via snapshot() until it has been
// called with the given uniqueID and a value satisfying want, or the timeout
// elapses. Needed for dimChannel, whose ramp applies asynchronously from a
// background goroutine rather than synchronously within the notification
// call, so exact timing/value can't be asserted immediately.
func waitForCommanderCallMatching(t *testing.T, mc *mockCommander, wantUniqueID string, want func(value float64) bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		called, uid, value := mc.snapshot()
		if called && uid == wantUniqueID && want(value) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for matching commander call uniqueid=%s, last seen called=%t uid=%s value=%v",
				wantUniqueID, called, uid, value)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
