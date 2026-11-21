package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/tui"
)

// fakeBackend is a scripted Backend for driving the app in tests.
type fakeBackend struct {
	snap      Snapshot
	verify    func(provider string) VerifyResult
	saveErr   error
	install   []skills.Result
	remove    []skills.Result
	combo     []string
	comboErr  error
	signIn    func(ctx context.Context, account string, onURL func(string)) (string, error)
	opened    []string
	allowTr   map[string]bool
	onboarded bool
	removed   []string
}

func (f *fakeBackend) Snapshot() Snapshot { return f.snap }

func (f *fakeBackend) SaveKey(provider, account, key string) error {
	if key == "bad-key!!" {
		return errors.New("key has unexpected characters")
	}
	return f.saveErr
}

func (f *fakeBackend) RemoveKey(provider, account string) error {
	f.removed = append(f.removed, provider+"/"+account)
	return nil
}

func (f *fakeBackend) SignInChatGPT(ctx context.Context, account string, onURL func(string)) (string, error) {
	if f.signIn != nil {
		return f.signIn(ctx, account, onURL)
	}
	return "", errors.New("not scripted")
}

func (f *fakeBackend) AddGateway(name, url, dialect string, local bool) error { return nil }

func (f *fakeBackend) ImportOpencode() (string, error) { return "added 0 providers", nil }

func (f *fakeBackend) Verify(ctx context.Context, provider string) VerifyResult {
	if f.verify != nil {
		return f.verify(provider)
	}
	return VerifyResult{OK: true, Model: provider + "/m", Millis: 12}
}

func (f *fakeBackend) BuildFreeCombo(allow map[string]bool) ([]string, error) {
	f.allowTr = allow
	return f.combo, f.comboErr
}

func (f *fakeBackend) SetAllowTrains(provider string, on bool) error {
	if f.allowTr == nil {
		f.allowTr = map[string]bool{}
	}
	f.allowTr[provider] = on
	return nil
}

func (f *fakeBackend) OpenURL(url string) error {
	f.opened = append(f.opened, url)
	return nil
}

func (f *fakeBackend) InstallSkills(targets []string) []skills.Result { return f.install }
func (f *fakeBackend) RemoveSkills(targets []string) []skills.Result  { return f.remove }

func (f *fakeBackend) MarkOnboarded() error {
	f.onboarded = true
	return nil
}

func (f *fakeBackend) ProbeLocals(ctx context.Context) <-chan map[string]bool {
	return nil // no probe in tests
}

// --- helpers ---------------------------------------------------------------

var testOpts = tui.Options{
	Look: "braille", Theme: "mono", Color: false, TrueColor: false,
	Anim: false, UTF8: true, W: 100, H: 30,
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func viewText(m tea.Model) string { return stripANSI(m.View().Content) }

// send delivers a message and pumps the resulting cmds until they settle.
// Slow cmds (verify, chat sign-in, ticks) are left pending: the fake backend
// resolves the fast ones instantly, and blocking ones are exercised via esc.
func send(m *App, msg tea.Msg) {
	for i := 0; i < 40; i++ {
		_, cmd := m.Update(msg)
		if cmd == nil {
			return
		}
		ch := make(chan tea.Msg, 1)
		go func() { ch <- cmd() }()
		var next tea.Msg
		select {
		case next = <-ch:
		case <-time.After(500 * time.Millisecond):
			return
		}
		if next == nil {
			return
		}
		switch next.(type) {
		case tickMsg, chatTickMsg:
			return
		}
		msg = next
	}
}

func key(m *App, code rune, text string) { send(m, tea.KeyPressMsg{Code: code, Text: text}) }

func press(m *App, text string) {
	code := rune(0)
	if len([]rune(text)) == 1 {
		code = []rune(text)[0]
	}
	key(m, code, text)
}

func enter(m *App) { key(m, tea.KeyEnter, "") }

func space(m *App) { key(m, ' ', " ") }

func esc(m *App) { key(m, tea.KeyEscape, "") }

func snapBase() Snapshot {
	return Snapshot{
		ClaudePath: "/usr/local/bin/claude",
		ConfigPath: "/tmp/cfg.json",
		Providers: []ProviderRow{
			{Name: "ollama", Group: "local", Ready: true, Tag: ""},
			{Name: "openrouter", Group: "free", Tag: "private"},
			{Name: "groq", Group: "free", Tag: "trains"},
			{Name: "chatgpt", Group: "plan", Tag: "plan"},
			{Name: "anthropic", Group: "paid", Ready: true, Accounts: []string{"anthropic"}},
		},
		Skills: []skills.StatusEntry{
			{Target: "claude", Skill: "tsunagi-free", State: "missing"},
			{Target: "agents", Skill: "tsunagi-free", State: "missing"},
		},
		Onboarded: true,
	}
}

func newTestApp(t *testing.T, be *fakeBackend, firstRun bool) *App {
	t.Helper()
	m, err := New(testOpts, be, firstRun)
	if err != nil {
		t.Fatal(err)
	}
	send(m, tea.WindowSizeMsg{Width: testOpts.W, Height: testOpts.H})
	return m
}

// --- tests -----------------------------------------------------------------

func TestMenuShowsRowsAndValues(t *testing.T) {
	be := &fakeBackend{snap: snapBase()}
	m := newTestApp(t, be, false)
	v := viewText(m)
	for _, want := range []string{"Start Claude Code", "Providers", "Skills", "Quit", "claude", "ready"} {
		if !strings.Contains(v, want) {
			t.Fatalf("menu missing %q:\n%s", want, v)
		}
	}
}

func TestClaudeNotFoundDisablesStart(t *testing.T) {
	snap := snapBase()
	snap.ClaudePath = ""
	be := &fakeBackend{snap: snap}
	m := newTestApp(t, be, false)
	if !strings.Contains(viewText(m), "not found") {
		t.Fatalf("expected 'not found':\n%s", viewText(m))
	}
}

func TestMenuPickStartQuitsWithStart(t *testing.T) {
	be := &fakeBackend{snap: snapBase()}
	m := newTestApp(t, be, false)
	press(m, "1") // Start Claude Code
	if m.Action() != Start {
		t.Fatalf("action = %v, want Start", m.Action())
	}
}

// driveToKeyInput advances a provider flow until the masked key input shows.
// The masked field renders only bullets, so the stage is checked directly.
func driveToKeyInput(t *testing.T, m *App) {
	t.Helper()
	for i := 0; i < 12; i++ {
		ps, ok := m.scr.(*providersScreen)
		if !ok {
			t.Fatalf("not on providers screen: %T\n%s", m.scr, viewText(m))
		}
		if ps.stage == psKey {
			return
		}
		if ps.stage == psPanel {
			press(m, "2") // I have a key
			continue
		}
		enter(m)
	}
	t.Fatalf("never reached key input:\n%s", viewText(m))
}

func TestWizardHappyPath(t *testing.T) {
	snap := snapBase()
	snap.Onboarded = false
	snap.Providers = []ProviderRow{{Name: "openrouter", Group: "free", Tag: "private"}}
	snap.Skills = []skills.StatusEntry{
		{Target: "claude", Skill: "tsunagi-free", State: "missing"},
		{Target: "agents", Skill: "tsunagi-free", State: "missing"},
	}
	be := &fakeBackend{
		snap:    snap,
		install: []skills.Result{{Target: "claude", Skill: "tsunagi-free", State: "installed"}},
	}
	m := newTestApp(t, be, true)
	enter(m) // welcome: Continue -> providers
	space(m) // select openrouter
	enter(m) // confirm -> flow
	driveToKeyInput(t, m)
	for _, r := range "testkey12345" {
		press(m, string(r))
	}
	enter(m) // submit -> verify OK -> result menu (Next default)
	enter(m) // Next -> queue done -> skills screen
	enter(m) // confirm skills -> install rows -> Done
	enter(m) // Done -> ready screen
	v := viewText(m)
	if !strings.Contains(v, "Start Claude Code") || !strings.Contains(v, "Back to menu") {
		t.Fatalf("expected ready screen:\n%s", v)
	}
	press(m, "2") // Back to menu -> finishWizard
	if !be.onboarded {
		t.Fatal("MarkOnboarded not called")
	}
	if !strings.Contains(viewText(m), "Start Claude Code") {
		t.Fatalf("expected main menu:\n%s", viewText(m))
	}
}

func TestWizardSkipAtWelcome(t *testing.T) {
	snap := snapBase()
	snap.Onboarded = false
	snap.Providers = nil
	be := &fakeBackend{snap: snap}
	m := newTestApp(t, be, true)
	esc(m) // skip welcome -> menu
	if !be.onboarded {
		t.Fatal("MarkOnboarded not called")
	}
	if !strings.Contains(viewText(m), "Start Claude Code") {
		t.Fatalf("expected menu after skip:\n%s", viewText(m))
	}
}

func TestProvidersScreenGroups(t *testing.T) {
	be := &fakeBackend{snap: snapBase()}
	m := newTestApp(t, be, false)
	press(m, "2") // Providers
	v := viewText(m)
	for _, want := range []string{"Free", "Your plans", "Paid keys", "Local and own", "openrouter", "chatgpt", "anthropic", "ollama", "Add a gateway"} {
		if !strings.Contains(v, want) {
			t.Fatalf("providers screen missing %q:\n%s", want, v)
		}
	}
}

func TestKeyEntryMaskedNeverRendered(t *testing.T) {
	snap := snapBase()
	snap.Providers = []ProviderRow{{Name: "openrouter", Group: "free", Tag: "private"}}
	be := &fakeBackend{snap: snap}
	m := newTestApp(t, be, false)
	press(m, "2")
	space(m) // select openrouter
	enter(m)
	// may be at signup panel or key input; navigate to key stage
	driveToKeyInput(t, m)
	// type a secret
	secret := "sk-test-SECRET-12345"
	for _, r := range secret {
		press(m, string(r))
	}
	if strings.Contains(viewText(m), secret) {
		t.Fatalf("secret rendered in view:\n%s", viewText(m))
	}
	enter(m) // submit
	// verify result screen should not contain the secret either
	if strings.Contains(viewText(m), secret) {
		t.Fatalf("secret rendered after submit:\n%s", viewText(m))
	}
}

func TestVerifyFailureOffersReenter(t *testing.T) {
	snap := snapBase()
	snap.Providers = []ProviderRow{{Name: "openrouter", Group: "free", Tag: "private"}}
	be := &fakeBackend{
		snap:   snap,
		verify: func(p string) VerifyResult { return VerifyResult{Err: "boom"} },
	}
	m := newTestApp(t, be, false)
	press(m, "2")
	space(m)
	enter(m)
	driveToKeyInput(t, m)
	for _, r := range "testkey12345" {
		press(m, string(r))
	}
	enter(m) // submit -> verify fails -> result menu
	v := viewText(m)
	if !strings.Contains(v, "Re-enter key") {
		t.Fatalf("expected result menu with Re-enter key:\n%s", v)
	}
}

func TestTrainsProviderAskedOnce(t *testing.T) {
	snap := snapBase()
	for i := range snap.Providers {
		switch snap.Providers[i].Name {
		case "groq":
			snap.Providers[i].Ready = true
			snap.Providers[i].Accounts = []string{"groq"}
		case "openrouter":
			snap.Providers[i].Ready = true
			snap.Providers[i].Accounts = []string{"openrouter"}
		case "anthropic":
			snap.Providers[i].Ready = false
			snap.Providers[i].Accounts = nil
			snap.Providers[i].Group = "paid"
		}
	}
	// add a third free provider to queue so the flow runs to the combo offer
	snap.Providers = append([]ProviderRow{{Name: "mistral", Group: "free", Tag: "private"}}, snap.Providers...)
	be := &fakeBackend{snap: snap, combo: []string{"openrouter/auto"}}
	m := newTestApp(t, be, false)
	press(m, "2")
	// flat order: mistral(0), openrouter(1), groq(2), chatgpt, anthropic, ollama, add-gateway
	// openrouter+groq are Done -> cycling to None so only mistral is selected
	press(m, "j") // -> openrouter
	space(m)      // Done -> None
	press(m, "j") // -> groq
	space(m)      // Done -> None
	press(m, "k") // -> openrouter
	press(m, "k") // -> mistral
	space(m)      // None -> Pending
	enter(m)      // Confirmed = [mistral] only
	driveToKeyInput(t, m)
	for _, r := range "testkey12345" {
		press(m, string(r))
	}
	enter(m) // submit -> verify OK -> result
	enter(m) // Next -> finishQueue -> trains ask for groq
	v := viewText(m)
	if !strings.Contains(v, "may train") {
		t.Fatalf("expected trains ask:\n%s", v)
	}
	press(m, "2") // Skip groq -> combo run
	for i := 0; i < 10 && !strings.Contains(viewText(m), "free/auto targets"); i++ {
		enter(m)
	}
	v = viewText(m)
	if !strings.Contains(v, "openrouter/auto") {
		t.Fatalf("expected combo targets:\n%s", v)
	}
	if be.allowTr["groq"] {
		t.Fatal("groq was allowed but user skipped")
	}
}

func TestSkillsScreenInstall(t *testing.T) {
	be := &fakeBackend{
		snap:    snapBase(),
		install: []skills.Result{{Target: "claude", Skill: "tsunagi-free", State: "installed"}},
	}
	m := newTestApp(t, be, false)
	press(m, "3") // Skills
	v := viewText(m)
	if !strings.Contains(v, "Claude Code") || !strings.Contains(v, "Codex") {
		t.Fatalf("skills screen missing targets:\n%s", v)
	}
	enter(m) // confirm both preselected
	// results stage shows rows then Done
	for i := 0; i < 5 && !strings.Contains(viewText(m), "Done"); i++ {
		enter(m)
	}
	if !strings.Contains(viewText(m), "claude/tsunagi-free") {
		t.Fatalf("expected install rows:\n%s", viewText(m))
	}
}

func TestSkillsForeignSkippedShown(t *testing.T) {
	be := &fakeBackend{
		snap:    snapBase(),
		install: []skills.Result{{Target: "claude", Skill: "tsunagi-free", State: "skipped", Reason: "foreign"}},
	}
	m := newTestApp(t, be, false)
	press(m, "3")
	enter(m)
	for i := 0; i < 5 && !strings.Contains(viewText(m), "Done"); i++ {
		enter(m)
	}
	v := viewText(m)
	if !strings.Contains(v, "skipped") || !strings.Contains(v, "foreign") {
		t.Fatalf("expected skipped (foreign):\n%s", v)
	}
}

func TestSmallTerminalNoWideLines(t *testing.T) {
	for _, sz := range [][2]int{{40, 12}, {60, 20}, {100, 30}} {
		o := testOpts
		o.W, o.H = sz[0], sz[1]
		be := &fakeBackend{snap: snapBase()}
		m, err := New(o, be, false)
		if err != nil {
			t.Fatal(err)
		}
		send(m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		for _, line := range strings.Split(viewText(m), "\n") {
			if n := len([]rune(strings.TrimRight(line, " "))); n > sz[0] {
				t.Fatalf("%dx%d: line wider than terminal (%d): %q", sz[0], sz[1], n, line)
			}
		}
	}
}

func TestMonoThemeNoColorEscapes(t *testing.T) {
	be := &fakeBackend{snap: snapBase()}
	m := newTestApp(t, be, false)
	press(m, "2")
	for _, line := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(line, "\x1b[") {
			t.Fatalf("color escape in mono theme: %q", line)
		}
	}
}

func TestEscSkipsWizardSteps(t *testing.T) {
	snap := snapBase()
	snap.Onboarded = false
	snap.Providers = []ProviderRow{{Name: "openrouter", Group: "free", Tag: "private"}}
	snap.Skills = nil
	be := &fakeBackend{snap: snap}
	m := newTestApp(t, be, true)
	enter(m) // continue -> providers
	esc(m)   // quit checklist -> wizSkip -> finishWizard -> menu
	if !be.onboarded {
		t.Fatal("esc at providers did not finish wizard")
	}
	if !strings.Contains(viewText(m), "Start Claude Code") {
		t.Fatalf("expected menu:\n%s", viewText(m))
	}
}

func TestChatGPTCancel(t *testing.T) {
	snap := snapBase()
	snap.Providers = []ProviderRow{{Name: "chatgpt", Group: "plan", Tag: "plan"}}
	started := make(chan struct{})
	be := &fakeBackend{
		snap: snap,
		signIn: func(ctx context.Context, account string, onURL func(string)) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	m := newTestApp(t, be, false)
	press(m, "2")
	space(m) // select chatgpt
	enter(m) // confirm -> startChat
	<-started
	if !strings.Contains(viewText(m), "signing in") {
		t.Fatalf("expected chat spinner:\n%s", viewText(m))
	}
	esc(m) // cancel -> back to list
	if !strings.Contains(viewText(m), "chatgpt") {
		t.Fatalf("expected providers list after cancel:\n%s", viewText(m))
	}
}
