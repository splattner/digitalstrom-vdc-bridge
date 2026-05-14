package zigbee2mqtt

import (
	"strings"
)

// bridgeDevice mirrors the JSON entries in <base>/bridge/devices.
//
// Z2M publishes a retained JSON array of every paired device. We only care
// about a small subset of fields — friendly_name (used as topic), ieee_address
// (stable id), software_build_id (firmware), and the definition (model,
// vendor, exposes).
type bridgeDevice struct {
	IEEE            string      `json:"ieee_address"`
	FriendlyName    string      `json:"friendly_name"`
	Type            string      `json:"type"` // EndDevice / Router / Coordinator
	Supported       bool        `json:"supported"`
	Disabled        bool        `json:"disabled"`
	PowerSource     string      `json:"power_source"`
	SoftwareBuildID string      `json:"software_build_id"`
	Definition      *definition `json:"definition"`
}

type definition struct {
	Model       string   `json:"model"`
	Vendor      string   `json:"vendor"`
	Description string   `json:"description"`
	Exposes     []expose `json:"exposes"`
}

// expose describes a feature tree. For composite types like "light" or
// "switch" the inner Features list holds the actual properties (state,
// brightness, color_xy, …). For multi-endpoint devices each feature carries
// an Endpoint string and the Property name is auto-suffixed with that
// endpoint (e.g. `state_l1`).
type expose struct {
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Property string   `json:"property"`
	Endpoint string   `json:"endpoint"`
	Access   int      `json:"access"`
	Features []expose `json:"features"`
}

// endpoint represents one bridgeable entity extracted from a device's exposes.
type endpoint struct {
	// Endpoint name (e.g. "l1", "l2"); empty for single-endpoint devices.
	Name string
	// Kind is the vDC output kind: "light" (binary), "dimmer", or "colorlight".
	Kind string
	// HasBrightness / HasColor reflect the supported features.
	HasBrightness bool
	HasColor      bool
	// Property names (already endpoint-suffixed where applicable).
	StateProp      string // e.g. "state" or "state_l1"
	BrightnessProp string // e.g. "brightness"
	ColorProp      string // e.g. "color"
}

// endpoints walks the device's exposes tree and returns one endpoint entry
// per bridgeable feature group (lights and switches). Returns nil for
// devices that expose nothing actionable (sensors, coordinators, …).
func (d *bridgeDevice) endpoints() []endpoint {
	if d.Definition == nil {
		return nil
	}
	var out []endpoint
	for _, ex := range d.Definition.Exposes {
		switch ex.Type {
		case "light":
			ep := endpoint{Name: ex.Endpoint, Kind: "dimmer"}
			for _, f := range ex.Features {
				switch f.Name {
				case "state":
					ep.StateProp = pickProp(f, "state")
				case "brightness":
					ep.HasBrightness = true
					ep.BrightnessProp = pickProp(f, "brightness")
				case "color_xy", "color_hs":
					ep.HasColor = true
					ep.ColorProp = pickProp(f, "color")
				}
			}
			if ep.HasColor {
				ep.Kind = "colorlight"
			} else if !ep.HasBrightness {
				ep.Kind = "light" // light without brightness ≈ binary
			}
			if ep.StateProp == "" {
				ep.StateProp = withEP("state", ep.Name)
			}
			out = append(out, ep)
		case "switch":
			ep := endpoint{Name: ex.Endpoint, Kind: "light"}
			for _, f := range ex.Features {
				if f.Name == "state" {
					ep.StateProp = pickProp(f, "state")
				}
			}
			if ep.StateProp == "" {
				ep.StateProp = withEP("state", ep.Name)
			}
			out = append(out, ep)
		}
	}
	return out
}

// pickProp returns the explicit Property name if z2m provided one, else
// composes "<base>_<endpoint>" — matching z2m's naming convention.
func pickProp(f expose, base string) string {
	if f.Property != "" {
		return f.Property
	}
	return withEP(base, f.Endpoint)
}

func withEP(base, ep string) string {
	if ep == "" {
		return base
	}
	return base + "_" + ep
}

// entityID is the vDC remote entity id for a given endpoint of this device.
// Single-endpoint devices use the IEEE alone; multi-endpoint devices append
// the endpoint name (e.g. "0x00158d0001234567:l1").
func (d *bridgeDevice) entityID(ep endpoint) string {
	if ep.Name == "" {
		return d.IEEE
	}
	return d.IEEE + ":" + ep.Name
}

// displayName returns a readable name for the given endpoint.
func (d *bridgeDevice) displayName(ep endpoint) string {
	name := d.FriendlyName
	if name == "" {
		name = d.IEEE
	}
	if ep.Name != "" {
		name = name + " (" + ep.Name + ")"
	}
	return name
}

// stateTopic returns "<base>/<friendly_name>".
func (d *bridgeDevice) stateTopic(base string) string {
	return base + "/" + d.FriendlyName
}

// setTopic returns "<base>/<friendly_name>/set".
func (d *bridgeDevice) setTopic(base string) string {
	return base + "/" + d.FriendlyName + "/set"
}

// availabilityTopic returns "<base>/<friendly_name>/availability".
func (d *bridgeDevice) availabilityTopic(base string) string {
	return base + "/" + d.FriendlyName + "/availability"
}

// parseEntityID splits a vDC remote entity id back into IEEE + endpoint name.
// Returns ("", "") if the id is empty.
func parseEntityID(id string) (ieee, endpointName string) {
	if id == "" {
		return "", ""
	}
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}
