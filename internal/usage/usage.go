// Package usage adds up what tsunagi routed (from the event files) and asks
// providers that publish a balance or quota endpoint how much is left.
package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/provider"
)

// Row is the traffic of one account.
type Row struct {
	Provider string
	Account  string
	Requests int
	Errors   int
	Skipped  int
	In, Out  int
}

// Aggregate sums events by account. Claude requests count under "claude".
func Aggregate(evs []events.Event) []Row {
	by := map[string]*Row{}
	for _, e := range evs {
		acct := e.Account
		if acct == "" {
			acct = e.Provider
		}
		r := by[acct]
		if r == nil {
			r = &Row{Provider: e.Provider, Account: acct}
			by[acct] = r
		}
		r.Requests++
		if e.Status >= 400 {
			r.Errors++
		}
		r.Skipped += e.Skipped
		r.In += e.In
		r.Out += e.Out
	}
	rows := make([]Row, 0, len(by))
	for _, r := range by {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Account < rows[j].Account })
	return rows
}

// Balance is what a provider says is left.
type Balance struct {
	Text string
	// Frac is the share still available, 0..1, or -1 when the provider reports
	// no limit.
	Frac float64
}

// Kind names the balance endpoint a provider has, or "".
func Kind(p provider.Provider) string {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	switch {
	case h == "openrouter.ai":
		return "openrouter"
	case h == "api.deepseek.com":
		return "deepseek"
	case h == "api.moonshot.ai" || h == "api.moonshot.cn":
		return "moonshot"
	}
	return ""
}

// Fetch asks the provider for its balance. ok is false when the provider has
// no endpoint tsunagi knows.
func Fetch(ctx context.Context, c *http.Client, p provider.Provider, key string) (b Balance, ok bool, err error) {
	kind := Kind(p)
	if kind == "" {
		return b, false, nil
	}
	u, _ := url.Parse(p.BaseURL)
	origin := u.Scheme + "://" + u.Host
	switch kind {
	case "openrouter":
		b, err = openRouter(ctx, c, origin+"/api/v1/key", key)
	case "deepseek":
		b, err = deepSeek(ctx, c, origin+"/user/balance", key)
	case "moonshot":
		b, err = moonshot(ctx, c, origin+"/v1/users/me/balance", key)
	}
	return b, true, err
}

func getJSON(ctx context.Context, c *http.Client, u, key string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

func openRouter(ctx context.Context, c *http.Client, u, key string) (Balance, error) {
	var d struct {
		Data struct {
			Usage          float64  `json:"usage"`
			Limit          *float64 `json:"limit"`
			LimitRemaining *float64 `json:"limit_remaining"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c, u, key, &d); err != nil {
		return Balance{}, err
	}
	if d.Data.Limit == nil || *d.Data.Limit <= 0 || d.Data.LimitRemaining == nil {
		return Balance{Text: fmt.Sprintf("$%.2f used, no limit", d.Data.Usage), Frac: -1}, nil
	}
	rem := *d.Data.LimitRemaining
	return Balance{Text: fmt.Sprintf("$%.2f left of $%.2f", rem, *d.Data.Limit), Frac: clamp(rem / *d.Data.Limit)}, nil
}

func deepSeek(ctx context.Context, c *http.Client, u, key string) (Balance, error) {
	var d struct {
		Available bool `json:"is_available"`
		Infos     []struct {
			Currency string `json:"currency"`
			Total    string `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := getJSON(ctx, c, u, key, &d); err != nil {
		return Balance{}, err
	}
	var parts []string
	for _, i := range d.Infos {
		parts = append(parts, i.Total+" "+i.Currency)
	}
	text := strings.Join(parts, ", ")
	if text == "" {
		text = "no balance reported"
	}
	if !d.Available {
		text += " (unavailable)"
	}
	return Balance{Text: text, Frac: -1}, nil
}

func moonshot(ctx context.Context, c *http.Client, u, key string) (Balance, error) {
	var d struct {
		Data struct {
			Available float64 `json:"available_balance"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c, u, key, &d); err != nil {
		return Balance{}, err
	}
	return Balance{Text: fmt.Sprintf("%.2f available", d.Data.Available), Frac: -1}, nil
}

func clamp(f float64) float64 {
	switch {
	case f < 0:
		return 0
	case f > 1:
		return 1
	}
	return f
}

// Since is the start of the window `usage` reports on.
func Since(days int) time.Time { return time.Now().Add(-time.Duration(days) * 24 * time.Hour) }
