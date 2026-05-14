package bridge

// ConfigSchema describes a plugin type's configuration so the UI can render an
// editable form for it. Schemas are static and registered alongside the
// Factory via Registry.Register.
type ConfigSchema struct {
	// Fields is the ordered list of top-level config fields.
	Fields []ConfigField `json:"fields"`
}

// ConfigField describes one configurable value.
type ConfigField struct {
	// Key is the JSON key under which the value lives in the plugin config.
	Key string `json:"key"`
	// Label is the human-readable form label.
	Label string `json:"label"`
	// Help is an optional help text rendered below the field.
	Help string `json:"help,omitempty"`
	// Type is one of: "string", "int", "bool", "password", "select", "object".
	Type string `json:"type"`
	// Default is the value used when the field is not present in the config.
	Default any `json:"default,omitempty"`
	// Required marks the field as mandatory (the UI will block submit).
	Required bool `json:"required,omitempty"`
	// Placeholder is shown in empty input fields.
	Placeholder string `json:"placeholder,omitempty"`
	// Options is the list of selectable values for Type "select".
	Options []SelectOption `json:"options,omitempty"`
	// Children is the nested field list for Type "object".
	Children []ConfigField `json:"children,omitempty"`
	// Min/Max are optional numeric bounds for Type "int".
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

// SelectOption is one entry of a "select" field.
type SelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FactoryEntry pairs a plugin Factory with metadata used by the UI and the
// HTTP config API.
type FactoryEntry struct {
	// DisplayName is the human-readable plugin type name (e.g. "Home Assistant").
	DisplayName string
	// Description is a short paragraph shown in the "add plugin" picker.
	Description string
	// Schema is the editable config schema. May be empty for plugins that
	// have no user-configurable fields.
	Schema ConfigSchema
	// Factory creates a new Plugin instance for this type.
	Factory Factory
	// Probe optionally validates a config without starting a real plugin.
	// It is invoked by POST /api/plugin-types/{type}/probe and
	// POST /api/plugins/{id}/probe. May be nil.
	Probe func(cfg map[string]any) error
}

// IsSecretField returns true for field types whose values must never be
// echoed back via the HTTP API.
func IsSecretField(f ConfigField) bool {
	return f.Type == "password"
}

// SecretKeys returns all top-level keys in s that are secret.
// Nested object fields are also walked; their keys are returned with a
// "parent.child" dotted path.
func (s ConfigSchema) SecretKeys() []string {
	var out []string
	walk(s.Fields, "", &out)
	return out
}

func walk(fields []ConfigField, prefix string, out *[]string) {
	for _, f := range fields {
		key := f.Key
		if prefix != "" {
			key = prefix + "." + f.Key
		}
		if IsSecretField(f) {
			*out = append(*out, key)
		}
		if f.Type == "object" && len(f.Children) > 0 {
			walk(f.Children, key, out)
		}
	}
}
