package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/colorprofile"

	"github.com/silent9669/tsunagi/internal/art"
)

// Options configures a TUI session. Look and Theme are already resolved;
// Color/TrueColor/Anim/UTF8 come from terminal detection and user settings.
type Options struct {
	Look      art.Look
	Theme     string
	Color     bool // any color support at all
	TrueColor bool // 24-bit color, else 256
	Anim      bool // animate the art
	UTF8      bool // unicode art and glyphs allowed
	W, H      int  // terminal size in cells
	In        io.Reader
	Out       io.Writer
}

// Request carries one source's raw look/theme values; empty means unset.
type Request struct {
	Look  string
	Theme string
}

// DefaultOptions returns the baseline: braille look, colour theme, color and
// animation on, UTF-8 assumed.
func DefaultOptions() Options {
	return Options{
		Look:  art.Braille,
		Theme: "colour",
		Color: true,
		Anim:  true,
		UTF8:  true,
		W:     80,
		H:     24,
	}
}

// Resolve applies flag > env > config > default precedence to look and
// theme and validates the winning values. cfgAnim, when set to false, turns
// animation off; the base Options.Anim (which already folded in --no-anim)
// can never be turned back on by config.
func Resolve(flag, env, cfg Request, cfgAnim *bool, base Options) (Options, error) {
	o := base
	look, err := resolveLook(flag.Look, env.Look, cfg.Look)
	if err != nil {
		return o, err
	}
	o.Look = look
	theme, err := resolveTheme(flag.Theme, env.Theme, cfg.Theme)
	if err != nil {
		return o, err
	}
	if theme == "" {
		// unset everywhere: colour on a color terminal, mono without one
		// (NO_COLOR, --no-color and --plain all leave base.Color false)
		theme = "mono"
		if base.Color {
			theme = "colour"
		}
	}
	o.Theme = theme
	if cfgAnim != nil && !*cfgAnim {
		o.Anim = false
	}
	return o, nil
}

func resolveLook(vals ...string) (art.Look, error) {
	for _, v := range vals {
		if v == "" {
			continue
		}
		l, ok := art.ParseLook(v)
		if !ok {
			return "", fmt.Errorf("invalid look %q (want one of %s)", v, lookNames())
		}
		return l, nil
	}
	return art.Braille, nil
}

func resolveTheme(vals ...string) (string, error) {
	for _, v := range vals {
		if v == "" {
			continue
		}
		if !validTheme(v) {
			return "", fmt.Errorf("invalid theme %q (want one of %s)", v, strings.Join(ThemeNames, ", "))
		}
		return v, nil
	}
	return "", nil
}

func lookNames() string {
	var n []string
	for _, l := range art.Looks {
		n = append(n, string(l))
	}
	return strings.Join(n, ", ")
}

func validTheme(s string) bool {
	for _, n := range ThemeNames {
		if n == s {
			return true
		}
	}
	return false
}

// DetectUTF8 reports whether the locale env asks for UTF-8. LC_ALL wins over
// LC_CTYPE over LANG; with none set UTF-8 is assumed.
func DetectUTF8(getenv func(string) string) bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := getenv(k)
		if v == "" {
			continue
		}
		lv := strings.ToLower(v)
		return strings.Contains(lv, "utf-8") || strings.Contains(lv, "utf8")
	}
	return true
}

// Profile maps the color options onto a colorprofile for degradation.
func (o Options) Profile() colorprofile.Profile {
	if !o.Color {
		return colorprofile.ASCII
	}
	if o.TrueColor {
		return colorprofile.TrueColor
	}
	return colorprofile.ANSI256
}

// ThemeOr builds the resolved theme for these options.
func (o Options) ThemeOr() (Theme, error) {
	name := o.Theme
	if name == "" {
		name = "mono"
	}
	return NewTheme(name, o.Profile())
}

// EnvSources reads TSUNAGI_LOOK / TSUNAGI_THEME from the environment.
func EnvSources(getenv func(string) string) Request {
	return Request{Look: getenv("TSUNAGI_LOOK"), Theme: getenv("TSUNAGI_THEME")}
}
