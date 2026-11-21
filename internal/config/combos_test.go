package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCombosAndTiersLoad(t *testing.T) {
	p := writeConfig(t, `{
		"default": "opencode",
		"combos": {"free/auto": {"targets": ["opencode/big-pickle", "openrouter/qwen/qwen3-coder:free@work"], "strategy": "fastest"}},
		"tiers": {"haiku": "free/auto", "opus": "claude-opus-4-5"}
	}`)
	cfg, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := cfg.Combos["free/auto"]
	if !ok || len(c.Targets) != 2 || c.Strategy != "fastest" {
		t.Fatalf("combo = %+v", c)
	}
	if cfg.Tiers["haiku"] != "free/auto" || cfg.Tiers["opus"] != "claude-opus-4-5" {
		t.Fatalf("tiers = %+v", cfg.Tiers)
	}
}

func TestComboValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want string
	}{
		{"empty name", `{"combos":{"":{"targets":["a/b"]}}}`, "combo name must not be empty"},
		{"claude name", `{"combos":{"claude-fast":{"targets":["a/b"]}}}`, `combo "claude-fast" must not start with claude-`},
		{"at name", `{"combos":{"a@b":{"targets":["a/b"]}}}`, `combo "a@b" must not contain @`},
		{"no targets", `{"combos":{"c":{"targets":[]}}}`, `combo "c" needs at least one target`},
		{"bad strategy", `{"combos":{"c":{"targets":["a/b"],"strategy":"shuffle"}}}`, `combo "c": strategy must be`},
		{"target no slash", `{"combos":{"c":{"targets":["ab"]}}}`, `target "ab" must be provider/model`},
		{"target empty model", `{"combos":{"c":{"targets":["a/"]}}}`, `target "a/" must be provider/model`},
		{"target empty provider", `{"combos":{"c":{"targets":["/b"]}}}`, `target "/b" must be provider/model`},
		{"claude target", `{"combos":{"c":{"targets":["claude-x/y"]}}}`, `target "claude-x/y" must not start with claude-`},
		{"nested combo", `{"combos":{"c":{"targets":["d/e"]},"d/e":{"targets":["a/b"]}}}`, `target "d/e" is itself a combo`},
		{"bad tier", `{"tiers":{"mega":"x"}}`, `tier "mega" must be opus, sonnet or haiku`},
		{"weights wrong strategy", `{"combos":{"c":{"targets":["a/b"],"weights":{"a/b":2}}}}`, `combo "c": weights need strategy weighted`},
		{"weight not a target", `{"combos":{"c":{"targets":["a/b"],"strategy":"weighted","weights":{"a/c":2}}}}`, `weight for "a/c" which is not a target`},
		{"weight too small", `{"combos":{"c":{"targets":["a/b"],"strategy":"weighted","weights":{"a/b":0}}}}`, `weight for "a/b" must be at least 1`},
		{"bad scenario name", `{"scenarios":{"coding":"a/b"}}`, `scenario "coding" must be think, long_context or web_search`},
		{"empty scenario", `{"scenarios":{"think":""}}`, `scenario "think" needs a target`},
		{"claude scenario target", `{"scenarios":{"think":"claude-opus-4-5"}}`, `scenarios.think`},
		{"bad scenario target", `{"scenarios":{"think":"noslash"}}`, `scenarios.think`},
		{"negative scenario_tokens", `{"scenario_tokens":-5}`, "scenario_tokens must not be negative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := writeConfig(t, tc.json)
			_, err := LoadFrom(p)
			if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want path and %q", err, tc.want)
			}
		})
	}
}

func TestScenariosAndWeightsLoad(t *testing.T) {
	p := writeConfig(t, `{
		"combos": {
			"free/auto": {"targets": ["a/b", "c/d"], "strategy": "weighted", "weights": {"a/b": 3}},
			"big": {"targets": ["e/f"], "strategy": "headroom"}
		},
		"scenarios": {"think": "free/auto", "long_context": "e/f", "web_search": "big"},
		"scenario_tokens": 120000
	}`)
	cfg, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Combos["free/auto"]
	if c.Strategy != "weighted" || c.Weights["a/b"] != 3 {
		t.Fatalf("combo = %+v", c)
	}
	if cfg.Combos["big"].Strategy != "headroom" {
		t.Fatalf("big = %+v", cfg.Combos["big"])
	}
	want := map[string]string{"think": "free/auto", "long_context": "e/f", "web_search": "big"}
	for k, v := range want {
		if cfg.Scenarios[k] != v {
			t.Fatalf("scenarios = %+v", cfg.Scenarios)
		}
	}
	if cfg.ScenarioTokens != 120000 {
		t.Fatalf("scenario_tokens = %d", cfg.ScenarioTokens)
	}
}

func TestFreeConfigAndComboAuto(t *testing.T) {
	p := writeConfig(t, `{
		"combos": {"free/auto": {"targets": ["a/b"], "auto": true}},
		"free": {"allow_trains": ["openrouter", "google"]}
	}`)
	cfg, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Combos["free/auto"].Auto {
		t.Fatal("auto flag not parsed")
	}
	if len(cfg.Free.AllowTrains) != 2 || cfg.Free.AllowTrains[0] != "openrouter" {
		t.Fatalf("allow_trains = %+v", cfg.Free.AllowTrains)
	}

	// an invalid provider name in allow_trains is rejected
	p = writeConfig(t, `{"free":{"allow_trains":["Bad Name!"]}}`)
	if _, err := LoadFrom(p); err == nil || !strings.Contains(err.Error(), "free.allow_trains") {
		t.Fatalf("err = %v", err)
	}
}

func TestSetKeyPreservesOtherKeys(t *testing.T) {
	p := writeConfig(t, `{
		"default": "opencode",
		"models": {"fast": "opencode/x"},
		"combos": {"free/auto": {"targets": ["a/b"]}},
		"custom": {"keep": [1, 2, 3]}
	}`)
	if err := SetKey(p, "default", "deepseek"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{`"models"`, `"combos"`, `"custom"`, `"deepseek"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("file lacks %s:\n%s", want, b)
		}
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", st.Mode().Perm())
	}
	cfg, err := LoadFrom(p)
	if err != nil || cfg.Default != "deepseek" {
		t.Fatalf("reload = %+v %v", cfg.Default, err)
	}
	if _, ok := cfg.Combos["free/auto"]; !ok {
		t.Fatal("combos lost on SetKey")
	}

	// a nil value removes the key, a missing file is created
	if err := SetKey(p, "custom", nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), `"custom"`) {
		t.Fatalf("custom not removed:\n%s", b)
	}
	p2 := filepath.Join(t.TempDir(), "new", "config.json")
	if err := SetKey(p2, "default", "zai"); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p2); st.Mode().Perm() != 0o600 {
		t.Fatalf("new file mode = %o", st.Mode().Perm())
	}
}

func TestClaudeFallbackValidation(t *testing.T) {
	good := map[string]string{
		"combo":  `{"combos": {"free/auto": {"targets": ["groq/x"]}}, "claude_fallback": "free/auto"}`,
		"target": `{"claude_fallback": "groq/llama-3.3-70b"}`,
		"off":    `{}`,
	}
	for name, body := range good {
		cfg, err := LoadFrom(writeConfig(t, body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if name == "combo" && cfg.ClaudeFallback != "free/auto" {
			t.Fatalf("ClaudeFallback = %q", cfg.ClaudeFallback)
		}
	}
	for name, body := range map[string]string{
		"bare name":    `{"claude_fallback": "free"}`,
		"claude model": `{"claude_fallback": "claude-haiku-5-5"}`,
	} {
		if _, err := LoadFrom(writeConfig(t, body)); err == nil || !strings.Contains(err.Error(), "claude_fallback") {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}
