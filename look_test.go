package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
)

// lookHome points HOME at a temp dir so config and cache stay isolated, and
// resets the global --look/--theme flag vars between tests.
func lookHome(t *testing.T) string {
	t.Helper()
	lookFlagLook, lookFlagTheme = "", ""
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return home
}

func writeConfig(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLookStaticPlain(t *testing.T) {
	lookHome(t)
	t.Setenv("LC_ALL", "C") // force the ASCII fallback path
	code, out := captureRun(t, "--plain", "look", "--static")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out, "braille") || !strings.Contains(out, "mono") {
		t.Fatalf("status line missing:\n%s", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("plain output contains escapes:\n%q", out)
	}
}

func TestLookStaticFlags(t *testing.T) {
	lookHome(t)
	code, out := captureRun(t, "--plain", "look", "--static", "--style", "dither", "--theme", "amber")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out, "dither amber") {
		t.Fatalf("out lacks selection:\n%s", out)
	}
	// --look is an alias for --style
	code, out = captureRun(t, "--plain", "look", "--static", "--look", "gradient")
	if code != 0 || !strings.Contains(out, "gradient") {
		t.Fatalf("--look alias failed: %d\n%s", code, out)
	}
}

func TestLookGlobalFlags(t *testing.T) {
	lookHome(t)
	code, out := captureRun(t, "--plain", "--look", "bricks", "--theme", "matrix", "look", "--static")
	if code != 0 {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out, "bricks matrix") {
		t.Fatalf("global flags not applied:\n%s", out)
	}
}

func TestLookInvalid(t *testing.T) {
	lookHome(t)
	code, out := captureRun(t, "--plain", "look", "--static", "--style", "bogus")
	if code == 0 || !strings.Contains(out, "invalid look") {
		t.Fatalf("invalid look: code=%d out=%s", code, out)
	}
	code, out = captureRun(t, "--plain", "look", "--static", "--theme", "bogus")
	if code == 0 || !strings.Contains(out, "invalid theme") {
		t.Fatalf("invalid theme: code=%d out=%s", code, out)
	}
}

func TestLookEnvAndConfig(t *testing.T) {
	home := lookHome(t)
	t.Setenv("TSUNAGI_LOOK", "halftone")
	t.Setenv("TSUNAGI_THEME", "newsprint")
	code, out := captureRun(t, "--plain", "look", "--static")
	if code != 0 || !strings.Contains(out, "halftone newsprint") {
		t.Fatalf("env not applied: %d\n%s", code, out)
	}
	// config beats env? no: env > config. config alone:
	t.Setenv("TSUNAGI_LOOK", "")
	t.Setenv("TSUNAGI_THEME", "")
	writeConfig(t, home, `{"ui":{"look":"bricks","theme":"amber"}}`)
	code, out = captureRun(t, "--plain", "look", "--static")
	if code != 0 || !strings.Contains(out, "bricks amber") {
		t.Fatalf("config ui not applied: %d\n%s", code, out)
	}
	// flag beats config
	code, out = captureRun(t, "--plain", "look", "--static", "--style", "gradient")
	if code != 0 || !strings.Contains(out, "gradient amber") {
		t.Fatalf("flag>config failed: %d\n%s", code, out)
	}
}

func TestLookNoTTY(t *testing.T) {
	lookHome(t)
	code, out := captureRun(t, "look")
	if code == 0 {
		t.Fatalf("interactive look without TTY should fail, out=%s", out)
	}
}

func TestConfigUIValidation(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, ".config", "tsunagi", "config.json")
	load := func() (config.Config, error) { return config.LoadFrom(cfgPath) }
	writeConfig(t, home, `{"ui":{"look":"bogus"}}`)
	if _, err := load(); err == nil {
		t.Fatal("invalid ui.look should fail config load")
	}
	writeConfig(t, home, `{"ui":{"theme":"bogus"}}`)
	if _, err := load(); err == nil {
		t.Fatal("invalid ui.theme should fail config load")
	}
	writeConfig(t, home, `{"ui":{"look":"dither","theme":"mono","anim":false}}`)
	cfg, err := load()
	if err != nil {
		t.Fatalf("valid ui config: %v", err)
	}
	if cfg.UI.Look != "dither" || cfg.UI.Theme != "mono" || cfg.UI.Anim == nil || *cfg.UI.Anim {
		t.Fatalf("ui config not parsed: %+v", cfg.UI)
	}
}
