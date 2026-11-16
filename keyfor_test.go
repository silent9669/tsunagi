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
	cfg := config.Config{Catalog: []provider.Provider{{Name: "acme", BaseURL: "https://evil.example/v1", Dialect: provider.OpenAIChat}}}
	_, err := keyFor(cfg)("acme")
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
	cfg := config.Config{Catalog: []provider.Provider{{Name: "acme", BaseURL: "https://api.acme.example/v1"}}}
	var ke router.KeyError
	if _, err := keyFor(cfg)("acme"); !errors.As(err, &ke) {
		t.Fatalf("an unpinned catalogue key must be denied, got %v", err)
	}
}
