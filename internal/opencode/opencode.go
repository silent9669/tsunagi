// Package opencode reads opencode's provider config and saved keys, so a user
// who already set up providers there does not have to do it twice.
package opencode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/silent9669/tsunagi/internal/jsonc"
)

// Provider is one entry of opencode's `provider` (v1) or `providers` (v2) block.
type Provider struct {
	ID      string
	Package string
	BaseURL string
	// Key is a literal apiKey from the config, or the value of the environment
	// variable it names. Empty when none.
	Key string
}

type rawProvider struct {
	NPM     string `json:"npm"`
	Package string `json:"package"`
	Options struct {
		BaseURL string `json:"baseURL"`
		APIKey  string `json:"apiKey"`
	} `json:"options"`
	Settings struct {
		BaseURL string `json:"baseURL"`
		APIKey  string `json:"apiKey"`
	} `json:"settings"`
}

func configDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

func dataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

// ConfigPaths lists the config files opencode would read, most specific first.
func ConfigPaths() []string {
	var p []string
	if c := os.Getenv("OPENCODE_CONFIG"); c != "" {
		p = append(p, c)
	}
	return append(p, filepath.Join(configDir(), "opencode.jsonc"), filepath.Join(configDir(), "opencode.json"))
}

func AuthPath() string { return filepath.Join(dataDir(), "auth.json") }

// resolveKey turns "{env:NAME}" into the variable's value and keeps literals.
// "{file:...}" is not followed.
func resolveKey(v string) string {
	if strings.HasPrefix(v, "{env:") && strings.HasSuffix(v, "}") {
		return os.Getenv(v[5 : len(v)-1])
	}
	if strings.HasPrefix(v, "{") {
		return ""
	}
	return v
}

// ReadConfig returns providers from the first config file that exists. The
// bool reports whether any file was found.
func ReadConfig(paths []string) ([]Provider, bool, error) {
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, true, err
		}
		var doc struct {
			Provider  map[string]rawProvider `json:"provider"`
			Providers map[string]rawProvider `json:"providers"`
		}
		if err := json.Unmarshal(jsonc.Clean(b), &doc); err != nil {
			return nil, true, errors.New(path + ": " + err.Error())
		}
		var out []Provider
		for _, m := range []map[string]rawProvider{doc.Provider, doc.Providers} {
			for id, r := range m {
				p := Provider{ID: id, Package: r.NPM, BaseURL: r.Options.BaseURL, Key: resolveKey(r.Options.APIKey)}
				if r.Package != "" {
					p.Package = r.Package
				}
				if r.Settings.BaseURL != "" {
					p.BaseURL = r.Settings.BaseURL
				}
				if k := resolveKey(r.Settings.APIKey); k != "" {
					p.Key = k
				}
				out = append(out, p)
			}
		}
		return out, true, nil
	}
	return nil, false, nil
}

// Auth is one entry of opencode's auth.json.
type Auth struct {
	Type string `json:"type"` // "api", "oauth", ...
	Key  string `json:"key"`
}

// ReadAuth returns opencode's saved logins by provider id.
func ReadAuth(path string) (map[string]Auth, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]Auth
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errors.New(path + ": " + err.Error())
	}
	return m, nil
}
