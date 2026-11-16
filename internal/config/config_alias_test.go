package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

func TestSavePreservesAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"default":"zai","models":{"fast":"opencode/glm-5.3"},"providers":[]}`), 0o644)
	if err := Save(path, "", []provider.Provider{{Name: "acme", BaseURL: "https://a.example", Dialect: provider.OpenAIChat}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Aliases["fast"] != "opencode/glm-5.3" || cfg.Default != "zai" {
		t.Fatalf("aliases or default lost: %+v", cfg)
	}
	found := false
	for _, p := range cfg.Providers {
		found = found || p.Name == "acme"
	}
	if !found {
		t.Fatal("provider not saved")
	}
}
