// Package config merges built-in providers with the user's config file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/provider"
)

type Config struct {
	Default string
	// Aliases rewrite a model name before routing, e.g. "fast" or the
	// "claude-haiku-5-5" Claude Code uses for background work.
	Aliases map[string]string
	// Providers are the built-in ones plus the user's own.
	Providers []provider.Provider
	// Catalog holds providers from the synced models.dev list that are not in
	// Providers.
	Catalog []provider.Provider
}

// All returns every provider the router may route to.
func (c Config) All() []provider.Provider {
	return append(append([]provider.Provider{}, c.Providers...), c.Catalog...)
}

type file struct {
	Default   string              `json:"default"`
	Providers []provider.Provider `json:"providers"`
	Models    map[string]string   `json:"models"`
}

// Path is ~/.config/tsunagi/config.json.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tsunagi", "config.json")
}

// Load reads the user's config and adds the synced catalogue behind it.
func Load() (Config, error) {
	cfg, err := LoadFrom(Path())
	if err != nil {
		return cfg, err
	}
	known := map[string]bool{}
	for _, p := range cfg.Providers {
		known[p.Name] = true
	}
	for _, e := range catalog.Load() {
		if !known[e.ID] {
			cfg.Catalog = append(cfg.Catalog, e.Provider())
		}
	}
	return cfg, nil
}

// Save writes the user's own providers and default back to the config file,
// keeping every other setting in it (such as model aliases).
func Save(path string, def string, user []provider.Provider) error {
	doc := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if user == nil {
		user = []provider.Provider{}
	}
	pb, err := json.Marshal(user)
	if err != nil {
		return err
	}
	doc["providers"] = pb
	if def != "" {
		db, _ := json.Marshal(def)
		doc["default"] = db
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	// Headers in this file can carry credentials, so keep it private.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600) // WriteFile keeps the mode of an existing file
}

// User returns only what the config file at path defines.
func User(path string) (string, []provider.Provider, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	return f.Default, f.Providers, nil
}

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
	cfg.Aliases = f.Models
	for _, up := range f.Providers {
		if up.Name == "" || up.BaseURL == "" {
			return cfg, fmt.Errorf("%s: provider needs name and base_url", path)
		}
		if !provider.ValidName(up.Name) {
			return cfg, fmt.Errorf("%s: provider name %q must be lowercase letters, digits and dashes", path, up.Name)
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
