package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

// limitedClaude replaces the test env's Claude upstream with one that
// answers status (and Retry-After) until `ok` flips, then 200.
func limitedClaude(t *testing.T, e *env, status int) (hits *atomic.Int32, ok *atomic.Bool, auth *atomic.Value) {
	t.Helper()
	hits, ok, auth = &atomic.Int32{}, &atomic.Bool{}, &atomic.Value{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		auth.Store(r.Header.Get("Authorization"))
		io.Copy(io.Discard, r.Body)
		if ok.Load() {
			io.WriteString(w, `{"from":"claude"}`)
			return
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(status)
		io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"usage limit"}}`)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	e.rt.Claude = u
	return
}

func fallbackEnv(t *testing.T, status int) (*env, *atomic.Int32, *atomic.Bool, *atomic.Value, *stubUpstream) {
	t.Helper()
	e := setup(t)
	hits, ok, auth := limitedClaude(t, e, status)
	up, stub := newStub(t, 200, anthropicSSE)
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{"free/auto": {Targets: []string{"zai/m2"}}}
	e.rt.ClaudeFallback = "free/auto"
	return e, hits, ok, auth, stub
}

const claudeReq = `{"model":"claude-sonnet-5-5","messages":[{"role":"user","content":"hi"}]}`

func TestClaudeLimitFallsBackToTheCombo(t *testing.T) {
	for _, status := range []int{429, 529} {
		e, claudeHits, _, _, zai := fallbackEnv(t, status)
		resp := e.post(t, "/v1/messages", claudeReq, map[string]string{"Authorization": "Bearer oauth-claude-login"})
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(b), "message_start") {
			t.Fatalf("%d: status %d body %s", status, resp.StatusCode, b)
		}
		if claudeHits.Load() != 1 || zai.hits.Load() != 1 {
			t.Fatalf("%d: claude hits %d, zai hits %d", status, claudeHits.Load(), zai.hits.Load())
		}
		zai.mu.Lock()
		got := zai.auth + zai.xapikey
		zai.mu.Unlock()
		if strings.Contains(got, "oauth-claude-login") {
			t.Fatalf("%d: the Claude login reached the fallback provider", status)
		}
		ev := <-e.events
		if ev.Provider != "zai" || ev.Via != "free/auto" || !strings.Contains(ev.Why, "claude "+itoaStatus(status)) {
			t.Fatalf("%d: event = %+v", status, ev)
		}
	}
}

func itoaStatus(n int) string {
	if n == 429 {
		return "429"
	}
	return "529"
}

func TestClaudeIsSkippedWhileItAsksToWait(t *testing.T) {
	e, claudeHits, _, _, zai := fallbackEnv(t, 429)
	for i := 0; i < 2; i++ {
		resp := e.post(t, "/v1/messages", claudeReq, nil)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if claudeHits.Load() != 1 {
		t.Fatalf("claude was asked %d times, want once (Retry-After was 120s)", claudeHits.Load())
	}
	if zai.hits.Load() != 2 {
		t.Fatalf("zai hits = %d", zai.hits.Load())
	}
}

func TestClaudeAnswerStaysWithClaude(t *testing.T) {
	e, claudeHits, ok, _, zai := fallbackEnv(t, 429)
	ok.Store(true)
	resp := e.post(t, "/v1/messages", claudeReq, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"from":"claude"`) {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	if claudeHits.Load() != 1 || zai.hits.Load() != 0 {
		t.Fatalf("claude %d zai %d", claudeHits.Load(), zai.hits.Load())
	}
}

func TestClaudeLimitIsUnchangedWithoutAFallback(t *testing.T) {
	e, _, _, _, zai := fallbackEnv(t, 429)
	e.rt.ClaudeFallback = ""
	resp := e.post(t, "/v1/messages", claudeReq, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 429 || !strings.Contains(string(b), "usage limit") {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	if zai.hits.Load() != 0 {
		t.Fatal("nothing may be sent elsewhere when no fallback is set")
	}
}

func TestOnlyChatRequestsFallBack(t *testing.T) {
	e, _, _, _, zai := fallbackEnv(t, 429)
	resp := e.post(t, "/v1/messages/count_tokens", claudeReq, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 429 || zai.hits.Load() != 0 {
		t.Fatalf("count_tokens: status %d, zai hits %d", resp.StatusCode, zai.hits.Load())
	}
}

func TestFallbackCanBeASingleModel(t *testing.T) {
	e, _, _, _, zai := fallbackEnv(t, 429)
	e.rt.Combos = nil
	e.rt.ClaudeFallback = "zai/glm-5"
	resp := e.post(t, "/v1/messages", claudeReq, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || zai.hits.Load() != 1 {
		t.Fatalf("status %d, zai hits %d", resp.StatusCode, zai.hits.Load())
	}
}

func TestPlanShowsTheFallback(t *testing.T) {
	e, _, _, _, _ := fallbackEnv(t, 429)
	plan := e.rt.Plan("claude-sonnet-5-5", Need{})
	if len(plan.Steps) != 2 || plan.Steps[0].Target != "claude" || plan.Steps[1].Target != "free/auto" ||
		!strings.Contains(plan.Steps[1].Reason, "429") {
		t.Fatalf("plan = %+v", plan)
	}
}
