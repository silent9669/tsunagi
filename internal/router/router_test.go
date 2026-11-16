package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/provider"
)

const token = "s3cret"

type seen struct {
	path   string
	header http.Header
	body   []byte
	hits   int
}

// upstream records what it receives and answers 200 with a tiny JSON body.
func upstream(t *testing.T) (*httptest.Server, *seen) {
	t.Helper()
	s := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits++
		s.path = r.URL.RequestURI()
		s.header = r.Header.Clone()
		s.body, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, s
}

type env struct {
	rt     *Router
	front  *httptest.Server
	claude *seen
	oc     *seen
	events chan events.Event
}

func setup(t *testing.T) *env {
	t.Helper()
	cs, claude := upstream(t)
	os_, oc := upstream(t)
	e := &env{claude: claude, oc: oc, events: make(chan events.Event, 16)}
	e.front = httptest.NewUnstartedServer(nil)
	e.front.Start()
	t.Cleanup(e.front.Close)

	providers := []provider.Provider{
		{Name: "opencode", BaseURL: os_.URL + "/inference/anthropic", Dialect: provider.Anthropic},
		{Name: "openai", BaseURL: "http://127.0.0.1:1", Dialect: provider.OpenAIResponses},
		{Name: "groq", BaseURL: "http://127.0.0.1:1", Dialect: provider.OpenAIChat},
	}
	key := func(_ provider.Provider, name string) (string, error) {
		if name == "opencode" {
			return "oc_key_123456", nil
		}
		return "", errors.New("none")
	}
	host := strings.TrimPrefix(e.front.URL, "http://")
	e.rt = New(host, token, providers, "opencode", key)
	cu, _ := url.Parse(cs.URL)
	e.rt.Claude = cu
	e.rt.OnEvent = func(ev events.Event) { e.events <- ev }
	e.front.Config.Handler = e.rt
	return e
}

func (e *env) post(t *testing.T, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", e.front.URL+"/"+token+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestClaudeBranchIsUntouched(t *testing.T) {
	e := setup(t)
	body := `{"model":"claude-sonnet-5-5",  "messages":[],"z":1}` // odd spacing must survive
	resp := e.post(t, "/v1/messages?beta=true", body, map[string]string{
		"Authorization":     "Bearer oauth-claude-login",
		"Anthropic-Version": "2023-06-01",
		"X-Claude-Marker":   "keep-me",
	})
	resp.Body.Close()
	if e.oc.hits != 0 {
		t.Fatal("claude request reached a provider")
	}
	if string(e.claude.body) != body {
		t.Fatalf("body changed:\n got %s\nwant %s", e.claude.body, body)
	}
	if e.claude.path != "/v1/messages?beta=true" {
		t.Fatalf("path = %s", e.claude.path)
	}
	for k, want := range map[string]string{
		"Authorization": "Bearer oauth-claude-login", "Anthropic-Version": "2023-06-01", "X-Claude-Marker": "keep-me",
	} {
		if got := e.claude.header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if e.claude.header.Get("X-Forwarded-For") != "" {
		t.Error("X-Forwarded-For was added")
	}
}

func TestProviderPrefixSwapsKeyAndStripsModelPrefix(t *testing.T) {
	e := setup(t)
	resp := e.post(t, "/v1/messages", `{"model":"opencode/minimax-m3","max_tokens":5,"big":12345678901234567890}`, map[string]string{
		"Authorization": "Bearer oauth-claude-login",
		"X-Api-Key":     "claude-api-key",
	})
	resp.Body.Close()
	if e.claude.hits != 0 {
		t.Fatal("provider request reached Claude")
	}
	if e.oc.path != "/inference/anthropic/v1/messages" {
		t.Fatalf("path = %s", e.oc.path)
	}
	if got := e.oc.header.Get("Authorization"); got != "Bearer oc_key_123456" {
		t.Fatalf("Authorization = %q", got)
	}
	if e.oc.header.Get("X-Api-Key") != "" {
		t.Fatal("x-api-key leaked to provider")
	}
	if bytes.Contains(e.oc.body, []byte("oauth-claude-login")) {
		t.Fatal("claude credential in body")
	}
	var m map[string]json.RawMessage
	json.Unmarshal(e.oc.body, &m)
	if string(m["model"]) != `"minimax-m3"` {
		t.Fatalf("model = %s", m["model"])
	}
	if string(m["big"]) != "12345678901234567890" {
		t.Fatalf("other fields were rewritten: %s", m["big"])
	}
}

func TestBareModelUsesDefaultProvider(t *testing.T) {
	e := setup(t)
	e.post(t, "/v1/messages", `{"model":"glm-5.3"}`, nil).Body.Close()
	if e.oc.hits != 1 || !bytes.Contains(e.oc.body, []byte(`"glm-5.3"`)) {
		t.Fatalf("hits=%d body=%s", e.oc.hits, e.oc.body)
	}
}

func TestUnknownPrefixIsPartOfDefaultProviderModel(t *testing.T) {
	e := setup(t)
	e.post(t, "/v1/messages", `{"model":"anthropic/claude-x"}`, nil).Body.Close()
	if !bytes.Contains(e.oc.body, []byte(`"anthropic/claude-x"`)) {
		t.Fatalf("body=%s", e.oc.body)
	}
}

func TestProviderCanServeClaudeModels(t *testing.T) {
	e := setup(t)
	e.post(t, "/v1/messages", `{"model":"opencode/claude-sonnet-5-5"}`, nil).Body.Close()
	if e.oc.hits != 1 || e.claude.hits != 0 {
		t.Fatalf("oc=%d claude=%d", e.oc.hits, e.claude.hits)
	}
}

func TestNoModelGoesToClaude(t *testing.T) {
	e := setup(t)
	req, _ := http.NewRequest("GET", e.front.URL+"/"+token+"/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if e.claude.hits != 1 || e.oc.hits != 0 {
		t.Fatalf("claude=%d oc=%d", e.claude.hits, e.oc.hits)
	}
}

func TestMissingKeyAndTranslatorGiveAnthropicShapedErrors(t *testing.T) {
	e := setup(t)
	e.rt.Key = func(provider.Provider, string) (string, error) { return "", errors.New("none") }
	for body, want := range map[string]struct {
		status int
		text   string
	}{
		`{"model":"opencode/x"}`: {401, "tsunagi login opencode"},
		`{"model":"groq/x"}`:     {401, "tsunagi login groq"},
		`{"model":"openai/gpt"}`: {401, "tsunagi login openai"},
		`{"model":"nope/x"}`:     {401, "tsunagi login opencode"}, // unknown prefix -> default provider
	} {
		resp := e.post(t, "/v1/messages", body, nil)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var out struct {
			Type  string `json:"type"`
			Error struct{ Type, Message string }
		}
		if json.Unmarshal(b, &out) != nil || out.Type != "error" {
			t.Fatalf("%s: not an Anthropic error: %s", body, b)
		}
		if resp.StatusCode != want.status || !strings.Contains(out.Error.Message, want.text) {
			t.Errorf("%s: status %d msg %q", body, resp.StatusCode, out.Error.Message)
		}
	}
	if e.oc.hits != 0 {
		t.Fatal("request went upstream without a key")
	}
}

func TestRejectsBrowserAndWrongTokenAndWrongHost(t *testing.T) {
	e := setup(t)
	cases := []struct {
		name string
		do   func() *http.Response
		want int
	}{
		{"origin", func() *http.Response {
			return e.post(t, "/v1/messages", `{"model":"opencode/x"}`, map[string]string{"Origin": "https://evil.example"})
		}, 403},
		{"no token", func() *http.Response {
			resp, _ := http.Post(e.front.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"opencode/x"}`))
			return resp
		}, 404},
		{"wrong token", func() *http.Response {
			resp, _ := http.Post(e.front.URL+"/nope/v1/messages", "application/json", strings.NewReader(`{}`))
			return resp
		}, 404},
		{"token prefix only", func() *http.Response {
			resp, _ := http.Post(e.front.URL+"/"+token+"x/v1/messages", "application/json", strings.NewReader(`{}`))
			return resp
		}, 404},
		{"host", func() *http.Response {
			req, _ := http.NewRequest("POST", e.front.URL+"/"+token+"/v1/messages", strings.NewReader(`{}`))
			req.Host = "evil.example"
			resp, _ := http.DefaultClient.Do(req)
			return resp
		}, 403},
	}
	for _, c := range cases {
		resp := c.do()
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}
	if e.oc.hits+e.claude.hits != 0 {
		t.Fatal("a rejected request reached upstream")
	}
}

func TestStreamsServerSentEventsIncrementally(t *testing.T) {
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: one\n\n"))
		w.(http.Flusher).Flush()
		<-release
		w.Write([]byte("data: two\n\n"))
	}))
	defer up.Close()
	e := setup(t)
	e.rt.Claude, _ = url.Parse(up.URL)

	resp := e.post(t, "/v1/messages", `{"model":"claude-x"}`, nil)
	defer resp.Body.Close()
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := resp.Body.Read(buf)
		got <- string(buf[:n])
	}()
	select {
	case s := <-got:
		if !strings.Contains(s, "one") {
			t.Fatalf("first chunk = %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first chunk was buffered until the stream ended")
	}
	close(release)
}

func TestEventsCarryNoSecrets(t *testing.T) {
	e := setup(t)
	e.post(t, "/v1/messages", `{"model":"opencode/m"}`, map[string]string{"Authorization": "Bearer oauth-claude-login"}).Body.Close()
	var ev events.Event
	select {
	case ev = <-e.events: // emitted after the response completes
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	b, _ := json.Marshal(ev)
	for _, secret := range []string{"oauth-claude-login", "oc_key_123456", token} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatalf("event contains %q: %s", secret, b)
		}
	}
	if ev.Provider != "opencode" || ev.Status != 200 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestResponsesProviderIsTranslatedBothWays(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotAuth, gotBody = r.URL.Path, r.Header.Get("Authorization"), string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"hi\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\"}}\n\n")
	}))
	defer up.Close()
	e := setup(t)
	e.rt.Providers["openai"] = provider.Provider{Name: "openai", BaseURL: up.URL + "/v1", Dialect: provider.OpenAIResponses}
	e.rt.Key = func(provider.Provider, string) (string, error) { return "sk-openai-123456", nil }

	resp := e.post(t, "/v1/messages", `{"model":"openai/gpt-x","stream":true,"max_tokens":9,"messages":[{"role":"user","content":"yo"}]}`,
		map[string]string{"Authorization": "Bearer oauth-claude-login"})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if gotPath != "/v1/responses" || gotAuth != "Bearer sk-openai-123456" {
		t.Fatalf("upstream saw path=%s auth=%s", gotPath, gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"gpt-x"`) || strings.Contains(gotBody, "oauth-claude-login") {
		t.Fatalf("upstream body = %s", gotBody)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %s", ct)
	}
	for _, want := range []string{"event: message_start", `"text":"hi"`, "event: message_stop"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("reply lacks %q:\n%s", want, b)
		}
	}
	if e.claude.hits != 0 {
		t.Fatal("reached Claude")
	}
}

func TestResponsesProviderUpstreamErrorKeepsStatusAndShape(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"message":"slow down"}}`)
	}))
	defer up.Close()
	e := setup(t)
	e.rt.Providers["openai"] = provider.Provider{Name: "openai", BaseURL: up.URL, Dialect: provider.OpenAIResponses}
	e.rt.Key = func(provider.Provider, string) (string, error) { return "sk-openai-123456", nil }
	resp := e.post(t, "/v1/messages", `{"model":"openai/gpt-x","messages":[]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 429 || !strings.Contains(string(b), `"rate_limit_error"`) || !strings.Contains(string(b), "slow down") {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
}

func TestResponsesProviderCountTokensIsLocal(t *testing.T) {
	e := setup(t)
	e.rt.Key = func(provider.Provider, string) (string, error) { return "sk-openai-123456", nil }
	resp := e.post(t, "/v1/messages/count_tokens", `{"model":"openai/gpt-x","messages":[{"role":"user","content":"hello"}]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "input_tokens") {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
}

func TestChatProviderIsTranslatedBothWays(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotAuth, gotBody = r.URL.Path, r.Header.Get("Authorization"), string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer up.Close()
	e := setup(t)
	e.rt.Providers["groq"] = provider.Provider{Name: "groq", BaseURL: up.URL + "/openai/v1", Dialect: provider.OpenAIChat}
	e.rt.Key = func(provider.Provider, string) (string, error) { return "gsk-123456789", nil }

	resp := e.post(t, "/v1/messages", `{"model":"groq/llama-x","stream":true,"max_tokens":9,"system":"sys","messages":[{"role":"user","content":"yo"}]}`,
		map[string]string{"Authorization": "Bearer oauth-claude-login"})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if gotPath != "/openai/v1/chat/completions" || gotAuth != "Bearer gsk-123456789" {
		t.Fatalf("upstream saw path=%s auth=%s", gotPath, gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"llama-x"`) || !strings.Contains(gotBody, `"role":"system"`) {
		t.Fatalf("upstream body = %s", gotBody)
	}
	for _, want := range []string{"event: message_start", `"text":"hi"`, `"stop_reason":"end_turn"`, "event: message_stop"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("reply lacks %q:\n%s", want, b)
		}
	}
}

func TestCatalogueProviderCannotReachLoopback(t *testing.T) {
	for _, h := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(h, "") // the guard is off when a proxy is configured
	}
	up, hits := upstream(t)
	for _, d := range []provider.Dialect{provider.Anthropic, provider.OpenAIChat, provider.OpenAIResponses} {
		e := setup(t)
		e.rt.guarded = guardedTransport(http.DefaultTransport.(*http.Transport))
		e.rt.Providers["acme"] = provider.Provider{Name: "acme", BaseURL: up.URL + "/v1", Dialect: d, Untrusted: true}
		e.rt.Key = func(provider.Provider, string) (string, error) { return "acme-key-123456", nil }
		resp := e.post(t, "/v1/messages", `{"model":"acme/x","messages":[]}`, nil)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 502 || !strings.Contains(string(b), "not allowed") {
			t.Errorf("%s: status %d body %s", d, resp.StatusCode, b)
		}
	}
	if hits.hits != 0 {
		t.Fatal("a catalogue provider reached a loopback server")
	}
}

func TestBlockedIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "192.168.0.1": true, "172.16.0.1": true, "169.254.169.254": true,
		"100.64.0.1": true, "0.0.0.0": true, "::1": true, "fe80::1": true, "fd00::1": true,
		"8.8.8.8": false, "104.18.0.1": false, "2606:4700::1": false,
	} {
		if got := blockedIP(net.ParseIP(ip)); got != want {
			t.Errorf("blockedIP(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestSignInProviderForcesStreamAndAssemblesPlainReply(t *testing.T) {
	var gotBody, gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotAuth = string(b), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"plain hi\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n")
	}))
	defer up.Close()
	e := setup(t)
	e.rt.Providers["chatgpt"] = provider.Provider{Name: "chatgpt", BaseURL: up.URL + "/v1", Dialect: provider.OpenAIResponses, Auth: "chatgpt"}
	e.rt.Key = func(_ provider.Provider, n string) (string, error) { return "access-token-xyz", nil }

	resp := e.post(t, "/v1/messages", `{"model":"chatgpt/gpt-x","max_tokens":9,"messages":[{"role":"user","content":"yo"}]}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(gotBody, `"stream":true`) || !strings.Contains(gotBody, `"store":false`) {
		t.Fatalf("upstream body = %s", gotBody)
	}
	if gotAuth != "Bearer access-token-xyz" {
		t.Fatalf("auth = %s", gotAuth)
	}
	var m struct {
		Type    string
		Content []struct{ Text string }
	}
	if json.Unmarshal(b, &m) != nil || m.Type != "message" || len(m.Content) != 1 || m.Content[0].Text != "plain hi" {
		t.Fatalf("reply = %s", b)
	}
}

func TestAliasesRewriteModelBeforeRouting(t *testing.T) {
	e := setup(t)
	e.rt.Aliases = map[string]string{
		"claude-haiku-5-5": "opencode/glm-5.3", // background work to a cheap provider
		"fast":             "claude-sonnet-5-5",
	}
	e.post(t, "/v1/messages", `{"model":"claude-haiku-5-5","messages":[]}`, nil).Body.Close()
	if e.oc.hits != 1 || !bytes.Contains(e.oc.body, []byte(`"model":"glm-5.3"`)) || e.claude.hits != 0 {
		t.Fatalf("haiku alias: oc=%d claude=%d body=%s", e.oc.hits, e.claude.hits, e.oc.body)
	}
	e.post(t, "/v1/messages", `{"model":"fast","messages":[]}`, nil).Body.Close()
	if e.claude.hits != 1 || !bytes.Contains(e.claude.body, []byte(`"model":"claude-sonnet-5-5"`)) {
		t.Fatalf("claude alias: hits=%d body=%s", e.claude.hits, e.claude.body)
	}
}
