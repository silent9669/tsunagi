package tui

import (
	"fmt"
	"os"

	"charm.land/bubbletea/v2"
)

// Run executes a Bubble Tea program on the alternate screen and returns the
// final model. The terminal is restored on every exit path: normal quit,
// ctrl+c, and panics inside the model (a panic is returned as an error after
// the program's own shutdown ran).
func Run(m tea.Model, o Options) (final tea.Model, err error) {
	in := o.In
	if in == nil {
		in = os.Stdin
	}
	out := o.Out
	if out == nil {
		out = os.Stdout
	}
	opts := []tea.ProgramOption{
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithColorProfile(o.Profile()),
	}
	if o.W > 0 && o.H > 0 {
		opts = append(opts, tea.WithWindowSize(o.W, o.H))
	}
	p := tea.NewProgram(altScreen{inner: m}, opts...)
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// p.Run's deferred shutdown already restored the terminal
				// while the panic unwound; just report it.
				final, err = nil, fmt.Errorf("tui: %v", r)
			}
			close(done)
		}()
		final, err = p.Run()
	}()
	<-done
	return final, err
}

// altScreen wraps a model so its view requests the alternate screen buffer.
type altScreen struct{ inner tea.Model }

func (a altScreen) Init() tea.Cmd { return a.inner.Init() }

func (a altScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := a.inner.Update(msg)
	a.inner = nm
	return a, cmd
}

func (a altScreen) View() tea.View {
	v := a.inner.View()
	v.AltScreen = true
	if os.Getenv("TSUNAGI_NO_MOUSE") == "" {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
