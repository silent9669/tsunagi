package app

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/tui"
)

// searchSnap is the realistic catalogue the matcher is exercised against:
// registry display names plus catalogue-only ids.
func searchSnap() Snapshot {
	s := snapBase()
	s.Providers = []ProviderRow{
		{Name: "groq", Group: "free", Tag: "private", FreeEntry: true},
		{Name: "opencode", Group: "paid"},
		{Name: "opencode-go", Group: "plan", Tag: "plan", Plan: &PlanInfo{Verified: true}},
		{Name: "openrouter", Group: "free", Tag: "trains", FreeEntry: true},
		{Name: "kilo", Group: "free", Tag: "trains", FreeEntry: true, Keyless: true},
		{Name: "zai", Group: "plan", Tag: "plan", Plan: &PlanInfo{Verified: true}},
		{Name: "zai-api", Group: "free", Tag: "private", FreeEntry: true},
		{Name: "google", Group: "free", Tag: "trains", FreeEntry: true},
		{Name: "together", Group: ""},
		{Name: "aigrow", Group: ""},
		{Name: "agentrouter", Group: ""},
		{Name: "superbot-gg", Group: ""},
		{Name: "frogbot", Group: ""},
		{Name: "zerogpu", Group: ""},
		{Name: "llmgateway", Group: ""},
	}
	return s
}

// homeLabels are the visible row labels of the filtered home list.
func homeLabels(t *testing.T, m *App) []string {
	t.Helper()
	hs, ok := m.scr.(*homeScreen)
	if !ok {
		t.Fatalf("not on home: %T", m.scr)
	}
	var labels []string
	for _, r := range hs.list.Rows {
		labels = append(labels, r.Label)
	}
	return labels
}

func TestSearchRealisticCatalogue(t *testing.T) {
	m := newTestApp(t, &fakeBackend{snap: searchSnap()}, false)
	typeIn(m, "gro")
	labels := homeLabels(t, m)
	if len(labels) == 0 || labels[0] != "Set up Groq" {
		t.Fatalf("gro must put Groq first: %v", labels)
	}
	for _, bad := range []string{"Set up together", "Set up agentrouter", "Set up superbot-gg", "Set up zerogpu", "Settings", "Routing", "Appearance"} {
		for _, l := range labels {
			if l == bad {
				t.Fatalf("gro must not match %q: %v", bad, labels)
			}
		}
	}
	esc(m) // clear

	typeIn(m, "o")
	labels = homeLabels(t, m)
	if len(labels) < 3 || labels[0] != "Set up opencode" || labels[1] != "Set up OpenCode Go" ||
		labels[2] != "Set up OpenRouter" {
		t.Fatalf("o must put the opencode rows and OpenRouter first: %v", labels)
	}
	for _, l := range labels {
		if l == "Settings" {
			t.Fatalf("o must not match Settings: %v", labels)
		}
	}
	esc(m)

	typeIn(m, "kilo")
	labels = homeLabels(t, m)
	if len(labels) != 1 || labels[0] != "Set up Kilo Gateway" {
		t.Fatalf("kilo must match Kilo Gateway only: %v", labels)
	}
	esc(m)

	typeIn(m, "open")
	labels = homeLabels(t, m)
	if len(labels) < 3 || labels[0] != "Set up opencode" || labels[1] != "Set up OpenCode Go" ||
		labels[2] != "Set up OpenRouter" {
		t.Fatalf("open must give opencode, OpenCode Go, OpenRouter: %v", labels)
	}
	esc(m)

	for q, want := range map[string]string{
		"set":  "Settings",
		"prov": "Providers",
		"free": "Free models",
	} {
		typeIn(m, q)
		labels = homeLabels(t, m)
		if len(labels) == 0 || labels[0] != want {
			t.Fatalf("%q must put %q first: %v", q, want, labels)
		}
		esc(m)
	}

	typeIn(m, "xyz")
	labels = homeLabels(t, m)
	if len(labels) != 1 || labels[0] != "No match" {
		t.Fatalf("xyz must show the empty state: %v", labels)
	}
}

func TestCursorResetsOnQueryAndChip(t *testing.T) {
	be := &fakeBackend{snap: searchSnap()}
	m := newTestApp(t, be, false)
	typeIn(m, "o")
	key(m, tea.KeyDown, "") // move off the first result
	typeIn(m, "p")          // now "op"
	hs := m.scr.(*homeScreen)
	if r, _ := hs.list.Selected(); r.Label != "Set up opencode" {
		t.Fatalf("cursor must reset to the first result on query change, got %q", r.Label)
	}
	esc(m)
	choose(m, "providers")
	typeIn(m, "o")
	key(m, tea.KeyDown, "")
	typeIn(m, "p")
	ps := m.scr.(*providersScreen)
	first := ""
	for _, r := range ps.list.Rows {
		if r.Kind == tui.RowItem {
			first = r.Value
			break
		}
	}
	if r, _ := ps.list.Selected(); r.Value != first {
		t.Fatalf("providers cursor must reset to the first result, got %q want %q", r.Value, first)
	}
	key(m, tea.KeyRight, "") // chip change also resets, to the new first row
	first = ""
	for _, r := range ps.list.Rows {
		if r.Kind == tui.RowItem {
			first = r.Value
			break
		}
	}
	if r, _ := ps.list.Selected(); r.Value != first {
		t.Fatalf("chip change must reset the cursor to %q, got %q", first, r.Value)
	}
}

func TestNoTabInAnyView(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	check := func(where string) {
		if strings.ContainsRune(viewText(m), '\t') {
			t.Fatalf("tab in %s view:\n%s", where, viewText(m))
		}
	}
	check("home")
	for _, nav := range []string{"providers", "skills", "free", "settings", "status"} {
		esc(m)
		choose(m, nav)
		check(nav)
	}
}

func TestDetailCardWrapsValues(t *testing.T) {
	be := &fakeBackend{snap: searchSnap()}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	typeIn(m, "kilo")
	enter(m) // detail card
	v := viewText(m)
	// kilo's limits and data note are long: both must render in full over
	// several lines, not truncate to a few characters
	for _, want := range []string{"limits", "200 requests/hour", "data", "routed to providers", "source", "checked 2026-11-21"} {
		if !strings.Contains(v, want) {
			t.Fatalf("detail card missing %q:\n%s", want, v)
		}
	}
}

func TestScrollIndicatorsAndEnd(t *testing.T) {
	be := moreBackend()
	for i := 0; i < 40; i++ {
		be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "cat" + itoa(i), Group: "paid"})
	}
	opts := testOpts
	opts.W, opts.H = 80, 20
	m, _ := New(opts, be, false)
	send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	choose(m, "providers")
	key(m, tea.KeyEnd, "")
	v := viewText(m)
	if !strings.Contains(v, "more") || !strings.Contains(v, "❯") {
		t.Fatalf("scrolled list must show indicators and keep the cursor:\n%s", v)
	}
	if !strings.Contains(v, "Back to menu") {
		t.Fatalf("End must reach the last row:\n%s", v)
	}
}

func TestEarlyKeystrokesReachTheSearchBox(t *testing.T) {
	opts := testOpts
	opts.Anim = true
	be := &fakeBackend{snap: snapBase()}
	m, err := New(opts, be, false)
	if err != nil {
		t.Fatal(err)
	}
	// no intro screen: the very first keystrokes land in the search box
	typeIn(m, "gro")
	hs, ok := m.scr.(*homeScreen)
	if !ok {
		t.Fatalf("first screen must be home, got %T", m.scr)
	}
	if hs.in.Value() != "gro" {
		t.Fatalf("keystrokes were swallowed: box = %q", hs.in.Value())
	}
}
