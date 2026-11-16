package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/provider"
)

func TestAggregateByAccount(t *testing.T) {
	rows := Aggregate([]events.Event{
		{Provider: "openai", Account: "openai", Status: 200, In: 10, Out: 5},
		{Provider: "openai", Account: "openai.work", Status: 429, Skipped: 0},
		{Provider: "openai", Account: "openai", Status: 200, In: 7, Out: 2, Skipped: 1},
		{Provider: "claude", Status: 200, In: 100, Out: 50},
	})
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	var main Row
	for _, r := range rows {
		if r.Account == "openai" {
			main = r
		}
	}
	if main.Requests != 2 || main.In != 17 || main.Out != 7 || main.Skipped != 1 {
		t.Fatalf("main = %+v", main)
	}
	if rows[0].Account != "claude" {
		t.Fatalf("claude requests must count under their own row, got %+v", rows[0])
	}
}

func serve(body string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func TestFetchers(t *testing.T) {
	ctx := context.Background()
	srv := serve(`{"data":{"usage":4.2,"limit":10,"limit_remaining":5.8}}`, 200)
	b, err := openRouter(ctx, srv.Client(), srv.URL, "k")
	if err != nil || b.Text != "$5.80 left of $10.00" || b.Frac < 0.57 || b.Frac > 0.59 {
		t.Fatalf("openrouter: %+v %v", b, err)
	}
	srv.Close()

	srv = serve(`{"data":{"usage":4.2,"limit":null,"limit_remaining":null}}`, 200)
	b, _ = openRouter(ctx, srv.Client(), srv.URL, "k")
	if b.Frac != -1 || b.Text != "$4.20 used, no limit" {
		t.Fatalf("openrouter unlimited: %+v", b)
	}
	srv.Close()

	srv = serve(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"110.00"}]}`, 200)
	b, err = deepSeek(ctx, srv.Client(), srv.URL, "k")
	if err != nil || b.Text != "110.00 CNY" {
		t.Fatalf("deepseek: %+v %v", b, err)
	}
	srv.Close()

	srv = serve(`{"code":0,"data":{"available_balance":49.58}}`, 200)
	b, err = moonshot(ctx, srv.Client(), srv.URL, "k")
	if err != nil || b.Text != "49.58 available" {
		t.Fatalf("moonshot: %+v %v", b, err)
	}
	srv.Close()

	srv = serve(`{}`, 401)
	if _, err = openRouter(ctx, srv.Client(), srv.URL, "wrong"); err == nil {
		t.Fatal("a rejected key must be an error")
	}
	srv.Close()
}

func TestKind(t *testing.T) {
	for url, want := range map[string]string{
		"https://openrouter.ai/api": "openrouter", "https://api.deepseek.com/anthropic": "deepseek",
		"https://api.moonshot.ai/anthropic": "moonshot", "https://example.com/v1": "",
	} {
		if got := Kind(provider.Provider{BaseURL: url}); got != want {
			t.Errorf("Kind(%s) = %q, want %q", url, got, want)
		}
	}
}
