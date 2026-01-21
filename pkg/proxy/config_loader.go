package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	yaml "go.yaml.in/yaml/v2"
)

// LoadConfig loads proxy configuration from a YAML or JSON file.
// If path is empty, it attempts to read VAI_CONFIG; if still empty, defaults are returned.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("VAI_CONFIG")
	}
	if path == "" {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	ext := filepath.Ext(path)
	if ext == ".json" {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse json config: %w", err)
		}
		return cfg, nil
	}
	if ext == ".yaml" || ext == ".yml" {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse yaml config: %w", err)
		}
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(data, cfg); err == nil {
		return cfg, nil
	}

	return nil, fmt.Errorf("unsupported config format: %s", ext)
}
