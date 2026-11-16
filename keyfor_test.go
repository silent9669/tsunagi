package main

import (
	"errors"
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
)

func TestKeyForRefusesChangedCatalogueEndpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.SetKeyed("acme", true, "api.acme.example"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Catalog: []provider.Provider{{Name: "acme", BaseURL: "https://evil.example/v1", Dialect: provider.OpenAIChat, Untrusted: true}}}
	_, err := keyFor(cfg)(cfg.Catalog[0], "acme")
	var ke router.KeyError
	if !errors.As(err, &ke) {
		t.Fatalf("want KeyError for a moved endpoint, got %v", err)
	}
}

func TestPinHostOnlyForCatalogue(t *testing.T) {
	cfg := config.Config{
		Providers: []provider.Provider{{Name: "mine", BaseURL: "https://mine.example"}},
		Catalog:   []provider.Provider{{Name: "acme", BaseURL: "https://api.acme.example/v1"}},
	}
	if got := pinHost(cfg, cfg.Catalog[0]); got != "api.acme.example" {
		t.Fatalf("catalogue pin = %q", got)
	}
	if got := pinHost(cfg, cfg.Providers[0]); got != "" {
		t.Fatalf("user provider must not be pinned, got %q", got)
	}
}

func TestKeyForDeniesCatalogueKeyWithoutPin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{Catalog: []provider.Provider{{Name: "acme", BaseURL: "https://api.acme.example/v1", Untrusted: true}}}
	var ke router.KeyError
	if _, err := keyFor(cfg)(cfg.Catalog[0], "acme"); !errors.As(err, &ke) {
		t.Fatalf("an unpinned catalogue key must be denied, got %v", err)
	}
}

func TestKeyForRejectsAnAccountOfAnotherProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// a catalogue provider whose name imitates "<provider>.<label>" of a trusted one
	evil := provider.Provider{Name: "chatgpt.work", BaseURL: "https://evil.example/v1", Untrusted: true}
	cfg := config.Config{Catalog: []provider.Provider{evil}}
	var ke router.KeyError
	// the router would ask for the account "chatgpt.work" to be sent to evil.example
	if tok, err := keyFor(cfg)(evil, "chatgpt.work"); !errors.As(err, &ke) || tok != "" {
		t.Fatalf("an untrusted provider must never get a sign-in token: %q %v", tok, err)
	}
	trusted := provider.Provider{Name: "openai", BaseURL: "https://api.openai.com/v1"}
	if _, err := keyFor(config.Config{})(trusted, "groq"); !errors.As(err, &ke) {
		t.Fatalf("account of another provider: %v", err)
	}
}
