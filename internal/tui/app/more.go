package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/art"
	"github.com/silent9669/tsunagi/internal/tui"
)

// screenBackMsg returns to the main menu from Free, Settings or Status.
type screenBackMsg struct{}

// rerunSetupMsg reopens the first-run wizard from Settings.
type rerunSetupMsg struct{}

// base holds what every screen below needs: theme, sizes and a way to build
// menus and inputs that already fit the content area.
type base struct {
	a    *App
	w, h int
	mtop int // body rows above the menu, set by View, for mouse clicks
}

// mouse shifts a click from body rows to the menu's rows; clicks above the
// menu are dropped.
func (b *base) mouse(msg tea.Msg) (tea.Msg, bool) {
	if v, ok := msg.(tea.MouseClickMsg); ok {
		mo := tea.Mouse(v)
		mo.Y -= b.mtop
		if mo.Y < 0 {
			return nil, false
		}
		return tea.MouseClickMsg(mo), true
	}
	return msg, true
}

func (b *base) menu(items []tui.Item, initial int) tui.Menu {
	return fit(tui.NewMenu(items, initial, b.a.theme, b.a.opts.UTF8), b.w, b.h).(tui.Menu)
}

func (b *base) input(placeholder string) tui.Input {
	return fit(tui.NewInput(placeholder, false, b.a.theme, b.a.opts.UTF8), b.w, b.h).(tui.Input)
}

func (b *base) list(sections []tui.Section) tui.Checklist {
	return fit(tui.NewChecklist(sections, b.a.theme, b.a.opts.UTF8), b.w, b.h).(tui.Checklist)
}

func labels(ls ...string) []tui.Item {
	items := make([]tui.Item, len(ls))
	for i, l := range ls {
		items[i] = tui.Item{Label: l, Value: l}
	}
	return items
}

// clip cuts a row to the block width so nothing wraps.
func (b *base) clip(s string) string { return truncate(s, b.w-1) }

func truncate(s string, w int) string {
	if w < 2 || tui.WidthOf(s) <= w {
		return s
	}
	var out strings.Builder
	n := 0
	for _, r := range s {
		rw := tui.WidthOf(string(r))
		if n+rw > w-1 {
			break
		}
		out.WriteRune(r)
		n += rw
	}
	return out.String() + "…"
}

func back() tea.Cmd { return func() tea.Msg { return screenBackMsg{} } }

func (b *base) sizeMsg(msg tea.Msg) {
	if v, ok := msg.(tea.WindowSizeMsg); ok {
		b.w, b.h = v.Width, v.Height
	}
}

// --- Free models ------------------------------------------------------------

const (
	fsMenu = iota
	fsBusy
	fsProbed
	fsCombo
	fsPolicy
)

type probeDoneMsg struct {
	rows []ProbeRow
	err  error
}

type comboBuiltMsg struct {
	targets []string
	err     error
}

type freeScreen struct {
	base
	stage   int
	busy    string
	menu    tui.Menu
	cl      tui.Checklist
	rows    []ProbeRow
	targets []string
	err     string
	was     map[string]bool
}

func newFreeScreen(a *App) *freeScreen {
	s := &freeScreen{base: base{a: a, w: blockWidth(a.opts.W), h: max(4, a.opts.H-2)}}
	s.toMenu(0)
	return s
}

func (s *freeScreen) title() string { return "Free models" }

func (s *freeScreen) setSnap(snap Snapshot)     { s.a.snap = snap }
func (s *freeScreen) setLocals(map[string]bool) {}

func (s *freeScreen) toMenu(initial int) {
	allowed := 0
	for _, ok := range s.a.snap.AllowTrains {
		if ok {
			allowed++
		}
	}
	policy := "-"
	if allowed > 0 {
		policy = itoa(allowed) + " allowed"
	}
	combo := "-"
	if n := len(s.a.snap.FreeTargets); n > 0 {
		combo = itoa(n) + " targets"
	}
	s.stage = fsMenu
	s.menu = s.menu2([]tui.Item{
		{Label: "Probe", Value: "probe"},
		{Label: "Build free/auto", Value: "combo", Tag: combo},
		{Label: "Data policy", Value: "policy", Tag: policy},
		{Label: "Back", Value: "back"},
	}, initial)
}

func (s *freeScreen) menu2(items []tui.Item, initial int) tui.Menu {
	return s.base.menu(items, initial)
}

func (s *freeScreen) Init() tea.Cmd { return nil }

func (s *freeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.sizeMsg(msg)
	msg, ok := s.mouse(msg)
	if !ok {
		return s, nil
	}
	switch v := msg.(type) {
	case probeDoneMsg:
		s.rows, s.err = v.rows, ""
		if v.err != nil {
			s.err = v.err.Error()
		}
		s.stage = fsProbed
		s.menu = s.base.menu(labels("Back"), 0)
		return s, nil
	case comboBuiltMsg:
		s.targets, s.err = v.targets, ""
		if v.err != nil {
			s.err = v.err.Error()
		}
		s.a.refresh()
		s.stage = fsCombo
		s.menu = s.base.menu(labels("Back"), 0)
		return s, nil
	}
	switch s.stage {
	case fsMenu:
		nm, cmd := s.menu.Update(msg)
		s.menu = nm.(tui.Menu)
		switch v := msg.(type) {
		case tui.Chosen:
			return s, s.pick(v.Value)
		case tui.Quit:
			return s, back()
		}
		return s, cmd
	case fsBusy:
		if k, ok := msg.(tea.KeyPressMsg); ok && (k.Code == tea.KeyEscape || k.Code == 'c' && k.Mod&tea.ModCtrl != 0) {
			s.toMenu(0)
		}
		return s, nil
	case fsPolicy:
		nm, cmd := s.cl.Update(msg)
		s.cl = nm.(tui.Checklist)
		switch v := msg.(type) {
		case tui.Confirmed:
			s.applyPolicy(v.Values)
			s.toMenu(2)
			return s, nil
		case tui.Quit:
			s.toMenu(2)
			return s, nil
		}
		return s, cmd
	default: // fsProbed, fsCombo
		nm, cmd := s.menu.Update(msg)
		s.menu = nm.(tui.Menu)
		switch msg.(type) {
		case tui.Chosen, tui.Quit:
			s.toMenu(0)
			return s, nil
		}
		return s, cmd
	}
}

func (s *freeScreen) pick(v string) tea.Cmd {
	be := s.a.be
	switch v {
	case "probe":
		s.stage, s.busy = fsBusy, "probing free models"
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rows, err := be.ProbeFree(ctx)
			return probeDoneMsg{rows: rows, err: err}
		}
	case "combo":
		s.stage, s.busy = fsBusy, "building free/auto"
		allow := s.a.snap.AllowTrains
		return func() tea.Msg {
			t, err := be.BuildFreeCombo(allow)
			return comboBuiltMsg{targets: t, err: err}
		}
	case "policy":
		s.openPolicy()
		return nil
	}
	return back()
}

// openPolicy lists the keyed free providers that train on prompts; ticking
// one allows it in free/auto.
func (s *freeScreen) openPolicy() {
	s.was = map[string]bool{}
	var items []tui.CItem
	for _, p := range s.a.snap.Providers {
		if !p.FreeEntry || p.Tag != "trains" {
			continue
		}
		st := tui.None
		if s.a.snap.AllowTrains[p.Name] {
			st = tui.Done
			s.was[p.Name] = true
		}
		items = append(items, tui.CItem{Label: p.Name, Value: p.Name, Tag: "trains", State: st})
	}
	s.stage = fsPolicy
	s.cl = s.list([]tui.Section{{Items: items}})
}

func (s *freeScreen) applyPolicy(values []string) {
	want := map[string]bool{}
	for _, v := range values {
		want[v] = true
	}
	next := map[string]bool{}
	for name := range s.was {
		if !want[name] {
			_ = s.a.be.SetAllowTrains(name, false)
		}
	}
	for name := range want {
		if !s.was[name] {
			_ = s.a.be.SetAllowTrains(name, true)
		}
		next[name] = true
	}
	s.a.snap.AllowTrains = next
	s.a.refresh()
	s.a.snap.AllowTrains = next
}

func (s *freeScreen) View() tea.View {
	th := s.a.theme
	var b strings.Builder
	switch s.stage {
	case fsMenu:
		s.mtop = 0
		b.WriteString(s.menu.View().Content)
	case fsBusy:
		b.WriteString(spinnerLine(th, s.a.opts.UTF8, s.busy))
	case fsPolicy:
		if len(s.cl.Sections[0].Items) == 0 {
			b.WriteString(th.Dim.Render("no free provider with a key trains on prompts"))
		} else {
			b.WriteString(s.cl.View().Content)
		}
	case fsProbed:
		if s.err != "" {
			b.WriteString(th.Err.Render(s.clip(s.err)) + "\n")
		}
		shown := 0
		for _, r := range s.rows {
			if shown >= max(1, s.h-5) {
				b.WriteString(th.Dim.Render("+"+itoa(len(s.rows)-shown)+" more") + "\n")
				break
			}
			shown++
			mark := th.OK.Render("ok  ")
			tail := itoa(int(r.Millis)) + "ms"
			if !r.OK {
				mark, tail = th.Err.Render("fail"), r.Err
			} else if !r.Tools {
				tail += " no tools"
			}
			b.WriteString(mark + " " + s.clip(r.Target+"  "+tail) + "\n")
		}
		if len(s.rows) == 0 && s.err == "" {
			b.WriteString(th.Dim.Render("no keyed free provider to probe") + "\n")
		}
		s.mtop = strings.Count(b.String(), "\n") + 1
		b.WriteString("\n" + s.menu.View().Content)
	case fsCombo:
		if s.err != "" {
			b.WriteString(th.Err.Render(s.clip(s.err)) + "\n")
		}
		for i, t := range s.targets {
			if i >= max(1, s.h-5) {
				b.WriteString(th.Dim.Render("+"+itoa(len(s.targets)-i)+" more") + "\n")
				break
			}
			b.WriteString(s.clip(t) + "\n")
		}
		s.mtop = strings.Count(b.String(), "\n") + 1
		b.WriteString("\n" + s.menu.View().Content)
	}
	return tea.NewView(b.String())
}

// --- Settings ---------------------------------------------------------------

const (
	ssRoot = iota
	ssRouting
	ssAppearance
	ssAdvanced
	ssTry
	ssTried
)

var tierNames = []string{"opus", "sonnet", "haiku"}

type settingsScreen struct {
	base
	stage int
	menu  tui.Menu
	in    tui.Input
	view  SettingsView
	plan  []string
	err   string
}

func newSettingsScreen(a *App) *settingsScreen {
	s := &settingsScreen{base: base{a: a, w: blockWidth(a.opts.W), h: max(4, a.opts.H-2)}}
	s.open(ssRoot, 0)
	return s
}

func (s *settingsScreen) title() string { return "Settings" }

func (s *settingsScreen) setSnap(snap Snapshot)     { s.a.snap = snap }
func (s *settingsScreen) setLocals(map[string]bool) {}

func (s *settingsScreen) open(stage, cursor int) {
	s.view = s.a.be.Settings()
	s.stage = stage
	switch stage {
	case ssRoot:
		s.menu = s.base.menu([]tui.Item{
			{Label: "Routing", Value: "routing"},
			{Label: "Appearance", Value: "appearance"},
			{Label: "Advanced", Value: "advanced"},
			{Label: "Back", Value: "back"},
		}, cursor)
	case ssRouting:
		items := make([]tui.Item, 0, 5)
		for _, t := range tierNames {
			v := s.view.Tiers[t]
			if v == "" {
				v = "claude"
			}
			items = append(items, tui.Item{Label: strings.ToUpper(t[:1]) + t[1:], Value: t, Tag: v})
		}
		items = append(items, tui.Item{Label: "Try a route", Value: "try"}, tui.Item{Label: "Back", Value: "back"})
		s.menu = s.base.menu(items, cursor)
	case ssAppearance:
		anim := "off"
		if s.view.Anim {
			anim = "on"
		}
		s.menu = s.base.menu([]tui.Item{
			{Label: "Look", Value: "look", Tag: s.view.Look},
			{Label: "Theme", Value: "theme", Tag: s.view.Theme},
			{Label: "Animation", Value: "anim", Tag: anim},
			{Label: "Back", Value: "back"},
		}, cursor)
	case ssAdvanced:
		s.menu = s.base.menu([]tui.Item{
			{Label: "Reset appearance", Value: "reset"},
			{Label: "Run setup again", Value: "setup"},
			{Label: "Back", Value: "back"},
		}, cursor)
	}
}

func (s *settingsScreen) Init() tea.Cmd { return nil }

func (s *settingsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.sizeMsg(msg)
	msg, ok := s.mouse(msg)
	if !ok {
		return s, nil
	}
	switch s.stage {
	case ssTry:
		nm, cmd := s.in.Update(msg)
		s.in = nm.(tui.Input)
		switch v := msg.(type) {
		case tui.Submitted:
			if model := strings.TrimSpace(v.Value); model != "" {
				s.plan = s.a.be.Plan(model)
				s.stage = ssTried
				s.menu = s.base.menu(labels("Back"), 0)
			}
			return s, nil
		case tui.Cancelled:
			s.open(ssRouting, 3)
			return s, nil
		}
		return s, cmd
	case ssTried:
		nm, cmd := s.menu.Update(msg)
		s.menu = nm.(tui.Menu)
		switch msg.(type) {
		case tui.Chosen, tui.Quit:
			s.open(ssRouting, 3)
			return s, nil
		}
		return s, cmd
	}
	nm, cmd := s.menu.Update(msg)
	s.menu = nm.(tui.Menu)
	switch v := msg.(type) {
	case tui.Chosen:
		return s, s.pick(v.Value)
	case tui.Quit:
		if s.stage == ssRoot {
			return s, back()
		}
		s.open(ssRoot, s.stage-1)
		return s, nil
	}
	return s, cmd
}

// cycle returns the option after cur in opts, wrapping around.
func cycle(opts []string, cur string) string {
	for i, o := range opts {
		if o == cur {
			return opts[(i+1)%len(opts)]
		}
	}
	return opts[0]
}

func (s *settingsScreen) pick(v string) tea.Cmd {
	be := s.a.be
	cur := s.menu.Cursor()
	switch s.stage {
	case ssRoot:
		switch v {
		case "routing":
			s.open(ssRouting, 0)
		case "appearance":
			s.open(ssAppearance, 0)
		case "advanced":
			s.open(ssAdvanced, 0)
		default:
			return back()
		}
	case ssRouting:
		switch v {
		case "back":
			s.open(ssRoot, 0)
		case "try":
			s.stage = ssTry
			s.in = s.base.input("model name")
		default:
			opts := append([]string{""}, s.view.Combos...)
			next := cycle(opts, s.view.Tiers[v])
			if err := be.SetTier(v, next); err != nil {
				s.err = err.Error()
			}
			s.open(ssRouting, cur)
		}
	case ssAppearance:
		if v == "back" {
			s.open(ssRoot, 1)
			return nil
		}
		look, theme, anim := s.view.Look, s.view.Theme, s.view.Anim
		switch v {
		case "look":
			ls := make([]string, len(art.Looks))
			for i, l := range art.Looks {
				ls[i] = string(l)
			}
			look = cycle(ls, look)
		case "theme":
			theme = cycle(tui.ThemeNames, theme)
		case "anim":
			anim = !anim
		}
		cmd := s.a.applyUI(look, theme, anim)
		s.open(ssAppearance, cur)
		return cmd
	case ssAdvanced:
		switch v {
		case "reset":
			_ = be.ResetUI()
			cmd := s.a.applyUI(string(art.Braille), "mono", true)
			s.open(ssAdvanced, cur)
			return cmd
		case "setup":
			return func() tea.Msg { return rerunSetupMsg{} }
		default:
			s.open(ssRoot, 2)
		}
	}
	return nil
}

func (s *settingsScreen) View() tea.View {
	th := s.a.theme
	var b strings.Builder
	switch s.stage {
	case ssTry:
		b.WriteString(th.Text.Render("Model") + "\n\n" + s.in.View().Content)
	case ssTried:
		for i, l := range s.plan {
			if i >= max(1, s.h-5) {
				b.WriteString(th.Dim.Render("+"+itoa(len(s.plan)-i)+" more") + "\n")
				break
			}
			b.WriteString(s.clip(l) + "\n")
		}
		s.mtop = strings.Count(b.String(), "\n") + 1
		b.WriteString("\n" + s.menu.View().Content)
	default:
		if s.err != "" {
			b.WriteString(th.Err.Render(s.clip(s.err)) + "\n\n")
		}
		s.mtop = strings.Count(b.String(), "\n")
		b.WriteString(s.menu.View().Content)
	}
	return tea.NewView(b.String())
}

// applyUI switches look, theme and animation live and saves them. It returns
// the tick command when animation was just turned on.
func (a *App) applyUI(look, theme string, anim bool) tea.Cmd {
	_ = a.be.SetUI(look, theme, anim)
	wasAnim := a.opts.Anim
	a.opts.Look, a.opts.Theme, a.opts.Anim = art.Look(look), theme, anim
	if th, err := a.opts.ThemeOr(); err == nil {
		a.theme = th
	}
	if anim && !wasAnim {
		return a.tickCmd()
	}
	return nil
}

// --- Status -----------------------------------------------------------------

type statusScreen struct {
	base
	rows []StatusRow
	menu tui.Menu
}

func newStatusScreen(a *App) *statusScreen {
	s := &statusScreen{base: base{a: a, w: blockWidth(a.opts.W), h: max(4, a.opts.H-2)}}
	s.rows = a.be.Status()
	s.menu = s.base.menu(labels("Back"), 0)
	return s
}

func (s *statusScreen) title() string { return "Status" }

func (s *statusScreen) setSnap(snap Snapshot)     { s.a.snap = snap }
func (s *statusScreen) setLocals(map[string]bool) {}

func (s *statusScreen) Init() tea.Cmd { return nil }

func (s *statusScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.sizeMsg(msg)
	msg, ok := s.mouse(msg)
	if !ok {
		return s, nil
	}
	nm, cmd := s.menu.Update(msg)
	s.menu = nm.(tui.Menu)
	switch msg.(type) {
	case tui.Chosen, tui.Quit:
		return s, back()
	}
	return s, cmd
}

func (s *statusScreen) View() tea.View {
	th := s.a.theme
	names := make([]string, 0, len(s.rows))
	for _, r := range s.rows {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	width := 0
	for _, n := range names {
		width = max(width, len(n))
	}
	var b strings.Builder
	for _, r := range s.rows {
		mark := th.OK.Render("ok")
		if !r.OK {
			mark = th.Err.Render("!!")
		}
		name := r.Name + strings.Repeat(" ", width-len(r.Name))
		b.WriteString(mark + "  " + name + "  " + th.Dim.Render(truncate(r.Detail, max(4, s.w-width-6))) + "\n")
	}
	s.mtop = strings.Count(b.String(), "\n") + 1
	b.WriteString("\n" + s.menu.View().Content)
	return tea.NewView(b.String())
}

func (s *freeScreen) legend() []string {
	if s.stage == fsPolicy {
		return []string{"↑↓|move", "space|allow", "↵|apply", "esc|back"}
	}
	if s.stage == fsBusy {
		return []string{"esc|cancel"}
	}
	return []string{"↑↓|move", "↵|select", "1-9|jump", "esc|back"}
}

func (s *settingsScreen) legend() []string {
	switch s.stage {
	case ssTry:
		return []string{"↵|show route", "esc|back"}
	}
	return []string{"↑↓|move", "↵|change", "1-9|jump", "esc|back"}
}

func (s *statusScreen) legend() []string { return []string{"↵|back", "esc|back"} }
