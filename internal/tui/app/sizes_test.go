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
	// keys are typed into the always-focused search boxes; \r picks the top row
	screens := map[string][]string{
		"home":       {},
		"providers":  {"p", "r", "o", "v", "\r"},
		"detail":     {"p", "r", "o", "v", "\r", "g", "r", "o", "\r"},
		"rail":       {"p", "r", "o", "v", "\r", "g", "r", "o", "\r", "1"},
		"envcard":    {"p", "r", "o", "v", "\r", "\r"},
		"skills":     {"s", "k", "i", "\r"},
		"free":       {"f", "r", "e", "\r"},
		"settings":   {"s", "e", "t", "\r"},
		"routing":    {"s", "e", "t", "\r", "1"},
		"status":     {"s", "t", "a", "t", "u", "\r"},
		"scrolled":   {"p", "r", "o", "v", "\r", "END"},
		"plans":      {"p", "r", "o", "v", "\r", "RIGHT", "RIGHT"},
		"plandetail": {"p", "r", "o", "v", "\r", "a", "l", "i", "\r"},
	}
	for name, keys := range screens {
		for _, W := range widths {
			for _, H := range heights {
				opts := testOpts
				opts.W, opts.H = W, H
				be := moreBackend()
				if name == "envcard" {
					be.envKeys = []EnvKey{{Provider: "groq", Var: "GROQ_API_KEY"}}
				}
				if name == "scrolled" {
					for i := 0; i < 40; i++ {
						be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "prov" + itoa(i), Group: "paid"})
					}
				}
				if name == "plans" || name == "plandetail" {
					be.snap.Providers = planSnap().Providers
				}
				m, err := New(opts, be, false)
				if err != nil {
					t.Fatal(err)
				}
				send(m, tea.WindowSizeMsg{Width: W, Height: H})
				for _, k := range keys {
					switch {
					case k == "\r":
						enter(m)
					case k == "END":
						key(m, tea.KeyEnd, "")
					case k == "RIGHT":
						key(m, tea.KeyRight, "")
					case k >= "0" && k <= "9":
						press(m, k)
					default:
						typeIn(m, k)
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

func TestListKeepsTheCursorVisible(t *testing.T) {
	be := moreBackend()
	for i := 0; i < 60; i++ {
		be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "prov" + itoa(i), Group: "paid"})
	}
	opts := testOpts
	opts.W, opts.H = 60, 14
	m, _ := New(opts, be, false)
	send(m, tea.WindowSizeMsg{Width: 60, Height: 14})
	choose(m, "providers")
	for i := 0; i < 40; i++ {
		key(m, tea.KeyDown, "")
	}
	v := viewText(m)
	if !strings.Contains(v, "❯") {
		t.Fatalf("the cursor row scrolled out of view:\n%s", v)
	}
	typeIn(m, "prov")
	key(m, tea.KeyEnd, "")
	if !strings.Contains(viewText(m), "prov59") {
		t.Fatalf("End did not reach the last row:\n%s", viewText(m))
	}
}

func TestSearchFindsCatalogueProviders(t *testing.T) {
	be := moreBackend()
	for i := 0; i < 10; i++ {
		be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "cat" + itoa(i), Group: ""})
	}
	be.snap.Providers = append(be.snap.Providers, ProviderRow{Name: "hyperbolic", Group: ""})
	m := newTestApp(t, be, false)
	choose(m, "providers")
	v := viewText(m)
	if strings.Contains(v, "hyperbolic") || !strings.Contains(v, "3 more") {
		t.Fatalf("catalogue providers must stay behind the search:\n%s", v)
	}
	typeIn(m, "hyper")
	v = viewText(m)
	if !strings.Contains(v, "hyperbolic") || strings.Contains(v, "cat0") {
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
	hs := m.scr.(*homeScreen)
	_ = viewText(m) // render once so mtop is set
	// home rows: Start, next-step, Providers, Free models, Skills, Settings, Status
	send(m, tea.MouseClickMsg{X: f.indent + 3, Y: f.headerRows + hs.mtop + 6, Button: tea.MouseLeft})
	if !strings.Contains(viewText(m), "no keys yet") {
		t.Fatalf("a click on Status did not open it:\n%s", viewText(m))
	}
	enter(m) // back
	send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	enter(m)
	if !strings.Contains(viewText(m), "Providers") || !strings.Contains(viewText(m), "OpenRouter") {
		// two wheel steps move the cursor from Start to Providers
		t.Fatalf("wheel + enter did not open Providers:\n%s", viewText(m))
	}
}

func TestRailAdaptsToSize(t *testing.T) {
	be := moreBackend()
	be.snap.Providers = []ProviderRow{{Name: "groq", Group: "free", Tag: "private", FreeEntry: true}}
	for _, w := range []int{100, 40} {
		opts := testOpts
		opts.W, opts.H = w, 24
		m, _ := New(opts, be, false)
		send(m, tea.WindowSizeMsg{Width: w, Height: 24})
		choose(m, "groq")
		press(m, "1") // Set up
		v := viewText(m)
		if w == 100 {
			for _, want := range []string{"Account", "Key", "Verify", "Done", "Set up"} {
				if !strings.Contains(v, want) {
					t.Fatalf("rail missing %q at %d:\n%s", want, w, v)
				}
			}
		}
		if w == 40 && !strings.Contains(v, "2/4") {
			t.Fatalf("compact stepper missing at %d:\n%s", w, v)
		}
	}
}
