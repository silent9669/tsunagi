package router

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

// Anthropic-dialect providers are coding-plan endpoints that expect the real
// Claude Code client: its User-Agent, session id and anthropic-beta headers
// must pass through untouched while Authorization is swapped for the stored
// key.
func TestAnthropicDialectPassesClientHeaders(t *testing.T) {
	e := setup(t)
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
		io.WriteString(w, anthropicSSE)
	}))
	t.Cleanup(srv.Close)
	e.rt.Providers["opencode-go"] = provider.Provider{
		Name: "opencode-go", BaseURL: srv.URL + "/inference/go/anthropic", Dialect: provider.Anthropic,
	}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) {
		if acct == "opencode-go" {
			return "oc_sk_plan", nil
		}
		return "", errors.New("none")
	}

	resp := e.post(t, "/v1/messages", `{"model":"opencode-go/some-model"}`, map[string]string{
		"Authorization":            "Bearer oauth-claude-login",
		"User-Agent":               "claude-cli/2.1.4 (external, cli)",
		"X-Claude-Code-Session-Id": "sess-abc-123",
		"Anthropic-Beta":           "oauth-2025-04-20",
		"Anthropic-Version":        "2023-06-01",
	})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	if got.Get("Authorization") != "Bearer oc_sk_plan" {
		t.Fatalf("auth = %q", got.Get("Authorization"))
	}
	if got.Get("User-Agent") != "claude-cli/2.1.4 (external, cli)" {
		t.Fatalf("user-agent = %q", got.Get("User-Agent"))
	}
	if got.Get("X-Claude-Code-Session-Id") != "sess-abc-123" {
		t.Fatalf("session id = %q", got.Get("X-Claude-Code-Session-Id"))
	}
	if got.Get("Anthropic-Beta") != "oauth-2025-04-20" {
		t.Fatalf("anthropic-beta = %q", got.Get("Anthropic-Beta"))
	}
}
