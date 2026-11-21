package main

import (
	"fmt"
	"os"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/ui"
)

// lookFlagLook and lookFlagTheme hold the global --look/--theme values.
var lookFlagLook, lookFlagTheme string

// cmdLook previews the interface art style and theme. --static prints one
// frame without needing a terminal; the interactive mode cycles looks with
// left/right and themes with up/down.
func cmdLook(t *ui.Term, args []string) int {
	var styleFlag, themeFlag string
	static := false
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--static":
			static = true
		case "--style", "--look", "--theme":
			if i+1 >= len(args) {
				return fail(t, "flag %s needs a value", a)
			}
			i++
			if a == "--theme" {
				themeFlag = args[i]
			} else {
				styleFlag = args[i]
			}
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 0 {
		return fail(t, "usage: tsunagi look [--style <look>] [--theme <theme>] [--static]")
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	flag := tui.Request{
		Look:  firstNonEmpty(styleFlag, lookFlagLook),
		Theme: firstNonEmpty(themeFlag, lookFlagTheme),
	}
	env := tui.EnvSources(os.Getenv)
	cfgReq := tui.Request{Look: cfg.UI.Look, Theme: cfg.UI.Theme}
	base := tui.Options{
		Color:     t.Color,
		TrueColor: t.True,
		Anim:      t.Anim,
		UTF8:      tui.DetectUTF8(os.Getenv),
		W:         t.W,
		H:         t.H,
	}
	opts, err := tui.Resolve(flag, env, cfgReq, cfg.UI.Anim, base)
	if err != nil {
		return fail(t, "%v", err)
	}
	th, err := opts.ThemeOr()
	if err != nil {
		return fail(t, "%v", err)
	}
	if static {
		fmt.Print(tui.PreviewFrame(opts, th, 0.25))
		fmt.Println()
		return 0
	}
	if !t.TTY {
		return fail(t, "tsunagi look needs a terminal; use --static for a single frame")
	}
	m, err := tui.NewPreview(opts)
	if err != nil {
		return fail(t, "%v", err)
	}
	if _, err := tui.Run(m, opts); err != nil {
		return fail(t, "%v", err)
	}
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
