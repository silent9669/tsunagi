// Package config merges built-in providers with the user's config file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silent9669/tsunagi/internal/provider"
)

type Config struct {
	Default   string
	Providers []provider.Provider
}

type file struct {
	Default   string              `json:"default"`
	Providers []provider.Provider `json:"providers"`
}

// Path is ~/.config/tsunagi/config.json.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tsunagi", "config.json")
}

func Load() (Config, error) { return LoadFrom(Path()) }

func LoadFrom(path string) (Config, error) {
	cfg := Config{Default: "opencode", Providers: provider.Builtin()}
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if f.Default != "" {
		cfg.Default = f.Default
	}
	for _, up := range f.Providers {
		if up.Name == "" || up.BaseURL == "" {
			return cfg, fmt.Errorf("%s: provider needs name and base_url", path)
		}
		if up.Dialect == "" {
			up.Dialect = provider.Anthropic
		}
		replaced := false
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == up.Name {
				cfg.Providers[i], replaced = up, true
			}
		}
		if !replaced {
			cfg.Providers = append(cfg.Providers, up)
		}
	}
	return cfg, nil
}
