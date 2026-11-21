package router

import (
	"io"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

func TestPeekNeed(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want Need
	}{
		{"empty", "", Need{}},
		{"plain", `{"model":"m","messages":[]}`, Need{}},
		{"think enabled", `{"thinking":{"type":"enabled","budget_tokens":1024}}`, Need{Think: true}},
		{"think adaptive", `{"thinking":{"type":"adaptive"}}`, Need{Think: true}},
		{"think disabled", `{"thinking":{"type":"disabled"}}`, Need{}},
		{"think null", `{"thinking":null}`, Need{}},
		{"web search tool", `{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, Need{Tools: true, Web: true}},
		{"normal tool", `{"tools":[{"name":"read","type":"custom"}]}`, Need{Tools: true}},
		{"tools empty", `{"tools":[]}`, Need{}},
	} {
		want := tc.want
		want.Tokens, want.Input = len(tc.body)/3, len(tc.body)/3
		if got := peekNeed([]byte(tc.body)); got != want {
			t.Errorf("%s: peekNeed = %+v, want %+v", tc.name, got, want)
		}
	}
	// max_tokens still adds to Tokens but never to Input
	body := `{"model":"m","max_tokens":100}`
	got := peekNeed([]byte(body))
	if got.Tokens != len(body)/3+100 || got.Input != len(body)/3 {
		t.Fatalf("peekNeed = %+v", got)
	}
}

// scenarioEnv wires a zai stub provider behind the shared test env.
func scenarioEnv(t *testing.T) (*env, *stubUpstream) {
	t.Helper()
	e := setup(t)
	up, z := newStub(t, 200, anthropicSSE)
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	return e, z
}

func TestScenarioThinkReroutesClaudeRequest(t *testing.T) {
	e, zai := scenarioEnv(t)
	e.rt.Scenarios = map[string]string{"think": "zai/glm"}

	resp := e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"enabled","budget_tokens":1024},"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if zai.hits.Load() != 1 || e.claude.hits != 0 {
		t.Fatalf("zai %d claude %d", zai.hits.Load(), e.claude.hits)
	}
	if !strings.Contains(string(zai.body), `"model":"glm"`) {
		t.Fatalf("upstream body = %s", zai.body)
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "scenario think -> zai/glm") {
		t.Fatalf("why = %q", ev.Why)
	}

	// the same request without thinking stays on Claude
	resp = e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if zai.hits.Load() != 1 || e.claude.hits != 1 {
		t.Fatalf("no-thinking: zai %d claude %d", zai.hits.Load(), e.claude.hits)
	}
	<-e.events
}

func TestScenarioPrecedenceAndFallback(t *testing.T) {
	e, zai := scenarioEnv(t)
	e.rt.Scenarios = map[string]string{
		"long_context": "zai/big",
		"web_search":   "zai/web",
		"think":        "zai/glm",
	}
	e.rt.ScenarioTokens = 5 // every request body beats it

	// all three match: long_context wins
	resp := e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"enabled"},"tools":[{"type":"web_search_20250305"}],"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(zai.body), `"model":"big"`) {
		t.Fatalf("body = %s", zai.body)
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "scenario long_context -> zai/big") {
		t.Fatalf("why = %q", ev.Why)
	}

	// below the threshold web_search beats think
	e.rt.ScenarioTokens = 60000
	resp = e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"enabled"},"tools":[{"type":"web_search_20250305"}],"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(zai.body), `"model":"web"`) {
		t.Fatalf("body = %s", zai.body)
	}
	ev = <-e.events
	if !strings.Contains(ev.Why, "scenario web_search -> zai/web") {
		t.Fatalf("why = %q", ev.Why)
	}
}

func TestScenarioSkippedWhenUnconfigured(t *testing.T) {
	e, zai := scenarioEnv(t)
	// only web_search is configured; a thinking request matches think but
	// falls through to normal routing (tier, then Claude)
	e.rt.Scenarios = map[string]string{"web_search": "zai/web"}
	e.rt.Tiers = map[string]string{"sonnet": "zai/tier"}

	resp := e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"enabled"},"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ev := <-e.events
	if !strings.Contains(ev.Why, "tier sonnet -> zai/tier") || !strings.Contains(string(zai.body), `"model":"tier"`) {
		t.Fatalf("why = %q body = %s", ev.Why, zai.body)
	}

	// nothing configured at all: Claude
	e.rt.Scenarios, e.rt.Tiers = nil, nil
	resp = e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"enabled"},"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if e.claude.hits != 1 || zai.hits.Load() != 1 {
		t.Fatalf("claude %d zai %d", e.claude.hits, zai.hits.Load())
	}
	<-e.events
}

func TestScenarioNeverReroutesNonClaudeModel(t *testing.T) {
	e, zai := scenarioEnv(t)
	e.rt.Scenarios = map[string]string{"think": "zai/glm"}
	ocHits := e.oc.hits
	resp := e.post(t, "/v1/messages", `{"model":"opencode/m1","thinking":{"type":"enabled"},"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if e.oc.hits != ocHits+1 || zai.hits.Load() != 0 {
		t.Fatalf("oc %d zai %d", e.oc.hits, zai.hits.Load())
	}
	<-e.events
}

func TestScenarioAliasWins(t *testing.T) {
	e, zai := scenarioEnv(t)
	e.rt.Scenarios = map[string]string{"think": "zai/glm"}
	e.rt.Aliases = map[string]string{"claude-sonnet-5-5": "opencode/direct"}
	ocHits := e.oc.hits
	resp := e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"enabled"},"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if e.oc.hits != ocHits+1 || zai.hits.Load() != 0 {
		t.Fatalf("oc %d zai %d", e.oc.hits, zai.hits.Load())
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "alias claude-sonnet-5-5 -> opencode/direct") {
		t.Fatalf("why = %q", ev.Why)
	}
}

func TestScenarioTargetCanBeACombo(t *testing.T) {
	e, zai := scenarioEnv(t)
	e.rt.Combos = map[string]Combo{"free/auto": {Targets: []string{"zai/m2"}}}
	e.rt.Scenarios = map[string]string{"think": "free/auto"}
	resp := e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","thinking":{"type":"adaptive"},"messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if zai.hits.Load() != 1 {
		t.Fatalf("zai %d", zai.hits.Load())
	}
	ev := <-e.events
	if ev.Via != "free/auto" || !strings.Contains(ev.Why, "scenario think -> free/auto") {
		t.Fatalf("event = %+v", ev)
	}
}

func TestScenarioLongContextThreshold(t *testing.T) {
	e, zai := scenarioEnv(t)
	e.rt.Scenarios = map[string]string{"long_context": "zai/big"}

	// default threshold 60000: ~63k tokens needs a body just over 180k
	big := `{"model":"claude-sonnet-5-5","messages":[{"role":"user","content":"` + strings.Repeat("x", 190*1024) + `"}]}`
	resp := e.post(t, "/v1/messages", big, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if zai.hits.Load() != 1 {
		t.Fatalf("default threshold: zai %d", zai.hits.Load())
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "scenario long_context -> zai/big") {
		t.Fatalf("why = %q", ev.Why)
	}

	// a smaller body with a custom threshold still triggers
	e.rt.ScenarioTokens = 10
	resp = e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","messages":[{"role":"user","content":"more than thirty bytes of input here"}]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if zai.hits.Load() != 2 {
		t.Fatalf("custom threshold: zai %d", zai.hits.Load())
	}
	<-e.events

	// and under the default threshold it stays on Claude
	e.rt.ScenarioTokens = 60000
	resp = e.post(t, "/v1/messages", `{"model":"claude-sonnet-5-5","messages":[{"role":"user","content":"more than thirty bytes of input here"}]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if e.claude.hits != 1 || zai.hits.Load() != 2 {
		t.Fatalf("below threshold: claude %d zai %d", e.claude.hits, zai.hits.Load())
	}
	<-e.events
}

func TestScenarioPlanAgreesWithServeHTTP(t *testing.T) {
	e, _ := scenarioEnv(t)
	e.rt.Scenarios = map[string]string{"think": "zai/glm"}
	plan := e.rt.Plan("claude-sonnet-5-5", Need{Think: true})
	if plan.Resolved != "zai/glm" || !strings.Contains(plan.Why, "scenario think -> zai/glm") {
		t.Fatalf("plan = %+v", plan)
	}
	plan = e.rt.Plan("claude-sonnet-5-5", Need{})
	if plan.Resolved != "claude-sonnet-5-5" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestComboWeightedDrawsAndFallsThrough(t *testing.T) {
	e := setup(t)
	up1, r1 := newStub(t, 200, anthropicSSE)
	up2, r2 := newStub(t, 429, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
	e.rt.Providers["opencode"] = provider.Provider{Name: "opencode", BaseURL: up1.URL, Dialect: provider.Anthropic}
	e.rt.Providers["zai"] = provider.Provider{Name: "zai", BaseURL: up2.URL, Dialect: provider.Anthropic}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) { return "k-" + acct, nil }
	e.rt.Combos = map[string]Combo{"free/auto": {
		Targets:  []string{"opencode/m1", "zai/m2"},
		Strategy: "weighted",
		Weights:  map[string]int{"opencode/m1": 3},
	}}
	// Efraimidis-Spirakis keys u^(1/w): the draw b=0.5 (weight 1, the
	// missing-weight default) beats a=0.001^(1/3)=0.1 (weight 3), so zai is
	// picked first; its 429 falls through to opencode
	draws := []float64{0.001, 0.5}
	i := 0
	e.rt.rand = func() float64 { v := draws[i%len(draws)]; i++; return v }
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if r2.hits.Load() != 1 || r1.hits.Load() != 1 {
		t.Fatalf("fall-through hits opencode %d zai %d", r1.hits.Load(), r2.hits.Load())
	}
	ev := <-e.events
	if !strings.Contains(ev.Why, "zai/m2 429") {
		t.Fatalf("why = %q", ev.Why)
	}
}

func TestComboWeightedPlanOrdersByWeight(t *testing.T) {
	e, _, _ := twoCombo(t, "weighted")
	e.rt.Combos["free/auto"] = Combo{
		Targets:  []string{"zai/m2", "opencode/m1", "groq/m3"},
		Strategy: "weighted",
		Weights:  map[string]int{"groq/m3": 5, "opencode/m1": 3},
	}
	plan := e.rt.Plan("free/auto", Need{})
	if len(plan.Steps) != 3 {
		t.Fatalf("steps = %+v", plan.Steps)
	}
	want := []struct {
		target string
		reason string
	}{
		{"groq/m3", "weight 5"},
		{"opencode/m1", "weight 3"},
		{"zai/m2", "weight 1"}, // missing weight defaults to 1
	}
	for i, w := range want {
		if plan.Steps[i].Target != w.target || plan.Steps[i].Reason != w.reason {
			t.Fatalf("step %d = %+v, want %s %s", i, plan.Steps[i], w.target, w.reason)
		}
	}
}

func TestComboHeadroomOrdersByQuota(t *testing.T) {
	e, r1, r2 := twoCombo(t, "headroom")
	e.rt.Headroom = func(p provider.Provider, acct string) (float64, bool) {
		switch p.Name {
		case "opencode":
			return 0.2, true
		case "zai":
			return 0.9, true
		}
		return 0, false
	}
	resp := e.post(t, "/v1/messages", `{"model":"free/auto","messages":[]}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if r2.hits.Load() != 1 || r1.hits.Load() != 0 {
		t.Fatalf("hits opencode %d zai %d: most-headroom target should win", r1.hits.Load(), r2.hits.Load())
	}
	<-e.events
}

func TestComboHeadroomUnknownLastAndNil(t *testing.T) {
	e, _, _ := twoCombo(t, "headroom")
	e.rt.Providers["groq"] = provider.Provider{Name: "groq", BaseURL: "http://127.0.0.1:1", Dialect: provider.OpenAIChat}
	e.rt.Combos["free/auto"] = Combo{
		Targets:  []string{"groq/m3", "opencode/m1", "zai/m2"},
		Strategy: "headroom",
	}
	e.rt.Headroom = func(p provider.Provider, acct string) (float64, bool) {
		if p.Name == "opencode" {
			return 0.1, true
		}
		return 0, false // zai and groq unknown
	}
	plan := e.rt.Plan("free/auto", Need{})
	if len(plan.Steps) != 3 || plan.Steps[0].Target != "opencode/m1" {
		t.Fatalf("steps = %+v", plan.Steps)
	}
	// unknown headroom keeps written order after known ones
	if plan.Steps[1].Target != "groq/m3" || plan.Steps[2].Target != "zai/m2" {
		t.Fatalf("steps = %+v", plan.Steps)
	}

	// nil Headroom keeps the written order entirely
	e.rt.Headroom = nil
	plan = e.rt.Plan("free/auto", Need{})
	if plan.Steps[0].Target != "groq/m3" || plan.Steps[1].Target != "opencode/m1" || plan.Steps[2].Target != "zai/m2" {
		t.Fatalf("nil headroom steps = %+v", plan.Steps)
	}
}
