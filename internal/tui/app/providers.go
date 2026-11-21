package app

import (
	"context"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/free"
	"github.com/silent9669/tsunagi/internal/tui"
)

// providersScreen is the grouped provider checklist plus the per-provider
// setup flow: facts panel, optional account id, masked key, verify, result.
type providersScreen struct {
	opts   tui.Options
	th     tui.Theme
	app    *App
	wizard bool
	w, h   int

	stage int
	cl    tui.Checklist
	menu  tui.Menu
	in    tui.Input

	// flow state
	prov        ProviderRow
	entry       *free.Entry
	account     string
	result      VerifyResult
	queue       []string
	qi          int
	trains      []string // keyed trains providers awaiting allow/skip
	trainsAllow []string // providers the user allowed this run
	ti          int
	targets     []string
	import_     string
	localUp     map[string]bool
	chatURL     string
	urlCh       chan string
	chatCancel  context.CancelFunc
	gwName      string
	gwURL       string
}

const (
	psList = iota
	psAccountMenu
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
	case "local":
		return "Local and own"
	}
	return "Paid keys"
}

var groupOrder = []string{"free", "plan", "paid", "local"}

func newProvidersScreen(a *App, wizard bool) *providersScreen {
	s := &providersScreen{opts: a.opts, th: a.theme, app: a, wizard: wizard, w: blockWidth(a.opts.W), h: max(4, a.opts.H-2)}
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

// buildSections turns the snapshot into grouped checklist sections.
func (s *providersScreen) buildSections() []tui.Section {
	secs := map[string]*tui.Section{}
	for _, g := range groupOrder {
		secs[g] = &tui.Section{Title: groupTitle(g)}
	}
	for _, p := range s.app.snap.Providers {
		sec := secs[p.Group]
		if sec == nil {
			sec = secs["paid"]
		}
		state := tui.None
		tag := p.Tag
		if p.Ready {
			state = tui.Done
		}
		if p.Group == "local" {
			if s.localUp != nil {
				if s.localUp[p.Name] {
					state = tui.Done
				} else {
					state = tui.None
				}
			} else if p.Ready {
				state = tui.Done
			}
		}
		if p.Setup != "" && tag == "" {
			tag = "setup"
		}
		sec.Items = append(sec.Items, tui.CItem{Label: p.Name, Value: p.Name, Tag: tag, State: state})
	}
	secs["local"].Items = append(secs["local"].Items, tui.CItem{Label: "Add a gateway", Value: "@add-gateway"})
	var out []tui.Section
	for _, g := range groupOrder {
		if len(secs[g].Items) > 0 {
			out = append(out, *secs[g])
		}
	}
	return out
}

func (s *providersScreen) build() {
	s.cl = fit(tui.NewChecklist(s.buildSections(), s.th, s.opts.UTF8), s.w, s.h).(tui.Checklist)
}

func (s *providersScreen) Init() tea.Cmd {
	if s.stage == psList {
		return s.cl.Init()
	}
	return nil
}

func (s *providersScreen) rowProvider(name string) (ProviderRow, bool) {
	for _, p := range s.app.snap.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return ProviderRow{}, false
}

func (s *providersScreen) itemState(value string) tui.State {
	for _, sec := range s.cl.Sections {
		for _, it := range sec.Items {
			if it.Value == value {
				return it.State
			}
		}
	}
	return tui.None
}

func (s *providersScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = v.Width, v.Height
	case verifyDoneMsg:
		s.result = v.result
		s.menu = s.resultMenu()
		s.stage = psResult
		return s, nil
	case comboDoneMsg:
		s.targets = v.targets
		if len(s.targets) == 0 {
			s.stage = psList
			s.build()
			return s, nil
		}
		s.menu = s.simpleMenu("Done")
		s.stage = psComboDone
		return s, nil
	case importDoneMsg:
		s.import_ = v.summary
		s.menu = s.simpleMenu("Done")
		s.stage = psImport
		return s, nil
	case chatDoneMsg:
		if v.err != nil {
			s.result = VerifyResult{Err: v.err.Error()}
			s.menu = s.resultMenu()
			s.stage = psResult
		} else {
			s.account = "chatgpt"
			return s, s.verifyCmd()
		}
		return s, nil
	case chatTickMsg:
		// poll handled in updateChat
	}
	switch s.stage {
	case psList:
		return s.updateList(msg)
	case psVerify:
		if k, ok := msg.(tea.KeyPressMsg); ok {
			code := k.Code
			if code == tea.KeyEscape || code == 'c' && k.Mod&tea.ModCtrl != 0 {
				s.result = VerifyResult{Err: "cancelled"}
				s.menu = s.resultMenu()
				s.stage = psResult
				return s, nil
			}
		}
		return s, nil
	case psChat:
		return s.updateChat(msg)
	case psComboRun:
		if _, ok := msg.(tea.KeyPressMsg); ok {
			s.stage = psList
			s.build()
		}
		return s, nil
	default:
		return s.updateMenuish(msg)
	}
}

func (s *providersScreen) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := s.cl.Update(msg)
	s.cl = nm.(tui.Checklist)
	switch v := msg.(type) {
	case tui.Confirmed:
		// Confirmed lists every non-empty item, including Done ones. Queue the
		// newly selected providers; only when nothing new was picked does a
		// lone Done item open its account menu.
		var pend []string
		for _, val := range v.Values {
			if val == "@add-gateway" {
				s.startGateway()
				return s, nil
			}
			if s.itemState(val) != tui.Done {
				pend = append(pend, val)
			}
		}
		if len(pend) == 0 {
			for _, val := range v.Values {
				if p, ok := s.rowProvider(val); ok && s.itemState(val) == tui.Done {
					s.openAccountMenu(p)
					return s, nil
				}
			}
			return s, cmd
		}
		s.queue = append(s.queue, pend...)
		if len(s.queue) > 0 {
			s.qi = 0
			return s, s.startNext()
		}
	case tui.Quit:
		return s, func() tea.Msg {
			if s.wizard {
				return wizSkipMsg{}
			}
			return providersDoneMsg{}
		}
	}
	return s, cmd
}

// startNext begins the setup flow for queue[qi] and returns any cmd it needs.
func (s *providersScreen) startNext() tea.Cmd {
	name := s.queue[s.qi]
	p, _ := s.rowProvider(name)
	s.prov = p
	s.entry = registryEntry(name)
	s.account = p.Name
	s.result = VerifyResult{}
	switch {
	case p.Group == "plan":
		return s.startChat()
	case p.Group == "local":
		return s.verifyCmd()
	case s.entry != nil && s.entry.Setup == "account-id":
		s.stage = psAcctID
		s.in = s.newInput("account id", false)
	case s.entry != nil && s.entry.Signup != "":
		s.stage = psPanel
		s.menu = s.flowMenu("Open signup page", "I have a key", "Skip")
	default:
		s.stage = psKey
		s.in = s.newInput("key for "+p.Name, true)
	}
	return nil
}

// advance moves to the next queued provider or the combo offer.
func (s *providersScreen) advance() tea.Cmd {
	s.qi++
	if s.qi < len(s.queue) {
		return s.startNext()
	}
	return s.finishQueue()
}

// finishQueue offers the free/auto combo when a free provider is keyed.
func (s *providersScreen) finishQueue() tea.Cmd {
	s.trains = nil
	hasFree := false
	for _, p := range s.app.snap.Providers {
		if p.Group == "free" && p.Ready {
			hasFree = true
			if p.Tag == "trains" {
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
	s.menu = s.flowMenu("Allow "+name+" (trains)", "Skip")
	s.stage = psComboAsk
	return nil
}

func (s *providersScreen) done() tea.Cmd {
	return func() tea.Msg { return providersDoneMsg{} }
}

func (s *providersScreen) newInput(placeholder string, masked bool) tui.Input {
	return fit(tui.NewInput(placeholder, masked, s.th, s.opts.UTF8), s.w, s.h).(tui.Input)
}

func (s *providersScreen) simpleMenu(labels ...string) tui.Menu {
	items := make([]tui.Item, len(labels))
	for i, l := range labels {
		items[i] = tui.Item{Label: l, Value: l}
	}
	return fit(tui.NewMenu(items, 0, s.th, s.opts.UTF8), s.w, s.h).(tui.Menu)
}

func (s *providersScreen) flowMenu(labels ...string) tui.Menu { return s.simpleMenu(labels...) }

func (s *providersScreen) openAccountMenu(p ProviderRow) {
	s.prov = p
	s.menu = s.flowMenu("Replace key", "Remove", "Back")
	s.stage = psAccountMenu
}

func (s *providersScreen) startGateway() {
	s.stage = psGWName
	s.in = s.newInput("gateway name", false)
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
	case tea.WindowSizeMsg:
		s.w, s.h = v.Width, v.Height
	case tui.Chosen:
		return s.menuChosen(v.Value)
	case tui.Submitted:
		return s.inputSubmitted(v.Value)
	case tui.Cancelled:
		return s.inputCancelled()
	case tui.Quit:
		s.stage = psList
		s.build()
	}
	return s, cmd
}

func (s *providersScreen) menuChosen(v string) (tea.Model, tea.Cmd) {
	switch s.stage {
	case psAccountMenu:
		switch v {
		case "Replace key":
			if s.prov.Group == "plan" {
				return s, s.startChat()
			}
			s.stage = psKey
			s.in = s.newInput("key for "+s.prov.Name, true)
		case "Remove":
			return s, s.doRemove()
		default:
			s.stage = psList
			s.build()
		}
	case psPanel:
		switch v {
		case "Open signup page":
			if s.entry != nil && s.entry.Signup != "" {
				_ = s.app.be.OpenURL(s.entry.Signup)
			}
			s.stage = psKey
			s.in = s.newInput("key for "+s.prov.Name, true)
		case "I have a key":
			s.stage = psKey
			s.in = s.newInput("key for "+s.prov.Name, true)
		default:
			return s, s.advance()
		}
	case psResult:
		switch v {
		case "Next":
			return s, s.advance()
		case "Retry":
			return s, s.verifyCmd()
		case "Re-enter key":
			s.stage = psKey
			s.in = s.newInput("key for "+s.prov.Name, true)
		case "Remove key":
			return s, s.doRemove()
		}
	case psComboAsk:
		name := s.trains[s.ti]
		if strings.HasPrefix(v, "Allow ") {
			_ = s.app.be.SetAllowTrains(name, true)
			s.trainsAllow = append(s.trainsAllow, name)
		}
		s.ti++
		return s, s.nextTrainAsk()
	case psComboDone, psImport:
		s.stage = psList
		s.build()
	}
	return s, nil
}

func (s *providersScreen) doRemove() tea.Cmd {
	if len(s.prov.Accounts) > 0 {
		_ = s.app.be.RemoveKey(s.prov.Name, s.prov.Accounts[0])
	}
	s.stage = psList
	s.app.refresh()
	s.build()
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
		s.stage = psKey
		s.in = s.newInput("key for "+name, true)
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
		s.app.refresh()
		s.stage = psList
		s.build()
	case psKey:
		key := v
		if err := s.app.be.SaveKey(s.prov.Name, s.account, key); err != nil {
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
		if s.entry != nil && s.entry.Signup != "" {
			s.stage = psPanel
			s.menu = s.flowMenu("Open signup page", "I have a key", "Skip")
		} else {
			s.stage = psKey
			s.in = s.newInput("key for "+s.prov.Name, true)
		}
	case psGWName:
		s.stage = psList
		s.build()
	case psGWURL:
		s.stage = psGWName
		s.in = s.newInput("gateway name", false)
	case psKey:
		if s.entry != nil && s.entry.Signup != "" {
			s.stage = psPanel
			s.menu = s.flowMenu("Open signup page", "I have a key", "Skip")
		} else {
			s.stage = psList
			s.build()
		}
	default:
		s.stage = psList
		s.build()
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

func (s *providersScreen) updateVerify(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, nil
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
	// drain pending signup URLs without blocking
	for {
		select {
		case u := <-s.urlCh:
			s.chatURL = u
		default:
			goto drained
		}
	}
drained:
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		if v.Code == tea.KeyEscape || v.Code == 'c' && v.Mod&tea.ModCtrl != 0 {
			if s.chatCancel != nil {
				s.chatCancel()
			}
			s.stage = psList
			s.build()
			return s, nil
		}
	case chatDoneMsg:
		// handled in Update
		_ = v
	}
	// poll the url channel on every message by re-dispatching a tick
	return s, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return chatTickMsg{} })
}

func (s *providersScreen) runCombo() tea.Cmd {
	allow := map[string]bool{}
	for _, n := range s.trainsAllow {
		allow[n] = true
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

func (s *providersScreen) View() tea.View {
	var b strings.Builder
	switch s.stage {
	case psList:
		b.WriteString(s.cl.View().Content)
	case psAccountMenu:
		b.WriteString(s.accountLines())
		b.WriteString("\n\n" + s.menu.View().Content)
	case psPanel:
		b.WriteString(s.panelLines())
		b.WriteString("\n\n" + s.menu.View().Content)
	case psAcctID:
		b.WriteString(s.th.Text.Render("Cloudflare account id") + "\n\n" + s.in.View().Content)
	case psGWName:
		b.WriteString(s.th.Text.Render("Gateway name") + "\n\n" + s.in.View().Content)
	case psGWURL:
		b.WriteString(s.th.Text.Render("Base url") + "\n\n" + s.in.View().Content)
	case psKey:
		b.WriteString(s.th.Text.Render(s.prov.Name) + "\n\n" + s.in.View().Content)
	case psVerify:
		b.WriteString(spinnerLine(s.th, s.opts.UTF8, "checking "+s.prov.Name))
	case psResult:
		b.WriteString(s.resultLines())
		b.WriteString("\n\n" + s.resultMenu().View().Content)
	case psChat:
		b.WriteString(s.chatLines())
	case psComboAsk:
		if s.ti < len(s.trains) {
			b.WriteString(s.th.Warn.Render(s.trains[s.ti]+" may train on your prompts") + "\n\n" + s.menu.View().Content)
		}
	case psComboRun:
		b.WriteString(spinnerLine(s.th, s.opts.UTF8, "building free/auto"))
	case psComboDone:
		b.WriteString(s.th.Text.Render("free/auto targets") + "\n")
		for _, t := range s.targets {
			b.WriteString("  " + t + "\n")
		}
		b.WriteString("\n" + s.menu.View().Content)
	case psImport:
		b.WriteString(s.th.Text.Render(s.import_) + "\n\n" + s.menu.View().Content)
	}
	return tea.NewView(b.String())
}

func (s *providersScreen) accountLines() string {
	var b strings.Builder
	b.WriteString(s.th.Title.Render(s.prov.Name) + "\n")
	b.WriteString(s.th.Dim.Render("has "+itoa(len(s.prov.Accounts))+" account(s)") + "\n")
	return b.String()
}

func (s *providersScreen) panelLines() string {
	var b strings.Builder
	name := s.prov.Name
	if s.entry != nil {
		name = s.entry.Name
	}
	b.WriteString(s.th.Title.Render(name) + "\n")
	if s.entry != nil {
		if s.entry.Needs != "" {
			b.WriteString(s.th.Dim.Render("needs  ") + s.entry.Needs + "\n")
		}
		if s.entry.Limits != "" {
			b.WriteString(s.th.Dim.Render("limits ") + s.entry.Limits + "\n")
		}
		dataStyle := s.th.Dim
		switch s.entry.Data {
		case free.Private:
			dataStyle = s.th.OK
		case free.Trains:
			dataStyle = s.th.Warn
		}
		b.WriteString(s.th.Dim.Render("data   ") + dataStyle.Render(s.entry.Data) + "\n")
		if s.entry.Signup != "" {
			b.WriteString(s.th.Dim.Render("signup ") + s.entry.Signup + "\n")
		}
		if s.entry.KeyURL != "" && s.entry.KeyURL != s.entry.Signup {
			b.WriteString(s.th.Dim.Render("key    ") + s.entry.KeyURL + "\n")
		}
	}
	return b.String()
}

func (s *providersScreen) resultLines() string {
	var b strings.Builder
	if s.result.OK {
		b.WriteString(s.th.OK.Render("ok") + "  " + s.result.Model + "  " + itoa(int(s.result.Millis)) + "ms\n")
	} else {
		err := s.result.Err
		if err == "" {
			err = "failed"
		}
		b.WriteString(s.th.Err.Render("fail") + "  " + err + "\n")
	}
	return b.String()
}

// resultMenu picks the default item: Re-enter key on failure.
func (s *providersScreen) resultMenu() tui.Menu {
	labels := []string{"Next", "Retry", "Re-enter key", "Remove key"}
	initial := 0
	if !s.result.OK {
		initial = 2
	}
	items := make([]tui.Item, len(labels))
	for i, l := range labels {
		items[i] = tui.Item{Label: l, Value: l}
	}
	return fit(tui.NewMenu(items, initial, s.th, s.opts.UTF8), s.w, s.h).(tui.Menu)
}

func (s *providersScreen) chatLines() string {
	var b strings.Builder
	b.WriteString(spinnerLine(s.th, s.opts.UTF8, "signing in with ChatGPT") + "\n")
	if s.chatURL != "" {
		b.WriteString(s.th.Text.Render(s.chatURL) + "\n")
	}
	return b.String()
}

func spinnerLine(th tui.Theme, utf8 bool, label string) string {
	sp := tui.NewSpinner(th, utf8)
	return sp.View().Content + " " + label
}

var acctIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
