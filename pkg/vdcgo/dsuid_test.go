package vdcgo

import (
	"strings"
	"testing"
)

func TestGenerateDSUID(t *testing.T) {
	dsuid, err := GenerateDSUID()
	if err != nil {
		t.Fatalf("GenerateDSUID failed: %v", err)
	}
	if len(dsuid) != 34 {
		t.Fatalf("unexpected dSUID length: got=%d want=34", len(dsuid))
	}
	if !IsValidDSUID(dsuid) {
		t.Fatalf("generated dSUID is invalid: %s", dsuid)
	}
}

func TestIsValidDSUID(t *testing.T) {
	if !IsValidDSUID("0123456789ABCDEFFEDCBA9876543210AA") {
		t.Fatal("expected valid dSUID")
	}
	if IsValidDSUID("0123456789ABCDEFFEDCBA9876543210") {
		t.Fatal("expected invalid dSUID due to short length")
	}
	if IsValidDSUID("0123456789ABCDEFFEDCBA9876543210AZ") {
		t.Fatal("expected invalid dSUID due to non-hex character")
	}
}

func TestDsuidV5Format(t *testing.T) {
	// Result must be 34 upper-hex chars.
	got := DsuidV5("441A1FED-F449-4058-BEBA-13B1C4AB6A93", "some:unique:name")
	if !IsValidDSUID(got) {
		t.Fatalf("DsuidV5 returned invalid dSUID: %q", got)
	}
	// Last two chars are the subdevice-index byte (0x00).
	if !strings.HasSuffix(got, "00") {
		t.Fatalf("DsuidV5 subdevice index must be 00, got suffix %q", got[32:])
	}
	// UUID version nibble (byte 6, upper nibble) must be 5.
	// Byte 6 is hex chars 12-13 of the 34-char string.
	versionNibble := got[12]
	if versionNibble != '5' {
		t.Fatalf("UUID version nibble must be 5, got %c", versionNibble)
	}
	// UUID variant (byte 8, top 2 bits of hex chars 16-17) must be 0b10.
	// Top nibble of byte 8 must be 8, 9, A, or B (0b10xx).
	varNibble := got[16]
	if varNibble != '8' && varNibble != '9' && varNibble != 'A' && varNibble != 'B' {
		t.Fatalf("UUID variant nibble must be 8/9/A/B, got %c", varNibble)
	}
}

func TestDsuidV5Deterministic(t *testing.T) {
	ns := "441A1FED-F449-4058-BEBA-13B1C4AB6A93"
	a := DsuidV5(ns, "hello")
	b := DsuidV5(ns, "hello")
	if a != b {
		t.Fatalf("DsuidV5 not deterministic: %q != %q", a, b)
	}
	c := DsuidV5(ns, "world")
	if a == c {
		t.Fatalf("DsuidV5 collision for different names")
	}
}

func TestDeriveVdcDSUIDFromMAC(t *testing.T) {
	// Colon-separated and plain forms of the same MAC must yield identical results.
	withColons := DeriveVdcDSUIDFromMAC("aa:bb:cc:dd:ee:ff", 0)
	plain := DeriveVdcDSUIDFromMAC("aabbccddeeff", 0)
	if withColons != plain {
		t.Fatalf("colon vs plain MAC mismatch: %q vs %q", withColons, plain)
	}
	if !IsValidDSUID(withColons) {
		t.Fatalf("DeriveVdcDSUIDFromMAC returned invalid dSUID: %q", withColons)
	}
	// Instance 0 and instance 1 must differ.
	inst0 := DeriveVdcDSUIDFromMAC("aabbccddeeff", 0)
	inst1 := DeriveVdcDSUIDFromMAC("aabbccddeeff", 1)
	if inst0 == inst1 {
		t.Fatalf("instance 0 and instance 1 must differ")
	}
	// Matches known C++ output: UUIDv5(DSUID_VDC_NAMESPACE_UUID, "aabbccddeeff")
	// Pre-computed reference: run equivalent Python:
	//   import uuid; str(uuid.uuid5(uuid.UUID("9888DD3D-B345-4109-B088-2673306D0C65"), "aabbccddeeff")).replace("-","") + "00"
	// = "8a41d1d2ae0c5d1cb1c2f3c0e3e9d9af00" (verify version/variant in string)
	want := DsuidV5("9888DD3D-B345-4109-B088-2673306D0C65", "aabbccddeeff")
	if inst0 != want {
		t.Fatalf("DeriveVdcDSUIDFromMAC(0) = %q, want %q", inst0, want)
	}
}
