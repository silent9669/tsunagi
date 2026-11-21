package main

import (
	"os"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/tui/app"
	"github.com/silent9669/tsunagi/internal/ui"
)

// appOptions resolves look/theme/anim the same way `tsunagi look` does and
// fills in terminal size and streams for the app.
func appOptions(t *ui.Term) (tui.Options, error) {
	cfg, err := config.Load()
	if err != nil {
		return tui.Options{}, err
	}
	flag := tui.Request{Look: lookFlagLook, Theme: lookFlagTheme}
	env := tui.EnvSources(os.Getenv)
	cfgReq := tui.Request{Look: cfg.UI.Look, Theme: cfg.UI.Theme}
	base := tui.Options{
		Color:     t.Color,
		TrueColor: t.True,
		Anim:      t.Anim,
		UTF8:      tui.DetectUTF8(os.Getenv),
		W:         t.W,
		H:         t.H,
		In:        os.Stdin,
		Out:       t.Out,
	}
	return tui.Resolve(flag, env, cfgReq, cfg.UI.Anim, base)
}

// needsWizard reports whether the app should open the first-run wizard:
// not onboarded and nothing set up yet.
func needsWizard(snap app.Snapshot) bool {
	if snap.Onboarded {
		return false
	}
	for _, p := range snap.Providers {
		if p.Ready && p.Group != "local" {
			return false
		}
	}
	for _, s := range snap.Skills {
		if s.State == "installed" {
			return false
		}
	}
	return true
}

// cmdApp runs the interactive shell. forceWizard opens it straight into the
// first-run wizard (`tsunagi setup`).
func cmdApp(t *ui.Term, forceWizard bool) int {
	opts, err := appOptions(t)
	if err != nil {
		return fail(t, "%v", err)
	}
	be := backend{}
	first := forceWizard || needsWizard(be.Snapshot())
	m, err := app.New(opts, be, first)
	if err != nil {
		return fail(t, "%v", err)
	}
	if _, err := tui.Run(m, opts); err != nil {
		return fail(t, "%v", err)
	}
	if m.Action() == app.Start {
		return launch(t, nil, false)
	}
	return 0
}

// cmdSetup opens the app into the first-run wizard.
func cmdSetup(t *ui.Term, args []string) int {
	if len(args) > 0 {
		return fail(t, "usage: tsunagi setup")
	}
	if !t.TTY {
		return fail(t, "tsunagi setup needs a terminal")
	}
	return cmdApp(t, true)
}
