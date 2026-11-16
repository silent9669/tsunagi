package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/usage"
)

func TestHeadroomAnswersAtOnceAndLearnsInTheBackground(t *testing.T) {
	calls := 0
	h := &headroom{
		keys: func(provider.Provider, string) (string, error) { return "k", nil },
		fetch: func(context.Context, provider.Provider, string) (usage.Balance, bool, error) {
			calls++
			return usage.Balance{Frac: 0.25}, true, nil
		},
		seen: map[string]entry{}, loading: map[string]bool{}, ttl: time.Hour,
	}
	p := provider.Provider{Name: "openrouter"}
	if _, ok := h.get(p, "openrouter"); ok {
		t.Fatal("the first lookup has nothing to report yet")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f, ok := h.get(p, "openrouter"); ok {
			if f != 0.25 || calls != 1 {
				t.Fatalf("frac=%v calls=%d (fresh data must not be fetched again)", f, calls)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("headroom never became known")
}

func TestHeadroomUnknownWhenNoLimitOrError(t *testing.T) {
	for name, fetch := range map[string]func(context.Context, provider.Provider, string) (usage.Balance, bool, error){
		"no limit": func(context.Context, provider.Provider, string) (usage.Balance, bool, error) {
			return usage.Balance{Frac: -1}, true, nil
		},
		"error": func(context.Context, provider.Provider, string) (usage.Balance, bool, error) {
			return usage.Balance{}, true, errors.New("boom")
		},
		"no endpoint": func(context.Context, provider.Provider, string) (usage.Balance, bool, error) {
			return usage.Balance{}, false, nil
		},
	} {
		h := &headroom{keys: func(provider.Provider, string) (string, error) { return "k", nil }, fetch: fetch,
			seen: map[string]entry{}, loading: map[string]bool{}, ttl: time.Hour}
		h.get(provider.Provider{}, "a")
		time.Sleep(100 * time.Millisecond)
		if _, ok := h.get(provider.Provider{}, "a"); ok {
			t.Errorf("%s: must stay unknown", name)
		}
	}
}
