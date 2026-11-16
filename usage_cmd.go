package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/ui"
	"github.com/silent9669/tsunagi/internal/usage"
)

// cmdUsage shows what each account carried (from the event files of every
// session) and, where the provider publishes one, what is left.
func cmdUsage(t *ui.Term, args []string) int {
	days := 1
	if len(args) == 2 && args[0] == "--days" {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 || n > 7 {
			return fail(t, "--days takes 1 to 7")
		}
		days = n
	} else if len(args) != 0 {
		return fail(t, "usage: tsunagi usage [--days N]")
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	dir, err := events.Dir()
	if err != nil {
		return fail(t, "%v", err)
	}
	rows := usage.Aggregate(events.ReadAll(dir, usage.Since(days)))

	// Ask providers that have a balance endpoint, all accounts at once.
	byName := map[string]provider.Provider{}
	for _, p := range cfg.All() {
		byName[p.Name] = p
	}
	type result struct {
		b   usage.Balance
		ok  bool
		err error
	}
	results := map[string]result{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	keys := keyFor(cfg)
	seen := map[string]bool{}
	var accounts []string
	for _, r := range rows {
		seen[r.Account] = true
		accounts = append(accounts, r.Account)
	}
	for _, a := range config.Keyed() {
		if !seen[a] {
			accounts = append(accounts, a)
		}
	}
	sp := t.Spin("balances")
	client := &http.Client{Timeout: 10 * time.Second}
	for _, a := range accounts {
		p, ok := byName[baseOf(a)]
		if !ok || usage.Kind(p) == "" {
			continue
		}
		wg.Add(1)
		go func(a string, p provider.Provider) {
			defer wg.Done()
			key, err := keys(p, a)
			if err != nil {
				mu.Lock()
				results[a] = result{err: err, ok: true}
				mu.Unlock()
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			b, ok, err := usage.Fetch(ctx, client, p, key)
			mu.Lock()
			results[a] = result{b, ok, err}
			mu.Unlock()
		}(a, p)
	}
	wg.Wait()
	sp.Stop(true, fmt.Sprintf("last %d day(s)", days))

	// One line per account: traffic from the events, balance from the provider.
	byAcct := map[string]usage.Row{}
	for _, r := range rows {
		byAcct[r.Account] = r
	}
	var table [][]string
	for _, a := range accounts {
		r := byAcct[a]
		bal := t.Dim("-")
		if res, ok := results[a]; ok {
			switch {
			case res.err != nil:
				bal = t.Paint(ui.Amber, "unavailable: "+res.err.Error())
			case res.b.Frac >= 0:
				bal = t.Gauge(res.b.Frac, 10) + " " + res.b.Text
			default:
				bal = res.b.Text
			}
		}
		skipped := ""
		if r.Skipped > 0 {
			skipped = " (" + strconv.Itoa(r.Skipped) + " failovers)"
		}
		errs := strconv.Itoa(r.Errors)
		if r.Errors > 0 {
			errs = t.Paint(ui.Amber, errs)
		}
		table = append(table, []string{a, strconv.Itoa(r.Requests) + skipped, errs, ui.Count(r.In), ui.Count(r.Out), bal})
	}
	if len(table) == 0 {
		t.Printf("  %s\n", t.Dim("nothing routed yet, and no accounts. Run: tsunagi login <provider>"))
		return 0
	}
	t.Table([]string{"account", "requests", "errors", "in", "out", "balance"}, table)
	return 0
}
