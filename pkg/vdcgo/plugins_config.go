package vdcgo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/splattner/vdcgo/pkg/bridge"
)

// loadPluginConfigs reads a JSON array of bridge.PluginConfig from path.
// A missing file is not an error — an empty slice is returned.
func loadPluginConfigs(path string) ([]bridge.PluginConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var configs []bridge.PluginConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}
	return configs, nil
}

// savePluginConfigs writes the plugin config list to path atomically.
// The file is created with mode 0600 since it may contain secrets.
func savePluginConfigs(path string, configs []bridge.PluginConfig) error {
	if configs == nil {
		configs = []bridge.PluginConfig{}
	}
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugins: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %q: %w", tmp, err)
	}
	// Best-effort: ensure permissions are tight even when the file already existed.
	_ = os.Chmod(path, 0o600)
	return nil
}
