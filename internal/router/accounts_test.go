package router

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/provider"
)

// keyed upstream answers by the bearer key it receives.
type keyed struct {
	mu   sync.Mutex
	hits map[string]int
}

func keyedUpstream(t *testing.T, status map[string]int, body string) (*httptest.Server, *keyed) {
	k := &keyed{hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		k.mu.Lock()
		k.hits[key]++
		k.mu.Unlock()
		io.Copy(io.Discard, r.Body)
		if st := status[key]; st != 0 && st != 200 {
			if st == 429 {
				w.Header().Set("Retry-After", "60")
			}
			w.WriteHeader(st)
			io.WriteString(w, `{"error":{"message":"limit"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, k
}

const anthropicSSE = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"cache_read_input_tokens\":4,\"output_tokens\":1}}}\n\n" +
	"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n"

func twoAccounts(t *testing.T, up *httptest.Server, evs chan events.Event) *env {
	e := setup(t)
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up.URL + "/anthropic", Dialect: provider.Anthropic}
	e.rt.Accounts = func(p string) []string { return []string{p, p + ".work"} }
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) {
		switch acct {
		case "opencode":
			return "k-main", nil
		case "opencode.work":
			return "k-work", nil
		}
		return "", errors.New("none")
	}
	e.rt.OnEvent = func(ev events.Event) { evs <- ev }
	return e
}

func TestFailoverToNextAccountIsInvisibleAndRemembered(t *testing.T) {
	up, k := keyedUpstream(t, map[string]int{"k-main": 429}, anthropicSSE)
	evs := make(chan events.Event, 8)
	e := twoAccounts(t, up, evs)

	resp := e.post(t, "/v1/messages", `{"model":"opencode/m","stream":true}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "message_delta") {
		t.Fatalf("client saw status %d body %s", resp.StatusCode, b)
	}
	ev := <-evs
	if ev.Account != "opencode.work" || ev.Skipped != 1 {
		t.Fatalf("event = %+v", ev)
	}
	if ev.In != 15 || ev.Out != 7 {
		t.Fatalf("tokens in/out = %d/%d, want 15/7 (cache reads count as input)", ev.In, ev.Out)
	}

	e.post(t, "/v1/messages", `{"model":"opencode/m","stream":true}`, nil).Body.Close()
	if k.hits["k-main"] != 1 {
		t.Fatalf("the rate-limited account was tried again: %v", k.hits)
	}
}

func TestPinnedAccountDoesNotRotate(t *testing.T) {
	up, k := keyedUpstream(t, map[string]int{"k-work": 429}, anthropicSSE)
	evs := make(chan events.Event, 8)
	e := twoAccounts(t, up, evs)

	resp := e.post(t, "/v1/messages", `{"model":"opencode@work/m"}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 429 || k.hits["k-main"] != 0 {
		t.Fatalf("status %d hits %v: a pinned account must not fall back", resp.StatusCode, k.hits)
	}
	resp = e.post(t, "/v1/messages", `{"model":"opencode@ghost/m"}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(b), "opencode.ghost") {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
}

func TestAllAccountsLimitedShowsLastUpstreamError(t *testing.T) {
	up, k := keyedUpstream(t, map[string]int{"k-main": 429, "k-work": 402}, anthropicSSE)
	e := twoAccounts(t, up, make(chan events.Event, 8))
	resp := e.post(t, "/v1/messages", `{"model":"opencode/m"}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 402 || k.hits["k-main"] != 1 || k.hits["k-work"] != 1 {
		t.Fatalf("status %d hits %v", resp.StatusCode, k.hits)
	}
}

func TestBadRequestIsNotRetriedOnAnotherAccount(t *testing.T) {
	up, k := keyedUpstream(t, map[string]int{"k-main": 400}, anthropicSSE)
	e := twoAccounts(t, up, make(chan events.Event, 8))
	resp := e.post(t, "/v1/messages", `{"model":"opencode/m"}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 400 || k.hits["k-work"] != 0 {
		t.Fatalf("status %d hits %v", resp.StatusCode, k.hits)
	}
}

func TestRoundRobinAlternatesAccounts(t *testing.T) {
	up, k := keyedUpstream(t, nil, anthropicSSE)
	e := twoAccounts(t, up, make(chan events.Event, 16))
	p := e.rt.Providers["opencode"]
	p.Strategy = "round_robin"
	e.rt.Providers["opencode"] = p
	for i := 0; i < 4; i++ {
		e.post(t, "/v1/messages", `{"model":"opencode/m"}`, nil).Body.Close()
	}
	if k.hits["k-main"] != 2 || k.hits["k-work"] != 2 {
		t.Fatalf("hits = %v", k.hits)
	}
}

func TestTranslatedProviderFailsOverToo(t *testing.T) {
	chat := "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n"
	up, k := keyedUpstream(t, map[string]int{"k-main": 429}, chat)
	evs := make(chan events.Event, 8)
	e := twoAccounts(t, up, evs)
	e.rt.Providers["groq"] = provider.Provider{Name: "groq", BaseURL: up.URL + "/v1", Dialect: provider.OpenAIChat}
	e.rt.Accounts = func(p string) []string { return []string{p, p + ".work"} }
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) {
		if acct == "groq" {
			return "k-main", nil
		}
		return "k-work", nil
	}
	resp := e.post(t, "/v1/messages", `{"model":"groq/m","stream":true,"messages":[{"role":"user","content":"x"}]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"text":"hi"`) || k.hits["k-work"] != 1 {
		t.Fatalf("status %d hits %v body %s", resp.StatusCode, k.hits, b)
	}
	ev := <-evs
	if ev.Account != "groq.work" || ev.In != 5 || ev.Out != 2 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestPlainJSONReplyTokensAreCounted(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"type":"message","usage":{"input_tokens":9,"output_tokens":3}}`)
	}))
	defer up.Close()
	evs := make(chan events.Event, 4)
	e := twoAccounts(t, up, evs)
	e.post(t, "/v1/messages", `{"model":"opencode/m"}`, nil).Body.Close()
	if ev := <-evs; ev.In != 9 || ev.Out != 3 {
		t.Fatalf("event = %+v", ev)
	}
}
