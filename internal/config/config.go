// Package config merges built-in providers with the user's config file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/provider"
)

// Combo is an ordered list of provider/model targets tried in turn.
type Combo struct {
	Targets  []string `json:"targets"`
	Strategy string   `json:"strategy,omitempty"` // priority (default), fastest, round_robin
	// Auto marks a combo tsunagi generated (free/auto); only those are ever
	// regenerated, user-written combos are never touched.
	Auto bool `json:"auto,omitempty"`
}

// FreeConfig holds settings for the free-model commands.
type FreeConfig struct {
	// AllowTrains names registry providers whose data policy is "trains" that
	// the user explicitly allows in free/auto.
	AllowTrains []string `json:"allow_trains,omitempty"`
}

// UIConfig holds appearance settings for the interactive interface. The
// valid look and theme names mirror internal/tui.
type UIConfig struct {
	Look  string `json:"look,omitempty"`
	Theme string `json:"theme,omitempty"`
	Anim  *bool  `json:"anim,omitempty"`
	// Onboarded marks that the first-run wizard has completed.
	Onboarded *bool `json:"onboarded,omitempty"`
}

// uiLooks and uiThemes are the accepted values for the ui config keys; they
// mirror art.Looks and tui.ThemeNames.
var (
	uiLooks  = []string{"braille", "dither", "halftone", "bricks", "gradient"}
	uiThemes = []string{"mono", "amber", "matrix", "newsprint", "colour"}
)

type Config struct {
	Default string
	// Aliases rewrite a model name before routing, e.g. "fast" or the
	// "claude-haiku-5-5" Claude Code uses for background work.
	Aliases map[string]string
	// Combos maps a model name to an ordered set of targets.
	Combos map[string]Combo
	// Tiers maps a Claude tier (opus, sonnet, haiku) to a model or combo.
	Tiers map[string]string
	// ClaudeFallback: see the router; "" means off.
	ClaudeFallback string
	// Free holds free-model settings.
	Free FreeConfig
	// UI holds appearance settings for the interactive interface.
	UI UIConfig
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
	Combos    map[string]Combo    `json:"combos"`
	Tiers     map[string]string   `json:"tiers"`
	// ClaudeFallback is a combo name or provider/model that takes a chat
	// request when Anthropic answers it with 429 or 529.
	ClaudeFallback string     `json:"claude_fallback"`
	Free           FreeConfig `json:"free"`
	UI             UIConfig   `json:"ui"`
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
		// Entries with an empty Dialect are capabilities-only: models.dev
		// providers tsunagi cannot speak to never become routable providers.
		if e.Dialect != "" && !known[e.ID] {
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

// SetKey rewrites one top-level key of the config file, keeping every other
// key as it was. A nil value removes the key.
func SetKey(path, key string, v any) error {
	doc := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if v == nil {
		delete(doc, key)
	} else {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		doc[key] = b
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
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
	comboNames := map[string]bool{}
	for name := range f.Combos {
		comboNames[name] = true
	}
	for name, c := range f.Combos {
		if name == "" {
			return cfg, fmt.Errorf("%s: combo name must not be empty", path)
		}
		if strings.HasPrefix(name, "claude-") {
			return cfg, fmt.Errorf("%s: combo %q must not start with claude-", path, name)
		}
		if strings.Contains(name, "@") {
			return cfg, fmt.Errorf("%s: combo %q must not contain @", path, name)
		}
		if len(c.Targets) == 0 {
			return cfg, fmt.Errorf("%s: combo %q needs at least one target", path, name)
		}
		switch c.Strategy {
		case "", "priority", "fastest", "round_robin", "cheapest", "context", "least_used":
		default:
			return cfg, fmt.Errorf("%s: combo %q: strategy must be priority, fastest, round_robin, cheapest, context or least_used", path, name)
		}
		for _, ts := range c.Targets {
			if err := checkComboTarget(ts, comboNames); err != nil {
				return cfg, fmt.Errorf("%s: combo %q: %v", path, name, err)
			}
		}
	}
	for tier := range f.Tiers {
		if tier != "opus" && tier != "sonnet" && tier != "haiku" {
			return cfg, fmt.Errorf("%s: tier %q must be opus, sonnet or haiku", path, tier)
		}
	}
	if fb := f.ClaudeFallback; fb != "" {
		if _, isCombo := f.Combos[fb]; !isCombo {
			if err := checkComboTarget(fb, comboNames); err != nil {
				return cfg, fmt.Errorf("%s: claude_fallback: %v", path, err)
			}
		}
	}
	for _, name := range f.Free.AllowTrains {
		if !provider.ValidName(name) {
			return cfg, fmt.Errorf("%s: free.allow_trains entry %q is not a valid provider name", path, name)
		}
	}
	if f.UI.Look != "" && !slices.Contains(uiLooks, f.UI.Look) {
		return cfg, fmt.Errorf("%s: ui.look must be one of %s", path, strings.Join(uiLooks, ", "))
	}
	if f.UI.Theme != "" && !slices.Contains(uiThemes, f.UI.Theme) {
		return cfg, fmt.Errorf("%s: ui.theme must be one of %s", path, strings.Join(uiThemes, ", "))
	}
	cfg.Combos = f.Combos
	cfg.Tiers = f.Tiers
	cfg.ClaudeFallback = f.ClaudeFallback
	cfg.Free = f.Free
	cfg.UI = f.UI
	return cfg, nil
}

// checkComboTarget validates one "provider[@label]/model" combo target. Claude
// models are not allowed as targets in v1, and a target cannot be another
// combo (no nesting).
func checkComboTarget(t string, comboNames map[string]bool) error {
	i := strings.Index(t, "/")
	if i <= 0 || i == len(t)-1 {
		return fmt.Errorf("target %q must be provider/model", t)
	}
	name, _, _ := strings.Cut(t[:i], "@")
	if name == "" {
		return fmt.Errorf("target %q has an empty provider", t)
	}
	if strings.HasPrefix(t, "claude-") {
		return fmt.Errorf("target %q must not start with claude-", t)
	}
	if comboNames[t] {
		return fmt.Errorf("target %q is itself a combo", t)
	}
	return nil
}
