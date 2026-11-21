// Package app is the tsunagi interactive shell: intro, main menu, the
// first-run wizard and the Providers/Skills screens. It talks to the rest of
// tsunagi only through Backend, so tests can drive it with a fake.
package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/ui"
)

// VerifyResult is one provider verification attempt.
type VerifyResult struct {
	OK     bool
	Model  string
	Millis int64
	Err    string
}

// ProviderRow is one provider line in the Providers screen.
type ProviderRow struct {
	Name      string
	Group     string // "free", "plan", "paid" or "local"
	Ready     bool   // a key or sign-in (or local) is stored
	Accounts  []string
	Tag       string // data label: private|trains|unknown|plan|""
	Setup     string // extra setup the provider needs, e.g. "account-id"
	FreeEntry bool   // Name matches a free.Registry entry
}

// StatusRow is one health check line in the Status screen.
type StatusRow struct {
	Name   string
	OK     bool
	Detail string
}

// ProbeRow is one free-model probe result.
type ProbeRow struct {
	Target string
	OK     bool
	Tools  bool
	Millis int64
	Err    string
}

// SettingsView is the editable part of the config the Settings screen shows.
type SettingsView struct {
	Tiers  map[string]string // opus|sonnet|haiku -> model or combo; absent = Claude
	Combos []string          // combo names a tier can point at, sorted
	Look   string
	Theme  string
	Anim   bool
}

// Snapshot is the world as the screens see it; Backend.Snapshot builds it.
type Snapshot struct {
	ClaudePath  string // "" when claude is not in PATH
	ConfigPath  string
	Providers   []ProviderRow
	FreeTargets []string // free/auto combo targets, nil when unset
	AllowTrains map[string]bool
	Skills      []skills.StatusEntry
	Onboarded   bool
}

// Backend is everything the screens need from tsunagi. Package main
// implements it; the app never touches keychain, network, browser, files or
// exec itself.
type Backend interface {
	Snapshot() Snapshot
	SaveKey(provider, account, key string) error
	RemoveKey(provider, account string) error
	SignInChatGPT(ctx context.Context, account string, onURL func(url string)) (email string, err error)
	AddGateway(name, url, dialect string, local bool) error
	ImportOpencode() (summary string, err error)
	Verify(ctx context.Context, provider string) VerifyResult
	BuildFreeCombo(allowTrains map[string]bool) ([]string, error)
	SetAllowTrains(provider string, on bool) error
	OpenURL(url string) error
	InstallSkills(targets []string) []skills.Result
	RemoveSkills(targets []string) []skills.Result
	MarkOnboarded() error
	// ProbeLocals reports which local providers answer, once, in the
	// background. A nil channel means "no probe".
	ProbeLocals(ctx context.Context) <-chan map[string]bool
	Status() []StatusRow
	Settings() SettingsView
	SetTier(tier, value string) error
	SetUI(look, theme string, anim bool) error
	ResetUI() error
	Plan(model string) []string
	ProbeFree(ctx context.Context) ([]ProbeRow, error)
}

// Action is what the app asks main to do when it exits.
type Action int

const (
	None  Action = iota
	Start        // run claude through the router
	Quit         // exit without launching
)

// App is the root model.
type App struct {
	opts    tui.Options
	theme   tui.Theme
	be      Backend
	snap    Snapshot
	scr     tea.Model
	phase   float64
	intro   *introScreen
	action  Action
	wizard  bool
	locals  map[string]bool
	localCh <-chan map[string]bool
}

// New builds the app. When firstRun is true it opens the wizard; otherwise
// the intro plays (when animation is on) and lands on the menu.
func New(o tui.Options, be Backend, firstRun bool) (*App, error) {
	th, err := o.ThemeOr()
	if err != nil {
		return nil, err
	}
	a := &App{opts: o, theme: th, be: be}
	a.refresh()
	if ch := be.ProbeLocals(context.Background()); ch != nil {
		a.localCh = ch
	}
	if firstRun {
		a.wizard = true
		a.setScreen(newWizardWelcome(a))
	} else if o.Anim {
		a.intro = newIntroScreen(o, th, a)
		a.scr = a.intro
	} else {
		a.setScreen(a.menuScreen())
	}
	return a, nil
}

// Action reports what main should do after Run returns.
func (a *App) Action() Action { return a.action }

func (a *App) refresh() { a.snap = a.be.Snapshot() }

// Init starts the animation tick.
func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd
	if a.scr != nil {
		if c := a.scr.Init(); c != nil {
			cmds = append(cmds, c)
		}
	}
	if a.opts.Anim {
		cmds = append(cmds, a.tickCmd())
	}
	return tea.Batch(cmds...)
}

func (a *App) tickCmd() tea.Cmd {
	return tea.Tick(artTickDur, func(time.Time) tea.Msg { return tickMsg{} })
}

const artTickDur = 83 * time.Millisecond

type tickMsg struct{}
type localsMsg map[string]bool
type introDone struct{}
type startMsg struct{}
type quitAppMsg struct{}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		// Screens size their scroll view to the content area: the wordmark,
		// blank lines and whatever eel room remains are chrome, not theirs.
		a.opts.W, a.opts.H = v.Width, v.Height
		if a.scr != nil {
			w, h := contentSize(a.scr, v.Width, v.Height)
			msg = tea.WindowSizeMsg{Width: w, Height: h}
		}
	case tickMsg:
		a.phase += 0.35
		if a.opts.Anim {
			return a, a.tickCmd()
		}
	case localsMsg:
		a.locals = msg.(localsMsg)
	case introDone:
		a.intro = nil
		return a, a.setScreen(a.menuScreen())
	case startMsg:
		a.action = Start
		return a, tea.Quit
	case quitAppMsg:
		a.action = Quit
		return a, tea.Quit
	case menuPickMsg:
		return a, a.onMenuPick(v.value)
	case providersDoneMsg:
		a.refresh()
		if a.wizard {
			return a, a.setScreen(newSkillsScreen(a, true))
		}
		return a, a.setScreen(a.menuScreen())
	case providersBackMsg:
		return a, a.setScreen(a.menuScreen())
	case skillsDoneMsg:
		a.refresh()
		if a.wizard {
			return a, a.setScreen(newWizardReady(a))
		}
		return a, a.setScreen(a.menuScreen())
	case skillsBackMsg, screenBackMsg:
		a.refresh()
		return a, a.setScreen(a.menuScreen())
	case rerunSetupMsg:
		a.wizard = true
		return a, a.setScreen(newWizardWelcome(a))
	case wizPickMsg:
		return a, a.onWizPick(v.value)
	case wizSkipMsg:
		return a, a.finishWizard()
	case tea.QuitMsg:
		if a.action == None {
			a.action = Quit
		}
		return a, tea.Quit
	}
	if a.localCh != nil {
		select {
		case v := <-a.localCh:
			a.localCh = nil
			a.locals = v
			a.refresh()
		default:
		}
	}
	if a.scr == nil {
		return a, nil
	}
	nm, cmd := a.scr.Update(msg)
	if sm, ok := nm.(tea.Model); ok {
		a.scr = sm
	}
	return a, cmd
}

func (a *App) onMenuPick(v string) tea.Cmd {
	switch v {
	case "start":
		a.action = Start
		return tea.Quit
	case "providers":
		return a.setScreen(newProvidersScreen(a, a.wizard))
	case "skills":
		return a.setScreen(newSkillsScreen(a, a.wizard))
	case "free":
		return a.setScreen(newFreeScreen(a))
	case "settings":
		return a.setScreen(newSettingsScreen(a))
	case "status":
		return a.setScreen(newStatusScreen(a))
	}
	a.action = Quit
	return tea.Quit
}

func (a *App) onWizPick(v string) tea.Cmd {
	switch v {
	case "continue":
		return a.setScreen(newProvidersScreen(a, true))
	case "start":
		a.wizard = false
		_ = a.be.MarkOnboarded()
		a.action = Start
		return tea.Quit
	case "menu":
		return a.finishWizard()
	}
	return a.finishWizard() // "skip"
}

// finishWizard marks onboarding done and lands on the menu.
func (a *App) finishWizard() tea.Cmd {
	a.wizard = false
	_ = a.be.MarkOnboarded()
	a.refresh()
	return a.setScreen(a.menuScreen())
}

// screenModel is implemented by screens that need snapshot/local updates.
type screenModel interface {
	setSnap(Snapshot)
	setLocals(map[string]bool)
}

// blockWidth is the width of the one centered column every screen below the
// art lives in.
func blockWidth(w int) int {
	switch {
	case w-4 > 52:
		return 52
	case w-4 < 30:
		return 30
	}
	return w - 4
}

// titled is implemented by screens that replace the wordmark and eel with a
// one-row title, leaving the terminal to their list.
type titled interface{ title() string }

// contentHeight is the terminal height left for screen content once the
// wordmark and its spacing are accounted for.
func contentHeight(w, h int) int {
	if tui.Compact(w, h) {
		return h
	}
	h -= len(ui.LogoLines()) + 3
	if h < 4 {
		h = 4
	}
	return h
}

// contentSize is the area a screen gets: the block width and the rows left
// after its chrome (a title row, or the wordmark).
func contentSize(scr tea.Model, w, h int) (int, int) {
	if _, ok := scr.(titled); ok {
		if h-2 < 4 {
			return blockWidth(w), 4
		}
		return blockWidth(w), h - 2
	}
	return blockWidth(w), contentHeight(w, h)
}

// fit resizes a freshly built component to the area the screen last saw;
// constructors default to 80x24, which overflows small terminals.
func fit(m tea.Model, w, h int) tea.Model {
	if w <= 0 || h <= 0 {
		return m
	}
	nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return nm
}

func (a *App) setScreen(s tea.Model) tea.Cmd {
	a.scr = s
	if sm, ok := s.(screenModel); ok {
		sm.setSnap(a.snap)
		if a.locals != nil {
			sm.setLocals(a.locals)
		}
	}
	if s == nil {
		return nil
	}
	// WindowSizeMsg only arrives once, at startup; screens created later get
	// the current content size injected here.
	if a.opts.W > 0 && a.opts.H > 0 {
		w, h := contentSize(s, a.opts.W, a.opts.H)
		nm, _ := s.Update(tea.WindowSizeMsg{Width: w, Height: h})
		a.scr = nm
	}
	return a.scr.Init()
}

// View composes wordmark, eel and the current screen. The eel shrinks to
// whatever the screen content leaves, so nothing is ever pushed off-frame.
// Everything under the art sits in one centered block of blockWidth cells.
func (a *App) View() tea.View {
	W, H := a.opts.W, a.opts.H
	var screen string
	if a.scr != nil {
		screen = strings.TrimRight(a.scr.View().Content, "\n")
	}
	bw := blockWidth(W)
	indent := strings.Repeat(" ", max(0, (W-bw)/2))
	block := func(s string) []string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			out = append(out, indent+l)
		}
		return out
	}
	var lines []string
	// every row is padded to the terminal width: Place would otherwise
	// center shorter rows one by one and break the block's left edge
	place := func(ls []string) tea.View {
		for i, l := range ls {
			if n := W - tui.WidthOf(l); n > 0 {
				ls[i] = l + strings.Repeat(" ", n)
			}
		}
		return tea.NewView(tui.Place(W, H, strings.Join(ls, "\n")))
	}
	if t, ok := a.scr.(titled); ok {
		lines = append(lines, indent+a.theme.Title.Render(t.title()), "")
		lines = append(lines, block(screen)...)
		return place(lines)
	}
	if !tui.Compact(W, H) {
		logo := tui.Wordmark(W, a.theme)
		screenH := 0
		if screen != "" {
			screenH = strings.Count(screen, "\n") + 1
		}
		artH := min(10, H-len(logo)-1-screenH-2-1)
		lines = append(lines, logo...)
		lines = append(lines, "")
		if artH > 0 {
			ao := a.opts
			ao.W, ao.H = min(56, W-2), artH
			ai := strings.Repeat(" ", max(0, (W-ao.W)/2))
			for _, l := range strings.Split(tui.EelArt(ao, a.theme, a.phase), "\n") {
				lines = append(lines, ai+l)
			}
			lines = append(lines, "")
		}
	}
	lines = append(lines, block(screen)...)
	return place(lines)
}

// menuScreen builds the main menu.
func (a *App) menuScreen() *menuScreen {
	m := newMenuScreen(a.opts, a.theme, a.menuItems(), 0)
	m.app = a
	return m
}

// menuItems builds the main menu rows from the snapshot.
func (a *App) menuItems() []tui.Item {
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
	if n := len(a.snap.Skills); n > 0 {
		full := map[string]int{}
		for _, s := range a.snap.Skills {
			if s.State == "installed" {
				full[s.Target]++
			}
		}
		n := 0
		for _, c := range full {
			if c == 4 {
				n++
			}
		}
		skillTag = itoa(n) + "/2"
		if n == 0 {
			skillTag = "-"
		}
	}
	return []tui.Item{
		{Label: "Start Claude Code", Value: "start", Tag: claude, Disabled: a.snap.ClaudePath == ""},
		{Label: "Providers", Value: "providers", Tag: itoa(ready) + " ready"},
		{Label: "Skills", Value: "skills", Tag: skillTag},
		{Label: "Free models", Value: "free", Tag: freeTag},
		{Label: "Settings", Value: "settings"},
		{Label: "Status", Value: "status"},
		{Label: "Quit", Value: "quit"},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
