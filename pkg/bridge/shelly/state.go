package shelly

import (
	"strings"
	"sync"
)

// deviceStatus is a merged, in-memory cache of a Shelly device's component
// status. NotifyStatus pushes are deltas (only the fields that changed, and a
// field set to JSON null means it disappeared), so this must merge onto the
// existing cache rather than replace it — a fresh device connection seeds it
// from a full Shelly.GetStatus response, which merges the same way.
type deviceStatus struct {
	mu         sync.Mutex
	components map[string]map[string]any // "switch:0" -> {"output": true, ...}
}

func newDeviceStatus() *deviceStatus {
	return &deviceStatus{components: make(map[string]map[string]any)}
}

// merge applies one status payload (a full snapshot or a NotifyStatus delta)
// onto the cache. The "ts" top-level key is not component data and is
// ignored.
func (s *deviceStatus) merge(payload map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range payload {
		if k == "ts" {
			continue
		}
		sub, ok := v.(map[string]any)
		if !ok {
			continue
		}
		existing := s.components[k]
		if existing == nil {
			existing = make(map[string]any, len(sub))
			s.components[k] = existing
		}
		for fk, fv := range sub {
			if fv == nil {
				delete(existing, fk)
			} else {
				existing[fk] = fv
			}
		}
	}
}

// snapshot returns a deep-enough copy of the cache (fields are not further
// copied, but the map structure is) so callers can read it without holding
// the lock.
func (s *deviceStatus) snapshot() map[string]map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]map[string]any, len(s.components))
	for k, v := range s.components {
		cp := make(map[string]any, len(v))
		for fk, fv := range v {
			cp[fk] = fv
		}
		out[k] = cp
	}
	return out
}

// component returns a copy of one component's cached fields.
func (s *deviceStatus) component(key string) (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.components[key]
	if !ok {
		return nil, false
	}
	cp := make(map[string]any, len(v))
	for fk, fv := range v {
		cp[fk] = fv
	}
	return cp, true
}

// lookupDotted reads a possibly-nested field, e.g. "aenergy.total" reads
// fields["aenergy"]["total"]. Returns the raw decoded value and whether it
// was present at every level of the path.
func lookupDotted(fields map[string]any, path string) (any, bool) {
	if fields == nil {
		return nil, false
	}
	var cur any = fields
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func numberFieldDotted(fields map[string]any, path string) (float64, bool) {
	v, ok := lookupDotted(fields, path)
	if !ok {
		return 0, false
	}
	n, ok := v.(float64)
	return n, ok
}

func boolField(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func numberField(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	n, ok := v.(float64)
	return n, ok
}

// switchChannelValue derives the dS channel-0 value (0 or 100) from a
// switch:N component's cached fields.
func switchChannelValue(fields map[string]any) (float64, bool) {
	on, ok := boolField(fields, "output")
	if !ok {
		return 0, false
	}
	if on {
		return 100, true
	}
	return 0, true
}

// lightChannelValue derives the dS channel-0 brightness (0-100) from a
// light:N component's cached fields. Shelly's brightness range is already
// 0-100, so this only clamps and forces 0 when off.
func lightChannelValue(fields map[string]any) (float64, bool) {
	on, ok := boolField(fields, "output")
	if !ok {
		return 0, false
	}
	if !on {
		return 0, true
	}
	if bri, ok := numberField(fields, "brightness"); ok {
		return clampF(bri, 0, 100), true
	}
	return 100, true
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
