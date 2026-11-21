package app

import (
	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/tui"
)

// Screen events the App routes on. Each screen emits only its own types.
type menuPickMsg struct{ value string }
type openProviderMsg struct {
	name string // open this provider's detail card
	chip string // preset filter chip ("free")
	env  bool   // open on the env-import section
}
type openSettingsMsg struct{ sub string } // routing|appearance|advanced
type providersDoneMsg struct{}
type providersBackMsg struct{}
type skillsDoneMsg struct{}
type skillsBackMsg struct{}
type wizPickMsg struct{ value string } // welcome: continue|skip; ready: start|menu

type wizSkipMsg struct{} // esc during a wizard step

// wizardWelcome is step 1: eel + one line + Continue/Skip.
type wizardWelcome struct {
	opts tui.Options
	th   tui.Theme
	app  *App
	menu tui.Menu
}

func newWizardWelcome(a *App) *wizardWelcome {
	th := a.theme
	items := []tui.Item{
		{Label: "Continue", Value: "continue"},
		{Label: "Skip setup", Value: "skip"},
	}
	return &wizardWelcome{opts: a.opts, th: th, app: a, menu: tui.NewMenu(items, 0, th, a.opts.UTF8)}
}

func (w *wizardWelcome) Init() tea.Cmd { return nil }

func (w *wizardWelcome) legend() []string {
	return []string{"↑↓|move", "↵|select", "esc|skip"}
}

func (w *wizardWelcome) crumbs() []string { return []string{"Setup"} }
func (w *wizardWelcome) wizStage() (int, bool) {
	return 0, true
}

func (r *wizardReady) legend() []string {
	return []string{"↑↓|move", "↵|select", "esc|skip"}
}

func (r *wizardReady) crumbs() []string { return []string{"Setup", "Ready"} }
func (r *wizardReady) wizStage() (int, bool) {
	return 2, true
}

func (w *wizardWelcome) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v, ok := msg.(tea.MouseClickMsg); ok {
		mo := tea.Mouse(v)
		if mo.Y -= 2; mo.Y < 0 {
			return w, nil
		}
		msg = tea.MouseClickMsg(mo)
	}
	nm, cmd := w.menu.Update(msg)
	w.menu = nm.(tui.Menu)
	switch msg.(type) {
	case tui.Chosen:
		return w, func() tea.Msg { return wizPickMsg{value: msg.(tui.Chosen).Value} }
	case tui.Quit:
		return w, func() tea.Msg { return wizSkipMsg{} }
	}
	return w, cmd
}

func (w *wizardWelcome) View() tea.View {
	return tea.NewView(w.th.Text.Render("Set up tsunagi") + "\n\n" + w.menu.View().Content)
}

// wizardReady is the last wizard step.
type wizardReady struct {
	opts tui.Options
	th   tui.Theme
	app  *App
	menu tui.Menu
}

func newWizardReady(a *App) *wizardReady {
	items := []tui.Item{
		{Label: "Start Claude Code", Value: "start", Disabled: a.snap.ClaudePath == ""},
		{Label: "Back to menu", Value: "menu"},
	}
	return &wizardReady{opts: a.opts, th: a.theme, app: a, menu: tui.NewMenu(items, 0, a.theme, a.opts.UTF8)}
}

func (r *wizardReady) Init() tea.Cmd { return nil }

func (r *wizardReady) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := r.menu.Update(msg)
	r.menu = nm.(tui.Menu)
	switch msg.(type) {
	case tui.Chosen:
		return r, func() tea.Msg { return wizPickMsg{value: msg.(tui.Chosen).Value} }
	case tui.Quit:
		return r, func() tea.Msg { return wizSkipMsg{} }
	}
	return r, cmd
}

func (r *wizardReady) View() tea.View {
	return tea.NewView(r.menu.View().Content)
}
