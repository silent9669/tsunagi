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
		{"bad strategy", `{"combos":{"c":{"targets":["a/b"],"strategy":"random"}}}`, `combo "c": strategy must be`},
		{"target no slash", `{"combos":{"c":{"targets":["ab"]}}}`, `target "ab" must be provider/model`},
		{"target empty model", `{"combos":{"c":{"targets":["a/"]}}}`, `target "a/" must be provider/model`},
		{"target empty provider", `{"combos":{"c":{"targets":["/b"]}}}`, `target "/b" must be provider/model`},
		{"claude target", `{"combos":{"c":{"targets":["claude-x/y"]}}}`, `target "claude-x/y" must not start with claude-`},
		{"nested combo", `{"combos":{"c":{"targets":["d/e"]},"d/e":{"targets":["a/b"]}}}`, `target "d/e" is itself a combo`},
		{"bad tier", `{"tiers":{"mega":"x"}}`, `tier "mega" must be opus, sonnet or haiku`},
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
