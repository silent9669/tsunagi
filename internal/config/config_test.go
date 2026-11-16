package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

func TestMissingFileGivesBuiltins(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || cfg.Default != "opencode" || len(cfg.Providers) != len(provider.Builtin()) {
		t.Fatalf("%+v %v", cfg, err)
	}
}

func TestUserProvidersOverrideAndAdd(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"default":"deepseek","providers":[
		{"name":"deepseek","base_url":"https://example.test/anthropic"},
		{"name":"mine","base_url":"http://127.0.0.1:9","local":true}]}`), 0o600)
	cfg, err := LoadFrom(p)
	if err != nil || cfg.Default != "deepseek" {
		t.Fatalf("%+v %v", cfg, err)
	}
	var ds, mine *provider.Provider
	for i := range cfg.Providers {
		switch cfg.Providers[i].Name {
		case "deepseek":
			ds = &cfg.Providers[i]
		case "mine":
			mine = &cfg.Providers[i]
		}
	}
	if ds == nil || ds.BaseURL != "https://example.test/anthropic" || ds.Dialect != provider.Anthropic {
		t.Fatalf("deepseek = %+v", ds)
	}
	if mine == nil || !mine.Local {
		t.Fatalf("mine = %+v", mine)
	}
}

func TestBadProviderIsAnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"providers":[{"name":"x"}]}`), 0o600)
	if _, err := LoadFrom(p); err == nil {
		t.Fatal("want error")
	}
}
