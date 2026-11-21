package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/silent9669/tsunagi/internal/provider"
)

// stubUpstream answers every request with the same status and body and records
// what it saw.
type stubUpstream struct {
	hits    atomic.Int32
	mu      sync.Mutex
	auth    string
	xapikey string
	body    []byte
}

func newStub(t *testing.T, status int, body string) (*httptest.Server, *stubUpstream) {
	t.Helper()
	r := &stubUpstream{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		r.mu.Lock()
		r.auth = req.Header.Get("Authorization")
		r.xapikey = req.Header.Get("X-Api-Key")
		r.body, _ = io.ReadAll(req.Body)
		r.mu.Unlock()
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, r
}

// newScript answers via fn(hit number) -> (status, body).
func newScript(t *testing.T, fn func(hit int32) (int, string)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h := hits.Add(1)
		io.Copy(io.Discard, req.Body)
		st, body := fn(h)
		w.WriteHeader(st)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

// twoCombo sets up a combo of two anthropic-dialect targets that both answer
// 200, on top of the shared test env.
func twoCombo(t *testing.T, strategy string) (*env, *stubUpstream, *stubUpstream) {
	t.Helper()
	e := setup(t)
	up1, r1 := newStub(t, 200, `{"ok":true,"from":"one"}`)
	up2, r2 := newStub(t, 200, `{"ok":true,"from":"two"}`)
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up1.URL, Dialect: provider.Anthropic}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up2.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m1", "zai/m2"}, Strategy: strategy},
	}
	return e, r1, r2
}

func TestComboPriorityFallsThroughOnRetryable(t *testing.T) {
	e := setup(t)
	up1, r1 := newStub(t, 429, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
	up2, r2 := newStub(t, 200, anthropicSSE)
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up1.URL, Dialect: provider.Anthropic}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up2.URL, Dialect: provider.Anthropic}
	e.rt.Accounts = func(p string) []string {
		if p == "opencode" {
			return []string{"opencode", "opencode.work"}
		}
		return []string{p}
	}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) {
		switch acct {
		case "opencode":
			return "k-main", nil
		case "opencode.work":
			return "k-work", nil
		}
		return "k-" + acct, nil
	}
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m1", "zai/m2"}},
	}

	resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`,
		map[string]string{"Authorization": "Bearer oauth-claude-login"})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "message_start") {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	if r1.hits.Load() != 2 { // both opencode accounts tried first
		t.Fatalf("opencode hits = %d", r1.hits.Load())
	}
	if r2.hits.Load() != 1 {
		t.Fatalf("zai hits = %d", r2.hits.Load())
	}
	// the rate-limited account is blocked for the next request
	e.rt.pool.mu.Lock()
	_, blocked := e.rt.pool.blocked["opencode"]
	e.rt.pool.mu.Unlock()
	if !blocked {
		t.Fatal("the rate-limited account was not blocked")
	}
	ev := <-e.events
	if ev.Via != "free/auto" || ev.Provider != "zai" {
		t.Fatalf("event = %+v", ev)
	}
	for _, want := range []string{"opencode/m1 429", "zai/m2 ok"} {
		if !strings.Contains(ev.Why, want) {
			t.Fatalf("why %q lacks %q", ev.Why, want)
		}
	}
	// the event explains the route but never carries credentials
	if strings.Contains(ev.Why, "oauth-claude-login") || strings.Contains(ev.Why, "k-main") || strings.Contains(ev.Why, "k-work") {
		t.Fatalf("why leaks a credential: %q", ev.Why)
	}
	if b, _ := json.Marshal(ev); bytes.Contains(b, []byte("oauth-claude-login")) || bytes.Contains(b, []byte("k-main")) || bytes.Contains(b, []byte("k-work")) {
		t.Fatalf("event leaks a credential: %s", b)
	}
	if ev.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1 account failover", ev.Skipped)
	}
}

func TestComboSkipsTargetsThatCannotServe(t *testing.T) {
	e, r1, r2 := twoCombo(t, "priority")
	e.rt.Caps = func(p, _ string) (Caps, bool) {
		if p == "opencode" {
			return Caps{Tools: false, Context: 100000}, true
		}
		return Caps{Tools: true, Context: 100000}, true
	}

	// a request with tools skips the target that cannot call them
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","tools":[{"name":"t"}],"messages":[]}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 200 || r1.hits.Load() != 0 || r2.hits.Load() != 1 {
		t.Fatalf("status %d hits %d/%d", resp.StatusCode, r1.hits.Load(), r2.hits.Load())
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "skipped opencode/m1: no tool calling") {
		t.Fatalf("why = %q", ev.Why)
	}

	// the same request without tools uses it
	resp = e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 200 || r1.hits.Load() != 1 {
		t.Fatalf("status %d hits %d", resp.StatusCode, r1.hits.Load())
	}
	<-e.events // drain this request's event

	// a request too large for the context window skips it too
	e.rt.Caps = func(p, _ string) (Caps, bool) {
		if p == "opencode" {
			return Caps{Tools: true, Context: 1}, true
		}
		return Caps{Tools: true, Context: 100000}, true
	}
	resp = e.post(t, "/v1/messages", `{"model":"free/auto","messages":[{"role":"user","content":"hello there"}]}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 200 || r1.hits.Load() != 1 || r2.hits.Load() != 2 {
		t.Fatalf("status %d hits %d/%d", resp.StatusCode, r1.hits.Load(), r2.hits.Load())
	}
	ev = <-e.events
	if !strings.Contains(ev.Why, "skipped opencode/m1: context too large") {
		t.Fatalf("why = %q", ev.Why)
	}
}

func TestComboAllSkippedIsABadRequest(t *testing.T) {
	e := setup(t)
	e.rt.Key = func(provider.Provider, string) (string, error) { return "", errors.New("none") }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"nope/x", "opencode/y"}},
	}
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	for _, want := range []string{"nope/x: unknown provider", "opencode/y: no key", "free/auto"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("body %s lacks %q", b, want)
		}
	}
	if e.oc.hits+e.claude.hits != 0 {
		t.Fatal("a request with no usable target reached upstream")
	}
}

func TestComboLastTargetDeliversAsIs(t *testing.T) {
	e := setup(t)
	up1, _ := newStub(t, 500, `{"error":"boom"}`)
	up2, r2 := newStub(t, 418, "teapot-body")
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up1.URL, Dialect: provider.Anthropic}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up2.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m1", "zai/m2"}},
	}
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 418 || string(b) != "teapot-body" || r2.hits.Load() != 1 {
		t.Fatalf("status %d body %q hits %d", resp.StatusCode, b, r2.hits.Load())
	}
}

func TestComboCircuitOpensThenHalfOpens(t *testing.T) {
	var mu sync.Mutex
	cur := time.Now()
	e := setup(t)
	e.rt.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	advance := func(d time.Duration) { mu.Lock(); cur = cur.Add(d); mu.Unlock() }

	up1, h1 := newStub(t, 500, `{"error":"boom"}`)
	up2, h2 := newScript(t, func(hit int32) (int, string) {
		if hit == 5 || hit == 7 {
			return 429, `{"error":"slow"}`
		}
		return 200, `{"ok":true}`
	})
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up1.URL, Dialect: provider.Anthropic}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up2.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m1", "zai/m2"}},
	}
	post := func() *http.Response { return e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil) }

	// three failures open the circuit; requests keep succeeding via target two
	for i := 0; i < 3; i++ {
		resp := post()
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("request %d: status %d", i+1, resp.StatusCode)
		}
		<-e.events // drain this request's event
	}
	if !e.rt.circuitOpen("opencode/m1") {
		t.Fatal("circuit did not open after 3 failures")
	}
	if h1.hits.Load() != 3 {
		t.Fatalf("target one hits = %d", h1.hits.Load())
	}

	// the plan shows the open target last
	plan := e.rt.Plan("free/auto", Need{})
	if len(plan.Steps) != 2 || plan.Steps[0].Target != "zai/m2" || plan.Steps[0].Status != "ready" ||
		plan.Steps[1].Target != "opencode/m1" || plan.Steps[1].Status != "circuit-open" {
		t.Fatalf("plan = %+v", plan.Steps)
	}

	// during cooldown target one is not hit at all
	resp := post()
	resp.Body.Close()
	if resp.StatusCode != 200 || h1.hits.Load() != 3 {
		t.Fatalf("cooldown request: status %d hits %d", resp.StatusCode, h1.hits.Load())
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "skipped opencode/m1: circuit open") {
		t.Fatalf("why = %q", ev.Why)
	}

	// past the cooldown exactly one probe gets through and fails: 60s backoff
	advance(31 * time.Second)
	resp = post()
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 500 || h1.hits.Load() != 4 {
		t.Fatalf("probe: status %d hits %d", resp.StatusCode, h1.hits.Load())
	}
	e.rt.cstate.mu.Lock()
	backoff := e.rt.cstate.health["opencode/m1"].backoff
	e.rt.cstate.mu.Unlock()
	if backoff != time.Minute {
		t.Fatalf("backoff = %v, want 1m", backoff)
	}

	// still cooling: no hit
	advance(31 * time.Second)
	resp = post()
	resp.Body.Close()
	if resp.StatusCode != 200 || h1.hits.Load() != 4 {
		t.Fatalf("second cooldown: status %d hits %d", resp.StatusCode, h1.hits.Load())
	}

	// past 60s the next probe fails again: 120s backoff
	advance(61 * time.Second)
	resp = post()
	resp.Body.Close()
	if resp.StatusCode != 500 || h1.hits.Load() != 5 {
		t.Fatalf("second probe: status %d hits %d", resp.StatusCode, h1.hits.Load())
	}
	e.rt.cstate.mu.Lock()
	backoff = e.rt.cstate.health["opencode/m1"].backoff
	e.rt.cstate.mu.Unlock()
	if backoff != 2*time.Minute {
		t.Fatalf("backoff = %v, want 2m", backoff)
	}
	if h2.Load() != 7 {
		t.Fatalf("target two hits = %d, want 7", h2.Load())
	}
}

func TestComboAllCircuitsOpenStillTriesSoonest(t *testing.T) {
	e := setup(t)
	up, hits := newScript(t, func(hit int32) (int, string) {
		if hit <= 4 {
			return 500, `{"error":"boom"}`
		}
		return 200, `{"ok":true}`
	})
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m1"}},
	}
	post := func() *http.Response { return e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil) }

	// three failures open the circuit on the sole target
	for i := 0; i < 3; i++ {
		resp := post()
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 500 {
			t.Fatalf("request %d: status %d", i+1, resp.StatusCode)
		}
		<-e.events
	}
	if !e.rt.circuitOpen("opencode/m1") {
		t.Fatal("circuit did not open")
	}

	// still cooling, but the combo is not blacked out: the request reaches the
	// upstream and gets its real error, not the 400 "no usable target"
	resp := post()
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 500 || string(b) != `{"error":"boom"}` || hits.Load() != 4 {
		t.Fatalf("status %d body %q hits %d", resp.StatusCode, b, hits.Load())
	}
	<-e.events

	// a forced attempt that succeeds closes the circuit again
	resp = post()
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != `{"ok":true}` || hits.Load() != 5 {
		t.Fatalf("status %d body %q hits %d", resp.StatusCode, b, hits.Load())
	}
	if e.rt.circuitOpen("opencode/m1") {
		t.Fatal("circuit still open after a successful forced attempt")
	}
	<-e.events
}

func TestComboFastestOrdersByLatency(t *testing.T) {
	var mu sync.Mutex
	cur := time.Now()
	e := setup(t)
	e.rt.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	advance := func(d time.Duration) { mu.Lock(); cur = cur.Add(d); mu.Unlock() }

	h1 := &atomic.Int32{}
	up1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h1.Add(1)
		advance(100 * time.Millisecond)
		io.WriteString(w, `{"ok":true,"from":"one"}`)
	}))
	t.Cleanup(up1.Close)
	h2 := &atomic.Int32{}
	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h2.Add(1)
		advance(10 * time.Millisecond)
		io.WriteString(w, `{"ok":true,"from":"two"}`)
	}))
	t.Cleanup(up2.Close)
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up1.URL, Dialect: provider.Anthropic}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up2.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m1", "zai/m2"}, Strategy: "fastest"},
	}

	for i := 0; i < 3; i++ {
		resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
		resp.Body.Close()
	}
	// first request measures target one (unmeasured targets go first and keep
	// their order); afterwards the faster target two wins
	if h1.Load() != 1 || h2.Load() != 2 {
		t.Fatalf("hits %d/%d", h1.Load(), h2.Load())
	}
}

func TestComboRoundRobinRotates(t *testing.T) {
	e, r1, r2 := twoCombo(t, "round_robin")
	for i := 0; i < 3; i++ {
		resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
		resp.Body.Close()
	}
	if r1.hits.Load() != 2 || r2.hits.Load() != 1 {
		t.Fatalf("hits %d/%d, want 2/1", r1.hits.Load(), r2.hits.Load())
	}
}

func TestComboPriorityKeepsOrder(t *testing.T) {
	e, r1, r2 := twoCombo(t, "priority")
	for i := 0; i < 3; i++ {
		resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
		resp.Body.Close()
	}
	if r1.hits.Load() != 3 || r2.hits.Load() != 0 {
		t.Fatalf("hits %d/%d, want 3/0", r1.hits.Load(), r2.hits.Load())
	}
}

func TestComboTiersAndAliases(t *testing.T) {
	e := setup(t)
	up, r := newStub(t, 200, `{"ok":true,"from":"zai"}`)
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"zai/m"}},
	}
	e.rt.Tiers = map[string]string{"haiku": "free/auto"}

	// a haiku model is rewritten to the combo
	resp := e.post(t, "/v1/messages", `{"model":"claude-haiku-5-5","messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || r.hits.Load() != 1 || e.claude.hits != 0 {
		t.Fatalf("status %d hits %d claude %d", resp.StatusCode, r.hits.Load(), e.claude.hits)
	}
	if !strings.Contains(string(r.body), `"model":"m"`) {
		t.Fatalf("upstream body = %s", r.body)
	}
	ev := <-e.events
	if ev.Via != "free/auto" || !strings.Contains(ev.Why, "tier haiku -> free/auto") {
		t.Fatalf("event = %+v", ev)
	}

	// sonnet has no tier and goes to Claude untouched
	body := `{"model":"claude-sonnet-5-5","messages":[],  "odd":  1}`
	resp = e.post(t, "/v1/messages", body, nil)
	resp.Body.Close()
	if string(e.claude.body) != body || r.hits.Load() != 1 {
		t.Fatalf("claude body = %s hits %d", e.claude.body, r.hits.Load())
	}
	<-e.events // drain this request's event

	// an alias beats the tier
	e.rt.Aliases = map[string]string{"claude-haiku-5-5": "opencode/direct"}
	ocHits := e.oc.hits
	resp = e.post(t, "/v1/messages", `{"model":"claude-haiku-5-5","messages":[]}`, nil)
	resp.Body.Close()
	if e.oc.hits != ocHits+1 || r.hits.Load() != 1 {
		t.Fatalf("alias: oc %d->%d zai %d", ocHits, e.oc.hits, r.hits.Load())
	}
	if !strings.Contains(string(e.oc.body), `"model":"direct"`) {
		t.Fatalf("alias body = %s", e.oc.body)
	}
	ev = <-e.events
	if ev.Via != "" || !strings.Contains(ev.Why, "alias claude-haiku-5-5 -> opencode/direct") {
		t.Fatalf("alias event = %+v", ev)
	}
}

func TestComboTranslatedTargetFallsThroughAndTranslates(t *testing.T) {
	chatBody := "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"id\":\"c1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	chatHits := &atomic.Int32{}
	chatUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h := chatHits.Add(1)
		io.Copy(io.Discard, req.Body)
		if h == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"slow down"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, chatBody)
	}))
	t.Cleanup(chatUp.Close)
	antUp, r2 := newStub(t, 200, `{"ok":true,"from":"ant"}`)

	e := setup(t)
	e.rt.Providers["groq"] = provider.Provider{Name: "groq", BaseURL: chatUp.URL + "/v1", Dialect: provider.OpenAIChat}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: antUp.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"groq/c1", "zai/m"}},
	}

	// the chat target's 429 falls through before a byte is written
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","stream":true,"messages":[{"role":"user","content":"x"}]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"from":"ant"`) || chatHits.Load() != 1 || r2.hits.Load() != 1 {
		t.Fatalf("status %d hits %d/%d body %s", resp.StatusCode, chatHits.Load(), r2.hits.Load(), b)
	}

	// a successful streamed reply from the chat target is translated back
	resp = e.post(t, "/v1/messages", `{"model":"free/auto","stream":true,"messages":[{"role":"user","content":"x"}]}`, nil)
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || chatHits.Load() != 2 || r2.hits.Load() != 1 {
		t.Fatalf("status %d hits %d/%d", resp.StatusCode, chatHits.Load(), r2.hits.Load())
	}
	for _, want := range []string{"event: message_start", `"text":"hi"`, "event: message_stop"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("reply lacks %q:\n%s", want, b)
		}
	}
}

func TestComboNeverForwardsClaudeCredentials(t *testing.T) {
	up, r := newStub(t, 200, `{"ok":true}`)
	e := setup(t)
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "key-" + acct, nil }
	e.rt.Combos = map[string]Combo{
		"free/auto": {Targets: []string{"opencode/m"}},
	}
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{
			"Authorization": "Bearer sk-ant-oat01-CLIENTSECRET",
			"X-Api-Key":     "sk-ant-api03-OTHERSECRET",
		})
	resp.Body.Close()
	if r.hits.Load() != 1 {
		t.Fatalf("hits = %d", r.hits.Load())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.auth != "Bearer key-opencode" || r.xapikey != "" {
		t.Fatalf("auth=%q x-api-key=%q", r.auth, r.xapikey)
	}
	if strings.Contains(string(r.body), "CLIENTSECRET") || strings.Contains(string(r.body), "OTHERSECRET") {
		t.Fatalf("claude credential reached upstream: %s", r.body)
	}
}

func TestPlanAgreesWithComboRouting(t *testing.T) {
	e, r1, r2 := twoCombo(t, "priority")
	e.rt.Caps = func(p, _ string) (Caps, bool) {
		if p == "opencode" {
			return Caps{Tools: false, Context: 100000}, true
		}
		return Caps{Tools: true, Context: 100000}, true
	}

	plan := e.rt.Plan("free/auto", Need{Tools: true})
	if plan.Combo != "free/auto" || plan.Resolved != "free/auto" || len(plan.Steps) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	// the plan lists the deliverable target first, then the skipped one
	if plan.Steps[0].Target != "zai/m2" || plan.Steps[0].Status != "ready" {
		t.Fatalf("step 1 = %+v", plan.Steps[0])
	}
	if plan.Steps[1].Target != "opencode/m1" || plan.Steps[1].Status != "skip" || plan.Steps[1].Reason != "no tool calling" {
		t.Fatalf("step 2 = %+v", plan.Steps[1])
	}

	// the real request agrees: target one is skipped, target two serves
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","tools":[{"name":"t"}],"messages":[]}`, nil)
	resp.Body.Close()
	if resp.StatusCode != 200 || r1.hits.Load() != 0 || r2.hits.Load() != 1 {
		t.Fatalf("status %d hits %d/%d", resp.StatusCode, r1.hits.Load(), r2.hits.Load())
	}

	// non-combo plans
	if p := e.rt.Plan("claude-sonnet-5-5", Need{}); len(p.Steps) != 1 || p.Steps[0].Target != "claude" || p.Steps[0].Status != "ready" {
		t.Fatalf("claude plan = %+v", p.Steps)
	}
	if p := e.rt.Plan("opencode/m1", Need{}); len(p.Steps) != 1 || p.Steps[0].Status != "ready" {
		t.Fatalf("provider plan = %+v", p.Steps)
	}
	if p := e.rt.Plan("opencode/m1", Need{}); p.Steps[0].Target != "opencode/m1" {
		t.Fatalf("provider plan target = %+v", p.Steps[0])
	}
}
