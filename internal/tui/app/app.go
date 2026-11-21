// Package app is the tsunagi interactive shell: intro, main menu, the
// first-run wizard and the Providers/Skills screens. It talks to the rest of
// tsunagi only through Backend, so tests can drive it with a fake.
package app

import (
	"context"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/tui"
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
	Tag       string    // data label: private|trains|unknown|plan|""
	Setup     string    // extra setup the provider needs, e.g. "account-id"
	FreeEntry bool      // Name matches a free.Registry entry
	Keyless   bool      // answers with no account and no key
	Limits    string    // registry limits text, "" outside the registry
	Plan      *PlanInfo // set on Group "plan" rows, nil for chatgpt
}

// PlanInfo is what the providers screen shows for a coding plan. Verified
// means the details come from the provider's own docs on Checked; an
// unverified plan only carries the flat-fee Terms line.
type PlanInfo struct {
	Price, Quota, Models, Needs, KeyHint, Terms, Source, Checked string
	Signup, KeyURL                                               string
	Verified                                                     bool
}

// EnvKey is a routable provider that an environment variable can key. The
// value itself is never exposed through the interface.
type EnvKey struct {
	Provider string
	Var      string
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
	Tiers     map[string]string // opus|sonnet|haiku -> model or combo; absent = Claude
	Scenarios map[string]string // think|long_context|web_search -> model or combo
	Combos    []string          // combo names a tier or scenario can point at, sorted
	Look      string
	Theme     string
	Anim      bool
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
	SetScenario(name, value string) error
	SetUI(look, theme string, anim bool) error
	ResetUI() error
	Plan(model string) []string
	ProbeFree(ctx context.Context) ([]ProbeRow, error)
	// EnvKeys lists unkeyed routable providers an environment variable can
	// key, names only. ImportEnvKey stores that variable's value for the
	// provider; DetectKey guesses providers from a pasted key's prefix.
	EnvKeys() []EnvKey
	ImportEnvKey(provider string) error
	DetectKey(key string) ([]string, error)
}

// Action is what the app asks main to do when it exits.
type Action int

const (
	None  Action = iota
	Start        // run claude through the router
	Quit         // exit without launching
)

// Version is shown in the brand bar; main stamps it from its own var.
var Version = "dev"

// App is the root model.
type App struct {
	opts    tui.Options
	theme   tui.Theme
	be      Backend
	snap    Snapshot
	envKeys []EnvKey
	scr     tea.Model
	phase   float64
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
	} else {
		a.setScreen(newHomeScreen(a))
	}
	return a, nil
}

// Action reports what main should do after Run returns.
func (a *App) Action() Action { return a.action }

func (a *App) refresh() {
	a.snap = a.be.Snapshot()
	a.envKeys = a.be.EnvKeys()
}

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
type startMsg struct{}
type quitAppMsg struct{}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		// Screens size their scroll view to the content area: the wordmark,
		// blank lines and whatever eel room remains are chrome, not theirs.
		a.opts.W, a.opts.H = v.Width, v.Height
		if a.scr != nil {
			f := a.frame(a.scr)
			msg = tea.WindowSizeMsg{Width: f.bodyW, Height: f.bodyH}
		}
	case tea.MouseClickMsg, tea.MouseWheelMsg:
		m, ok := a.bodyMouse(msg)
		if !ok {
			return a, nil
		}
		msg = m
	case tickMsg:
		a.phase += 0.35
		if a.opts.Anim {
			return a, a.tickCmd()
		}
	case localsMsg:
		a.locals = msg.(localsMsg)
	case startMsg:
		a.action = Start
		return a, tea.Quit
	case quitAppMsg:
		a.action = Quit
		return a, tea.Quit
	case menuPickMsg:
		return a, a.onMenuPick(v.value)
	case openProviderMsg:
		s := newProvidersScreen(a, a.wizard)
		if v.chip == "free" {
			s.chip = chipFree
			s.rebuild()
		}
		if v.name != "" {
			_, _ = s.open(v.name)
		}
		return a, a.setScreen(s)
	case openSettingsMsg:
		s := newSettingsScreen(a)
		switch v.sub {
		case "routing":
			s.open(ssRouting, 0)
		case "appearance":
			s.open(ssAppearance, 0)
		case "advanced":
			s.open(ssAdvanced, 0)
		}
		return a, a.setScreen(s)
	case providersDoneMsg:
		a.refresh()
		if a.wizard {
			return a, a.setScreen(newSkillsScreen(a, true))
		}
		return a, a.setScreen(newHomeScreen(a))
	case providersBackMsg:
		return a, a.setScreen(newHomeScreen(a))
	case skillsDoneMsg:
		a.refresh()
		if a.wizard {
			return a, a.setScreen(newWizardReady(a))
		}
		return a, a.setScreen(newHomeScreen(a))
	case skillsBackMsg, screenBackMsg:
		a.refresh()
		return a, a.setScreen(newHomeScreen(a))
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

// finishWizard marks onboarding done and lands on home.
func (a *App) finishWizard() tea.Cmd {
	a.wizard = false
	_ = a.be.MarkOnboarded()
	a.refresh()
	return a.setScreen(newHomeScreen(a))
}

// screenModel is implemented by screens that need snapshot/local updates.
type screenModel interface {
	setSnap(Snapshot)
	setLocals(map[string]bool)
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
		f := a.frame(s)
		nm, _ := s.Update(tea.WindowSizeMsg{Width: f.bodyW, Height: f.bodyH})
		a.scr = nm
	}
	return a.scr.Init()
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
