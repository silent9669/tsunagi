// Package dash draws tsunagi's two live screens: the watch dashboard and the
// launch card. Both show the sea with the eel above their rows.
package dash

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/art"
	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/tui"
)

// Source feeds the dashboard: Poll returns the events that arrived since the
// last call and whether a session is running.
type Source interface {
	Poll() (evs []events.Event, live bool)
}

type tickMsg time.Time

const tick = 83 * time.Millisecond

// clean drops control characters: model names come from request bodies and
// must not inject terminal escape sequences.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func cut(s string, n int) string {
	if n < 1 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	out, w := []rune{}, 0
	for _, r := range s {
		if w+1 > n-1 {
			break
		}
		out = append(out, r)
		w++
	}
	return string(out) + "…"
}

func seaRowsFor(w, h int) int {
	switch {
	case w < 40 || h < 18:
		return 0
	case h < 28:
		return 4
	}
	return 6
}

func sea(o tui.Options, th tui.Theme, rows int, phase float64) []string {
	ao := o
	ao.W, ao.H = min(o.W-2, 72), rows
	if !o.Anim {
		phase = art.OceanStaticPhase
	}
	pad := strings.Repeat(" ", max((o.W-ao.W)/2, 0))
	var out []string
	for _, l := range strings.Split(tui.OceanArt(ao, th, phase), "\n") {
		out = append(out, pad+l)
	}
	return out
}

// Model is the watch dashboard.
type Model struct {
	o       tui.Options
	th      tui.Theme
	src     Source
	log     []events.Event
	counts  map[string]int
	live    bool
	phase   float64
	started time.Time
}

// New builds the dashboard.
func New(o tui.Options, src Source) (*Model, error) {
	th, err := o.ThemeOr()
	if err != nil {
		return nil, err
	}
	return &Model{o: o, th: th, src: src, counts: map[string]int{}, started: time.Now()}, nil
}

func (m *Model) Init() tea.Cmd {
	return tea.Tick(tick, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func acct(e events.Event) string {
	if e.Account != "" {
		return e.Account
	}
	return e.Provider
}

func (m *Model) recent(d time.Duration) int {
	n := 0
	for _, e := range m.log {
		if time.Since(e.Time) < d {
			n++
		}
	}
	return n
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.o.W, m.o.H = v.Width, v.Height
	case tea.KeyPressMsg:
		switch {
		case v.Text == "q" || v.Code == tea.KeyEscape || (v.Code == 'c' && v.Mod&tea.ModCtrl != 0):
			return m, tea.Quit
		case v.Text == "c":
			m.log, m.counts = nil, map[string]int{}
		}
	case tickMsg:
		evs, live := m.src.Poll()
		m.live = live
		for _, e := range evs {
			m.log = append(m.log, e)
			m.counts[acct(e)]++
		}
		if len(evs) > 0 {
			sort.SliceStable(m.log, func(i, j int) bool { return m.log[i].Time.Before(m.log[j].Time) })
		}
		if len(m.log) > 500 {
			m.log = m.log[len(m.log)-500:]
		}
		// the eel swims faster while requests are flowing
		m.phase += 0.3 * (1 + 0.5*float64(min(m.recent(3*time.Second), 6)))
		return m, tea.Tick(tick, func(t time.Time) tea.Msg { return tickMsg(t) })
	}
	return m, nil
}

func (m *Model) statusStyle(code int) lipgloss.Style {
	switch {
	case code >= 500:
		return m.th.Err
	case code >= 400:
		return m.th.Warn
	}
	return m.th.OK
}

func (m *Model) spark(buckets int) string {
	if buckets < 1 {
		return ""
	}
	counts := make([]int, buckets)
	peak := 0
	now := time.Now()
	for _, e := range m.log {
		age := int(now.Sub(e.Time) / (5 * time.Second))
		if age >= 0 && age < buckets {
			counts[buckets-1-age]++
			peak = max(peak, counts[buckets-1-age])
		}
	}
	ramp := []rune("▁▂▃▄▅▆▇█")
	if !m.o.UTF8 {
		ramp = []rune(" .:-=+*#")
	}
	var b strings.Builder
	for _, n := range counts {
		switch {
		case n == 0 && m.o.UTF8:
			b.WriteRune('▁')
		case n == 0:
			b.WriteRune('_')
		default:
			b.WriteRune(ramp[max(1, n*(len(ramp)-1)/peak)])
		}
	}
	return b.String()
}

func (m *Model) View() tea.View {
	W, H := m.o.W, m.o.H
	if W <= 0 || H <= 0 {
		W, H = 80, 24
	}
	bw := max(min(W-4, 110), min(W, 20))
	pad := strings.Repeat(" ", max((W-bw)/2, 0))
	var lines []string
	if rows := seaRowsFor(W, H); rows > 0 {
		lines = append(lines, sea(m.o, m.th, rows, m.phase)...)
		lines = append(lines, "")
	}
	state := "waiting for a session"
	if m.live {
		state = fmt.Sprintf("%d requests", len(m.log))
	}
	lines = append(lines, pad+m.th.Title.Render("watch")+"  "+m.th.Dim.Render(state))

	if len(m.counts) > 0 {
		names := make([]string, 0, len(m.counts))
		for n := range m.counts {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool {
			if m.counts[names[i]] != m.counts[names[j]] {
				return m.counts[names[i]] > m.counts[names[j]]
			}
			return names[i] < names[j]
		})
		top := m.counts[names[0]]
		for i, n := range names {
			if i >= 4 && H < 30 || i >= 6 {
				break
			}
			barW := max(min(bw-lipgloss.Width(n)-8, 24), 0)
			fill := 0
			if top > 0 {
				fill = max(1, m.counts[n]*barW/top)
			}
			bar := strings.Repeat("█", fill)
			if !m.o.UTF8 {
				bar = strings.Repeat("#", fill)
			}
			lines = append(lines, pad+m.th.Text.Render(cut(clean(n), 20))+"  "+m.th.Accent.Render(bar)+" "+m.th.Dim.Render(fmt.Sprint(m.counts[n])))
		}
	}
	if W >= 50 {
		lines = append(lines, pad+m.th.Dim.Render("5 min ")+m.th.Accent.Render(m.spark(min(bw-8, 60))))
	}
	lines = append(lines, "")

	// the request table takes whatever rows are left above the legend
	rows := H - len(lines) - 3
	cols := []int{8}
	head := []string{"time"}
	switch {
	case bw >= 90:
		cols = append(cols, 16, 26, 4, 6, 11)
		head = append(head, "account", "model", "http", "ms", "tokens")
	case bw >= 60:
		cols = append(cols, 16, 4, 6)
		head = append(head, "account", "http", "ms")
	default:
		cols = append(cols, 4, 6)
		head = append(head, "http", "ms")
	}
	used := 0
	for _, c := range cols {
		used += c + 1
	}
	whyW := max(bw-used, 0)
	fmtRow := func(cells []string) string {
		var b strings.Builder
		for i, c := range cells {
			b.WriteString(c + strings.Repeat(" ", max(cols[i]-lipgloss.Width(c), 0)) + " ")
		}
		return b.String()
	}
	if rows > 1 {
		hdr := fmtRow(head)
		if bw >= 90 {
			hdr += "route"
		}
		lines = append(lines, pad+m.th.Dim.Render(cut(hdr, bw)))
		show := m.log
		if len(show) > rows-1 {
			show = show[len(show)-(rows-1):]
		}
		for _, e := range show {
			code := m.statusStyle(e.Status).Render(fmt.Sprintf("%-4d", e.Status))
			var cells []string
			ts := e.Time.Format("15:04:05")
			a := cut(clean(acct(e)), 16)
			switch {
			case bw >= 90:
				model := clean(e.Model)
				if model == "" {
					model = "-"
				}
				tok := "-"
				if e.In+e.Out > 0 {
					tok = count(e.In) + "/" + count(e.Out)
				}
				cells = []string{ts, a, cut(model, 26), code, fmt.Sprint(e.Millis), tok}
			case bw >= 60:
				cells = []string{ts, a, code, fmt.Sprint(e.Millis)}
			default:
				cells = []string{ts, code, fmt.Sprint(e.Millis)}
			}
			row := fmtRow(cells)
			if bw >= 90 && whyW > 0 {
				why := clean(e.Why)
				if why == "" && e.Skipped > 0 {
					why = fmt.Sprintf("+%d failover", e.Skipped)
				}
				row += m.th.Dim.Render(cut(why, whyW))
			}
			lines = append(lines, pad+row)
		}
	}
	for len(lines) < H-1 {
		lines = append(lines, "")
	}
	if len(lines) > H-1 {
		lines = lines[:H-1]
	}
	lines = append(lines, pad+tui.Legend(m.th, m.o.UTF8, bw, "c|clear", "q|quit"))
	clip := lipgloss.NewStyle().MaxWidth(W)
	for i, l := range lines {
		if lipgloss.Width(l) > W {
			lines[i] = clip.Render(l)
		}
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

func count(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// Info is what the launch card shows.
type Info struct {
	Router  string   // 127.0.0.1:port
	Claude  string   // path of the claude binary
	Targets []string // anthropic first, then the providers with a key
	Notes   []string // settings conflicts and the like
}

// Launch is the card shown while the router starts and before Claude Code
// takes over the terminal: the sea, and what the session is connected to.
type Launch struct {
	o     tui.Options
	th    tui.Theme
	info  Info
	phase float64
	n     int
}

// NewLaunch builds the launch card.
func NewLaunch(o tui.Options, info Info) (*Launch, error) {
	th, err := o.ThemeOr()
	if err != nil {
		return nil, err
	}
	return &Launch{o: o, th: th, info: info}, nil
}

func (l *Launch) Init() tea.Cmd {
	return tea.Tick(tick, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (l *Launch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		l.o.W, l.o.H = v.Width, v.Height
	case tea.KeyPressMsg:
		return l, tea.Quit
	case tickMsg:
		l.phase += 0.7
		l.n++
		if l.n > 16 { // about 1.4 s
			return l, tea.Quit
		}
		return l, tea.Tick(tick, func(t time.Time) tea.Msg { return tickMsg(t) })
	}
	return l, nil
}

func (l *Launch) View() tea.View {
	W, H := l.o.W, l.o.H
	if W <= 0 || H <= 0 {
		W, H = 80, 24
	}
	bw := max(min(W-4, 64), min(W, 20))
	pad := strings.Repeat(" ", max((W-bw)/2, 0))
	var lines []string
	if rows := seaRowsFor(W, H); rows > 0 {
		lines = append(lines, sea(l.o, l.th, rows, l.phase)...)
		lines = append(lines, "")
	}
	row := func(k, v string) {
		lines = append(lines, pad+l.th.Dim.Render(fmt.Sprintf("%-8s", k))+l.th.Text.Render(cut(clean(v), bw-9)))
	}
	row("router", l.info.Router)
	row("claude", l.info.Claude)
	route := strings.Join(l.info.Targets, "  ")
	row("routes", route)
	for _, n := range l.info.Notes {
		lines = append(lines, pad+l.th.Warn.Render(cut(clean(n), bw)))
	}
	if len(lines) > H {
		lines = lines[len(lines)-H:]
	}
	clip := lipgloss.NewStyle().MaxWidth(W)
	for i, ln := range lines {
		if lipgloss.Width(ln) > W {
			lines[i] = clip.Render(ln)
		}
	}
	return tea.NewView(strings.Join(lines, "\n"))
}
