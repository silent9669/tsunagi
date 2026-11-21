package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/silent9669/tsunagi/internal/catalog"
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

// A catalogue entry with an empty Dialect (a models.dev provider tsunagi
// cannot speak to) contributes capabilities but never becomes a routable
// provider.
func TestCapsOnlyCatalogueEntryIsNotRoutable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg"))
	dir := filepath.Join(home, "Library", "Caches", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cache := `{"fetched":"2026-11-21T00:00:00Z","providers":[
		{"id":"weird","name":"Weird","api":"https://weird.example/v1","dialect":"",
		 "models":["m"],"caps":{"m":{"tools":true,"context":1000}}},
		{"id":"routable","name":"Routable","api":"https://r.example/v1","dialect":"openai-chat",
		 "models":["m"],"caps":{"m":{"tools":true}}}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range cfg.All() {
		if p.Name == "weird" {
			t.Fatalf("caps-only entry became routable: %+v", p)
		}
	}
	found := false
	for _, p := range cfg.All() {
		if p.Name == "routable" {
			found = true
		}
	}
	if !found {
		t.Fatal("routable catalogue entry missing")
	}
	if mi, ok := catalog.Caps("weird", "m"); !ok || !mi.Tools {
		t.Fatalf("caps for caps-only entry = %+v ok=%v", mi, ok)
	}
}
