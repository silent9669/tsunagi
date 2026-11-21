package app

import (
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/plans"
	"github.com/silent9669/tsunagi/internal/tui"
)

// homeScreen is the command palette: an always-focused search box over the
// actions, providers and settings shortcuts, plus the smart next step.
type homeScreen struct {
	opts tui.Options
	th   tui.Theme
	app  *App
	w, h int
	in   tui.Input
	list tui.List
	mtop int // list top row in body coordinates, for mouse clicks
}

// homeRow is one palette candidate.
type homeRow struct {
	label      string
	value      string
	note       string
	noteCol    bool // note is the left-aligned limits column, not a right tag
	noteAccent bool
	badges     []string
	glyph      string
	match      string // extra text the query also scores against
	disabled   bool
}

func newHomeScreen(a *App) *homeScreen {
	s := &homeScreen{opts: a.opts, th: a.theme, app: a}
	ph := "Search actions, providers..."
	if a.opts.UTF8 {
		ph = "Search actions, providers…"
	}
	s.in = tui.NewInput(ph, false, s.th, a.opts.UTF8)
	s.list = tui.NewList(s.th, a.opts.UTF8)
	f := a.frame(s)
	s.w, s.h = f.bodyW, f.bodyH
	s.size()
	s.rebuild()
	return s
}

func (s *homeScreen) setSnap(snap Snapshot)     { s.app.snap = snap }
func (s *homeScreen) setLocals(map[string]bool) {}

func (s *homeScreen) size() {
	fieldRows := s.fieldRows()
	lh := max(s.h-fieldRows, 1)
	s.in = fit(s.in, s.w-6, 1).(tui.Input)
	s.list = fit(s.list, s.w, lh).(tui.List)
}

func (s *homeScreen) fieldRows() int {
	if tui.CardsOK(s.w, s.h) {
		return 4 // boxed field + gap
	}
	return 2
}

// actionRows are the rows listed on an empty query, in order.
func (s *homeScreen) actionRows() []homeRow {
	a := s.app
	claude := "not found"
	if a.snap.ClaudePath != "" {
		claude = filepath.Base(a.snap.ClaudePath)
	}
	ready := 0
	for _, p := range a.snap.Providers {
		if p.Ready {
			ready++
		}
	}
	freeTag := "-"
	if n := len(a.snap.FreeTargets); n > 0 {
		freeTag = itoa(n) + " ready"
	}
	skillTag := "-"
	if n := installedTargets(a.snap); n > 0 {
		skillTag = itoa(n) + "/2"
	}
	rows := []homeRow{
		{label: "Start Claude Code", value: "start", note: claude, disabled: a.snap.ClaudePath == ""},
	}
	if next := a.nextStep(); next != nil {
		rows = append(rows, *next)
	}
	rows = append(rows,
		homeRow{label: "Providers", value: "providers", note: itoa(ready) + " ready"},
		homeRow{label: "Free models", value: "free", note: freeTag},
		homeRow{label: "Skills", value: "skills", note: skillTag},
		homeRow{label: "Settings", value: "settings"},
		homeRow{label: "Status", value: "status"},
		homeRow{label: "Quit", value: "quit"},
	)
	return rows
}

func installedTargets(snap Snapshot) int {
	full := map[string]int{}
	for _, e := range snap.Skills {
		if e.State == "installed" {
			full[e.Target]++
		}
	}
	n := 0
	for _, c := range full {
		if c == 4 {
			n++
		}
	}
	return n
}

// nextStep is the one smart suggestion row, first rule that applies wins.
func (a *App) nextStep() *homeRow {
	ready := false
	for _, p := range a.snap.Providers {
		if p.Ready && !p.Keyless && p.Group != "local" {
			ready = true
		}
	}
	if !ready {
		return &homeRow{label: "Add a provider", value: "next/free", note: "next", noteAccent: true}
	}
	if n := len(a.envKeys); n > 0 {
		return &homeRow{label: "Import " + itoa(n) + " keys from your environment", value: "next/env", note: "next", noteAccent: true}
	}
	freeReady := false
	for _, p := range a.snap.Providers {
		if p.Ready && p.Group == "free" {
			freeReady = true
		}
	}
	if freeReady && len(a.snap.FreeTargets) == 0 {
		return &homeRow{label: "Build the free/auto combo", value: "next/combo", note: "next", noteAccent: true}
	}
	if !anyDone(a.snap.Skills) {
		return &homeRow{label: "Install agent skills", value: "next/skills", note: "next", noteAccent: true}
	}
	return nil
}

// providerRows are the searchable provider candidates.
func (s *homeScreen) providerRows() []homeRow {
	check, open := "✓", "○"
	if !s.opts.UTF8 {
		check, open = "x", "o"
	}
	var rows []homeRow
	for _, p := range s.app.snap.Providers {
		disp := p.Name
		if e := registryEntry(p.Name); e != nil {
			disp = e.Name
		} else if e, ok := plans.Find(p.Name); ok {
			disp = e.Name
		}
		label := disp
		if !p.Ready && p.Group != "local" {
			label = "Set up " + disp
		}
		r := homeRow{
			label: label, value: "prov/" + p.Name,
			glyph: open, note: p.Limits, noteCol: true,
			match: p.Name + " " + disp,
		}
		if p.Ready {
			r.glyph = check
		}
		if p.FreeEntry {
			r.badges = append(r.badges, "free")
		} else if g := p.Group; g == "plan" || g == "paid" || g == "local" {
			r.badges = append(r.badges, g)
		}
		switch p.Tag {
		case "private", "trains", "unknown":
			r.badges = append(r.badges, p.Tag)
		}
		if p.Keyless {
			r.badges = append(r.badges, "keyless")
		}
		rows = append(rows, r)
	}
	return rows
}

// candidates is everything the query filters over, in stable order.
func (s *homeScreen) candidates() []homeRow {
	rows := s.actionRows()
	rows = append(rows, s.providerRows()...)
	rows = append(rows,
		homeRow{label: "Routing", value: "sub/routing"},
		homeRow{label: "Appearance", value: "sub/appearance"},
		homeRow{label: "Advanced", value: "sub/advanced"},
	)
	return rows
}

func (s *homeScreen) rebuild() {
	q := strings.TrimSpace(s.in.Value())
	var rows []homeRow
	if q == "" {
		rows = s.actionRows()
	} else {
		type scored struct {
			r     homeRow
			score int
		}
		var cand []scored
		for _, r := range s.candidates() {
			sc := tui.RankMatch(q, r.label)
			if m := tui.RankMatch(q, r.match); m > sc {
				sc = m
			}
			if sc > 0 {
				cand = append(cand, scored{r, sc})
			}
		}
		sort.SliceStable(cand, func(i, j int) bool { return cand[i].score > cand[j].score })
		for _, c := range cand {
			rows = append(rows, c.r)
		}
	}
	if q != "" && len(rows) == 0 {
		lr := []tui.ListRow{{Kind: tui.RowDim, Label: "No match"}}
		s.list.SetRows(lr)
		return
	}
	lr := make([]tui.ListRow, len(rows))
	for i, r := range rows {
		kind := tui.RowItem
		label := r.label
		if r.disabled {
			kind = tui.RowDim
			if r.note != "" {
				label += "  " + r.note
			}
		}
		lr[i] = tui.ListRow{Kind: kind, Label: label, Value: r.value, Note: r.note,
			NoteCol: r.noteCol, NoteAccent: r.noteAccent,
			Badges: r.badges, Glyph: r.glyph, Ref: i}
	}
	s.list.SetRows(lr)
}

func (s *homeScreen) activate(r tui.ListRow) (tea.Model, tea.Cmd) {
	v := r.Value
	switch {
	case strings.HasPrefix(v, "prov/"):
		name := strings.TrimPrefix(v, "prov/")
		return s, func() tea.Msg { return openProviderMsg{name: name} }
	case strings.HasPrefix(v, "sub/"):
		sub := strings.TrimPrefix(v, "sub/")
		return s, func() tea.Msg { return openSettingsMsg{sub: sub} }
	case v == "next/free":
		return s, func() tea.Msg { return openProviderMsg{chip: "free"} }
	case v == "next/env":
		return s, func() tea.Msg { return openProviderMsg{env: true} }
	case v == "next/combo":
		return s, func() tea.Msg { return menuPickMsg{value: "free"} }
	case v == "next/skills":
		return s, func() tea.Msg { return menuPickMsg{value: "skills"} }
	}
	return s, func() tea.Msg { return menuPickMsg{value: v} }
}

func (s *homeScreen) Init() tea.Cmd { return s.in.Init() }

func (s *homeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = v.Width, v.Height
		s.size()
		s.rebuild()
		return s, nil
	case tea.MouseClickMsg:
		mo := tea.Mouse(v)
		mo.Y -= s.mtop
		if mo.Y < 0 {
			return s, nil
		}
		nm, cmd := s.list.Update(tea.MouseClickMsg(mo))
		s.list = nm.(tui.List)
		return s, cmd
	case tea.MouseWheelMsg:
		nm, cmd := s.list.Update(msg)
		s.list = nm.(tui.List)
		return s, cmd
	case tui.Chosen:
		return s.activate(tui.ListRow{Value: v.Value, Ref: v.Ref})
	case tea.KeyPressMsg:
		switch {
		case v.Code == tea.KeyEscape:
			if s.in.Value() != "" {
				s.in.SetValue("")
				s.rebuild()
				s.list.Reset()
				return s, nil
			}
			return s, func() tea.Msg { return quitAppMsg{} }
		case v.Code == 'c' && v.Mod&tea.ModCtrl != 0:
			return s, func() tea.Msg { return quitAppMsg{} }
		case v.Code == tea.KeyEnter:
			nm, cmd := s.list.Update(msg)
			s.list = nm.(tui.List)
			return s, cmd
		case v.Code == tea.KeyUp || v.Code == tea.KeyDown ||
			v.Code == tea.KeyPgUp || v.Code == tea.KeyPgDown ||
			v.Code == tea.KeyHome || v.Code == tea.KeyEnd:
			nm, cmd := s.list.Update(msg)
			s.list = nm.(tui.List)
			return s, cmd
		}
	}
	nm, cmd := s.in.Update(msg)
	s.in = nm.(tui.Input)
	s.rebuild()
	s.list.Reset()
	return s, cmd
}

func (s *homeScreen) legend() []string {
	return []string{"type|to search", "↑↓|move", "↵|open", "esc|quit"}
}

func (s *homeScreen) crumbs() []string { return nil }

func (s *homeScreen) View() tea.View {
	var lines []string
	if tui.CardsOK(s.w, s.h) {
		lines = append(lines, s.in.Boxed(s.w)...)
	} else {
		lines = append(lines, s.in.View().Content)
	}
	lines = append(lines, "")
	s.mtop = len(lines)
	lines = append(lines, s.list.View().Content)
	return tea.NewView(strings.Join(lines, "\n"))
}
