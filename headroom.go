package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/usage"
)

// headroom remembers how much quota each account has left, for providers that
// publish it. Lookups never wait: a stale or missing entry starts a refresh in
// the background and answers with what is known.
type headroom struct {
	keys    func(provider.Provider, string) (string, error)
	fetch   func(ctx context.Context, p provider.Provider, key string) (usage.Balance, bool, error)
	mu      sync.Mutex
	seen    map[string]entry
	loading map[string]bool
	ttl     time.Duration
}

type entry struct {
	frac float64
	ok   bool
	at   time.Time
}

func newHeadroom(cfg config.Config) *headroom {
	c := &http.Client{Timeout: 5 * time.Second}
	return &headroom{
		keys: keyFor(cfg),
		fetch: func(ctx context.Context, p provider.Provider, key string) (usage.Balance, bool, error) {
			return usage.Fetch(ctx, c, p, key)
		},
		seen: map[string]entry{}, loading: map[string]bool{}, ttl: 2 * time.Minute,
	}
}

func (h *headroom) get(p provider.Provider, account string) (float64, bool) {
	h.mu.Lock()
	e, have := h.seen[account]
	stale := !have || time.Since(e.at) > h.ttl
	start := stale && !h.loading[account]
	if start {
		h.loading[account] = true
	}
	h.mu.Unlock()
	if start {
		go h.refresh(p, account)
	}
	return e.frac, have && e.ok
}

func (h *headroom) refresh(p provider.Provider, account string) {
	e := entry{at: time.Now()}
	if key, err := h.keys(p, account); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if b, ok, err := h.fetch(ctx, p, key); ok && err == nil && b.Frac >= 0 {
			e.frac, e.ok = b.Frac, true
		}
		cancel()
	}
	h.mu.Lock()
	h.seen[account] = e
	delete(h.loading, account)
	h.mu.Unlock()
}
