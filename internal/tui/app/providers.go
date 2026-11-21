package app

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/free"
	"github.com/silent9669/tsunagi/internal/tui"
)

// providersScreen is the searchable provider list plus the per-provider
// setup flow: account, key, verify, done, drawn under a stepper.
type providersScreen struct {
	opts   tui.Options
	th     tui.Theme
	app    *App
	wizard bool
	w, h   int

	stage int
	pk    tui.Picker
	menu  tui.Menu
	in    tui.Input

	// flow state
	prov       ProviderRow
	entry      *free.Entry
	account    string
	result     VerifyResult
	keyedFree  bool // a free provider was verified in this visit
	trains     []string
	ti         int
	targets    []string
	import_    string
	localUp    map[string]bool
	chatURL    string
	urlCh      chan string
	chatCancel context.CancelFunc
	gwName     string
	menuTop    int // body rows above the menu, for mouse clicks
}

const (
	psList = iota
	psDetail
	psPanel
	psAcctID
	psGWName
	psGWURL
	psKey
	psVerify
	psResult
	psChat
	psComboAsk
	psComboRun
	psComboDone
	psImport
)

func groupTitle(g string) string {
	switch g {
	case "free":
		return "Free"
	case "plan":
		return "Your plans"
	case "paid":
		return "Paid keys"
	case "local":
		return "Local and own"
	}
	return "Catalogue"
}

var groupOrder = []string{"free", "plan", "paid", "local", ""}

func newProvidersScreen(a *App, wizard bool) *providersScreen {
	f := a.frame(&providersScreen{})
	s := &providersScreen{opts: a.opts, th: a.theme, app: a, wizard: wizard, w: f.bodyW, h: f.bodyH}
	s.build()
	return s
}

func (s *providersScreen) setSnap(snap Snapshot) { s.app.snap = snap }
func (s *providersScreen) setLocals(m map[string]bool) {
	s.localUp = m
	if s.stage == psList {
		s.build()
	}
}

// registryEntry finds the free-registry facts for a provider name.
func registryEntry(name string) *free.Entry {
	reg := free.Registry()
	for i := range reg {
		if reg[i].ID == name {
			return &reg[i]
		}
	}
	return nil
}

// pickItems turns the snapshot into the picker's rows: the curated providers
// by group, the gateway and import actions, and the whole catalogue behind
// the search.
func (s *providersScreen) pickItems() []tui.PickItem {
	var items []tui.PickItem
	if s.wizard {
		items = append(items, tui.PickItem{Label: "Continue", Value: "@continue", Group: "", Action: true, Featured: true, Glyph: "→", AsciiGlyph: ">"})
	}
	for _, p := range s.app.snap.Providers {
		state := tui.None
		if p.Ready {
			state = tui.Done
		}
		if p.Group == "local" && s.localUp != nil {
			state = tui.None
			if s.localUp[p.Name] {
				state = tui.Done
			}
		}
		tag := p.Tag
		if p.Setup != "" && tag == "" {
			tag = "setup"
		}
		items = append(items, tui.PickItem{
			Label: p.Name, Value: p.Name, Tag: tag, Group: groupTitle(p.Group),
			State: state, Featured: p.Group != "" || p.Ready,
		})
	}
	items = append(items,
		tui.PickItem{Label: "Add a gateway", Value: "@add-gateway", Group: groupTitle("local"), Action: true, Featured: true},
		tui.PickItem{Label: "Import from opencode", Value: "@import", Group: groupTitle("local"), Action: true, Featured: true},
	)
	rank := map[string]int{}
	for i, g := range groupOrder {
		rank[groupTitle(g)] = i
	}
	sort.SliceStable(items, func(i, j int) bool {
		gi, gj := items[i].Group, items[j].Group
		if items[i].Value == "@continue" || items[j].Value == "@continue" {
			return items[i].Value == "@continue" && items[j].Value != "@continue"
		}
		return rank[gi] < rank[gj]
	})
	return items
}

func (s *providersScreen) build() {
	keep := ""
	if it, ok := s.pk.Selected(); ok {
		keep = it.Value
	}
	s.pk = fit(tui.NewPicker(s.pickItems(), s.th, s.opts.UTF8), s.w, s.h).(tui.Picker)
	if keep != "" {
		s.pk.Select(keep)
	}
}

func (s *providersScreen) Init() tea.Cmd { return nil }

func (s *providersScreen) rowProvider(name string) (ProviderRow, bool) {
	for _, p := range s.app.snap.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return ProviderRow{}, false
}

// steps are the stages the flow for the current provider goes through.
func (s *providersScreen) steps() []string {
	switch {
	case s.prov.Group == "plan":
		return []string{"Sign in", "Verify", "Done"}
	case s.prov.Group == "local":
		return []string{"Verify", "Done"}
	case s.entry != nil && (s.entry.Signup != "" || s.entry.Setup != ""):
		return []string{"Account", "Key", "Verify", "Done"}
	}
	return []string{"Key", "Verify", "Done"}
}

// stepIndex is where the current stage sits in steps().
func (s *providersScreen) stepIndex() int {
	st := s.steps()
	find := func(name string) int {
		for i, x := range st {
			if x == name {
				return i
			}
		}
		return 0
	}
	switch s.stage {
	case psPanel, psAcctID:
		return find("Account")
	case psKey:
		return find("Key")
	case psChat:
		return find("Sign in")
	case psVerify:
		return find("Verify")
	case psResult:
		if s.result.OK {
			return len(st) - 1
		}
		return find("Verify")
	}
	return 0
}

func (s *providersScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = v.Width, v.Height
	case verifyDoneMsg:
		s.result = v.result
		if v.result.OK && s.prov.Group == "free" {
			s.keyedFree = true
		}
		s.app.refresh()
		s.menu = s.resultMenu()
		s.stage = psResult
		return s, nil
	case comboDoneMsg:
		s.targets = v.targets
		if len(s.targets) == 0 {
			return s, s.done()
		}
		s.menu = s.simpleMenu("Done")
		s.stage = psComboDone
		return s, nil
	case importDoneMsg:
		s.import_ = v.summary
		s.app.refresh()
		s.menu = s.simpleMenu("Done")
		s.stage = psImport
		return s, nil
	case chatDoneMsg:
		if v.err != nil {
			s.result = VerifyResult{Err: v.err.Error()}
			s.menu = s.resultMenu()
			s.stage = psResult
			return s, nil
		}
		s.account = "chatgpt"
		return s, s.verifyCmd()
	case tea.MouseClickMsg:
		if s.stage != psList {
			mo := tea.Mouse(v)
			mo.Y -= s.menuTop
			if mo.Y < 0 {
				return s, nil
			}
			msg = tea.MouseClickMsg(mo)
		}
	}
	switch s.stage {
	case psList:
		return s.updateList(msg)
	case psVerify:
		if k, ok := msg.(tea.KeyPressMsg); ok {
			if k.Code == tea.KeyEscape || k.Code == 'c' && k.Mod&tea.ModCtrl != 0 {
				s.result = VerifyResult{Err: "cancelled"}
				s.menu = s.resultMenu()
				s.stage = psResult
			}
		}
		return s, nil
	case psChat:
		return s.updateChat(msg)
	case psComboRun:
		if _, ok := msg.(tea.KeyPressMsg); ok {
			return s, s.done()
		}
		return s, nil
	default:
		return s.updateMenuish(msg)
	}
}

func (s *providersScreen) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := s.pk.Update(msg)
	s.pk = nm.(tui.Picker)
	switch v := msg.(type) {
	case tui.Chosen:
		return s.open(v.Value)
	case tui.Quit:
		if s.wizard {
			return s, func() tea.Msg { return wizSkipMsg{} }
		}
		return s, s.leave()
	}
	return s, cmd
}

// leave ends the visit: when a free provider was just set up it first offers
// the free/auto combo; the wizard also skips ahead.
func (s *providersScreen) leave() tea.Cmd {
	if s.keyedFree {
		return s.finishQueue()
	}
	return s.done()
}

// open starts what choosing a row means: an action, the detail of a provider
// that is already set up, or its setup flow.
func (s *providersScreen) open(value string) (tea.Model, tea.Cmd) {
	switch value {
	case "@continue":
		return s, s.leave()
	case "@add-gateway":
		s.stage = psGWName
		s.in = s.newInput("gateway name", false)
		return s, nil
	case "@import":
		return s, func() tea.Msg {
			sum, err := s.app.be.ImportOpencode()
			if err != nil {
				sum = err.Error()
			}
			return importDoneMsg{summary: sum}
		}
	}
	p, ok := s.rowProvider(value)
	if !ok {
		return s, nil
	}
	s.prov, s.entry, s.account, s.result = p, registryEntry(value), p.Name, VerifyResult{}
	ready := p.Ready
	if p.Group == "local" {
		ready = s.localUp[p.Name]
	}
	if ready && p.Group != "local" {
		s.stage = psDetail
		s.menu = s.simpleMenu("Test again", "Replace key", "Remove", "Back")
		if p.Group == "plan" {
			s.menu = s.simpleMenu("Test again", "Sign in again", "Remove", "Back")
		}
		return s, nil
	}
	return s, s.startSetup()
}

// startSetup begins the flow for s.prov and returns any cmd it needs.
func (s *providersScreen) startSetup() tea.Cmd {
	p := s.prov
	switch {
	case p.Group == "plan":
		return s.startChat()
	case p.Group == "local":
		return s.verifyCmd()
	case s.entry != nil && s.entry.Setup == "account-id":
		s.stage = psAcctID
		s.in = s.newInput("account id", false)
	case s.entry != nil && s.entry.Signup != "":
		s.toPanel()
	default:
		s.toKey()
	}
	return nil
}

func (s *providersScreen) toPanel() {
	s.stage = psPanel
	s.menu = s.simpleMenu("Open signup page", "I have a key", "Skip")
}

func (s *providersScreen) toKey() {
	s.stage = psKey
	s.in = s.newInput("key for "+s.prov.Name, true)
}

func (s *providersScreen) toList() {
	s.stage = psList
	s.app.refresh()
	s.build()
}

// finishQueue offers the free/auto combo when a free provider is keyed.
func (s *providersScreen) finishQueue() tea.Cmd {
	s.trains = nil
	hasFree := false
	for _, p := range s.app.snap.Providers {
		if p.Group == "free" && p.Ready {
			hasFree = true
			if p.Tag == "trains" && !s.app.snap.AllowTrains[p.Name] {
				s.trains = append(s.trains, p.Name)
			}
		}
	}
	if !hasFree {
		return s.done()
	}
	s.ti = 0
	return s.nextTrainAsk()
}

func (s *providersScreen) nextTrainAsk() tea.Cmd {
	if s.ti >= len(s.trains) {
		s.stage = psComboRun
		return s.runCombo()
	}
	name := s.trains[s.ti]
	s.menu = s.simpleMenu("Allow "+name+" (trains)", "Skip")
	s.stage = psComboAsk
	return nil
}

func (s *providersScreen) done() tea.Cmd {
	s.keyedFree = false
	return func() tea.Msg { return providersDoneMsg{} }
}

func (s *providersScreen) newInput(placeholder string, masked bool) tui.Input {
	return fit(tui.NewInput(placeholder, masked, s.th, s.opts.UTF8), s.w, 1).(tui.Input)
}

func (s *providersScreen) simpleMenu(labels ...string) tui.Menu {
	items := make([]tui.Item, len(labels))
	for i, l := range labels {
		items[i] = tui.Item{Label: l, Value: l}
	}
	return fit(tui.NewMenu(items, 0, s.th, s.opts.UTF8), s.w, max(s.h-4, 2)).(tui.Menu)
}

// --- sub-state updates ---------------------------------------------------

func (s *providersScreen) updateMenuish(msg tea.Msg) (tea.Model, tea.Cmd) {
	var nm tea.Model
	var cmd tea.Cmd
	switch s.stage {
	case psAcctID, psGWName, psGWURL, psKey:
		nm, cmd = s.in.Update(msg)
		s.in = nm.(tui.Input)
	default:
		nm, cmd = s.menu.Update(msg)
		s.menu = nm.(tui.Menu)
	}
	switch v := msg.(type) {
	case tui.Chosen:
		return s.menuChosen(v.Value)
	case tui.Submitted:
		return s.inputSubmitted(v.Value)
	case tui.Cancelled:
		return s.inputCancelled()
	case tui.Quit:
		s.toList()
	}
	return s, cmd
}

func (s *providersScreen) menuChosen(v string) (tea.Model, tea.Cmd) {
	switch s.stage {
	case psDetail:
		switch v {
		case "Test again":
			return s, s.verifyCmd()
		case "Replace key", "Sign in again":
			if s.prov.Group == "plan" {
				return s, s.startChat()
			}
			s.toKey()
		case "Remove":
			return s, s.doRemove()
		default:
			s.toList()
		}
	case psPanel:
		switch v {
		case "Open signup page":
			if s.entry != nil && s.entry.Signup != "" {
				_ = s.app.be.OpenURL(s.entry.Signup)
			}
			s.toKey()
		case "I have a key":
			s.toKey()
		default:
			s.toList()
		}
	case psResult:
		switch v {
		case "Done":
			s.toList()
		case "Retry":
			return s, s.verifyCmd()
		case "Re-enter key":
			s.toKey()
		case "Remove key":
			return s, s.doRemove()
		}
	case psComboAsk:
		name := s.trains[s.ti]
		if strings.HasPrefix(v, "Allow ") {
			_ = s.app.be.SetAllowTrains(name, true)
			s.app.snap.AllowTrains = withAllowed(s.app.snap.AllowTrains, name)
		}
		s.ti++
		return s, s.nextTrainAsk()
	case psComboDone, psImport:
		if s.stage == psComboDone {
			return s, s.done()
		}
		s.toList()
	}
	return s, nil
}

func withAllowed(m map[string]bool, name string) map[string]bool {
	out := map[string]bool{name: true}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *providersScreen) doRemove() tea.Cmd {
	if len(s.prov.Accounts) > 0 {
		_ = s.app.be.RemoveKey(s.prov.Name, s.prov.Accounts[0])
	}
	s.toList()
	return nil
}

func (s *providersScreen) inputSubmitted(v string) (tea.Model, tea.Cmd) {
	switch s.stage {
	case psAcctID:
		if !acctIDRe.MatchString(v) {
			return s, nil // input stays, user retypes
		}
		name := "cloudflare"
		url := "https://api.cloudflare.com/client/v4/accounts/" + v + "/ai/v1"
		if err := s.app.be.AddGateway(name, url, "openai-chat", false); err != nil {
			s.import_ = err.Error()
			s.menu = s.simpleMenu("Done")
			s.stage = psImport
			return s, nil
		}
		s.account = name
		s.toPanel()
	case psGWName:
		s.gwName = strings.TrimSpace(v)
		if s.gwName == "" {
			return s, nil
		}
		s.stage = psGWURL
		s.in = s.newInput("base url", false)
	case psGWURL:
		url := strings.TrimSpace(v)
		local := strings.HasPrefix(url, "http://127.0.0.1") || strings.HasPrefix(url, "http://localhost")
		if err := s.app.be.AddGateway(s.gwName, url, "", local); err != nil {
			s.import_ = err.Error()
			s.menu = s.simpleMenu("Done")
			s.stage = psImport
			return s, nil
		}
		s.toList()
	case psKey:
		if err := s.app.be.SaveKey(s.prov.Name, s.account, v); err != nil {
			s.result = VerifyResult{Err: err.Error()}
			s.menu = s.resultMenu()
			s.stage = psResult
			return s, nil
		}
		s.in = s.newInput("", true)
		return s, s.verifyCmd()
	}
	return s, nil
}

func (s *providersScreen) inputCancelled() (tea.Model, tea.Cmd) {
	switch s.stage {
	case psAcctID:
		s.toList()
	case psGWName:
		s.toList()
	case psGWURL:
		s.stage = psGWName
		s.in = s.newInput("gateway name", false)
	case psKey:
		if s.entry != nil && s.entry.Signup != "" {
			s.toPanel()
		} else {
			s.toList()
		}
	default:
		s.toList()
	}
	return s, nil
}

// --- verify / chat / combo ------------------------------------------------

type verifyDoneMsg struct{ result VerifyResult }
type comboDoneMsg struct{ targets []string }
type importDoneMsg struct{ summary string }
type chatDoneMsg struct {
	email string
	err   error
}
type chatTickMsg struct{}

// verifyCmd runs Backend.Verify for the current provider.
func (s *providersScreen) verifyCmd() tea.Cmd {
	s.stage = psVerify
	name := s.prov.Name
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		return verifyDoneMsg{result: s.app.be.Verify(ctx, name)}
	}
}

// startChat begins ChatGPT sign-in; the URL arrives via urlCh so View can
// show it while the browser flow runs.
func (s *providersScreen) startChat() tea.Cmd {
	s.stage = psChat
	s.chatURL = ""
	s.urlCh = make(chan string, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.chatCancel = cancel
	account := s.account
	if account == "" {
		account = s.prov.Name
	}
	return func() tea.Msg {
		email, err := s.app.be.SignInChatGPT(ctx, account, func(u string) {
			select {
			case s.urlCh <- u:
			default:
			}
		})
		cancel()
		return chatDoneMsg{email: email, err: err}
	}
}

func (s *providersScreen) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	for {
		select {
		case u := <-s.urlCh:
			s.chatURL = u
			continue
		default:
		}
		break
	}
	if v, ok := msg.(tea.KeyPressMsg); ok {
		if v.Code == tea.KeyEscape || v.Code == 'c' && v.Mod&tea.ModCtrl != 0 {
			if s.chatCancel != nil {
				s.chatCancel()
			}
			s.toList()
			return s, nil
		}
	}
	// poll the url channel on every message by re-dispatching a tick
	return s, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return chatTickMsg{} })
}

func (s *providersScreen) runCombo() tea.Cmd {
	allow := map[string]bool{}
	for n, ok := range s.app.snap.AllowTrains {
		if ok {
			allow[n] = true
		}
	}
	return func() tea.Msg {
		targets, err := s.app.be.BuildFreeCombo(allow)
		if err != nil {
			return comboDoneMsg{}
		}
		return comboDoneMsg{targets: targets}
	}
}

// --- View ------------------------------------------------------------------

func (s *providersScreen) title() string { return "Providers" }

func (s *providersScreen) legend() []string {
	switch s.stage {
	case psList:
		return []string{"type|to search", "↑↓|move", "↵|open", "esc|back"}
	case psAcctID, psGWName, psGWURL, psKey:
		return []string{"↵|confirm", "esc|back"}
	case psVerify, psChat, psComboRun:
		return []string{"esc|cancel"}
	}
	return []string{"↑↓|move", "↵|select", "1-9|jump", "esc|back"}
}

func (s *providersScreen) View() tea.View {
	if s.stage == psList {
		return tea.NewView(s.pk.View().Content)
	}
	var lines []string
	if s.hasStepper() {
		lines = append(lines, tui.Stepper(s.steps(), s.stepIndex(), s.w, s.th, s.opts.UTF8), "")
	}
	lines = append(lines, s.card()...)
	var content []string
	switch s.stage {
	case psAcctID:
		content = []string{s.th.Text.Render("Cloudflare account id"), s.in.View().Content}
	case psGWName:
		content = []string{s.th.Text.Render("Gateway name"), s.in.View().Content}
	case psGWURL:
		content = []string{s.th.Text.Render("Base url"), s.in.View().Content}
	case psKey:
		content = []string{s.th.Text.Render(s.prov.Name + " key"), s.in.View().Content}
	case psVerify:
		content = []string{spinnerLine(s.th, s.opts.UTF8, "checking "+s.prov.Name)}
	case psChat:
		content = []string{spinnerLine(s.th, s.opts.UTF8, "signing in with ChatGPT")}
		if s.chatURL != "" {
			content = append(content, s.th.Text.Render(s.chatURL))
		}
	case psComboRun:
		content = []string{spinnerLine(s.th, s.opts.UTF8, "building free/auto")}
	case psResult:
		content = []string{s.resultLine()}
	case psComboAsk:
		if s.ti < len(s.trains) {
			content = []string{s.th.Warn.Render(s.trains[s.ti] + " may train on your prompts")}
		}
	case psComboDone:
		content = []string{s.th.Text.Render("free/auto targets")}
		for i, t := range s.targets {
			if i >= max(s.h-8, 1) {
				content = append(content, s.th.Dim.Render("+"+itoa(len(s.targets)-i)+" more"))
				break
			}
			content = append(content, "  "+truncate(t, s.w-3))
		}
	case psImport:
		content = []string{s.th.Text.Render(truncate(s.import_, s.w-1))}
	}
	if len(content) > 0 && len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, content...)
	if s.hasMenu() {
		lines = append(lines, "")
		s.menuTop = len(lines)
		lines = append(lines, s.menu.View().Content)
	} else {
		s.menuTop = len(lines)
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

func (s *providersScreen) hasMenu() bool {
	switch s.stage {
	case psDetail, psPanel, psResult, psComboAsk, psComboDone, psImport:
		return true
	}
	return false
}

// hasStepper reports whether the stepper is shown: for a provider's own
// flow, not for its detail, the gateway form or the combo questions.
func (s *providersScreen) hasStepper() bool {
	switch s.stage {
	case psPanel, psAcctID, psKey, psVerify, psResult, psChat:
		return true
	}
	return false
}

// card is the provider's facts above the flow: name and the lines that fit.
func (s *providersScreen) card() []string {
	switch s.stage {
	case psGWName, psGWURL, psComboAsk, psComboRun, psComboDone, psImport:
		return nil
	}
	name := s.prov.Name
	if s.entry != nil {
		name = s.entry.Name
	}
	lines := []string{s.th.Title.Render(name)}
	budget := s.h - 9 // stepper, spacing, content and menu
	add := func(label, value string, style func(string) string) {
		if budget <= 0 || value == "" {
			return
		}
		budget--
		room := max(s.w-len(label)-2, 4)
		lines = append(lines, s.th.Dim.Render(label)+" "+style(truncate(value, room)))
	}
	plain := func(x string) string { return s.th.Text.Render(x) }
	if e := s.entry; e != nil {
		add("limits", e.Limits, plain)
		dataStyle := plain
		switch e.Data {
		case free.Private:
			dataStyle = func(x string) string { return s.th.OK.Render(x) }
		case free.Trains:
			dataStyle = func(x string) string { return s.th.Warn.Render(x) }
		}
		add("data  ", e.Data+" - "+e.DataNote, dataStyle)
		add("needs ", e.Needs, plain)
		switch s.stage {
		case psPanel, psAcctID:
			add("signup", e.Signup, plain)
		case psKey:
			add("key   ", e.KeyURL, plain)
		}
	}
	return lines
}

func (s *providersScreen) resultLine() string {
	if s.result.OK {
		return s.th.OK.Render("ok") + "  " + truncate(s.result.Model, s.w-14) + "  " + itoa(int(s.result.Millis)) + "ms"
	}
	err := s.result.Err
	if err == "" {
		err = "failed"
	}
	return s.th.Err.Render("fail") + "  " + truncate(err, s.w-7)
}

// resultMenu picks the default item: Re-enter key on failure.
func (s *providersScreen) resultMenu() tui.Menu {
	labels := []string{"Done", "Retry", "Re-enter key", "Remove key"}
	initial := 0
	if !s.result.OK {
		initial = 2
	}
	items := make([]tui.Item, len(labels))
	for i, l := range labels {
		items[i] = tui.Item{Label: l, Value: l}
	}
	return fit(tui.NewMenu(items, initial, s.th, s.opts.UTF8), s.w, max(s.h-4, 2)).(tui.Menu)
}

func spinnerLine(th tui.Theme, utf8 bool, label string) string {
	sp := tui.NewSpinner(th, utf8)
	return sp.View().Content + " " + label
}

var acctIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
