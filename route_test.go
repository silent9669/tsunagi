package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureRun runs a command with stdout redirected to a pipe and returns its
// exit code and output.
func captureRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := run(args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return code, string(out)
}

func TestRouteCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := filepath.Join(home, ".config", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// only Local providers, so no key lookup ever reaches the keychain
	cfg := `{
		"combos": {"free/auto": {"targets": ["ollama/llama3", "nope/x", "lmstudio/qwen"], "strategy": "priority"}},
		"tiers": {"haiku": "free/auto"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out := captureRun(t, "route", "free/auto")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	for _, want := range []string{"free/auto", "ollama/llama3", "ready", "nope/x", "unknown provider", "lmstudio/qwen"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}

	code, out = captureRun(t, "route", "claude-haiku-5-5")
	if code != 0 || !strings.Contains(out, "tier haiku -> free/auto") || !strings.Contains(out, "ollama/llama3") {
		t.Fatalf("tier route code %d out %s", code, out)
	}

	code, out = captureRun(t, "route", "--json", "--tools", "free/auto")
	if code != 0 || !strings.Contains(out, `"combo": "free/auto"`) || !strings.Contains(out, `"status": "ready"`) {
		t.Fatalf("json route code %d out %s", code, out)
	}

	code, out = captureRun(t, "route", "claude-sonnet-5-5")
	if code != 0 || !strings.Contains(out, "claude") {
		t.Fatalf("claude route code %d out %s", code, out)
	}
}

func TestRouteScenarioFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := filepath.Join(home, ".config", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := `{
		"scenarios": {"long_context": "ollama/big", "web_search": "ollama/web", "think": "ollama/brain"},
		"scenario_tokens": 100
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// all three scenario signals set: long_context wins the precedence
	code, out := captureRun(t, "route", "--think", "--web-search", "--tokens", "150", "--json", "claude-sonnet-5-5")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out, `"scenario long_context`) || !strings.Contains(out, `"resolved": "ollama/big"`) {
		t.Fatalf("scenario route out %s", out)
	}

	// under the token threshold web_search beats think
	code, out = captureRun(t, "route", "--think", "--web-search", "claude-sonnet-5-5")
	if code != 0 || !strings.Contains(out, "scenario web_search -> ollama/web") {
		t.Fatalf("web route code %d out %s", code, out)
	}

	// think alone fires too; no flags stays on Claude
	code, out = captureRun(t, "route", "--think", "claude-sonnet-5-5")
	if code != 0 || !strings.Contains(out, "scenario think -> ollama/brain") {
		t.Fatalf("think route code %d out %s", code, out)
	}
	code, out = captureRun(t, "route", "claude-sonnet-5-5")
	if code != 0 || strings.Contains(out, "scenario") {
		t.Fatalf("plain route code %d out %s", code, out)
	}
}
