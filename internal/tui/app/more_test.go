package app

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
)

func moreBackend() *fakeBackend {
	snap := snapBase()
	snap.Providers = []ProviderRow{
		{Name: "openrouter", Group: "free", Tag: "trains", FreeEntry: true, Ready: true},
		{Name: "groq", Group: "free", Tag: "private", FreeEntry: true, Ready: true},
	}
	return &fakeBackend{
		snap:     snap,
		combo:    []string{"groq/llama", "openrouter/qwen:free"},
		probe:    []ProbeRow{{Target: "groq/llama", OK: true, Tools: true, Millis: 120}, {Target: "openrouter/x", Err: "HTTP 429"}},
		status:   []StatusRow{{Name: "claude", OK: true, Detail: "/usr/local/bin/claude"}, {Name: "accounts", Detail: "no keys yet"}},
		settings: SettingsView{Combos: []string{"free/auto", "work"}, Look: "braille", Theme: "mono", Anim: true},
	}
}

func TestMenuHasNewRows(t *testing.T) {
	m := newTestApp(t, moreBackend(), false)
	v := viewText(m)
	for _, want := range []string{"Free models", "Settings", "Status", "Quit"} {
		if !strings.Contains(v, want) {
			t.Fatalf("menu missing %q:\n%s", want, v)
		}
	}
	press(m, "7")
	if m.Action() != Quit {
		t.Fatalf("7 should be Quit, action = %v", m.Action())
	}
}

func TestStatusScreen(t *testing.T) {
	m := newTestApp(t, moreBackend(), false)
	press(m, "6")
	v := viewText(m)
	for _, want := range []string{"Status", "ok  claude", "!!  accounts", "no keys yet"} {
		if !strings.Contains(v, want) {
			t.Fatalf("status missing %q:\n%s", want, v)
		}
	}
	enter(m) // Back
	if !strings.Contains(viewText(m), "Start Claude Code") {
		t.Fatalf("Back did not return to the menu:\n%s", viewText(m))
	}
}

func TestFreeProbeAndCombo(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	press(m, "4")
	if !strings.Contains(viewText(m), "Build free/auto") {
		t.Fatalf("free menu:\n%s", viewText(m))
	}
	press(m, "1") // Probe
	v := viewText(m)
	for _, want := range []string{"ok", "groq/llama", "120ms", "fail", "HTTP 429"} {
		if !strings.Contains(v, want) {
			t.Fatalf("probe result missing %q:\n%s", want, v)
		}
	}
	enter(m) // Back to the free menu
	press(m, "2")
	v = viewText(m)
	if !strings.Contains(v, "groq/llama") || !strings.Contains(v, "openrouter/qwen:free") {
		t.Fatalf("combo targets missing:\n%s", v)
	}
	if len(be.allowTr) != 0 {
		t.Fatalf("no trains provider may be allowed without asking: %v", be.allowTr)
	}
}

func TestFreeDataPolicy(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	press(m, "4")
	press(m, "3") // Data policy
	v := viewText(m)
	if !strings.Contains(v, "openrouter") || strings.Contains(v, "groq") {
		t.Fatalf("policy must list only trains providers:\n%s", v)
	}
	space(m) // Pending
	space(m) // Done
	enter(m)
	if !be.allowTr["openrouter"] {
		t.Fatalf("openrouter not allowed: %v", be.allowTr)
	}
	if !strings.Contains(viewText(m), "1 allowed") {
		t.Fatalf("policy tag missing:\n%s", viewText(m))
	}
}

func TestSettingsRoutingCyclesTier(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	press(m, "5") // Settings
	press(m, "1") // Routing
	if !strings.Contains(viewText(m), "claude") {
		t.Fatalf("unset tier should read claude:\n%s", viewText(m))
	}
	press(m, "3") // Haiku
	if got := be.settings.Tiers["haiku"]; got != "free/auto" {
		t.Fatalf("haiku = %q, want free/auto (tiers %v)", got, be.tiers)
	}
	if !strings.Contains(viewText(m), "free/auto") {
		t.Fatalf("tier value not shown:\n%s", viewText(m))
	}
	press(m, "3")
	press(m, "3") // free/auto -> work -> back to claude
	if got := be.settings.Tiers["haiku"]; got != "" {
		t.Fatalf("haiku = %q after a full cycle, want cleared", got)
	}
}

func TestSettingsTryRoute(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	press(m, "5")
	press(m, "1")
	press(m, "4") // Try a route
	for _, r := range "free/auto" {
		press(m, string(r))
	}
	enter(m)
	if len(be.plans) != 1 || be.plans[0] != "free/auto" {
		t.Fatalf("plan not requested: %v", be.plans)
	}
	if !strings.Contains(viewText(m), "opencode/big-pickle") {
		t.Fatalf("plan not shown:\n%s", viewText(m))
	}
	enter(m) // Back to routing
	if !strings.Contains(viewText(m), "Try a route") {
		t.Fatalf("did not return to routing:\n%s", viewText(m))
	}
}

func TestSettingsAppearanceAppliesLive(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	press(m, "5")
	press(m, "2") // Appearance
	press(m, "1") // Look: braille -> dither
	if m.opts.Look != "dither" || be.settings.Look != "dither" {
		t.Fatalf("look = %q saved %q", m.opts.Look, be.settings.Look)
	}
	press(m, "2") // Theme: mono -> amber
	if m.opts.Theme != "amber" {
		t.Fatalf("theme = %q", m.opts.Theme)
	}
	press(m, "3") // Animation off
	if m.opts.Anim || be.settings.Anim {
		t.Fatal("animation should be off")
	}
	if !strings.Contains(viewText(m), "dither") || !strings.Contains(viewText(m), "amber") {
		t.Fatalf("values not shown:\n%s", viewText(m))
	}
}

func TestSettingsAdvanced(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	press(m, "5")
	press(m, "3") // Advanced
	press(m, "1") // Reset appearance
	if !be.resetUI || m.opts.Look != "braille" || m.opts.Theme != "mono" {
		t.Fatalf("reset: %v look=%q theme=%q", be.resetUI, m.opts.Look, m.opts.Theme)
	}
	press(m, "2") // Run setup again
	if !strings.Contains(viewText(m), "Set up tsunagi") {
		t.Fatalf("wizard did not open:\n%s", viewText(m))
	}
}

func TestMoreScreensFitSmallTerminals(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {60, 20}, {40, 12}} {
		for _, key := range []string{"4", "5", "6"} {
			opts := testOpts
			opts.W, opts.H = sz[0], sz[1]
			m, err := New(opts, moreBackend(), false)
			if err != nil {
				t.Fatal(err)
			}
			send(m, tea.WindowSizeMsg{Width: opts.W, Height: opts.H})
			press(m, key)
			for i, line := range strings.Split(viewText(m), "\n") {
				if n := len([]rune(line)); n > opts.W {
					t.Fatalf("%dx%d key %s line %d is %d wide: %q", opts.W, opts.H, key, i, n, line)
				}
			}
		}
	}
}
