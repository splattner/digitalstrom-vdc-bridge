package shelly

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// component identifies one Shelly RPC component instance, e.g. "switch:0".
type component struct {
	Kind  string
	Index int
}

func (c component) key() string {
	return fmt.Sprintf("%s:%d", c.Kind, c.Index)
}

// entityID builds the bridge.RemoteEntity.ID for one component of a device.
func entityID(devID string, c component) string {
	return devID + ":" + c.key()
}

// parseEntityID reverses entityID, splitting off the trailing "kind:index".
func parseEntityID(id string) (devID string, c component, ok bool) {
	idx := strings.LastIndex(id, ":")
	if idx <= 0 || idx == len(id)-1 {
		return "", component{}, false
	}
	n, err := strconv.Atoi(id[idx+1:])
	if err != nil {
		return "", component{}, false
	}
	rest := id[:idx]
	idx2 := strings.LastIndex(rest, ":")
	if idx2 <= 0 {
		return "", component{}, false
	}
	return rest[:idx2], component{Kind: rest[idx2+1:], Index: n}, true
}

// parseComponents extracts the component list from a Shelly.GetStatus result
// keyed by "kind:index" (e.g. "switch:0", "input:1"). Non-component keys
// (top-level services like "sys", "wifi", "cloud", "mqtt", "ble") have no
// numeric suffix and are skipped.
func parseComponents(status map[string]map[string]any) []component {
	var out []component
	for key := range status {
		idx := strings.LastIndex(key, ":")
		if idx <= 0 || idx == len(key)-1 {
			continue
		}
		n, err := strconv.Atoi(key[idx+1:])
		if err != nil {
			continue
		}
		out = append(out, component{Kind: key[:idx], Index: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Index < out[j].Index
	})
	return out
}

// bridgeKindFor returns the bridge.RemoteEntity Kind for a component, and
// whether this plugin currently bridges components of that kind at all.
// Unrecognised/unsupported component kinds (pm1, input, script, ...) are not
// an error — they simply aren't exposed as entities yet.
func bridgeKindFor(c component) (kind string, ok bool) {
	switch c.Kind {
	case "switch":
		return "light", true
	case "light":
		return "dimmer", true
	default:
		return "", false
	}
}

// bridgeableCount returns how many of the given components are currently
// exposed as bridgeable entities.
func bridgeableCount(components []component) int {
	n := 0
	for _, c := range components {
		if _, ok := bridgeKindFor(c); ok {
			n++
		}
	}
	return n
}

func sameComponents(a, b []component) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
