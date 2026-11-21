package app

import (
	"strings"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/tui"
)

// skillsScreen installs or removes the skill set per target.
type skillsScreen struct {
	opts   tui.Options
	th     tui.Theme
	app    *App
	wizard bool
	w, h   int

	stage int
	cl    tui.Checklist
	menu  tui.Menu
	was   map[string]bool // targets that were fully installed at build time
	rows  []skills.Result
}

const (
	skList = iota
	skDone
)

func newSkillsScreen(a *App, wizard bool) *skillsScreen {
	s := &skillsScreen{opts: a.opts, th: a.theme, app: a, wizard: wizard, w: blockWidth(a.opts.W), h: max(4, a.opts.H-2)}
	s.build()
	return s
}

func (s *skillsScreen) setSnap(snap Snapshot)     { s.app.snap = snap }
func (s *skillsScreen) setLocals(map[string]bool) {}

// targetState derives Done/None/tag for one install target.
func (s *skillsScreen) targetState(id string) (tui.State, string) {
	inst, total := 0, 0
	modified := false
	for _, e := range s.app.snap.Skills {
		if e.Target != id {
			continue
		}
		total++
		switch e.State {
		case "installed":
			inst++
		case "modified", "foreign", "outdated":
			modified = true
		}
	}
	tag := "-"
	if total > 0 {
		tag = itoa(inst) + "/" + itoa(total)
	}
	if inst == total && total > 0 {
		return tui.Done, tag
	}
	if modified {
		return tui.Pending, tag // partially there: selectable for repair
	}
	return tui.None, tag
}

func (s *skillsScreen) build() {
	s.was = map[string]bool{}
	var items []tui.CItem
	for _, tg := range skills.Targets() {
		st, tag := s.targetState(tg.ID)
		if st == tui.Done {
			s.was[tg.ID] = true
		}
		// preselect both when nothing is installed anywhere
		items = append(items, tui.CItem{Label: tg.Name, Value: tg.ID, Tag: tag, State: st})
	}
	if len(s.app.snap.Skills) == 0 || !anyDone(s.app.snap.Skills) {
		for i := range items {
			items[i].State = tui.Pending
		}
	}
	s.cl = fit(tui.NewChecklist([]tui.Section{{Title: "", Items: items}}, s.th, s.opts.UTF8), s.w, s.h).(tui.Checklist)
}

func anyDone(entries []skills.StatusEntry) bool {
	for _, e := range entries {
		if e.State == "installed" {
			return true
		}
	}
	return false
}

func (s *skillsScreen) Init() tea.Cmd {
	if s.stage == skList {
		return s.cl.Init()
	}
	return nil
}

func (s *skillsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch s.stage {
	case skList:
		nm, cmd := s.cl.Update(msg)
		s.cl = nm.(tui.Checklist)
		switch v := msg.(type) {
		case tea.WindowSizeMsg:
			s.w, s.h = v.Width, v.Height
		case tui.Confirmed:
			return s, s.apply(v.Values)
		case tui.Quit:
			if s.wizard {
				return s, func() tea.Msg { return wizSkipMsg{} }
			}
			return s, func() tea.Msg { return skillsBackMsg{} }
		}
		return s, cmd
	default: // skDone
		nm, cmd := s.menu.Update(msg)
		s.menu = nm.(tui.Menu)
		switch v := msg.(type) {
		case tea.WindowSizeMsg:
			s.w, s.h = v.Width, v.Height
		case tui.Chosen:
			s.stage = skList
			s.app.refresh()
			s.build()
			return s, func() tea.Msg {
				if s.wizard {
					return skillsDoneMsg{}
				}
				return skillsBackMsg{}
			}
		case tui.Quit:
			s.stage = skList
			s.build()
		}
		return s, cmd
	}
}

// apply installs newly-selected targets and removes deselected ones.
func (s *skillsScreen) apply(values []string) tea.Cmd {
	selected := map[string]bool{}
	for _, v := range values {
		selected[v] = true
	}
	var install, remove []string
	for _, tg := range skills.Targets() {
		switch {
		case selected[tg.ID] && !s.was[tg.ID]:
			install = append(install, tg.ID)
		case !selected[tg.ID] && s.was[tg.ID]:
			remove = append(remove, tg.ID)
		}
	}
	var rows []skills.Result
	if len(install) > 0 {
		rows = append(rows, s.app.be.InstallSkills(install)...)
	}
	if len(remove) > 0 {
		rows = append(rows, s.app.be.RemoveSkills(remove)...)
	}
	s.rows = rows
	if len(rows) == 0 {
		// nothing changed: still count as done for the wizard
		if s.wizard {
			return func() tea.Msg { return skillsDoneMsg{} }
		}
		return func() tea.Msg { return skillsBackMsg{} }
	}
	s.menu = s.simpleMenu("Done")
	s.stage = skDone
	return nil
}

func (s *skillsScreen) simpleMenu(labels ...string) tui.Menu {
	items := make([]tui.Item, len(labels))
	for i, l := range labels {
		items[i] = tui.Item{Label: l, Value: l}
	}
	return fit(tui.NewMenu(items, 0, s.th, s.opts.UTF8), s.w, s.h).(tui.Menu)
}

func (s *skillsScreen) title() string { return "Skills" }

func (s *skillsScreen) View() tea.View {
	var b strings.Builder
	if s.stage == skDone {
		for _, r := range s.rows {
			line := r.Target + "/" + r.Skill + "  " + r.State
			if r.Reason != "" {
				line += " (" + r.Reason + ")"
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + s.menu.View().Content)
		return tea.NewView(b.String())
	}
	b.WriteString(s.cl.View().Content)
	return tea.NewView(b.String())
}
