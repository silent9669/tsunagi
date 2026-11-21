package dash

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/tui"
)

type fakeSource struct {
	next []events.Event
	live bool
}

func (f *fakeSource) Poll() ([]events.Event, bool) {
	out := f.next
	f.next = nil
	return out, f.live
}

var opts = tui.Options{Look: "braille", Theme: "mono", UTF8: true, Anim: false, W: 100, H: 30}

func evs() []events.Event {
	now := time.Now()
	return []events.Event{
		{Time: now, Provider: "claude", Model: "claude-sonnet-5-5", Status: 200, Millis: 812, In: 1200, Out: 90},
		{Time: now, Provider: "openrouter", Account: "openrouter", Model: "inclusionai/ling-3.2-flash:free", Status: 429, Millis: 90, Why: "openrouter/x 429 -> groq/y ok", Skipped: 1},
		{Time: now, Provider: "groq", Model: "gpt-oss\x1b[2J-120b", Status: 502, Millis: 15},
	}
}

func strip(s string) string {
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

func TestWatchShowsTrafficAndNeverEscapes(t *testing.T) {
	src := &fakeSource{next: evs(), live: true}
	m, err := New(opts, src)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m.Update(tickMsg(time.Now()))
	raw := m.View().Content
	v := strip(raw)
	for _, want := range []string{"watch", "3 requests", "claude", "openrouter", "429", "groq", "ling-3.2", "1.2k/90", "openrouter/x 429 -> groq/y ok"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(raw, "\x1b") {
		t.Fatalf("a model name injected a control sequence:\n%q", raw)
	}
	if !strings.Contains(v, "gpt-oss[2J-120b") {
		t.Fatalf("the model name should show without its escape byte:\n%s", v)
	}
}

func TestWatchWaitsAndClears(t *testing.T) {
	m, _ := New(opts, &fakeSource{})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tickMsg(time.Now()))
	if !strings.Contains(strip(m.View().Content), "waiting for a session") {
		t.Fatal("no waiting state")
	}
	m, _ = New(opts, &fakeSource{next: evs(), live: true})
	m.Update(tickMsg(time.Now()))
	m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if len(m.log) != 0 || len(m.counts) != 0 {
		t.Fatal("c did not clear the log")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q did not quit")
	}
}

func TestDashboardAndLaunchFitEverySize(t *testing.T) {
	for _, W := range []int{20, 30, 40, 60, 80, 120, 200} {
		for _, H := range []int{4, 6, 8, 12, 16, 24, 30, 50} {
			o := opts
			o.W, o.H = W, H
			m, _ := New(o, &fakeSource{next: evs(), live: true})
			m.Update(tea.WindowSizeMsg{Width: W, Height: H})
			m.Update(tickMsg(time.Now()))
			l, _ := NewLaunch(o, Info{Router: "127.0.0.1:61763", Claude: "/opt/homebrew/bin/claude",
				Targets: []string{"anthropic", "openrouter", "google", "ollama-cloud"}, Notes: []string{"the local settings send Claude elsewhere; this session overrides it"}})
			l.Update(tea.WindowSizeMsg{Width: W, Height: H})
			for name, v := range map[string]string{"watch": m.View().Content, "launch": l.View().Content} {
				lines := strings.Split(v, "\n")
				if len(lines) > H {
					t.Fatalf("%s %dx%d: %d rows", name, W, H, len(lines))
				}
				for i, ln := range lines {
					if w := lipgloss.Width(ln); w > W {
						t.Fatalf("%s %dx%d: row %d is %d wide", name, W, H, i, w)
					}
				}
			}
		}
	}
}

func TestLaunchShowsWhatItIsConnectedToAndEnds(t *testing.T) {
	l, _ := NewLaunch(opts, Info{Router: "127.0.0.1:1", Claude: "/x/claude", Targets: []string{"anthropic", "groq"}, Notes: []string{"heads up"}})
	v := strip(l.View().Content)
	for _, want := range []string{"router", "127.0.0.1:1", "claude", "/x/claude", "anthropic  groq", "heads up"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q:\n%s", want, v)
		}
	}
	for i := 0; i < 16; i++ {
		l.Update(tickMsg(time.Now()))
	}
	_, cmd := l.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("the card never ended")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("after about a second the card must quit, got %T", cmd())
	}
	if _, cmd := l.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd == nil {
		t.Fatal("a key must dismiss the card")
	}
}
