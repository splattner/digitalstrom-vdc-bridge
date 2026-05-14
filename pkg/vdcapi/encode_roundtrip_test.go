package vdcapi

import (
	"encoding/hex"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/splattner/vdcgo/pkg/runtime"
)

// parseGetPropertyResponseElements parses the property elements from an encoded
// vdc_ResponseGetProperty message (field 103 of the outer envelope).
func parseGetPropertyResponseElements(frame []byte) ([]pbufPropertyElement, error) {
	// strip 2-byte length prefix
	if len(frame) >= 2 {
		frame = frame[2:]
	}
	_, _, _, sub, err := parsePbufEnvelope(frame)
	if err != nil {
		return nil, err
	}
	// field 103 = vdc_ResponseGetProperty sub-message
	inner, ok := sub[103]
	if !ok {
		return nil, nil
	}
	// parse repeated PropertyElement at field 1
	var elems []pbufPropertyElement
	for len(inner) > 0 {
		num, typ, n := protowire.ConsumeTag(inner)
		if n < 0 {
			break
		}
		inner = inner[n:]
		if num == 1 && typ == protowire.BytesType {
			b, n := protowire.ConsumeBytes(inner)
			if n < 0 {
				break
			}
			e, err := parsePbufPropertyElement(b)
			if err != nil {
				return nil, err
			}
			elems = append(elems, e)
			inner = inner[n:]
		} else {
			n2 := protowire.ConsumeFieldValue(num, typ, inner)
			if n2 < 0 {
				break
			}
			inner = inner[n2:]
		}
	}
	return elems, nil
}

// findElem looks for a named element in a slice.
func findElem(elems []pbufPropertyElement, name string) (pbufPropertyElement, bool) {
	for _, e := range elems {
		if e.Name == name {
			return e, true
		}
	}
	return pbufPropertyElement{}, false
}

// findElemInt returns the int64 value of a named element.
func findElemInt(elems []pbufPropertyElement, name string) (int64, bool) {
	e, ok := findElem(elems, name)
	if !ok {
		return 0, false
	}
	switch v := e.Value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		return int64(v), true
	}
	return 0, false
}

// TestContainerPropertiesRoundTrip verifies that container properties (maps) are
// correctly encoded in the pbuf getProperty response and can be decoded back.
func TestContainerPropertiesRoundTrip(t *testing.T) {
	state := NewStateStore()
	state.HandleEvent(runtime.Event{
		Type:     runtime.EventInit,
		Output:   "light",
		Name:     "test light",
		UniqueID: "test-uid-1",
	})
	state.HandleEvent(runtime.Event{
		Type:     runtime.EventActive,
		UniqueID: "test-uid-1",
		Active:   true,
	})
	state.HandleEvent(runtime.Event{
		Type:     runtime.EventChannel,
		UniqueID: "test-uid-1",
		Index:    0,
		Value:    50.0,
	})

	const vdcDSUID = "0123456789ABCDEFFEDCBA9876543210AA"
	s := &PbufServer{ServerConfig: ServerConfig{
		DSUID:       vdcDSUID,
		Description: "test vdc",
		State:       state,
	}}
	sess := &session{active: true, vdsmDSUID: "AABBCCDDEEFF00112233445566778899AA", apiVersion: APIVersionMax}

	// Build getProperty request for the device DSUID
	snap := state.Snapshot()
	var deviceDSUID string
	for k := range snap.Devices {
		d := snap.Devices[k]
		deviceDSUID = deviceDSUID_(vdcDSUID, d, k)
		break
	}
	if deviceDSUID == "" {
		t.Fatal("no device in state")
	}
	t.Logf("device DSUID: %s", deviceDSUID)

	// Build a getProperty request with empty query (returns all properties)
	body := make([]byte, 0, 64)
	body = protowire.AppendTag(body, 1, protowire.BytesType)
	body = protowire.AppendString(body, deviceDSUID)
	// no query elements = return all properties

	req := make([]byte, 0, 128)
	req = protowire.AppendTag(req, 1, protowire.VarintType)
	req = protowire.AppendVarint(req, pbufTypeVdsmRequestGetProp)
	req = protowire.AppendTag(req, 2, protowire.VarintType)
	req = protowire.AppendVarint(req, 7) // message_id
	req = protowire.AppendTag(req, 102, protowire.BytesType)
	req = protowire.AppendBytes(req, body)

	frames, _ := s.processPbufMessage(req, sess)
	if len(frames) != 1 {
		t.Fatalf("expected 1 response frame, got %d", len(frames))
	}
	t.Logf("response frame: %d bytes", len(frames[0]))
	t.Logf("response hex: %s", hex.EncodeToString(frames[0]))

	elems, err := parseGetPropertyResponseElements(frames[0])
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	t.Logf("top-level properties returned: %d", len(elems))
	for _, e := range elems {
		t.Logf("  prop: %q value=%v elements=%d", e.Name, e.Value, len(e.Elements))
	}

	// Verify outputDescription exists and has function=1
	outDesc, ok := findElem(elems, "outputDescription")
	if !ok {
		t.Fatal("outputDescription not found in response")
	}
	t.Logf("outputDescription has %d sub-elements", len(outDesc.Elements))
	for _, sub := range outDesc.Elements {
		t.Logf("  outputDescription.%s = %v (%T)", sub.Name, sub.Value, sub.Value)
	}

	fn, ok := findElemInt(outDesc.Elements, "function")
	if !ok {
		t.Fatal("outputDescription.function not found")
	}
	if fn != 1 {
		t.Errorf("outputDescription.function: got %d, want 1", fn)
	}

	// Verify channelDescriptions[0].channelType exists
	chDescs, ok := findElem(elems, "channelDescriptions")
	if !ok {
		t.Fatal("channelDescriptions not found in response")
	}
	t.Logf("channelDescriptions has %d channels", len(chDescs.Elements))

	ch0, ok := findElem(chDescs.Elements, "0")
	if !ok {
		t.Fatal("channelDescriptions[0] not found")
	}
	t.Logf("channelDescriptions[0] has %d sub-elements", len(ch0.Elements))
	for _, sub := range ch0.Elements {
		t.Logf("  channelDescriptions[0].%s = %v (%T)", sub.Name, sub.Value, sub.Value)
	}

	chType, ok := findElemInt(ch0.Elements, "channelType")
	if !ok {
		t.Fatal("channelDescriptions[0].channelType not found")
	}
	if chType != 1 {
		t.Errorf("channelDescriptions[0].channelType: got %d, want 1", chType)
	}

	// Verify modelFeatures exists
	mf, ok := findElem(elems, "modelFeatures")
	if !ok {
		t.Fatal("modelFeatures not found in response")
	}
	t.Logf("modelFeatures has %d sub-elements", len(mf.Elements))

	// Verify outputSettings.mode = 2
	outSettings, ok := findElem(elems, "outputSettings")
	if !ok {
		t.Fatal("outputSettings not found in response")
	}
	mode, ok := findElemInt(outSettings.Elements, "mode")
	if !ok {
		t.Fatal("outputSettings.mode not found")
	}
	if mode != 2 {
		t.Errorf("outputSettings.mode: got %d, want 2", mode)
	}
}

// deviceDSUID_ is a wrapper for deviceDSUID used in tests
func deviceDSUID_(vdcDSUID string, d ExternalDeviceState, fallbackKey string) string {
	return deviceDSUID(vdcDSUID, d, fallbackKey)
}
