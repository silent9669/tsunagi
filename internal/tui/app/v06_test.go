package app

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/tui"
)

// --- home: search ranking, next-step rules, esc ------------------------------

func TestHomeFilterRanksActionsProvidersSettings(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	hs := m.scr.(*homeScreen)
	sel := func() string {
		r, ok := hs.list.Selected()
		if !ok {
			return ""
		}
		return r.Value
	}
	typeIn(m, "prov")
	if got := sel(); got != "providers" {
		t.Fatalf("\"prov\" picked %q, want providers", got)
	}
	esc(m)
	typeIn(m, "groq")
	if got := sel(); got != "prov/groq" {
		t.Fatalf("\"groq\" picked %q, want prov/groq", got)
	}
	esc(m)
	typeIn(m, "rout")
	if got := sel(); got != "sub/routing" {
		t.Fatalf("\"rout\" picked %q, want sub/routing", got)
	}
	esc(m)
	typeIn(m, "adv")
	if got := sel(); got != "sub/advanced" {
		t.Fatalf("\"adv\" picked %q, want sub/advanced", got)
	}
}

func TestHomeNextStepRules(t *testing.T) {
	snap := func() Snapshot {
		s := snapBase()
		s.Providers = nil
		s.FreeTargets = nil
		s.Skills = nil
		return s
	}
	cases := []struct {
		name  string
		setup func(*fakeBackend)
		want  string // "" means no next-step row
	}{
		{"only local ready", func(be *fakeBackend) {
			be.snap.Providers = []ProviderRow{{Name: "ollama", Group: "local", Ready: true}}
		}, "next/free"},
		{"keyless alone does not count", func(be *fakeBackend) {
			be.snap.Providers = []ProviderRow{{Name: "kilo", Group: "free", Keyless: true}}
		}, "next/free"},
		{"env keys offered", func(be *fakeBackend) {
			be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Ready: true}}
			be.snap.FreeTargets = []string{"groq/m"}
			be.envKeys = []EnvKey{{Provider: "groq", Var: "GROQ_API_KEY"}}
		}, "next/env"},
		{"free ready, no combo", func(be *fakeBackend) {
			be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Ready: true}}
		}, "next/combo"},
		{"skills missing", func(be *fakeBackend) {
			be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Ready: true}}
			be.snap.FreeTargets = []string{"groq/m"}
			be.snap.Skills = snapBase().Skills
		}, "next/skills"},
		{"all done", func(be *fakeBackend) {
			be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Ready: true}}
			be.snap.FreeTargets = []string{"groq/m"}
			be.snap.Skills = []skills.StatusEntry{{Target: "claude", Skill: "tsunagi-free", State: "installed"}}
		}, ""},
	}
	for _, c := range cases {
		s := snap()
		be := &fakeBackend{snap: s}
		c.setup(be)
		m := newTestApp(t, be, false)
		next := m.nextStep()
		got := ""
		if next != nil {
			got = next.value
		}
		if got != c.want {
			t.Fatalf("%s: next = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestHomeEscClearsQueryThenQuits(t *testing.T) {
	m := newTestApp(t, moreBackend(), false)
	typeIn(m, "zzz")
	esc(m)
	if strings.Contains(viewText(m), "zzz") {
		t.Fatalf("esc did not clear the query:\n%s", viewText(m))
	}
	if m.Action() == Quit {
		t.Fatalf("esc with a query must not quit")
	}
	esc(m)
	if m.Action() != Quit {
		t.Fatalf("esc on an empty query should quit, action = %v", m.Action())
	}
}

// --- providers: chips, sections, flat ranking --------------------------------

func provItems(t *testing.T, m *App) []string {
	t.Helper()
	ps, ok := m.scr.(*providersScreen)
	if !ok {
		t.Fatalf("not on providers: %T", m.scr)
	}
	var vals []string
	for _, r := range ps.list.Rows {
		if r.Kind == tui.RowItem {
			vals = append(vals, r.Value)
		}
	}
	return vals
}

func TestProvidersDefaultChipAndCycling(t *testing.T) {
	// nothing non-local ready -> the Free chip is the default
	snap := snapBase()
	snap.Providers = []ProviderRow{
		{Name: "groq", Group: "free", Tag: "private", FreeEntry: true},
		{Name: "chatgpt", Group: "plan", Tag: "plan"},
	}
	m := newTestApp(t, &fakeBackend{snap: snap}, false)
	choose(m, "providers")
	v := viewText(m)
	if strings.Contains(v, "chatgpt") || !strings.Contains(v, "Groq") {
		t.Fatalf("default chip should be Free:\n%s", v)
	}
	key(m, tea.KeyLeft, "") // Free -> All
	v = viewText(m)
	if !strings.Contains(v, "chatgpt") {
		t.Fatalf("cycling to All must show chatgpt:\n%s", v)
	}
}

func TestProvidersSectionsAndStableOrder(t *testing.T) {
	m := newTestApp(t, &fakeBackend{snap: snapBase()}, false)
	choose(m, "providers")
	items := provItems(t, m)
	// sections: Ready(anthropic) then Plans(chatgpt) then Free(openrouter,
	// groq) then Local(ollama); action rows last.
	want := []string{"anthropic", "chatgpt", "openrouter", "groq", "ollama",
		"@add-gateway", "@import", "@back"}
	if len(items) != len(want) {
		t.Fatalf("rows = %v, want %v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("row %d = %q, want %q (all %v)", i, items[i], want[i], items)
		}
	}
}

func TestProvidersFlatRankingReadyFirst(t *testing.T) {
	snap := snapBase()
	snap.Providers = []ProviderRow{
		{Name: "zeta", Group: "paid"},
		{Name: "groq", Group: "free", Ready: true, Tag: "private"},
		{Name: "openrouter", Group: "free", Tag: "private"},
	}
	m := newTestApp(t, &fakeBackend{snap: snap}, false)
	choose(m, "providers")
	typeIn(m, "o") // substring of groq and openrouter, flat list
	items := provItems(t, m)
	if len(items) < 3 || items[0] != "groq" {
		t.Fatalf("flat ranking must put ready first: %v", items)
	}
}

// --- env import + paste-a-key ------------------------------------------------

func TestEnvImportCardShowsVarNameOnly(t *testing.T) {
	be := moreBackend()
	be.envKeys = []EnvKey{{Provider: "groq", Var: "GROQ_API_KEY"}}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	v := viewText(m)
	if !strings.Contains(v, "Found in your environment") || !strings.Contains(v, "$GROQ_API_KEY") {
		t.Fatalf("env section missing:\n%s", v)
	}
	enter(m) // open the import card
	v = viewText(m)
	if !strings.Contains(v, "$GROQ_API_KEY") || !strings.Contains(v, "Import") {
		t.Fatalf("import card missing:\n%s", v)
	}
	press(m, "1") // Import -> ImportEnvKey -> verify
	if len(be.imported) != 1 || be.imported[0] != "groq" {
		t.Fatalf("ImportEnvKey not called: %v", be.imported)
	}
}

func TestEnvImportSkip(t *testing.T) {
	be := moreBackend()
	be.envKeys = []EnvKey{{Provider: "groq", Var: "GROQ_API_KEY"}}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	enter(m)      // env card
	press(m, "2") // Skip
	v := viewText(m)
	if strings.Contains(v, "GROQ_API_KEY") {
		t.Fatalf("a skipped env key must hide for the session:\n%s", v)
	}
	if len(be.imported) != 0 {
		t.Fatalf("skip must not import: %v", be.imported)
	}
}

func paste(m *App, key string) { send(m, tea.PasteMsg{Content: key}) }

func TestPasteKeySingleCandidate(t *testing.T) {
	filler := "gsk_" + strings.Repeat("x", 36)
	be := moreBackend()
	be.detected = []string{"groq"}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	paste(m, filler)
	v := viewText(m)
	if !strings.Contains(v, "Looks like Groq") {
		t.Fatalf("candidate row missing:\n%s", v)
	}
	if strings.Contains(v, strings.Repeat("x", 36)) {
		t.Fatalf("the pasted key must render masked:\n%s", v)
	}
	if !strings.Contains(v, "gsk_") {
		t.Fatalf("the masked field should keep the prefix:\n%s", v)
	}
	enter(m) // use this key -> SaveKey + verify
	ps := m.scr.(*providersScreen)
	if ps.pendingKey != "" {
		t.Fatal("the held key must be cleared after SaveKey")
	}
	if strings.Contains(viewText(m), strings.Repeat("x", 36)) {
		t.Fatalf("key must never render unmasked:\n%s", viewText(m))
	}
}

func TestPasteKeyManyCandidates(t *testing.T) {
	be := moreBackend()
	be.detected = []string{"groq", "openrouter"}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	paste(m, "sk-test-"+strings.Repeat("y", 32))
	v := viewText(m)
	if !strings.Contains(v, "Looks like Groq") || !strings.Contains(v, "Looks like OpenRouter") {
		t.Fatalf("both candidates should be offered:\n%s", v)
	}
	if strings.Contains(v, strings.Repeat("y", 32)) {
		t.Fatalf("key rendered unmasked:\n%s", v)
	}
}

func TestPasteKeyClaudeRefusal(t *testing.T) {
	be := moreBackend()
	be.detectErr = errors.New("Claude uses your own login, not a key")
	m := newTestApp(t, be, false)
	choose(m, "providers")
	paste(m, "sk-ant-"+strings.Repeat("z", 36))
	v := viewText(m)
	if !strings.Contains(v, "Claude uses your own login") {
		t.Fatalf("claude refusal row missing:\n%s", v)
	}
	if strings.Contains(v, strings.Repeat("z", 36)) {
		t.Fatalf("key rendered unmasked:\n%s", v)
	}
	enter(m) // the refusal row is not selectable: nothing may be stored
	ps := m.scr.(*providersScreen)
	if ps.stage != psList {
		t.Fatalf("refusal row must not act, stage = %v", ps.stage)
	}
}

func TestPasteKeyNoCandidateAsks(t *testing.T) {
	filler := strings.Repeat("k", 40)
	be := moreBackend()
	m := newTestApp(t, be, false)
	choose(m, "providers")
	paste(m, filler)
	v := viewText(m)
	if !strings.Contains(v, "Which provider is this key for?") {
		t.Fatalf("the ask row missing:\n%s", v)
	}
	if strings.Contains(v, filler) {
		t.Fatalf("key rendered unmasked:\n%s", v)
	}
	typeIn(m, "groq")
	enter(m) // choose groq -> SaveKey + verify, key wiped
	ps := m.scr.(*providersScreen)
	if ps.pendingKey != "" {
		t.Fatal("the held key must be cleared after SaveKey")
	}
}

func TestTypedKeyDetectsAndMasks(t *testing.T) {
	filler := "gsk_" + strings.Repeat("x", 36)
	be := moreBackend()
	be.detected = []string{"groq"}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	typeIn(m, filler)
	v := viewText(m)
	if strings.Contains(v, strings.Repeat("x", 36)) {
		t.Fatalf("typed key rendered unmasked:\n%s", v)
	}
	if !strings.Contains(v, "Looks like Groq") {
		t.Fatalf("candidate row missing:\n%s", v)
	}
}

// --- keyless + rail ----------------------------------------------------------

func TestKeylessDetailActions(t *testing.T) {
	snap := snapBase()
	snap.Providers = []ProviderRow{{Name: "kilo", Group: "free", Tag: "trains", FreeEntry: true, Keyless: true}}
	m := newTestApp(t, &fakeBackend{snap: snap}, false)
	choose(m, "kilo")
	v := viewText(m)
	if !strings.Contains(v, "Use now, no key needed") || !strings.Contains(v, "Add a key (optional)") {
		t.Fatalf("keyless actions missing:\n%s", v)
	}
	if strings.Contains(v, "Replace key") || strings.Contains(v, "Remove") {
		t.Fatalf("a keyless provider without a key must not show key management:\n%s", v)
	}
	enter(m) // Use now -> verify with no key -> result
	if !strings.Contains(viewText(m), "ok") {
		t.Fatalf("keyless verify should run:\n%s", viewText(m))
	}
}

func TestRailShowsDoneCurrentPending(t *testing.T) {
	be := moreBackend()
	be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Tag: "private", FreeEntry: true}}
	opts := testOpts
	opts.W, opts.H = 100, 24
	m, _ := New(opts, be, false)
	send(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	choose(m, "groq")
	press(m, "1") // Set up -> Account opened -> Key
	v := viewText(m)
	if !strings.Contains(v, "◆") || !strings.Contains(v, "○") {
		t.Fatalf("rail must show current and pending glyphs:\n%s", v)
	}
	typeIn(m, "testkey12345")
	enter(m) // verify ok -> result rail: earlier steps done
	v = viewText(m)
	if !strings.Contains(v, "◇") || !strings.Contains(v, "ok groq/m 12ms") {
		t.Fatalf("rail must mark done steps with the result:\n%s", v)
	}
}

// --- fallbacks ---------------------------------------------------------------

func TestASCIIFallbackNoGlyphs(t *testing.T) {
	opts := testOpts
	opts.UTF8 = false
	m, err := New(opts, moreBackend(), false)
	if err != nil {
		t.Fatal(err)
	}
	send(m, tea.WindowSizeMsg{Width: opts.W, Height: opts.H})
	bad := []string{"❯", "◆", "◇", "○", "✓", "╭", "│", "›", "•", "·", "↑", "↓", "↵", "←", "→", "…"}
	for _, g := range bad {
		if strings.Contains(viewText(m), g) {
			t.Fatalf("non-ascii glyph %q on an ascii terminal:\n%s", g, viewText(m))
		}
	}
	choose(m, "providers")
	v := viewText(m)
	if !strings.Contains(v, ">") {
		t.Fatalf("ascii cursor missing:\n%s", v)
	}
	for _, g := range bad {
		if strings.Contains(v, g) {
			t.Fatalf("non-ascii glyph %q on an ascii terminal:\n%s", g, v)
		}
	}
}

func TestNoColorForcesMono(t *testing.T) {
	opts := testOpts
	opts.Color = false
	m, err := New(opts, moreBackend(), false)
	if err != nil {
		t.Fatal(err)
	}
	send(m, tea.WindowSizeMsg{Width: opts.W, Height: opts.H})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, "38;") || strings.Contains(line, "48;") {
			t.Fatalf("NO_COLOR emitted a color escape: %q", line)
		}
	}
}
