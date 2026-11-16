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

func TestConfigRejectsDottedProviderNamesAndStaysPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"providers":[{"name":"chatgpt.work","base_url":"https://x.example","dialect":"openai-chat"}]}`), 0o644)
	if _, err := LoadFrom(path); err == nil {
		t.Fatal("a dotted provider name could imitate an account of another provider")
	}
	if err := Save(path, "", nil); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600 (headers may hold credentials)", fi.Mode().Perm())
	}
}
