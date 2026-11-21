package app

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// every screen must stay inside any terminal: no row wider than W, no more
// than H rows, and the cursor row must still be on screen.
func TestEveryScreenFitsEverySize(t *testing.T) {
	widths := []int{20, 24, 30, 40, 50, 60, 80, 100, 140}
	heights := []int{5, 6, 8, 10, 12, 16, 20, 24, 30, 40, 50}
	screens := map[string][]string{
		"menu":      {},
		"providers": {"2"},
		"flow":      {"2", "g", "r", "o", "\r"},
		"skills":    {"3"},
		"free":      {"4"},
		"settings":  {"5"},
		"routing":   {"5", "1"},
		"status":    {"6"},
	}
	for name, keys := range screens {
		for _, W := range widths {
			for _, H := range heights {
				opts := testOpts
				opts.W, opts.H = W, H
				be := moreBackend()
				m, err := New(opts, be, false)
				if err != nil {
					t.Fatal(err)
				}
				send(m, tea.WindowSizeMsg{Width: W, Height: H})
				for _, k := range keys {
					if k == "\r" {
						enter(m)
					} else {
						press(m, k)
					}
				}
				out := m.View().Content
				lines := strings.Split(out, "\n")
				if len(lines) > H {
					t.Fatalf("%s %dx%d: %d rows", name, W, H, len(lines))
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w > W {
						t.Fatalf("%s %dx%d: row %d is %d wide: %q", name, W, H, i, w, stripANSI(l))
					}
				}
			}
		}
	}
}

func TestPickerKeepsTheCursorVisible(t *testing.T) {
	be := moreBackend()
	for i := 0; i < 60; i++ {
		be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "prov" + itoa(i), Group: "paid"})
	}
	opts := testOpts
	opts.W, opts.H = 60, 14
	m, _ := New(opts, be, false)
	send(m, tea.WindowSizeMsg{Width: 60, Height: 14})
	press(m, "2")
	for i := 0; i < 40; i++ {
		key(m, tea.KeyDown, "")
	}
	v := viewText(m)
	if !strings.Contains(v, "▏") {
		t.Fatalf("the cursor row scrolled out of view:\n%s", v)
	}
	key(m, tea.KeyEnd, "")
	if !strings.Contains(viewText(m), "prov59") {
		t.Fatalf("End did not reach the last row:\n%s", viewText(m))
	}
}

func TestSearchFindsCatalogueProviders(t *testing.T) {
	be := moreBackend()
	be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "hyperbolic", Group: ""}, ProviderRow{Name: "novita-ai", Group: ""})
	m := newTestApp(t, be, false)
	press(m, "2")
	v := viewText(m)
	if strings.Contains(v, "hyperbolic") || !strings.Contains(v, "+2 more") {
		t.Fatalf("catalogue providers must stay behind the search:\n%s", v)
	}
	for _, r := range "hyper" {
		press(m, string(r))
	}
	v = viewText(m)
	if !strings.Contains(v, "hyperbolic") || strings.Contains(v, "novita") {
		t.Fatalf("search must filter:\n%s", v)
	}
	esc(m) // clears the query first
	if strings.Contains(viewText(m), "hyperbolic") || !strings.Contains(viewText(m), "Providers") {
		t.Fatalf("esc must clear the search, not leave:\n%s", viewText(m))
	}
}

func TestMouseClickOpensARowAndWheelMoves(t *testing.T) {
	be := moreBackend()
	m := newTestApp(t, be, false)
	f := m.frame(m.scr)
	// click on "Status" (row 6 of the menu)
	send(m, tea.MouseClickMsg{X: f.indent + 3, Y: f.headerRows + 5, Button: tea.MouseLeft})
	if !strings.Contains(viewText(m), "no keys yet") {
		t.Fatalf("a click on Status did not open it:\n%s", viewText(m))
	}
	enter(m) // back
	send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	enter(m)
	if !strings.Contains(viewText(m), "Codex, Cursor, opencode") {
		// two wheel steps move the cursor from Start Claude Code to Skills
		t.Fatalf("wheel + enter did not open Skills:\n%s", viewText(m))
	}
}

func TestStepperAdaptsToWidth(t *testing.T) {
	be := moreBackend()
	be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Tag: "private", FreeEntry: true}}
	for _, w := range []int{100, 40} {
		opts := testOpts
		opts.W, opts.H = w, 24
		m, _ := New(opts, be, false)
		send(m, tea.WindowSizeMsg{Width: w, Height: 24})
		press(m, "2")
		enter(m)
		v := viewText(m)
		if w == 100 && !strings.Contains(v, "Account") || !strings.Contains(v, "Key") && w == 100 {
			t.Fatalf("full stepper missing at %d:\n%s", w, v)
		}
		if w == 40 && !strings.Contains(v, "1/4") {
			t.Fatalf("compact stepper missing at %d:\n%s", w, v)
		}
	}
}
