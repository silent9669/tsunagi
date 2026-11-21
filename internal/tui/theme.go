// Package tui holds the Bubble Tea interface: themes, options resolution and
// the shared components the commands compose. It is the only package that
// imports Charm.
package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// ThemeNames lists every palette in display order.
var ThemeNames = []string{"mono", "amber", "matrix", "newsprint", "colour"}

type palette struct {
	fg, dim, accent, ok, warn, err string
	cyan, badgebg                  string    // free/keyless badges and their tint
	art                            [3]string // gradient stops, tail to head
	mono                           bool
}

var palettes = map[string]palette{
	"mono": {mono: true},
	"amber": {
		fg: "#FFB000", dim: "#8A5F00", accent: "#FFD060",
		ok: "#FFB000", warn: "#FF8C00", err: "#FF5A4D",
		cyan: "#FFD060", badgebg: "#2E2100",
		art: [3]string{"#5A3C00", "#FFB000", "#FFE08A"},
	},
	"matrix": {
		fg: "#00FF66", dim: "#0B6B34", accent: "#B5FFD0",
		ok: "#00FF66", warn: "#E6FF4D", err: "#FF4D6D",
		cyan: "#7DFCB0", badgebg: "#03200F",
		art: [3]string{"#04361A", "#00D95A", "#CFFFE0"},
	},
	"newsprint": {
		fg: "#D8D4C8", dim: "#8B877C", accent: "#FFFFFF",
		ok: "#BFD8B0", warn: "#E0C080", err: "#D9534F",
		cyan: "#FFFFFF", badgebg: "#33302A",
		art: [3]string{"#4A4840", "#D8D4C8", "#FFFFFF"},
	},
	"colour": {
		fg: "#DEE4F0", dim: "#6E7686", accent: "#8FA8E8",
		ok: "#5FC98F", warn: "#D9A84E", err: "#DD6873",
		cyan: "#5FC7D9", badgebg: "#1B2333",
		art: [3]string{"#141C33", "#3A5680", "#8FA8E8"},
	},
}

// Theme is a resolved palette: lipgloss styles for the roles the interface
// uses, plus the art gradient. Colors are already degraded to the terminal's
// color profile; mono has no colors at all, only attributes.
type Theme struct {
	Name     string
	prof     colorprofile.Profile
	pal      palette
	Text     lipgloss.Style
	Dim      lipgloss.Style
	Accent   lipgloss.Style
	OK       lipgloss.Style
	Warn     lipgloss.Style
	Err      lipgloss.Style
	Selected lipgloss.Style
	Title    lipgloss.Style
	Border   lipgloss.Style
}

// NewTheme builds the named palette for a color profile. An unknown name is
// an error listing the valid ones.
func NewTheme(name string, prof colorprofile.Profile) (Theme, error) {
	p, ok := palettes[name]
	if !ok {
		return Theme{}, fmt.Errorf("unknown theme %q (want one of %s)", name, strings.Join(ThemeNames, ", "))
	}
	t := Theme{Name: name, prof: prof, pal: p}
	fg := func(hex string) lipgloss.Style {
		s := lipgloss.NewStyle()
		if c := t.color(hex); c != nil {
			s = s.Foreground(c)
		}
		return s
	}
	if p.mono {
		if prof <= colorprofile.ASCII {
			// a colorless terminal gets plain text, no attributes either
			t.Text = lipgloss.NewStyle()
			t.Dim = lipgloss.NewStyle()
			t.Accent = lipgloss.NewStyle()
			t.OK = lipgloss.NewStyle()
			t.Warn = lipgloss.NewStyle()
			t.Err = lipgloss.NewStyle()
			t.Selected = lipgloss.NewStyle()
			t.Title = lipgloss.NewStyle()
			t.Border = lipgloss.NewStyle()
			return t, nil
		}
		t.Text = lipgloss.NewStyle()
		t.Dim = lipgloss.NewStyle().Faint(true)
		t.Accent = lipgloss.NewStyle().Bold(true)
		t.OK = lipgloss.NewStyle().Bold(true)
		t.Warn = lipgloss.NewStyle().Underline(true)
		t.Err = lipgloss.NewStyle().Reverse(true)
		t.Selected = lipgloss.NewStyle().Reverse(true)
		t.Title = lipgloss.NewStyle().Bold(true)
		t.Border = lipgloss.NewStyle().Faint(true)
		return t, nil
	}
	t.Text = fg(p.fg)
	t.Dim = fg(p.dim)
	t.Accent = fg(p.accent).Bold(true)
	t.OK = fg(p.ok)
	t.Warn = fg(p.warn)
	t.Err = fg(p.err)
	t.Selected = fg(p.accent).Bold(true)
	t.Title = fg(p.accent).Bold(true)
	t.Border = fg(p.dim)
	return t, nil
}

// Plain reports whether the theme carries no color: mono, or any theme on a
// colorless terminal. Primitives use it to pick bracket/ASCII forms.
func (t Theme) Plain() bool {
	return t.pal.mono || t.prof <= colorprofile.ASCII
}

// Badge renders a short status chip: " free " tinted by meaning in colour
// themes, "[free]" under mono and colorless terminals. Width stays <= 9.
func (t Theme) Badge(kind string) string {
	if t.Plain() {
		return "[" + kind + "]"
	}
	fg := t.pal.dim
	switch kind {
	case "private", "ready":
		fg = t.pal.ok
	case "trains":
		fg = t.pal.warn
	case "free", "keyless":
		fg = t.pal.cyan
		if fg == "" {
			fg = t.pal.accent
		}
	case "plan", "next":
		fg = t.pal.accent
	}
	st := lipgloss.NewStyle().Foreground(t.color(fg))
	if t.pal.badgebg != "" {
		st = st.Background(t.color(t.pal.badgebg))
	}
	return st.Render(" " + kind + " ")
}

// color converts a palette hex to the profile's color space. It returns nil
// when the profile has no color, so styles emit no color escapes at all.
func (t Theme) color(hex string) color.Color {
	if t.prof <= colorprofile.ASCII {
		return nil
	}
	return t.prof.Convert(hexRGB(hex))
}

// Gradient maps t in [0,1] along the art gradient, tail to head. It returns
// nil for mono and colorless profiles, meaning "terminal default".
func (t Theme) Gradient(tt float64) color.Color {
	if t.pal.mono || t.prof <= colorprofile.ASCII {
		return nil
	}
	if tt < 0 {
		tt = 0
	}
	if tt > 1 {
		tt = 1
	}
	// three-stop gradient: 0 -> 0.5 -> 1
	var a, b string
	var u float64
	if tt < 0.5 {
		a, b, u = t.pal.art[0], t.pal.art[1], tt*2
	} else {
		a, b, u = t.pal.art[1], t.pal.art[2], (tt-0.5)*2
	}
	ca, cb := hexRGB(a), hexRGB(b)
	r := uint8(float64(ca.R) + (float64(cb.R)-float64(ca.R))*u + 0.5)
	g := uint8(float64(ca.G) + (float64(cb.G)-float64(ca.G))*u + 0.5)
	bl := uint8(float64(ca.B) + (float64(cb.B)-float64(ca.B))*u + 0.5)
	return t.prof.Convert(color.RGBA{R: r, G: g, B: bl, A: 255})
}

// hexRGB parses "#RRGGBB"; malformed input yields black.
func hexRGB(s string) color.RGBA {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{A: 255}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{A: 255}
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}
