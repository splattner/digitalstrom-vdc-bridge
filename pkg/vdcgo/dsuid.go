package vdcgo

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var dsuidRE = regexp.MustCompile(`^[0-9A-Fa-f]{34}$`)

// Namespace UUIDs matching C++ p44vdc constants (dsuid.hpp).
const (
	// dsuidVdcNamespaceUUID is used to derive a vDC host DSUID from a MAC address string.
	// Matches C++ DSUID_VDC_NAMESPACE_UUID.
	dsuidVdcNamespaceUUID = "9888DD3D-B345-4109-B088-2673306D0C65"
)

// DsuidV5 returns a 34-char dSUID derived via UUIDv5 (RFC 4122), matching
// C++ DsUid::setNameInSpace(name, namespaceUUID).
// namespaceHex is a UUID string (dashes optional, must decode to exactly 16 bytes).
// The 17th byte (subdevice index) is always 0x00.
func DsuidV5(namespaceHex, name string) string {
	nsHex := strings.ReplaceAll(namespaceHex, "-", "")
	nsBytes, _ := hex.DecodeString(nsHex)
	h := sha1.New()
	h.Write(nsBytes[:16])
	h.Write([]byte(name))
	sum := h.Sum(nil)
	// RFC 4122 §4.3: set version field (bits 12-15 of byte 6) to 5
	sum[6] = (sum[6] & 0x0F) | 0x50
	// RFC 4122 §4.3: set variant field (bits 6-7 of byte 8) to 0b10
	sum[8] = (sum[8] & 0x3F) | 0x80
	// dSUID = 16 UUID bytes (upper-hex) + subdevice index byte "00"
	return strings.ToUpper(hex.EncodeToString(sum[:16])) + "00"
}

// DeriveVdcDSUIDFromMAC returns a deterministic vDC host DSUID from a MAC address string,
// matching C++ VdcHost::loadAndFixDsUID (DSUID_VDC_NAMESPACE_UUID + mac string).
// macAddress may be "aabbccddeeff", "AA:BB:CC:DD:EE:FF", or similar colon/dash-separated form.
// instanceNo should be 0 for a single instance, or ≥1 to append "_instanceNo" (matching C++
// mVdcHostInstance > 0 case).
func DeriveVdcDSUIDFromMAC(macAddress string, instanceNo int) string {
	m := strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(macAddress))
	if instanceNo > 0 {
		m = fmt.Sprintf("%s_%d", m, instanceNo)
	}
	return DsuidV5(dsuidVdcNamespaceUUID, m)
}

// GenerateDSUID creates a random 34-hex-digit dSUID.
// Use DeriveVdcDSUIDFromMAC instead when a stable MAC address is available.
func GenerateDSUID() (string, error) {
	buf := make([]byte, 17)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}

// IsValidDSUID reports whether s is a valid 34-digit hexadecimal dSUID.
func IsValidDSUID(s string) bool {
	return dsuidRE.MatchString(s)
}
