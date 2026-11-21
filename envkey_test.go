package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/tui/app"
)

// envKeyHome gives the test a temp HOME with a config.json and a catalogue
// cache whose entries carry env var names. It returns the cache dir.
func envKeyHome(t *testing.T, cfgBody, catalogBody string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(home, "Library", "Caches", "tsunagi")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "models.json"), []byte(catalogBody), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

const envKeyCatalog = `{"fetched":"2026-11-21T00:00:00Z","providers":[
	{"id":"acme","name":"Acme","api":"https://api.acme.example/v1","dialect":"openai-chat","env":["ACME_API_KEY","ACME_ALT_KEY"],"models":["m1"]},
	{"id":"keyed","name":"Keyed","api":"https://keyed.example/v1","dialect":"openai-chat","env":["KEYED_API_KEY"],"models":["m1"]},
	{"id":"noroute","name":"NoRoute","api":"https://nr.example/v1","dialect":"","env":["NOROUTE_API_KEY"],"models":["m1"]},
	{"id":"anthropic","name":"Anthropic","api":"https://api.anthropic.example/v1","dialect":"openai-chat","env":["ANTHROPIC_API_KEY"],"models":["m1"]}
]}`

func TestEnvKeysListsNamesOnly(t *testing.T) {
	envKeyHome(t, `{}`, envKeyCatalog)
	t.Setenv("ACME_API_KEY", "acme-secret-value")
	t.Setenv("KEYED_API_KEY", "keyed-secret-value")
	t.Setenv("NOROUTE_API_KEY", "noroute-secret-value")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret-value")
	if err := config.SetKeyed("keyed", true, "keyed.example"); err != nil {
		t.Fatal(err)
	}
	old := hasAccount
	hasAccount = func(provider.Provider, string) bool { return true }
	t.Cleanup(func() { hasAccount = old })
	got := (backend{}).EnvKeys()
	if len(got) != 1 || got[0].Provider != "acme" || got[0].Var != "ACME_API_KEY" {
		t.Fatalf("EnvKeys = %+v", got)
	}
	for _, ek := range got {
		if strings.Contains(ek.Provider, "secret") || strings.Contains(ek.Var, "secret") {
			t.Fatalf("EnvKeys leaked a value: %+v", ek)
		}
	}
}

func TestImportEnvKey(t *testing.T) {
	envKeyHome(t, `{}`, envKeyCatalog)
	t.Setenv("ACME_API_KEY", "acme-secret-value")
	var saved map[string]string
	old := storeKey
	storeKey = func(account, key string) error {
		if saved == nil {
			saved = map[string]string{}
		}
		saved[account] = key
		return nil
	}
	t.Cleanup(func() { storeKey = old })

	if err := (backend{}).ImportEnvKey("acme"); err != nil {
		t.Fatalf("ImportEnvKey: %v", err)
	}
	if saved["acme"] != "acme-secret-value" {
		t.Fatalf("saved = %+v", saved)
	}
	keyed := false
	for _, n := range config.Keyed() {
		keyed = keyed || n == "acme"
	}
	if !keyed {
		t.Fatal("acme was not marked keyed")
	}
	for _, bad := range []string{"keyed", "noroute", "anthropic", "claude", "nosuch"} {
		err := (backend{}).ImportEnvKey(bad)
		if err == nil {
			t.Fatalf("ImportEnvKey(%q) succeeded", bad)
		}
		if strings.Contains(err.Error(), "secret-value") {
			t.Fatalf("ImportEnvKey(%q) leaked the value: %v", bad, err)
		}
	}
}

func TestNeedsWizardIgnoresKeyless(t *testing.T) {
	snap := app.Snapshot{Providers: []app.ProviderRow{
		{Name: "kilo", Group: "free", Ready: true, Keyless: true},
	}}
	if !needsWizard(snap) {
		t.Fatal("a keyless provider alone must not skip the wizard")
	}
	snap.Providers = append(snap.Providers, app.ProviderRow{Name: "groq", Group: "free", Ready: true})
	if needsWizard(snap) {
		t.Fatal("a keyed provider skips the wizard")
	}
}

func TestDetectKeyFiltersToSnapshot(t *testing.T) {
	envKeyHome(t, `{}`, envKeyCatalog)
	names, err := (backend{}).DetectKey("gsk_abc123")
	if err != nil || len(names) != 1 || names[0] != "groq" {
		t.Fatalf("DetectKey = %v, %v", names, err)
	}
	if _, err := (backend{}).DetectKey("sk-ant-secret"); err == nil {
		t.Fatal("DetectKey accepted a Claude key")
	} else if strings.Contains(err.Error(), "sk-ant-secret") {
		t.Fatalf("DetectKey leaked the key: %v", err)
	}
	if names, err := (backend{}).DetectKey("unrecognized-key"); err != nil || len(names) != 0 {
		t.Fatalf("DetectKey = %v, %v", names, err)
	}
}
