package app

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/free"
	"github.com/silent9669/tsunagi/internal/plans"
	"github.com/silent9669/tsunagi/internal/tui"
)

// providersScreen is the searchable provider browser plus the per-provider
// setup flow: a search field with filter chips and sections, a detail card,
// then a vertical rail through the flow's steps.
type providersScreen struct {
	opts   tui.Options
	th     tui.Theme
	app    *App
	wizard bool
	w, h   int

	stage  int
	search tui.Input // psList search box, always focused
	list   tui.List
	menu   tui.Menu
	in     tui.Input // stage inputs (key, account id, gateway)

	chip    int
	envSkip map[string]bool
	envProv string

	pendingKey   string // a pasted or detected key, masked everywhere
	pendingNames []string
	pendingErr   error
	pendingAsk   bool // no candidate: ask which provider it is for

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
	menuTop    int   // body rows above the menu, for mouse clicks
	mtop       int   // body rows above the list, for mouse clicks
	chipY      int   // chip row in body coordinates, for mouse clicks
	chipX      []int // chip cell x starts, for mouse clicks
}

const (
	psList = iota
	psDetail
	psEnv
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

const (
	chipAll = iota
	chipFree
	chipPlans
	chipReady
	chipPaid
	chipLocal
)

func groupTitle(g string) string {
	switch g {
	case "free":
		return "Free"
	case "plan":
		return "Plans"
	case "paid":
		return "Paid"
	case "local":
		return "Local"
	}
	return "Catalogue"
}

func newProvidersScreen(a *App, wizard bool) *providersScreen {
	f := a.frame(&providersScreen{})
	s := &providersScreen{opts: a.opts, th: a.theme, app: a, wizard: wizard, w: f.bodyW, h: f.bodyH, chip: -1}
	s.build()
	return s
}

func (s *providersScreen) setSnap(snap Snapshot) { s.app.snap = snap }
func (s *providersScreen) setLocals(m map[string]bool) {
	s.localUp = m
	if s.stage == psList {
		s.rebuild()
	}
}
func (s *providersScreen) wizStage() (int, bool) { return 0, s.wizard }

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

func (s *providersScreen) display(p ProviderRow) string {
	if e := registryEntry(p.Name); e != nil {
		return e.Name
	}
	if e, ok := plans.Find(p.Name); ok {
		return e.Name
	}
	return p.Name
}

func (s *providersScreen) ready(p ProviderRow) bool {
	if p.Group == "local" && s.localUp != nil {
		return s.localUp[p.Name]
	}
	return p.Ready
}

// chipOK reports whether the provider belongs to the active filter chip.
func (s *providersScreen) chipOK(p ProviderRow) bool {
	switch s.chip {
	case chipFree:
		return p.Group == "free"
	case chipPlans:
		return p.Group == "plan"
	case chipReady:
		return s.ready(p)
	case chipPaid:
		return p.Group == "paid"
	case chipLocal:
		return p.Group == "local"
	}
	return true
}

func (s *providersScreen) glyphs() (check, open string) {
	if s.opts.UTF8 {
		return "✓", "○"
	}
	return "x", "o"
}

// badges is the group badge slot then the data/access slots, in that order.
func (s *providersScreen) badges(p ProviderRow) []string {
	var b []string
	if p.FreeEntry {
		b = append(b, "free")
	} else if g := p.Group; g == "plan" || g == "paid" || g == "local" {
		b = append(b, g)
	}
	switch p.Tag {
	case "private", "trains", "unknown":
		b = append(b, p.Tag)
	}
	if p.Keyless {
		b = append(b, "keyless")
	}
	if p.Plan != nil && !p.Plan.Verified {
		b = append(b, "unverified")
	}
	return b
}

func (s *providersScreen) provRow(p ProviderRow, keyfor bool) tui.ListRow {
	check, open := s.glyphs()
	glyph := open
	if s.ready(p) {
		glyph = check
	}
	value := p.Name
	if keyfor {
		value = "keyfor/" + p.Name
	}
	note := p.Limits
	if p.Plan != nil {
		note = p.Plan.Terms
		if p.Plan.Verified {
			note = p.Plan.Price
			if p.Plan.Quota != "" {
				note += " · " + p.Plan.Quota
			}
		}
	}
	return tui.ListRow{
		Kind: tui.RowItem, Label: s.display(p), Value: value,
		Glyph: glyph, Badges: s.badges(p), Note: note, NoteCol: true,
	}
}

func (s *providersScreen) matchOK(p ProviderRow, q string) bool {
	if q == "" {
		return true
	}
	text := p.Name + " " + s.display(p)
	return tui.RankMatch(q, text) > 0
}

// flatRows is the one ranked list used with a query or a non-All chip:
// ready first, then free-registry order, then the rest alphabetically.
func (s *providersScreen) flatRows(q string, keyfor bool) []tui.ListRow {
	var ready, freeRest, other []ProviderRow
	for _, p := range s.app.snap.Providers {
		if !keyfor && !s.chipOK(p) {
			continue
		}
		if !s.matchOK(p, q) {
			continue
		}
		switch {
		case s.ready(p):
			ready = append(ready, p)
		case p.Group == "free":
			freeRest = append(freeRest, p)
		default:
			other = append(other, p)
		}
	}
	verified := func(p ProviderRow) bool { return p.Plan != nil && p.Plan.Verified }
	sort.SliceStable(other, func(i, j int) bool {
		if vi, vj := verified(other[i]), verified(other[j]); vi != vj {
			return vi
		}
		return other[i].Name < other[j].Name
	})
	var rows []tui.ListRow
	for _, g := range [][]ProviderRow{ready, freeRest, other} {
		for _, p := range g {
			rows = append(rows, s.provRow(p, keyfor))
		}
	}
	return rows
}

// sectionRows is the All-chip, empty-query layout: Ready, Plans, Free,
// Local, Paid, then a capped More section for catalogue providers.
func (s *providersScreen) sectionRows() []tui.ListRow {
	var rows []tui.ListRow
	head := func(t string) {
		if len(rows) > 0 {
			rows = append(rows, tui.ListRow{Kind: tui.RowDim})
		}
		rows = append(rows, tui.ListRow{Kind: tui.RowHead, Label: t})
	}
	var ready, freeRest, plan, local, paid, catalog []ProviderRow
	for _, p := range s.app.snap.Providers {
		switch {
		case p.Group == "":
			catalog = append(catalog, p)
		case s.ready(p) && p.Group != "local":
			ready = append(ready, p)
		case p.Group == "free":
			freeRest = append(freeRest, p)
		case p.Group == "plan":
			plan = append(plan, p)
		case p.Group == "local":
			local = append(local, p)
		default:
			paid = append(paid, p)
		}
	}
	emit := func(title string, ps []ProviderRow) {
		if len(ps) == 0 {
			return
		}
		head(title)
		for _, p := range ps {
			rows = append(rows, s.provRow(p, false))
		}
	}
	emit("Ready", ready)
	emit("Plans", plan)
	emit("Free", freeRest)
	emit("Local", local)
	emit("Paid", paid)
	if len(catalog) > 0 {
		sort.SliceStable(catalog, func(i, j int) bool { return catalog[i].Name < catalog[j].Name })
		head("More")
		for i, p := range catalog {
			if i >= 8 {
				rows = append(rows, tui.ListRow{Kind: tui.RowDim, Label: itoa(len(catalog)-i) + " more, type to search"})
				break
			}
			rows = append(rows, s.provRow(p, false))
		}
	}
	return rows
}

// listRows builds the whole list for the current state: env rows, the paste
// flow, or the chip/query provider list plus the trailing actions.
func (s *providersScreen) listRows() []tui.ListRow {
	q := strings.TrimSpace(s.search.Value())
	var rows []tui.ListRow
	switch {
	case s.pendingErr != nil:
		return []tui.ListRow{{Kind: tui.RowDim, Label: s.pendingErr.Error()}}
	case s.pendingKey != "" && !s.pendingAsk:
		for _, n := range s.pendingNames {
			disp := n
			if p, ok := s.rowProvider(n); ok {
				disp = s.display(p)
			}
			rows = append(rows, tui.ListRow{Kind: tui.RowItem, Label: "Looks like " + disp + " - use this key", Value: "keyfor/" + n})
		}
		return rows
	}
	if q == "" && !s.pendingAsk {
		if env := s.envRows(); len(env) > 0 {
			rows = append(rows, tui.ListRow{Kind: tui.RowHead, Label: "Found in your environment"})
			rows = append(rows, env...)
		}
	}
	if s.pendingAsk {
		rows = append(rows, tui.ListRow{Kind: tui.RowDim, Label: "Which provider is this key for?"})
	}
	if s.chip != chipAll || q != "" || s.pendingAsk {
		rows = append(rows, s.flatRows(q, s.pendingAsk)...)
	} else {
		rows = append(rows, s.sectionRows()...)
	}
	if !s.pendingAsk {
		var actions []tui.ListRow
		actions = append(actions,
			tui.ListRow{Kind: tui.RowItem, Label: "Add a gateway", Value: "@add-gateway"},
			tui.ListRow{Kind: tui.RowItem, Label: "Import from opencode", Value: "@import"},
		)
		if s.wizard {
			actions = append(actions, tui.ListRow{Kind: tui.RowItem, Label: "Continue", Value: "@continue"})
		} else {
			actions = append(actions, tui.ListRow{Kind: tui.RowItem, Label: "Back to menu", Value: "@back"})
		}
		if q != "" {
			var kept []tui.ListRow
			for _, a := range actions {
				if tui.RankMatch(q, a.Label) > 0 {
					kept = append(kept, a)
				}
			}
			actions = kept
		}
		if len(actions) > 0 {
			rows = append(rows, tui.ListRow{Kind: tui.RowDim}, tui.ListRow{Kind: tui.RowHead, Label: "Other"})
			rows = append(rows, actions...)
		}
	}
	if q != "" && !s.pendingAsk {
		has := false
		for _, r := range rows {
			if r.Kind == tui.RowItem {
				has = true
				break
			}
		}
		if !has {
			rows = append(rows, tui.ListRow{Kind: tui.RowDim, Label: "No match"})
		}
	}
	return rows
}

func (s *providersScreen) envRows() []tui.ListRow {
	arrow := "->"
	if s.opts.UTF8 {
		arrow = "↳"
	}
	var rows []tui.ListRow
	for _, e := range s.app.be.EnvKeys() {
		if s.envSkip[e.Provider] {
			continue
		}
		rows = append(rows, tui.ListRow{
			Kind: tui.RowItem, Label: e.Provider, Value: "env/" + e.Provider,
			Glyph: arrow, Note: "$" + e.Var + "  import", NoteCol: true,
		})
	}
	return rows
}

func (s *providersScreen) rebuild() {
	s.list.SetRows(s.listRows())
}

func (s *providersScreen) build() {
	s.search = tui.NewInput("Search "+itoa(len(s.app.snap.Providers))+" providers, or paste a key", false, s.th, s.opts.UTF8)
	s.search = fit(s.search, s.w-6, 1).(tui.Input)
	s.list = fit(tui.NewList(s.th, s.opts.UTF8), s.w, s.listH()).(tui.List)
	if s.chip < 0 {
		s.chip = chipAll
		any := false
		for _, p := range s.app.snap.Providers {
			if s.ready(p) && !p.Keyless && p.Group != "local" {
				any = true
			}
		}
		if !any {
			s.chip = chipFree
		}
	}
	s.rebuild()
}

// listH is the row budget for the list under the field and chip rows.
func (s *providersScreen) listH() int {
	fr := 2
	if tui.CardsOK(s.w, s.h) {
		fr = 4
	}
	return max(s.h-fr-2, 1)
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
	switch s.stage {
	case psGWName, psGWURL:
		return []string{"Name", "URL", "Done"}
	}
	switch {
	case s.prov.Group == "plan" && s.prov.Plan == nil:
		return []string{"Sign in", "Verify", "Done"}
	case s.prov.Group == "local":
		return []string{"Verify", "Done"}
	case s.entry != nil && (s.entry.Signup != "" || s.entry.Setup != ""):
		return []string{"Account", "Key", "Verify", "Done"}
	case s.prov.Plan != nil && s.prov.Plan.Signup != "":
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
	case psGWName:
		return 0
	case psGWURL:
		return 1
	case psAcctID:
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
		if s.stage == psList {
			s.list = fit(s.list, s.w, s.listH()).(tui.List)
		}
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
	switch v := msg.(type) {
	case tea.PasteMsg:
		s.onPaste(v.Content)
		s.rebuild()
		return s, nil
	case tea.MouseClickMsg:
		mo := tea.Mouse(v)
		if mo.Y == s.chipY {
			s.clickChip(mo.X)
			s.rebuild()
			s.list.Reset()
			return s, nil
		}
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
		return s.listChosen(v.Value)
	case tea.KeyPressMsg:
		switch {
		case v.Code == tea.KeyEscape || (v.Code == 'c' && v.Mod&tea.ModCtrl != 0):
			return s, s.escList()
		case v.Code == tea.KeyLeft || v.Code == tea.KeyRight || v.Code == tea.KeyTab:
			d := 1
			if v.Code == tea.KeyLeft {
				d = -1
			}
			s.chip = (s.chip + d + 5) % 5
			s.rebuild()
			s.list.Reset()
			return s, nil
		case v.Code == tea.KeyEnter || v.Code == tea.KeyUp || v.Code == tea.KeyDown ||
			v.Code == tea.KeyPgUp || v.Code == tea.KeyPgDown ||
			v.Code == tea.KeyHome || v.Code == tea.KeyEnd:
			nm, cmd := s.list.Update(msg)
			s.list = nm.(tui.List)
			return s, cmd
		}
	}
	if (s.pendingKey != "" && !s.pendingAsk) || s.pendingErr != nil {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyBackspace {
			s.clearPaste()
			s.rebuild()
		}
		return s, nil
	}
	nm, cmd := s.search.Update(msg)
	s.search = nm.(tui.Input)
	if _, ok := msg.(tea.KeyPressMsg); ok {
		s.detectKey()
	}
	s.rebuild()
	s.list.Reset()
	return s, cmd
}

func (s *providersScreen) escList() tea.Cmd {
	if s.pendingKey != "" || s.pendingErr != nil {
		s.clearPaste()
		s.rebuild()
		return nil
	}
	if s.search.Value() != "" {
		s.search.SetValue("")
		s.rebuild()
		s.list.Reset()
		return nil
	}
	if s.wizard {
		return func() tea.Msg { return wizSkipMsg{} }
	}
	return s.leave()
}

// detectKey treats a long spaceless search value as a pasted key when the
// detector recognises its prefix.
func (s *providersScreen) detectKey() {
	v := strings.TrimSpace(s.search.Value())
	if len(v) < 24 || strings.ContainsAny(v, " \t") {
		return
	}
	names, err := s.app.be.DetectKey(v)
	if err != nil || len(names) > 0 {
		s.pendingKey, s.pendingNames, s.pendingErr = v, names, err
	}
}

// onPaste holds a bracketed paste as a key candidate: candidate rows when it
// has a known prefix, an ask row when it does not.
func (s *providersScreen) onPaste(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	names, err := s.app.be.DetectKey(text)
	s.pendingKey = text
	s.pendingNames = names
	s.pendingErr = err
	s.pendingAsk = err == nil && len(names) == 0
	s.search.SetValue("")
}

func (s *providersScreen) clearPaste() {
	s.pendingKey = ""
	s.pendingNames = nil
	s.pendingErr = nil
	s.pendingAsk = false
	s.search.SetValue("")
}

// listChosen starts what picking a row means.
func (s *providersScreen) listChosen(v string) (tea.Model, tea.Cmd) {
	switch {
	case strings.HasPrefix(v, "env/"):
		s.envProv = strings.TrimPrefix(v, "env/")
		s.menu = s.simpleMenu("Import", "Skip")
		s.stage = psEnv
		return s, nil
	case strings.HasPrefix(v, "keyfor/"):
		return s.usePasted(strings.TrimPrefix(v, "keyfor/"))
	case v == "@continue" || v == "@back":
		return s, s.leave()
	case v == "@add-gateway":
		s.stage = psGWName
		s.in = s.newInput("gateway name", false)
		return s, nil
	case v == "@import":
		return s, func() tea.Msg {
			sum, err := s.app.be.ImportOpencode()
			if err != nil {
				sum = err.Error()
			}
			return importDoneMsg{summary: sum}
		}
	}
	return s.open(v)
}

// usePasted stores the held key for the chosen provider and goes straight to
// verify; the key is wiped from memory right after SaveKey returns.
func (s *providersScreen) usePasted(name string) (tea.Model, tea.Cmd) {
	p, ok := s.rowProvider(name)
	if !ok {
		return s, nil
	}
	s.prov, s.entry, s.account, s.result = p, registryEntry(name), name, VerifyResult{}
	err := s.app.be.SaveKey(name, name, s.pendingKey)
	s.clearPaste()
	if err != nil {
		s.result = VerifyResult{Err: err.Error()}
		s.menu = s.resultMenu()
		s.stage = psResult
		return s, nil
	}
	return s, s.verifyCmd()
}

// leave ends the visit: when a free provider was just set up it first offers
// the free/auto combo; the wizard also skips ahead.
func (s *providersScreen) leave() tea.Cmd {
	if s.keyedFree {
		return s.finishQueue()
	}
	return s.done()
}

// open shows the provider's detail card.
func (s *providersScreen) open(value string) (tea.Model, tea.Cmd) {
	p, ok := s.rowProvider(value)
	if !ok {
		return s, nil
	}
	s.prov, s.entry, s.account, s.result = p, registryEntry(value), p.Name, VerifyResult{}
	s.stage = psDetail
	s.menu = s.detailMenu()
	return s, nil
}

// detailMenu is the action list under the detail card.
func (s *providersScreen) detailMenu() tui.Menu {
	p := s.prov
	hasKey := p.Ready && len(p.Accounts) > 0
	var ls []string
	switch {
	case p.Group == "plan" && p.Plan == nil:
		if p.Ready {
			ls = []string{"Test again", "Sign in again", "Remove", "Back"}
		} else {
			ls = []string{"Sign in", "Back"}
		}
	case p.Keyless && !hasKey:
		ls = []string{"Use now, no key needed", "Add a key (optional)"}
	case p.Ready:
		ls = []string{"Test again", "Replace key", "Remove"}
	case p.Group == "local":
		ls = []string{"Test connection", "I have a key"}
	case p.Plan != nil && !p.Plan.Verified:
		ls = []string{"I have a key"}
	default:
		if s.entry != nil || p.Plan != nil {
			ls = []string{"Set up", "I have a key"}
		} else {
			ls = []string{"I have a key"}
		}
	}
	if s.entry != nil && !(p.Group == "plan") {
		if s.entry.Signup != "" {
			ls = append(ls, "Open sign-up")
		}
		if s.entry.KeyURL != "" {
			ls = append(ls, "Open key page")
		}
	}
	if p.Plan != nil && p.Plan.Verified {
		if p.Plan.Signup != "" {
			ls = append(ls, "Open sign-up")
		}
		if p.Plan.KeyURL != "" {
			ls = append(ls, "Open key page")
		}
	}
	if ls[len(ls)-1] != "Back" {
		ls = append(ls, "Back")
	}
	return s.simpleMenu(ls...)
}

// continueSetup moves from the detail card into the flow.
func (s *providersScreen) continueSetup() {
	if s.entry != nil && s.entry.Setup == "account-id" {
		s.stage = psAcctID
		s.in = s.newInput("account id", false)
		return
	}
	s.toKey()
}

func (s *providersScreen) toKey() {
	s.stage = psKey
	s.in = s.newInput("key for "+s.prov.Name, true)
}

func (s *providersScreen) toList() {
	s.stage = psList
	s.app.refresh()
	s.rebuild()
}

// finishQueue offers the free/auto combo when a free provider is keyed.
func (s *providersScreen) finishQueue() tea.Cmd {
	s.trains = nil
	hasFree := false
	for _, p := range s.app.snap.Providers {
		if p.Group == "free" && s.ready(p) {
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

// linkURLs are the sign-up and key pages for the detail card's provider,
// from the plan or the free registry entry.
func (s *providersScreen) linkURLs() (signup, keyPage string) {
	if s.prov.Plan != nil {
		return s.prov.Plan.Signup, s.prov.Plan.KeyURL
	}
	if s.entry != nil {
		return s.entry.Signup, s.entry.KeyURL
	}
	return "", ""
}

func (s *providersScreen) menuChosen(v string) (tea.Model, tea.Cmd) {
	switch s.stage {
	case psDetail:
		signup, keyPage := s.linkURLs()
		switch v {
		case "Set up":
			if signup != "" {
				_ = s.app.be.OpenURL(signup)
			}
			s.continueSetup()
		case "I have a key":
			s.continueSetup()
		case "Use now, no key needed", "Test again", "Test connection":
			return s, s.verifyCmd()
		case "Add a key (optional)", "Replace key":
			s.toKey()
		case "Sign in", "Sign in again":
			return s, s.startChat()
		case "Remove":
			return s, s.doRemove()
		case "Open sign-up":
			if signup != "" {
				_ = s.app.be.OpenURL(signup)
			}
		case "Open key page":
			if keyPage != "" {
				_ = s.app.be.OpenURL(keyPage)
			}
		default:
			s.toList()
		}
	case psEnv:
		if v == "Import" {
			p, _ := s.rowProvider(s.envProv)
			s.prov, s.entry, s.account = p, registryEntry(s.envProv), s.envProv
			if err := s.app.be.ImportEnvKey(s.envProv); err != nil {
				s.result = VerifyResult{Err: err.Error()}
				s.menu = s.resultMenu()
				s.stage = psResult
				return s, nil
			}
			return s, s.verifyCmd()
		}
		if s.envSkip == nil {
			s.envSkip = map[string]bool{}
		}
		s.envSkip[s.envProv] = true
		s.toList()
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
		s.toKey()
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
	case psGWURL:
		s.stage = psGWName
		s.in = s.newInput("gateway name", false)
	case psAcctID, psKey:
		s.stage = psDetail
		s.menu = s.detailMenu()
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

func (s *providersScreen) crumbs() []string {
	c := []string{"Providers"}
	step := map[int]string{
		psAcctID: "Account", psKey: "Key", psVerify: "Verify",
		psResult: "Done", psChat: "Sign in",
	}
	switch s.stage {
	case psDetail:
		c = append(c, s.display(s.prov))
	case psEnv:
		c = append(c, "Import")
	case psGWName, psGWURL:
		c = append(c, "Gateway")
	case psImport:
		c = append(c, "Import")
	case psComboAsk, psComboRun, psComboDone:
		c = append(c, "Free combo")
	default:
		if lbl, ok := step[s.stage]; ok {
			c = append(c, s.display(s.prov), lbl)
		}
	}
	return c
}

func (s *providersScreen) legend() []string {
	switch s.stage {
	case psList:
		return []string{"type|to search", "↑↓|move", "←→|filter", "↵|open", "esc|back"}
	case psAcctID, psGWName, psGWURL, psKey:
		return []string{"↵|confirm", "esc|back"}
	case psVerify, psChat, psComboRun:
		return []string{"esc|cancel"}
	}
	return []string{"↑↓|move", "↵|select", "esc|back"}
}

// railOK reports whether the vertical rail fits; below it the horizontal
// stepper fallback is used.
func (s *providersScreen) railOK() bool {
	return s.app.opts.H >= 16 && s.app.opts.W >= 50
}

func (s *providersScreen) View() tea.View {
	if s.stage == psList {
		return tea.NewView(s.listView())
	}
	var lines []string
	switch s.stage {
	case psDetail, psEnv:
		lines = s.cardView()
	case psComboAsk, psComboDone, psImport:
		lines = s.plainView()
	default:
		if s.railOK() {
			lines = s.railView()
		} else {
			lines = s.stepperView()
		}
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

// listView is the search field, the chip row and the list.
func (s *providersScreen) listView() string {
	var lines []string
	switch {
	case s.pendingErr != nil:
		lines = append(lines, s.maskedField("key refused")...)
	case s.pendingKey != "":
		lines = append(lines, s.maskedField("")...)
	default:
		if tui.CardsOK(s.w, s.h) {
			lines = append(lines, s.search.Boxed(s.w)...)
		} else {
			lines = append(lines, s.search.View().Content)
		}
	}
	lines = append(lines, "")
	s.chipY = len(lines)
	lines = append(lines, s.chipRow())
	s.mtop = len(lines)
	lines = append(lines, s.list.View().Content)
	return strings.Join(lines, "\n")
}

// maskedField renders the held key as "<prefix>••••" - the value itself is
// never drawn.
func (s *providersScreen) maskedField(note string) []string {
	bullet, prompt := "*", ">"
	if s.opts.UTF8 {
		bullet, prompt = "•", "›"
	}
	prefix := s.pendingKey
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	line := s.th.Accent.Render(prompt) + " " + s.th.Text.Render(prefix+strings.Repeat(bullet, 4))
	if note != "" {
		line += " " + s.th.Dim.Render(note)
	}
	if !tui.CardsOK(s.w, s.h) {
		return []string{line}
	}
	return s.th.FieldBox(line, s.w, s.opts.UTF8)
}

// chipRow renders "All 76  Free 11  Plans 8  Ready 4  Paid 10  Local 4"
// with the active chip inverted.
func (s *providersScreen) chipRow() string {
	counts := []struct {
		label string
		n     int
	}{{label: "All", n: len(s.app.snap.Providers)}}
	freeN, planN, readyN, paidN, localN := 0, 0, 0, 0, 0
	for _, p := range s.app.snap.Providers {
		switch p.Group {
		case "free":
			freeN++
		case "plan":
			planN++
		case "paid":
			paidN++
		case "local":
			localN++
		}
		if s.ready(p) {
			readyN++
		}
	}
	counts = append(counts,
		struct {
			label string
			n     int
		}{"Free", freeN},
		struct {
			label string
			n     int
		}{"Plans", planN},
		struct {
			label string
			n     int
		}{"Ready", readyN},
		struct {
			label string
			n     int
		}{"Paid", paidN},
		struct {
			label string
			n     int
		}{"Local", localN},
	)
	s.chipX = s.chipX[:0]
	var parts []string
	x := 0
	for i, c := range counts {
		cell := " " + c.label + " " + itoa(c.n) + " "
		s.chipX = append(s.chipX, x)
		x += lipgloss.Width(cell) + 1
		if i == s.chip {
			parts = append(parts, s.th.Selected.Render(cell))
		} else {
			parts = append(parts, s.th.Dim.Render(cell))
		}
	}
	return strings.Join(parts, " ")
}

// clickChip maps a click on the chip row to a chip.
func (s *providersScreen) clickChip(x int) {
	for i := len(s.chipX) - 1; i >= 0; i-- {
		if x >= s.chipX[i] {
			s.chip = i
			return
		}
	}
}

// cardView draws the detail/import card with its action menu.
func (s *providersScreen) cardView() []string {
	var lines []string
	switch s.stage {
	case psDetail:
		title := s.display(s.prov)
		body := s.detailLines()
		if tui.CardsOK(s.w, s.h) {
			lines = append(lines, s.th.Card(title, s.badges(s.prov), body, s.w, s.opts.UTF8)...)
		} else {
			lines = append(lines, s.th.Title.Render(title))
			lines = append(lines, body...)
		}
	case psEnv:
		p, _ := s.rowProvider(s.envProv)
		title := "Import key"
		if p.Name != "" {
			title = s.display(p)
		}
		v := ""
		for _, e := range s.app.be.EnvKeys() {
			if e.Provider == s.envProv {
				v = e.Var
			}
		}
		body := []string{"Import the key from $" + v + " into your keychain?"}
		if tui.CardsOK(s.w, s.h) {
			lines = append(lines, s.th.Card(title, nil, body, s.w, s.opts.UTF8)...)
		} else {
			lines = append(lines, body...)
		}
	}
	lines = append(lines, "")
	s.menuTop = len(lines)
	lines = append(lines, s.menu.View().Content)
	return lines
}

// detailLines is the metadata under the provider's name: limits, the data
// label and note, needs, source, then the sign-up/key links. Values wrap
// under the label gutter; when height is short the signup/key rows go first,
// then whole rows from the bottom up.
func (s *providersScreen) detailLines() []string {
	type kv struct{ label, value string }
	var rows []kv
	e := s.entry
	if pl := s.prov.Plan; pl != nil {
		src := ""
		if pl.Source != "" {
			src = strings.TrimPrefix(strings.TrimPrefix(pl.Source, "https://"), "http://")
			if i := strings.Index(src, "/"); i > 0 {
				src = src[:i]
			}
			src += "  checked " + pl.Checked
		}
		rows = []kv{
			{"price", pl.Price}, {"quota", pl.Quota}, {"models", pl.Models},
			{"needs", pl.Needs}, {"terms", pl.Terms}, {"source", src},
		}
		if pl.KeyHint != "" {
			rows = append(rows[:5], append([]kv{{"key", "starts with " + pl.KeyHint}}, rows[5:]...)...)
		}
	} else if e == nil {
		state := "not set up"
		if s.ready(s.prov) {
			state = "ready"
		}
		rows = []kv{{"state", state}, {"group", groupTitle(s.prov.Group)}}
	} else {
		data := e.Data
		if e.DataNote != "" {
			data += " - " + e.DataNote
		}
		src := ""
		if e.Source != "" {
			src = strings.TrimPrefix(strings.TrimPrefix(e.Source, "https://"), "http://")
			if i := strings.Index(src, "/"); i > 0 {
				src = src[:i]
			}
			src += "  checked " + e.Checked
		}
		rows = []kv{
			{"limits", e.Limits}, {"data", data}, {"needs", e.Needs},
			{"source", src}, {"signup", e.Signup}, {"key", e.KeyURL},
		}
	}
	var keep []kv
	for _, r := range rows {
		if r.value != "" {
			keep = append(keep, r)
		}
	}
	budget := max(s.h-8, 1)
	valueW := max(s.w-12, 8)
	render := func(rs []kv) []string {
		var out []string
		for _, r := range rs {
			for i, l := range wrapWords(r.value, valueW) {
				if i == 0 {
					out = append(out, s.th.Dim.Render(fmt.Sprintf("%-8s", r.label))+s.th.Text.Render(l))
				} else {
					out = append(out, "        "+s.th.Text.Render(l))
				}
			}
		}
		return out
	}
	// signup/key rows drop first; whole rows drop from the bottom up, never
	// mid-row
	for len(keep) > 0 {
		lines := render(keep)
		if len(lines) <= budget {
			return lines
		}
		keep = keep[:len(keep)-1]
	}
	return nil
}

// wrapWords splits s into lines of at most w cells on word boundaries; a
// word longer than w is hard-wrapped.
func wrapWords(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var lines []string
	var cur strings.Builder
	cw := 0
	flush := func() {
		if cw > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			cw = 0
		}
	}
	for _, word := range strings.Fields(s) {
		for tui.WidthOf(word) > w {
			flush()
			piece := cutCells(word, w)
			lines = append(lines, piece)
			word = word[len(piece):]
		}
		if word == "" {
			continue
		}
		ww := tui.WidthOf(word)
		if cw > 0 && cw+1+ww > w {
			flush()
		}
		if cw > 0 {
			cur.WriteByte(' ')
			cw++
		}
		cur.WriteString(word)
		cw += ww
	}
	flush()
	return lines
}

// cutCells returns the longest prefix of s at most w cells wide.
func cutCells(s string, w int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		rw := tui.WidthOf(string(r))
		if n+rw > w {
			break
		}
		b.WriteRune(r)
		n += rw
	}
	return b.String()
}

// plainView is the non-flow screens: one line plus the menu.
func (s *providersScreen) plainView() []string {
	var lines []string
	switch s.stage {
	case psComboAsk:
		if s.ti < len(s.trains) {
			lines = append(lines, s.th.Warn.Render(s.trains[s.ti]+" may train on your prompts"))
		}
	case psComboDone:
		lines = append(lines, s.th.Text.Render("free/auto targets"))
		for i, t := range s.targets {
			if i >= max(s.h-8, 1) {
				lines = append(lines, s.th.Dim.Render("+"+itoa(len(s.targets)-i)+" more"))
				break
			}
			lines = append(lines, "  "+truncate(t, s.w-3))
		}
	case psImport:
		lines = append(lines, s.th.Text.Render(truncate(s.import_, s.w-1)))
	}
	lines = append(lines, "")
	s.menuTop = len(lines)
	lines = append(lines, s.menu.View().Content)
	return lines
}

// railTitle names the flow at the top of the rail.
func (s *providersScreen) railTitle() string {
	switch s.stage {
	case psGWName, psGWURL:
		return "Add a gateway"
	case psImport:
		return "Import from opencode"
	}
	if s.prov.Group == "plan" {
		return "Sign in " + s.display(s.prov)
	}
	return "Set up " + s.display(s.prov)
}

// railSteps adds the dim result notes to the flow's step list.
func (s *providersScreen) railSteps() []tui.RailStep {
	st := s.steps()
	cur := s.stepIndex()
	out := make([]tui.RailStep, len(st))
	for i, n := range st {
		out[i].Label = n
		if i >= cur {
			continue
		}
		switch n {
		case "Account":
			out[i].Note = "opened"
		case "Key":
			out[i].Note = "saved"
		case "Sign in":
			out[i].Note = "ok"
		case "Verify":
			if s.result.OK {
				out[i].Note = "ok " + truncate(s.result.Model, 20) + " " + itoa(int(s.result.Millis)) + "ms"
			} else if s.result.Err != "" {
				out[i].Note = "fail"
			}
		}
	}
	return out
}

// railBody is the content embedded under the current step.
func (s *providersScreen) railBody() []string {
	bw := max(s.w-4, 8)
	switch s.stage {
	case psAcctID:
		return append([]string{s.th.Dim.Render("Cloudflare account id")}, s.in.Boxed(bw)...)
	case psGWName:
		return append([]string{s.th.Dim.Render("Gateway name")}, s.in.Boxed(bw)...)
	case psGWURL:
		return append([]string{s.th.Dim.Render("Base url")}, s.in.Boxed(bw)...)
	case psKey:
		return append([]string{s.th.Dim.Render("paste your key")}, s.in.Boxed(bw)...)
	case psVerify:
		return []string{spinnerLine(s.th, s.opts.UTF8, "checking "+s.prov.Name)}
	case psChat:
		lines := []string{spinnerLine(s.th, s.opts.UTF8, "signing in with ChatGPT")}
		if s.chatURL != "" {
			lines = append(lines, s.th.Text.Render(truncate(s.chatURL, bw)))
		}
		return lines
	case psResult:
		return []string{s.resultLine(), ""}
	case psImport:
		return []string{s.th.Text.Render(truncate(s.import_, bw))}
	}
	return nil
}

// railView draws the flow as a vertical rail when there is room.
func (s *providersScreen) railView() []string {
	body := s.railBody()
	var menu string
	if s.hasMenu() {
		menu = s.menu.View().Content
	}
	body = append(body, strings.Split(menu, "\n")...)
	lines := s.th.Rail(s.railTitle(), s.railSteps(), s.stepIndex(), body, s.w, s.opts.UTF8)
	if menu != "" {
		s.menuTop = len(lines) - len(strings.Split(menu, "\n"))
	} else {
		s.menuTop = len(lines)
	}
	return lines
}

// stepperView is the compact fallback: the one-line stepper, the facts card
// and the stage content.
func (s *providersScreen) stepperView() []string {
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
	return lines
}

func (s *providersScreen) hasMenu() bool {
	return s.stage == psResult
}

// hasStepper reports whether the compact layout shows the stepper.
func (s *providersScreen) hasStepper() bool {
	switch s.stage {
	case psAcctID, psKey, psVerify, psResult, psChat:
		return true
	}
	return false
}

// card is the provider's facts under the compact layout.
func (s *providersScreen) card() []string {
	switch s.stage {
	case psGWName, psGWURL, psImport:
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
		case psAcctID:
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

// resultMenu picks the default item: Re-enter key on failure. A keyless
// provider with no stored key gets no key actions at all.
func (s *providersScreen) resultMenu() tui.Menu {
	labels := []string{"Done", "Retry"}
	if !(s.prov.Keyless && len(s.prov.Accounts) == 0) {
		labels = append(labels, "Re-enter key", "Remove key")
	}
	initial := 0
	if !s.result.OK {
		initial = len(labels) - 2
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
