package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// PickItem is one row of a Picker.
type PickItem struct {
	Label string
	Value string
	Tag   string // short right-aligned dim annotation, also searched
	Group string // section heading while no query is typed, also searched
	State State  // None, Pending or Done (a check mark)
	// Featured rows are listed without a query; the rest only appear when a
	// search matches them.
	Featured bool
	// Action rows (Add a gateway) have no state mark.
	Action bool
	// Glyph and AsciiGlyph replace the mark of an action row.
	Glyph, AsciiGlyph string
}

// Picker is a searchable list: type to filter, arrows to move, enter to
// choose. Without a query it shows the Featured rows under their group
// headings and counts the rest.
type Picker struct {
	Items []PickItem
	q     string
	sel   int // index into vis
	top   int // first visible row of the rendered list
	vis   []int
	theme Theme
	utf8  bool
	w, h  int
}

// NewPicker builds a picker with every item visible per the empty query.
func NewPicker(items []PickItem, th Theme, utf8 bool) Picker {
	p := Picker{Items: items, theme: th, utf8: utf8, w: 40, h: 12}
	p.refilter("")
	return p
}

// Query is what the user has typed.
func (p Picker) Query() string { return p.q }

// Selected returns the highlighted item.
func (p Picker) Selected() (PickItem, bool) {
	if p.sel < 0 || p.sel >= len(p.vis) {
		return PickItem{}, false
	}
	return p.Items[p.vis[p.sel]], true
}

// Select moves the cursor to the item with the given value, if visible.
func (p *Picker) Select(value string) {
	for i, ix := range p.vis {
		if p.Items[ix].Value == value {
			p.sel = i
			p.reveal()
			return
		}
	}
}

// Hidden counts the items a query would reveal.
func (p Picker) Hidden() int {
	if p.q != "" {
		return 0
	}
	n := 0
	for _, it := range p.Items {
		if !it.Featured {
			n++
		}
	}
	return n
}

func (p *Picker) refilter(q string) {
	keep := ""
	if it, ok := p.Selected(); ok {
		keep = it.Value
	}
	p.q = q
	p.vis = p.vis[:0]
	toks := strings.Fields(strings.ToLower(q))
	var prefix, rest []int
	for i, it := range p.Items {
		if len(toks) == 0 {
			if it.Featured {
				p.vis = append(p.vis, i)
			}
			continue
		}
		hay := strings.ToLower(it.Label + " " + it.Tag + " " + it.Group)
		ok := true
		for _, t := range toks {
			if !strings.Contains(hay, t) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.ToLower(it.Label), toks[0]) {
			prefix = append(prefix, i)
		} else {
			rest = append(rest, i)
		}
	}
	if len(toks) > 0 {
		p.vis = append(append(p.vis, prefix...), rest...)
	}
	p.sel, p.top = 0, 0
	if keep != "" && len(toks) == 0 {
		p.Select(keep)
	}
}

func (p *Picker) move(d int) {
	if len(p.vis) == 0 {
		return
	}
	p.sel = max(0, min(p.sel+d, len(p.vis)-1))
}

// rows renders the list body: group headings (no query), items and the
// "more" line, as lines tagged with the visible index they select (-1 for a
// heading).
type pickRow struct {
	text string
	idx  int
}

func (p Picker) layout() []pickRow {
	var rows []pickRow
	group := "\x00"
	for vi, ix := range p.vis {
		it := p.Items[ix]
		if p.q == "" && it.Group != group {
			group = it.Group
			if it.Group != "" {
				rows = append(rows, pickRow{p.theme.Title.Render(it.Group), -1})
			}
		}
		rows = append(rows, pickRow{p.itemLine(it, vi == p.sel), vi})
	}
	return rows
}

func (p Picker) glyph(it PickItem) string {
	if it.Action {
		switch {
		case p.utf8 && it.Glyph != "":
			return it.Glyph
		case !p.utf8 && it.AsciiGlyph != "":
			return it.AsciiGlyph
		case p.utf8:
			return "+"
		}
		return "+"
	}
	switch it.State {
	case Done:
		if p.utf8 {
			return "✓"
		}
		return "x"
	case Pending:
		return "~"
	}
	return " "
}

func (p Picker) itemLine(it PickItem, focused bool) string {
	marker := "  "
	if focused {
		marker = ">" + " "
		if p.utf8 {
			marker = "❯ "
		}
	}
	w := max(p.w, 12)
	tag := it.Tag
	tagW := 0
	if tag != "" {
		tagW = lipgloss.Width(tag) + 1
	}
	g := p.glyph(it)
	label := truncateCells(it.Label, max(w-lipgloss.Width(marker)-lipgloss.Width(g)-1-tagW, 1))
	line := marker + g + " " + label
	pad := max(w-lipgloss.Width(line)-lipgloss.Width(tag), 0)
	if tag != "" {
		pad = max(pad, 1)
	}
	line += strings.Repeat(" ", pad)
	switch {
	case focused:
		return p.theme.Selected.Render(line) + p.theme.Selected.Render(tag)
	case it.State == Done:
		return p.theme.Dim.Render(line) + p.theme.Dim.Render(tag)
	}
	return p.theme.Text.Render(line) + p.theme.Dim.Render(tag)
}

// listH is the number of rows for the list: the box minus the search line and
// the "more" line.
func (p Picker) listH() int {
	h := p.h - 1
	if p.Hidden() > 0 {
		h--
	}
	return max(h, 1)
}

func (p *Picker) reveal() {
	rows := p.layout()
	at := -1
	for i, r := range rows {
		if r.idx == p.sel {
			at = i
		}
	}
	if at < 0 {
		return
	}
	h := p.listH()
	if at < p.top {
		p.top = at
		// keep the group heading above the first item in view
		if at > 0 && rows[at-1].idx == -1 {
			p.top = at - 1
		}
	}
	if at >= p.top+h {
		p.top = at - h + 1
	}
	if mx := len(rows) - h; p.top > max(mx, 0) {
		p.top = max(mx, 0)
	}
}

func (p Picker) Init() tea.Cmd { return nil }

func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := p.update(msg)
	pp := nm.(Picker)
	pp.reveal()
	return pp, cmd
}

func (p Picker) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		p.w, p.h = v.Width, v.Height
	case tea.MouseClickMsg:
		mo := tea.Mouse(v)
		if mo.Button != tea.MouseLeft || mo.Y < 1 {
			break
		}
		rows := p.layout()
		if i := p.top + mo.Y - 1; i >= 0 && i < len(rows) && rows[i].idx >= 0 && mo.Y-1 < p.listH() {
			p.sel = rows[i].idx
			it := p.Items[p.vis[p.sel]]
			return p, func() tea.Msg { return Chosen{Value: it.Value} }
		}
	case tea.MouseWheelMsg:
		switch tea.Mouse(v).Button {
		case tea.MouseWheelUp:
			p.move(-1)
		case tea.MouseWheelDown:
			p.move(1)
		}
	case tea.KeyPressMsg:
		code, text := v.Code, v.Text
		ctrl := v.Mod&tea.ModCtrl != 0
		switch {
		case code == tea.KeyUp:
			p.move(-1)
		case code == tea.KeyDown:
			p.move(1)
		case code == tea.KeyPgUp:
			p.move(-p.listH())
		case code == tea.KeyPgDown:
			p.move(p.listH())
		case code == tea.KeyHome:
			p.sel = 0
		case code == tea.KeyEnd:
			p.sel = max(len(p.vis)-1, 0)
		case code == tea.KeyEnter || code == '\r':
			if it, ok := p.Selected(); ok {
				return p, func() tea.Msg { return Chosen{Value: it.Value} }
			}
		case code == tea.KeyEscape:
			if p.q != "" {
				p.refilter("")
				return p, nil
			}
			return p, func() tea.Msg { return Quit{} }
		case ctrl && code == 'c':
			return p, func() tea.Msg { return Quit{} }
		case ctrl && code == 'u':
			p.refilter("")
		case code == tea.KeyBackspace:
			if p.q != "" {
				_, n := utf8.DecodeLastRuneInString(p.q)
				p.refilter(p.q[:len(p.q)-n])
			}
		case text != "" && !ctrl && v.Mod&tea.ModAlt == 0:
			p.refilter(p.q + text)
		}
	}
	return p, nil
}

// View draws the search line, the list and, without a query, how many more
// providers a search would reveal.
func (p Picker) View() tea.View {
	p.reveal()
	w := max(p.w, 12)
	icon := ">"
	if p.utf8 {
		icon = "⌕"
	}
	var bar string
	if p.q == "" {
		bar = p.theme.Accent.Render(icon) + " " + p.theme.Dim.Render("type to search")
	} else {
		count := fmt.Sprintf("%d", len(p.vis))
		room := max(w-lipgloss.Width(count)-4, 1)
		q := p.q
		if lipgloss.Width(q) > room {
			q = "…" + truncateCells(q[max(len(q)-room, 0):], room-1)
		}
		bar = p.theme.Accent.Render(icon) + " " + p.theme.Text.Render(q) + p.theme.Accent.Render("▏")
		pad := max(w-lipgloss.Width(icon)-1-lipgloss.Width(q)-1-lipgloss.Width(count), 1)
		bar += strings.Repeat(" ", pad) + p.theme.Dim.Render(count)
	}
	lines := []string{bar}
	rows := p.layout()
	h := p.listH()
	for i := p.top; i < len(rows) && i < p.top+h; i++ {
		lines = append(lines, rows[i].text)
	}
	if len(rows) == 0 && p.q != "" {
		lines = append(lines, p.theme.Dim.Render("no match"))
	}
	if n := p.Hidden(); n > 0 {
		for len(lines) < 1+h {
			lines = append(lines, "")
		}
		lines = append(lines, p.theme.Dim.Render(fmt.Sprintf("+%d more, type to search", n)))
	}
	return tea.NewView(strings.Join(lines, "\n"))
}
