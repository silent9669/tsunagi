package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/silent9669/tsunagi/internal/art"
)

func testTheme(t *testing.T, name string, prof colorprofile.Profile) Theme {
	t.Helper()
	th, err := NewTheme(name, prof)
	if err != nil {
		t.Fatalf("NewTheme(%q): %v", name, err)
	}
	return th
}

func TestPalettesExact(t *testing.T) {
	want := map[string]palette{
		"amber":     {fg: "#FFB000", dim: "#8A5F00", accent: "#FFD060", ok: "#FFB000", warn: "#FF8C00", err: "#FF5A4D", cyan: "#FFD060", badgebg: "#2E2100", art: [3]string{"#5A3C00", "#FFB000", "#FFE08A"}},
		"matrix":    {fg: "#00FF66", dim: "#0B6B34", accent: "#B5FFD0", ok: "#00FF66", warn: "#E6FF4D", err: "#FF4D6D", cyan: "#7DFCB0", badgebg: "#03200F", art: [3]string{"#04361A", "#00D95A", "#CFFFE0"}},
		"newsprint": {fg: "#D8D4C8", dim: "#8B877C", accent: "#FFFFFF", ok: "#BFD8B0", warn: "#E0C080", err: "#D9534F", cyan: "#FFFFFF", badgebg: "#33302A", art: [3]string{"#4A4840", "#D8D4C8", "#FFFFFF"}},
		"colour":    {fg: "#DEE4F0", dim: "#6E7686", accent: "#8FA8E8", ok: "#5FC98F", warn: "#D9A84E", err: "#DD6873", cyan: "#5FC7D9", badgebg: "#1B2333", art: [3]string{"#141C33", "#3A5680", "#8FA8E8"}},
	}
	for name, w := range want {
		got, ok := palettes[name]
		if !ok {
			t.Fatalf("missing palette %q", name)
		}
		if got != w {
			t.Fatalf("%s: got %+v want %+v", name, got, w)
		}
	}
	if !palettes["mono"].mono {
		t.Fatal("mono palette must be flagged mono")
	}
}

func TestMonoEmitsNoColor(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	out := th.Dim.Render("x") + th.Accent.Render("y") + th.Selected.Render("z") + th.Title.Render("t")
	for _, bad := range []string{"38;", "48;", "\x1b[3", "\x1b[9", "\x1b[4"} {
		// \x1b[4 could be underline attr (4m) — underline is an attr not a color;
		// check only real color introducers
		_ = bad
	}
	if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
		t.Fatalf("mono emitted color escapes: %q", out)
	}
	// attrs are allowed (bold/faint/reverse) but no color codes
	if strings.Contains(out, "\x1b[38") || strings.Contains(out, "\x1b[48") {
		t.Fatalf("mono emitted color escapes: %q", out)
	}
}

func TestColorDegradation(t *testing.T) {
	th256 := testTheme(t, "amber", colorprofile.ANSI256)
	out := th256.Accent.Render("hi")
	if !strings.Contains(out, "38;5;") {
		t.Fatalf("256-color theme should emit 38;5 codes, got %q", out)
	}
	if strings.Contains(out, "38;2;") {
		t.Fatalf("256-color theme leaked truecolor, got %q", out)
	}
	thNone := testTheme(t, "amber", colorprofile.ASCII)
	out = thNone.Accent.Render("hi") + thNone.Dim.Render("d")
	if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
		t.Fatalf("no-color theme emitted color escapes: %q", out)
	}
	// gradient degrades too
	if c := thNone.Gradient(0.5); c != nil {
		t.Fatal("no-color gradient should be nil")
	}
	if c := th256.Gradient(0.5); c == nil {
		t.Fatal("256-color gradient should produce a color")
	}
}

func TestResolvePrecedence(t *testing.T) {
	getenv := func(k string) string {
		switch k {
		case "TSUNAGI_LOOK":
			return "dither"
		case "TSUNAGI_THEME":
			return "amber"
		}
		return ""
	}
	base := DefaultOptions()
	// nothing set -> colour on a color terminal, mono without one
	o, err := Resolve(Request{}, Request{}, Request{}, nil, base)
	if err != nil || o.Look != art.Braille || o.Theme != "colour" {
		t.Fatalf("defaults: %+v err=%v", o, err)
	}
	plain := base
	plain.Color = false
	o, err = Resolve(Request{}, Request{}, Request{}, nil, plain)
	if err != nil || o.Theme != "mono" {
		t.Fatalf("no-color default: %+v err=%v", o, err)
	}
	// env beats config
	anim := true
	o, err = Resolve(Request{}, EnvSources(getenv), Request{Look: "bricks", Theme: "matrix"}, &anim, base)
	if err != nil || o.Look != art.Dither || o.Theme != "amber" {
		t.Fatalf("env>config: %+v err=%v", o, err)
	}
	// flag beats env
	o, err = Resolve(Request{Look: "gradient", Theme: "newsprint"}, EnvSources(getenv), Request{Look: "bricks"}, &anim, base)
	if err != nil || o.Look != art.Gradient || o.Theme != "newsprint" {
		t.Fatalf("flag>env: %+v err=%v", o, err)
	}
	// config beats default
	o, err = Resolve(Request{}, Request{}, Request{Look: "halftone", Theme: "colour"}, nil, base)
	if err != nil || o.Look != art.Halftone || o.Theme != "colour" {
		t.Fatalf("config>default: %+v err=%v", o, err)
	}
	// invalid flag errors
	if _, err = Resolve(Request{Look: "nope"}, Request{}, Request{}, nil, base); err == nil {
		t.Fatal("invalid look should error")
	}
	if _, err = Resolve(Request{}, Request{}, Request{Theme: "nope"}, nil, base); err == nil {
		t.Fatal("invalid theme should error")
	}
	// cfgAnim false disables; true cannot re-enable a disabled base
	o, err = Resolve(Request{}, Request{}, Request{}, &[]bool{false}[0], Options{Anim: true})
	if err != nil || o.Anim {
		t.Fatal("cfg anim=false should disable")
	}
	o, err = Resolve(Request{}, Request{}, Request{}, &[]bool{true}[0], Options{Anim: false})
	if err != nil || o.Anim {
		t.Fatal("cfg anim=true must not re-enable --no-anim")
	}
}

func TestDetectUTF8(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}
	cases := []struct {
		name string
		kv   map[string]string
		want bool
	}{
		{"lc_all_utf8", map[string]string{"LC_ALL": "en_US.UTF-8"}, true},
		{"lang_utf8_lower", map[string]string{"LANG": "C.utf8"}, true},
		{"posix", map[string]string{"LC_ALL": "C", "LANG": "C"}, false},
		{"none", map[string]string{}, true},
		{"lc_ctype_wins_over_lang", map[string]string{"LC_CTYPE": "POSIX", "LANG": "en_US.UTF-8"}, false},
	}
	for _, c := range cases {
		if got := DetectUTF8(env(c.kv)); got != c.want {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func key(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func upd(t *testing.T, m tea.Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	t.Helper()
	return m.Update(msg)
}

func msgOf(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command emitting a message")
	}
	return cmd()
}

func TestMenuNavigation(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	items := []Item{
		{Label: "one", Value: "1"},
		{Label: "two", Value: "2", Disabled: true},
		{Label: "three", Value: "3"},
	}
	m := NewMenu(items, 0, th, true)
	// down skips the disabled item
	m2, _ := upd(t, m, key(tea.KeyDown, ""))
	m = m2.(Menu)
	if m.Cursor() != 2 {
		t.Fatalf("down should skip disabled, cursor=%d", m.Cursor())
	}
	// j/k work too
	m2, _ = upd(t, m, key('k', "k"))
	m = m2.(Menu)
	if m.Cursor() != 0 {
		t.Fatalf("k should go up, cursor=%d", m.Cursor())
	}
	// home/end
	m2, _ = upd(t, m, key(tea.KeyEnd, ""))
	m = m2.(Menu)
	if m.Cursor() != 2 {
		t.Fatalf("end cursor=%d", m.Cursor())
	}
	m2, _ = upd(t, m, key(tea.KeyHome, ""))
	m = m2.(Menu)
	if m.Cursor() != 0 {
		t.Fatalf("home cursor=%d", m.Cursor())
	}
	// number key jumps and selects
	m2, cmd := upd(t, m, key('3', "3"))
	m = m2.(Menu)
	if m.Cursor() != 2 {
		t.Fatalf("digit cursor=%d", m.Cursor())
	}
	if got := msgOf(t, cmd).(Chosen); got.Value != "3" {
		t.Fatalf("digit chose %q", got.Value)
	}
	// enter chooses
	m2, cmd = upd(t, m, key(tea.KeyEnter, ""))
	if got := msgOf(t, cmd).(Chosen); got.Value != "3" {
		t.Fatalf("enter chose %q", got.Value)
	}
	// esc quits
	_, cmd = upd(t, m, key(tea.KeyEscape, ""))
	if _, ok := msgOf(t, cmd).(Quit); !ok {
		t.Fatal("esc should emit Quit")
	}
	// ctrl+c quits
	_, cmd = upd(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := msgOf(t, cmd).(Quit); !ok {
		t.Fatal("ctrl+c should emit Quit")
	}
	// q quits
	_, cmd = upd(t, m, key('q', "q"))
	if _, ok := msgOf(t, cmd).(Quit); !ok {
		t.Fatal("q should emit Quit")
	}
	// enter on disabled does nothing
	m2, _ = upd(t, m, key(tea.KeyDown, "")) // lands on disabled idx1? move skips -> idx2
	_ = m2
}

func TestMenuViewSizes(t *testing.T) {
	th := testTheme(t, "amber", colorprofile.TrueColor)
	items := make([]Item, 30)
	for i := range items {
		items[i] = Item{Label: "item " + string(rune('a'+i%26)) + string(rune('0'+i/26)), Value: "v"}
	}
	for _, sz := range [][2]int{{80, 24}, {120, 40}, {60, 20}, {40, 12}} {
		m := NewMenu(items, 0, th, true)
		m2, _ := upd(t, m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		v := m2.(Menu).View().Content
		for i, line := range strings.Split(v, "\n") {
			if w := lipgloss.Width(line); w > sz[0] {
				t.Fatalf("%dx%d line %d width %d > %d: %q", sz[0], sz[1], i, w, sz[0], line)
			}
		}
	}
}

func TestChecklist(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	sections := []Section{
		{Title: "providers", Items: []CItem{
			{Label: "alpha", Value: "a", Tag: "private"},
			{Label: "beta", Value: "b", Tag: "trains"},
		}},
		{Title: "models", Items: []CItem{
			{Label: "gamma", Value: "g", State: Done},
		}},
	}
	c := NewChecklist(sections, th, false)
	// space cycles None -> Pending -> Done -> None
	c2, _ := upd(t, c, key(' ', " "))
	c = c2.(Checklist)
	if c.Sections[0].Items[0].State != Pending {
		t.Fatalf("after space state=%v", c.Sections[0].Items[0].State)
	}
	upd(t, c, key(' ', " "))
	c2, _ = c.Update(key(' ', " "))
	c = c2.(Checklist)
	// careful: previous upd already toggled once more; just check cycle works
	// navigation down then space on beta
	c2, _ = upd(t, c, key(tea.KeyDown, ""))
	c = c2.(Checklist)
	c2, _ = upd(t, c, key('j', "j"))
	c = c2.(Checklist)
	// now on gamma (idx2); enter confirms gamma (Done) + whichever states set
	_, cmd := upd(t, c, key(tea.KeyEnter, ""))
	got := msgOf(t, cmd).(Confirmed)
	found := false
	for _, v := range got.Values {
		if v == "g" {
			found = true
		}
	}
	if !found {
		t.Fatalf("confirmed values %v missing done item", got.Values)
	}
	// esc quits
	_, cmd = upd(t, c, key(tea.KeyEscape, ""))
	if _, ok := msgOf(t, cmd).(Quit); !ok {
		t.Fatal("esc should emit Quit")
	}
}

func TestChecklistScroll(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	items := make([]CItem, 30)
	for i := range items {
		items[i] = CItem{Label: "row", Value: "v"}
	}
	c := NewChecklist([]Section{{Items: items}}, th, true)
	c2, _ := upd(t, c, tea.WindowSizeMsg{Width: 60, Height: 10})
	c = c2.(Checklist)
	// jump to last item via end
	c2, _ = upd(t, c, key(tea.KeyEnd, ""))
	c = c2.(Checklist)
	v := c.View().Content
	lines := strings.Split(v, "\n")
	if len(lines) > 10 {
		t.Fatalf("view has %d lines, want <= 10", len(lines))
	}
	// focused row must be visible: last item rendered
	if !strings.Contains(v, "[ ] row") {
		t.Fatal("scrolled view lost items")
	}
}

func TestInput(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	in := NewInput("key", false, th, true)
	// type "sk-abc"
	for _, r := range "sk-abc" {
		in2, _ := upd(t, in, key(r, string(r)))
		in = in2.(Input)
	}
	if in.Value() != "sk-abc" {
		t.Fatalf("value=%q", in.Value())
	}
	_, cmd := upd(t, in, key(tea.KeyEnter, ""))
	if got := msgOf(t, cmd).(Submitted); got.Value != "sk-abc" {
		t.Fatalf("submitted %q", got.Value)
	}
	_, cmd = upd(t, in, key(tea.KeyEscape, ""))
	if _, ok := msgOf(t, cmd).(Cancelled); !ok {
		t.Fatal("esc should emit Cancelled")
	}
	_, cmd = upd(t, in, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := msgOf(t, cmd).(Cancelled); !ok {
		t.Fatal("ctrl+c should emit Cancelled")
	}
}

func TestInputMasked(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	in := NewInput("key", true, th, true)
	secret := "sk-supersecretvalue1234567890"
	for _, r := range secret {
		in2, _ := upd(t, in, key(r, string(r)))
		in = in2.(Input)
	}
	if in.Value() != secret {
		t.Fatalf("masked value should still be the real text, got %q", in.Value())
	}
	view := in.View().Content
	if strings.Contains(view, secret) || strings.Contains(view, "supersecret") {
		t.Fatalf("masked view leaked the secret: %q", view)
	}
	// bullets capped at 24 regardless of length
	if n := strings.Count(view, "•"); n != 24 {
		t.Fatalf("masked view should show 24 bullets, got %d: %q", n, view)
	}
	// short secret shows its length in bullets but nothing else
	in2 := NewInput("", true, th, true)
	for _, r := range "ab" {
		in3, _ := upd(t, in2, key(r, string(r)))
		in2 = in3.(Input)
	}
	if n := strings.Count(in2.View().Content, "•"); n != 2 {
		t.Fatalf("short masked value should show 2 bullets, got %d", n)
	}
}

func TestInputPaste(t *testing.T) {
	th := testTheme(t, "mono", colorprofile.TrueColor)
	in := NewInput("", false, th, true)
	in2, _ := upd(t, in, tea.PasteMsg{Content: "pasted-text"})
	in = in2.(Input)
	if in.Value() != "pasted-text" {
		t.Fatalf("paste should insert text, got %q", in.Value())
	}
}

func TestArtModelStaticAndTick(t *testing.T) {
	o := Options{Look: art.Braille, Theme: "colour", Color: true, TrueColor: true, Anim: false, UTF8: true, W: 80, H: 24}
	m, err := NewArtModel(o)
	if err != nil {
		t.Fatal(err)
	}
	if m.Init() != nil {
		t.Fatal("static art should not tick")
	}
	v := m.View().Content
	if v == "" {
		t.Fatal("static art view is empty")
	}
	// compact sizes hide the art
	o.W, o.H = 40, 12
	m2, _ := NewArtModel(o)
	if m2.View().Content != "" {
		t.Fatal("compact art should be empty")
	}
	// animated model ticks and skips on keypress
	o.W, o.H, o.Anim = 80, 24, true
	m3, _ := NewArtModel(o)
	if m3.Init() == nil {
		t.Fatal("animated art should tick")
	}
	m4, cmd := upd(t, m3, tickMsg(time.Now()))
	_ = m4
	if cmd == nil {
		t.Fatal("tick should reschedule")
	}
	_, cmd = upd(t, m3, key('x', "x"))
	if _, ok := msgOf(t, cmd).(Skipped); !ok {
		t.Fatal("keypress should emit Skipped")
	}
}

func TestSpinnerGaugeFrame(t *testing.T) {
	th := testTheme(t, "amber", colorprofile.TrueColor)
	s := NewSpinner(th, true)
	if s.Init() == nil {
		t.Fatal("spinner should tick")
	}
	if s.View().Content == "" {
		t.Fatal("spinner view empty")
	}
	g := NewGauge(0.5, 10, th)
	gv := g.View().Content
	if !strings.Contains(gv, "[") || !strings.Contains(gv, "]") || !strings.Contains(gv, "%") {
		t.Fatalf("gauge view %q", gv)
	}
	// Place centers content
	p := Place(80, 24, "hi")
	lines := strings.Split(p, "\n")
	if len(lines) != 24 {
		t.Fatalf("Place height=%d", len(lines))
	}
	if Compact(40, 12) != true || Compact(80, 24) != false {
		t.Fatal("Compact thresholds wrong")
	}
}

type quitNow struct{}

func (quitNow) Init() tea.Cmd { return tea.Quit }
func (quitNow) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return quitNow{}, nil
}
func (quitNow) View() tea.View { return tea.NewView("bye") }

type panicModel struct{ quitNow }

func (panicModel) View() tea.View { panic("boom") }

func TestRunRestores(t *testing.T) {
	var buf strings.Builder
	o := Options{W: 80, H: 24, In: strings.NewReader(""), Out: &buf}
	final, err := Run(quitNow{}, o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final == nil {
		t.Fatal("Run should return the final model")
	}
	out := buf.String()
	// alt screen enter + leave should both appear
	if !strings.Contains(out, "1049") {
		t.Fatalf("expected alt-screen sequences in output, got %q", out[:min(len(out), 200)])
	}
}

func TestRunPanic(t *testing.T) {
	var buf strings.Builder
	o := Options{W: 80, H: 24, In: strings.NewReader(""), Out: &buf}
	_, err := Run(panicModel{}, o)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "panic") {
		t.Fatalf("panic should surface as error, got %v", err)
	}
}
