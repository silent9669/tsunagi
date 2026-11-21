package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/free"
	"github.com/silent9669/tsunagi/internal/provider"
)

const testORFixture = `{"data":[
	{"id":"qwen/qwen3-coder:free","context_length":262144,
	 "pricing":{"prompt":"0","completion":"0"},
	 "supported_parameters":["tools","reasoning"]},
	{"id":"openai/gpt-4o","context_length":128000,
	 "pricing":{"prompt":"0.0000025","completion":"0.00001"},
	 "supported_parameters":["tools"]},
	{"id":"openrouter/free","context_length":200000,
	 "pricing":{"prompt":"0","completion":"0"},
	 "supported_parameters":["tools"]}
]}`

const testZenFixture = `{"data":[
	{"id":"big-pickle","limit":{"context":200000},
	 "capabilities":{"tools":{"supported":true},"reasoning":{"supported":true}},
	 "cost":{"text":{"input":{"price":0},"output":{"price":0}}}},
	{"id":"gpt-5","limit":{"context":400000},
	 "capabilities":{"tools":{"supported":true},"reasoning":{"supported":false}},
	 "cost":{"text":{"input":{"price":0.000005},"output":{"price":0.00003}}}}
]}`

// freeEnv points discovery at fake servers, stubs the key lookup, and gives
// the test a temp HOME with the given config.json body.
func freeEnv(t *testing.T, cfgBody string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg"))
	dir := filepath.Join(home, ".config", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if cfgBody != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfgBody), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testORFixture))
	}))
	zen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testZenFixture))
	}))
	t.Cleanup(or.Close)
	t.Cleanup(zen.Close)
	oldOR, oldZen := free.OpenRouterURL, free.ZenURL
	free.OpenRouterURL, free.ZenURL = or.URL, zen.URL
	t.Cleanup(func() { free.OpenRouterURL, free.ZenURL = oldOR, oldZen })

	oldKey, oldLive := hasFreeKey, liveFreeModels
	hasFreeKey = func(config.Config, string) bool { return false }
	liveFreeModels = func(context.Context, config.Config, provider.Provider) ([]string, error) {
		return nil, nil
	}
	t.Cleanup(func() { hasFreeKey, liveFreeModels = oldKey, oldLive })
}

func configPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".config", "tsunagi", "config.json")
}

func TestFreeTable(t *testing.T) {
	freeEnv(t, "")
	code, out := captureRun(t, "free")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	for _, want := range []string{"opencode", "openrouter", "groq", "cloudflare", "all models", "setup", "private", "trains", "kilo", "no key needed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	// two free OpenRouter models and one Zen model are counted
	if !strings.Contains(out, "openrouter") || !strings.Contains(out, "\n") {
		t.Fatalf("out = %s", out)
	}
}

func TestFreeTableJSON(t *testing.T) {
	freeEnv(t, "")
	code, out := captureRun(t, "free", "--json")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	type row struct {
		Provider string `json:"provider"`
		Free     any    `json:"free"`
		Data     string `json:"data"`
		Key      string `json:"key"`
		Access   string `json:"access"`
	}
	var doc struct {
		Providers []row `json:"providers"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("bad json %q: %v", out, err)
	}
	if len(doc.Providers) != 12 {
		t.Fatalf("providers = %+v", doc.Providers)
	}
	var or, cf, kilo *row
	for i := range doc.Providers {
		switch doc.Providers[i].Provider {
		case "openrouter":
			or = &doc.Providers[i]
		case "cloudflare":
			cf = &doc.Providers[i]
		case "kilo":
			kilo = &doc.Providers[i]
		}
	}
	if or == nil || or.Free != float64(2) {
		t.Fatalf("openrouter row = %+v", or)
	}
	if cf == nil || cf.Key != "setup" {
		t.Fatalf("cloudflare row = %+v", cf)
	}
	if kilo == nil || kilo.Key != "no key needed" || kilo.Access != "keyless" {
		t.Fatalf("kilo row = %+v", kilo)
	}
}

func TestFreeProviderDetail(t *testing.T) {
	freeEnv(t, "")
	code, out := captureRun(t, "free", "openrouter")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	for _, want := range []string{"qwen/qwen3-coder:free", "openrouter/free", "signup", "openrouter.ai", "checked", "tools"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	// priced model must not appear
	if strings.Contains(out, "gpt-4o") {
		t.Fatalf("priced model shown:\n%s", out)
	}

	code, out = captureRun(t, "free", "groq")
	if code != 0 {
		t.Fatalf("groq code %d out %s", code, out)
	}
	for _, want := range []string{"console.groq.com", "private", "needs a key"} {
		if !strings.Contains(out, want) {
			t.Fatalf("groq output lacks %q:\n%s", want, out)
		}
	}

	code, _ = captureRun(t, "free", "bogus")
	if code == 0 {
		t.Fatal("unknown provider accepted")
	}
}

func TestFreeAllowDeny(t *testing.T) {
	freeEnv(t, `{"free":{"allow_trains":["google"]}}`)

	code, out := captureRun(t, "free", "allow", "openrouter")
	if code != 0 || !strings.Contains(out, "openrouter") {
		t.Fatalf("allow code %d out %s", code, out)
	}
	b, _ := os.ReadFile(configPath(t))
	if !strings.Contains(string(b), `"openrouter"`) || !strings.Contains(string(b), `"google"`) {
		t.Fatalf("config = %s", b)
	}

	// Private providers need no allow-list entry
	code, _ = captureRun(t, "free", "allow", "groq")
	if code == 0 {
		t.Fatal("private provider allowed")
	}
	// unknown names error
	code, _ = captureRun(t, "free", "allow", "bogus")
	if code == 0 {
		t.Fatal("unknown provider allowed")
	}

	code, out = captureRun(t, "free", "deny", "google")
	if code != 0 {
		t.Fatalf("deny code %d out %s", code, out)
	}
	b, _ = os.ReadFile(configPath(t))
	if strings.Contains(string(b), `"google"`) {
		t.Fatalf("google not removed:\n%s", b)
	}
	if !strings.Contains(string(b), `"openrouter"`) {
		t.Fatalf("openrouter lost:\n%s", b)
	}
}

func TestFreeComboDryRun(t *testing.T) {
	freeEnv(t, `{"free":{"allow_trains":["opencode","openrouter"]}}`)

	// no keys anywhere: nothing can be a candidate
	code, out := captureRun(t, "free", "combo", "--dry-run", "--no-probe")
	if code == 0 {
		t.Fatalf("combo with no keys wrote targets:\n%s", out)
	}

	// key only the two ModelsMode providers
	old := hasFreeKey
	hasFreeKey = func(_ config.Config, name string) bool { return name == "opencode" || name == "openrouter" }
	defer func() { hasFreeKey = old }()

	code, out = captureRun(t, "free", "combo", "--dry-run", "--no-probe")
	if code != 0 {
		t.Fatalf("combo code %d out %s", code, out)
	}
	for _, want := range []string{"opencode/big-pickle", "openrouter/qwen/qwen3-coder:free", "openrouter/openrouter/free"} {
		if !strings.Contains(out, want) {
			t.Fatalf("targets lack %q:\n%s", want, out)
		}
	}
	// registry order: opencode before openrouter
	if strings.Index(out, "opencode/") > strings.Index(out, "openrouter/") {
		t.Fatalf("order wrong:\n%s", out)
	}
	// dry-run must not write the combo
	if _, err := os.Stat(configPath(t)); err == nil {
		b, _ := os.ReadFile(configPath(t))
		if strings.Contains(string(b), "free/auto") {
			t.Fatalf("dry-run wrote config:\n%s", b)
		}
	}
}

func TestFreeComboRefusesUserCombo(t *testing.T) {
	freeEnv(t, `{"combos":{"free/auto":{"targets":["a/b"]}}}`)
	code, out := captureRun(t, "free", "combo", "--dry-run", "--no-probe")
	if code == 0 || !strings.Contains(out, "auto") {
		t.Fatalf("code %d out %s", code, out)
	}
}

func TestFreeRefresh(t *testing.T) {
	freeEnv(t, "")
	code, out := captureRun(t, "free", "refresh")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out, "[ok]") {
		t.Fatalf("out = %s", out)
	}
	snap, ok := free.Load()
	if !ok || len(snap.Models) != 3 {
		t.Fatalf("snapshot = %+v ok=%v", snap, ok)
	}
}

// TestCatalogFreeCands proves the FromCatalog path: zero-priced catalogue
// models become candidates, paid or unpriced ones do not.
func TestCatalogFreeCands(t *testing.T) {
	zero, price := 0.0, 0.5
	entries := []catalog.Entry{
		{ID: "zai", Caps: map[string]catalog.ModelInfo{
			"glm-4.5-flash": {Tools: true, Context: 131072, In: &zero, Out: &zero},
			"glm-4.7-flash": {Tools: true, Context: 200000, In: &zero, Out: &zero},
			"glm-5":         {Tools: true, Context: 204800, In: &price, Out: &price},
			"unpriced":      {Tools: true, Context: 100},
			"half":          {In: &zero},
		}},
		{ID: "other", Caps: map[string]catalog.ModelInfo{
			"free-x": {In: &zero, Out: &zero},
		}},
	}
	got := catalogFreeCands(entries, "zai")
	if len(got) != 2 || got[0].ID != "glm-4.5-flash" || got[1].ID != "glm-4.7-flash" {
		t.Fatalf("cands = %+v", got)
	}
	if got[0].Context != 131072 || got[0].Tools == nil || !*got[0].Tools {
		t.Fatalf("cand = %+v", got[0])
	}
	if n := catalogFreeCands(entries, "other"); len(n) != 1 || n[0].ID != "free-x" {
		t.Fatalf("other = %+v", n)
	}
}

func TestZaiRegistryEntry(t *testing.T) {
	e, ok := registryEntry("zai")
	if !ok {
		t.Fatal("zai missing from the registry")
	}
	if e.Mode != free.ModelsMode || !e.FromCatalog || e.Data != free.Private {
		t.Fatalf("zai entry = %+v", e)
	}
	if e.Needs != "" || e.Setup != "" {
		t.Fatalf("zai entry = %+v", e)
	}
	if _, found := free.Excluded["modelscope"]; !found {
		t.Fatal("modelscope missing from Excluded")
	}
}
