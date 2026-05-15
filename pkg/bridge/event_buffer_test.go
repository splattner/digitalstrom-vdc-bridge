package bridge_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/splattner/vdcgo/pkg/bridge"
)

// ── EventBuffer ───────────────────────────────────────────────────────────────

func TestEventBuffer_PublishAndSnapshot(t *testing.T) {
	buf := bridge.NewEventBuffer(10, 100)

	buf.Publish(bridge.PluginEvent{PluginID: "p1", Level: bridge.LevelInfo, Code: "a", Message: "first"})
	buf.Publish(bridge.PluginEvent{PluginID: "p1", Level: bridge.LevelWarn, Code: "b", Message: "second"})
	buf.Publish(bridge.PluginEvent{PluginID: "p2", Level: bridge.LevelInfo, Code: "c", Message: "other"})

	// All for p1
	evs := buf.Snapshot("p1", 0, "", 0)
	if len(evs) != 2 {
		t.Fatalf("expected 2 p1 events, got %d", len(evs))
	}

	// Level filter
	warns := buf.Snapshot("p1", 0, bridge.LevelWarn, 0)
	if len(warns) != 1 || warns[0].Message != "second" {
		t.Fatalf("expected 1 warn event, got %+v", warns)
	}

	// sinceSeq — skip events at or before the first p1 event
	firstSeq := evs[0].Seq
	after := buf.Snapshot("p1", firstSeq, "", 0)
	if len(after) != 1 || after[0].Message != "second" {
		t.Fatalf("expected 1 event after seq %d, got %+v", firstSeq, after)
	}

	// Global log
	all := buf.Snapshot("", 0, "", 0)
	if len(all) != 3 {
		t.Fatalf("expected 3 global events, got %d", len(all))
	}

	// Limit
	limited := buf.Snapshot("", 0, "", 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 with limit, got %d", len(limited))
	}
}

func TestEventBuffer_SeqIsMonotonicallyIncreasing(t *testing.T) {
	buf := bridge.NewEventBuffer(100, 100)
	for i := 0; i < 10; i++ {
		buf.Publish(bridge.PluginEvent{PluginID: "p", Level: bridge.LevelInfo, Code: "x", Message: fmt.Sprintf("m%d", i)})
	}
	evs := buf.Snapshot("p", 0, "", 0)
	if len(evs) != 10 {
		t.Fatalf("expected 10, got %d", len(evs))
	}
	for i := 1; i < len(evs); i++ {
		if evs[i].Seq <= evs[i-1].Seq {
			t.Fatalf("seq not monotonically increasing at index %d: %d <= %d", i, evs[i].Seq, evs[i-1].Seq)
		}
	}
}

func TestEventBuffer_PerPluginRingCap(t *testing.T) {
	buf := bridge.NewEventBuffer(3, 1000)
	for i := 0; i < 5; i++ {
		buf.Publish(bridge.PluginEvent{PluginID: "p1", Level: bridge.LevelInfo, Code: "x", Message: fmt.Sprintf("m%d", i)})
	}
	evs := buf.Snapshot("p1", 0, "", 0)
	if len(evs) != 3 {
		t.Fatalf("expected ring cap 3, got %d", len(evs))
	}
	// Should be the last 3: m2, m3, m4
	if evs[0].Message != "m2" {
		t.Fatalf("expected oldest retained to be m2, got %q", evs[0].Message)
	}
}

func TestEventBuffer_GlobalRingCap(t *testing.T) {
	buf := bridge.NewEventBuffer(1000, 4)
	for i := 0; i < 6; i++ {
		buf.Publish(bridge.PluginEvent{PluginID: "p", Level: bridge.LevelInfo, Code: "x", Message: fmt.Sprintf("g%d", i)})
	}
	all := buf.Snapshot("", 0, "", 0)
	if len(all) != 4 {
		t.Fatalf("expected global cap 4, got %d", len(all))
	}
	if all[0].Message != "g2" {
		t.Fatalf("expected oldest retained g2, got %q", all[0].Message)
	}
}

func TestEventBuffer_ClearPlugin(t *testing.T) {
	buf := bridge.NewEventBuffer(100, 100)
	buf.Publish(bridge.PluginEvent{PluginID: "p1", Level: bridge.LevelInfo, Code: "x", Message: "m"})
	buf.Publish(bridge.PluginEvent{PluginID: "p2", Level: bridge.LevelInfo, Code: "x", Message: "m"})

	buf.ClearPlugin("p1")

	evs := buf.Snapshot("p1", 0, "", 0)
	if len(evs) != 0 {
		t.Fatalf("expected 0 after clear, got %d", len(evs))
	}
	// p2 unaffected
	evs2 := buf.Snapshot("p2", 0, "", 0)
	if len(evs2) != 1 {
		t.Fatalf("expected p2 unaffected, got %d", len(evs2))
	}
}

func TestEventBuffer_Subscribe(t *testing.T) {
	buf := bridge.NewEventBuffer(100, 100)
	ch, cancel := buf.Subscribe()
	defer cancel()

	buf.Publish(bridge.PluginEvent{PluginID: "p1", Level: bridge.LevelInfo, Code: "x", Message: "live"})

	select {
	case ev := <-ch:
		if ev.Message != "live" {
			t.Fatalf("unexpected message: %q", ev.Message)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for subscriber event")
	}
}

func TestEventBuffer_SubscribeCancel(t *testing.T) {
	buf := bridge.NewEventBuffer(100, 100)
	ch, cancel := buf.Subscribe()
	cancel()

	// Should not panic; channel just won't receive anything
	buf.Publish(bridge.PluginEvent{PluginID: "p", Level: bridge.LevelInfo, Code: "x", Message: "m"})

	select {
	case <-ch:
		t.Fatal("received on cancelled subscription")
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestEventBuffer_PluginIDs(t *testing.T) {
	buf := bridge.NewEventBuffer(100, 100)
	buf.Publish(bridge.PluginEvent{PluginID: "a", Level: bridge.LevelInfo, Code: "x", Message: "m"})
	buf.Publish(bridge.PluginEvent{PluginID: "b", Level: bridge.LevelInfo, Code: "x", Message: "m"})
	buf.Publish(bridge.PluginEvent{PluginID: "a", Level: bridge.LevelInfo, Code: "x", Message: "m"})

	ids := buf.PluginIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 plugin IDs, got %d: %v", len(ids), ids)
	}
}

func TestEventBuffer_TimestampSet(t *testing.T) {
	buf := bridge.NewEventBuffer(100, 100)
	before := time.Now()
	buf.Publish(bridge.PluginEvent{PluginID: "p", Level: bridge.LevelInfo, Code: "x", Message: "m"})
	after := time.Now()

	evs := buf.Snapshot("p", 0, "", 0)
	if len(evs) == 0 {
		t.Fatal("no events")
	}
	if evs[0].Time.Before(before) || evs[0].Time.After(after) {
		t.Fatalf("timestamp %v outside expected range [%v, %v]", evs[0].Time, before, after)
	}
}
