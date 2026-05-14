package vdcapi

import (
	"testing"
	"time"

	"github.com/splattner/vdcgo/pkg/runtime"
)

func TestStateStoreSubscribeReceivesTypedUpdates(t *testing.T) {
	store := NewStateStore()
	id, ch := store.Subscribe()
	defer store.Unsubscribe(id)

	store.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "ext", UniqueID: "u1"})
	store.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u1", Index: 0, Value: 12.5})

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case up := <-ch:
			if up.Type == runtime.EventChannel && up.Device.UniqueID == "u1" && up.Device.Channels[0] == 12.5 {
				return
			}
		case <-timer.C:
			t.Fatal("did not receive expected subscribed state update")
		}
	}
}

func TestStateStoreTracksMultipleDevices(t *testing.T) {
	store := NewStateStore()
	store.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "A", UniqueID: "u1", Tag: "A", Connection: "c1"})
	store.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "B", UniqueID: "u2", Tag: "B", Connection: "c1"})
	store.HandleEvent(runtime.Event{Type: runtime.EventChannel, UniqueID: "u2", Index: 0, Value: 80})

	snap := store.Snapshot()
	if len(snap.Devices) != 2 {
		t.Fatalf("expected two devices, got %d", len(snap.Devices))
	}
	if snap.Devices["uid:u2"].Channels[0] != 80 {
		t.Fatalf("expected uid:u2 channel=80, got %+v", snap.Devices["uid:u2"])
	}
}

func TestStateStoreTracksNonLightTypedValues(t *testing.T) {
	store := NewStateStore()
	store.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "sensor", Name: "S", UniqueID: "u3", Tag: "S", Connection: "c1"})
	store.HandleEvent(runtime.Event{Type: runtime.EventSensor, UniqueID: "u3", Index: 2, Value: 21.5})
	store.HandleEvent(runtime.Event{Type: runtime.EventInput, UniqueID: "u3", Index: 0, Value: 1})
	store.HandleEvent(runtime.Event{Type: runtime.EventButton, UniqueID: "u3", Index: 1, Value: 0})
	store.HandleEvent(runtime.Event{Type: runtime.EventButtonAction, UniqueID: "u3", Index: 1, Action: "tip"})

	snap := store.Snapshot()
	dev, ok := snap.Devices["uid:u3"]
	if !ok {
		t.Fatal("expected uid:u3 in snapshot")
	}
	if dev.Output != "sensor" {
		t.Fatalf("expected output=sensor, got %q", dev.Output)
	}
	if got := dev.Sensors[2]; got != 21.5 {
		t.Fatalf("expected sensor index 2 to be 21.5, got %v", got)
	}
	if got := dev.Inputs[0]; got != 1 {
		t.Fatalf("expected input index 0 to be 1, got %v", got)
	}
	if got := dev.Buttons[1]; got != 0 {
		t.Fatalf("expected button index 1 to be 0, got %v", got)
	}
	if got := dev.ButtonActions[1]; got != "tip" {
		t.Fatalf("expected button action index 1 to be tip, got %q", got)
	}
}
