package tui

import (
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// RowKind is what a List row does.
type RowKind int

const (
	RowHead RowKind = iota // dim section label, not selectable
	RowItem                // selectable row
	RowDim                 // dim one-liner, not selectable
)

// ListRow is one rendered row. Glyph is a leading state mark ("✓", "○",
// "↳"); Badges are badge kinds; Note is a dim right-aligned tag, or a
// left-aligned column after the badge slots when NoteCol is set.
// NoteAccent renders the note in accent instead of dim.
type ListRow struct {
	Kind       RowKind
	Label      string
	Value      string // emitted by Chosen on enter
	Note       string
	NoteCol    bool
	NoteAccent bool
	Badges     []string
	Glyph      string
	Ref        int // caller index, echoed back in Chosen
}

// List is a scrollable selectable row list with the ❯ cursor. The caller
// owns the rows and what choosing one means.
type List struct {
	Rows  []ListRow
	sel   int
	top   int
	theme Theme
	utf8  bool
	w, h  int
}

// NewList builds an empty list.
func NewList(th Theme, utf8 bool) List {
	return List{theme: th, utf8: utf8, w: 80, h: 24}
}

// SetRows replaces the rows, keeping the cursor on the same Value when it
// still exists.
func (l *List) SetRows(rows []ListRow) {
	keep := ""
	if r, ok := l.Selected(); ok {
		keep = r.Value
	}
	l.Rows = rows
	l.sel = l.firstSelectable()
	if keep != "" {
		l.Select(keep)
	}
	l.reveal()
}

// Reset puts the cursor on the first selectable row; callers use it when the
// query or filter changed, so Enter always acts on the first result.
func (l *List) Reset() {
	l.sel = l.firstSelectable()
	l.top = 0
	l.reveal()
}

// Selected returns the highlighted row.
func (l List) Selected() (ListRow, bool) {
	if l.sel < 0 || l.sel >= len(l.Rows) || l.Rows[l.sel].Kind != RowItem {
		return ListRow{}, false
	}
	return l.Rows[l.sel], true
}

// Select moves the cursor to the row with the given value.
func (l *List) Select(value string) {
	for i, r := range l.Rows {
		if r.Kind == RowItem && r.Value == value {
			l.sel = i
			l.reveal()
			return
		}
	}
}

func (l List) firstSelectable() int {
	for i, r := range l.Rows {
		if r.Kind == RowItem {
			return i
		}
	}
	return 0
}

func (l *List) move(d int) {
	n := len(l.Rows)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		l.sel = (l.sel + d + n) % n
		if l.Rows[l.sel].Kind == RowItem {
			return
		}
	}
}

func (l *List) reveal() {
	if l.h < 1 {
		return
	}
	// the up/down "N more" indicators each take a row when the list scrolls
	room := l.h
	if len(l.Rows) > room {
		room -= 2
	}
	room = max(room, 1)
	if l.sel < l.top {
		l.top = l.sel
		// a heading directly above stays in view
		if l.sel > 0 && l.Rows[l.sel-1].Kind != RowItem {
			l.top = l.sel - 1
		}
	}
	if l.sel >= l.top+room {
		l.top = l.sel - room + 1
	}
	if mx := len(l.Rows) - room; l.top > max(mx, 0) {
		l.top = max(mx, 0)
	}
	if l.top < 0 {
		l.top = 0
	}
}

func (l List) Init() tea.Cmd { return nil }

// Update moves the cursor. Mouse clicks must already be in list coordinates.
func (l List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		l.w, l.h = v.Width, v.Height
		l.reveal()
	case tea.MouseClickMsg:
		if mo := tea.Mouse(v); mo.Button == tea.MouseLeft && mo.Y >= 0 {
			y := mo.Y
			if l.top > 0 {
				y-- // the "N more" indicator occupies the first row
			}
			if i := l.top + y; y >= 0 && i < len(l.Rows) && i < l.top+l.h && l.Rows[i].Kind == RowItem {
				l.sel = i
				return l, func() tea.Msg { return Chosen{Value: l.Rows[i].Value, Ref: l.Rows[i].Ref} }
			}
		}
	case tea.MouseWheelMsg:
		switch tea.Mouse(v).Button {
		case tea.MouseWheelUp:
			l.move(-1)
		case tea.MouseWheelDown:
			l.move(1)
		}
	case tea.KeyPressMsg:
		switch v.Code {
		case tea.KeyUp:
			l.move(-1)
		case tea.KeyDown:
			l.move(1)
		case tea.KeyPgUp:
			l.move(-max(l.h, 1))
		case tea.KeyPgDown:
			l.move(max(l.h, 1))
		case tea.KeyHome:
			l.sel = l.firstSelectable()
		case tea.KeyEnd:
			for i := len(l.Rows) - 1; i >= 0; i-- {
				if l.Rows[i].Kind == RowItem {
					l.sel = i
					break
				}
			}
		case tea.KeyEnter:
			if r, ok := l.Selected(); ok {
				return l, func() tea.Msg { return Chosen{Value: r.Value, Ref: r.Ref} }
			}
		case tea.KeyEscape:
			return l, func() tea.Msg { return Quit{} }
		}
	}
	l.reveal()
	return l, nil
}

// badgeSlot is the column width each badge slot takes so rows stay aligned:
// slot 0 is the group badge, slot 1 the data/access badge.
func badgeSlot(i int) int {
	if i == 0 {
		return 5
	}
	return 7
}

// View renders the windowed rows: ❯ cursor, state glyph, name column,
// padded badge slots and a dim note cut on a word boundary; headings and
// dim rows carry no cursor. Dim ↑/↓ "N more" rows mark a scrolled window.
func (l List) View() tea.View {
	l.reveal()
	marker := "> "
	ell, up, down := "...", "^", "v"
	if l.utf8 {
		marker, ell, up, down = "❯ ", "…", "↑", "↓"
	}
	w := max(l.w, 8)
	// the name column is the longest visible name, capped at 28 cells
	nameW := 4
	slots := 0
	for _, r := range l.Rows {
		if r.Kind != RowItem {
			continue
		}
		if n := lipgloss.Width(r.Label); n > nameW {
			nameW = n
		}
		if len(r.Badges) > slots {
			slots = len(r.Badges)
		}
	}
	nameW = min(nameW, 28)
	var rows []string
	h := max(l.h, 1)
	room := h
	if l.top > 0 {
		room-- // the "N more" indicator takes a row
	}
	if len(l.Rows)-(l.top+room) > 0 {
		room--
	}
	end := min(len(l.Rows), l.top+max(room, 1))
	if l.top > 0 {
		rows = append(rows, "  "+l.theme.Dim.Render(up+" "+itoa2(l.top)+" more"))
	}
	for i := l.top; i < end; i++ {
		r := l.Rows[i]
		switch r.Kind {
		case RowHead, RowDim:
			label := r.Label
			if label != "" {
				label = truncateCells(label, w-2)
			}
			rows = append(rows, "  "+l.theme.Dim.Render(label))
			continue
		}
		lead := "  "
		if i == l.sel {
			lead = l.theme.Accent.Render(marker)
		}
		glyph := "  "
		if r.Glyph != "" {
			glyph = r.Glyph + " "
		}
		avail := w - lipgloss.Width(marker) - lipgloss.Width(glyph)
		name := padOrCut(r.Label, nameW, ell)
		var badgeCells []string
		badgeW := 0
		note := ""
		noteW := 0
		if avail >= 20 {
			n := len(r.Badges)
			if r.NoteCol {
				n = slots // reserve the badge columns so the note aligns
			}
			for bi := 0; bi < n; bi++ {
				bw := badgeSlot(bi) + 2
				cell := strings.Repeat(" ", bw)
				if bi < len(r.Badges) {
					rendered := l.theme.Badge(r.Badges[bi])
					bw = max(bw, lipgloss.Width(rendered))
					cell = rendered + strings.Repeat(" ", bw-lipgloss.Width(rendered))
				}
				badgeCells = append(badgeCells, cell)
				badgeW += bw
			}
			if badgeW > 0 && avail-nameW-badgeW-2 < 4 {
				badgeCells, badgeW = nil, 0
			}
			if avail >= 30 && r.Note != "" {
				nw := avail - nameW - badgeW - len(badgeCells) - 2
				if nw >= 6 {
					note = truncateWord(r.Note, nw, ell)
					noteW = lipgloss.Width(note) + 1
				}
			}
		}
		line := lead + glyph + name
		for _, c := range badgeCells {
			line += " " + c
		}
		style := l.theme.Text
		if i == l.sel {
			style = l.theme.Selected
		}
		noteStyle := l.theme.Dim
		if r.NoteAccent {
			noteStyle = l.theme.Accent
		}
		if r.NoteCol {
			if note != "" {
				used := lipgloss.Width(line) + 2 + lipgloss.Width(note)
				line = style.Render(line+"  ") + noteStyle.Render(note)
				if fill := w - used; fill > 0 {
					line += style.Render(strings.Repeat(" ", fill))
				}
			} else {
				line = style.Render(line)
			}
		} else {
			pad := max(w-lipgloss.Width(line)-noteW, 0)
			if note != "" {
				pad = max(pad, 1)
			}
			line = style.Render(line + strings.Repeat(" ", pad))
			if note != "" {
				line += noteStyle.Render(note)
			}
		}
		rows = append(rows, line)
	}
	if rest := len(l.Rows) - end; rest > 0 {
		rows = append(rows, "  "+l.theme.Dim.Render(down+" "+itoa2(rest)+" more"))
	}
	return tea.NewView(strings.Join(rows, "\n"))
}

// padOrCut pads s to w cells or truncates it with an ellipsis.
func padOrCut(s string, w int, ell string) string {
	sw := lipgloss.Width(s)
	if sw == w {
		return s
	}
	if sw < w {
		return s + strings.Repeat(" ", w-sw)
	}
	if ell == "" {
		return truncateCells(s, w)
	}
	return truncateCells(s, w-1) + ell
}

// truncateWord cuts s to w cells on the last space before the edge where one
// exists, then appends the ellipsis.
func truncateWord(s string, w int, ell string) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w < 4 {
		return truncateCells(s, w)
	}
	cut := truncateCells(s, w-lipgloss.Width(ell))
	if i := strings.LastIndex(cut, " "); i >= lipgloss.Width(ell) {
		cut = strings.TrimRight(cut[:i], " ")
	}
	return cut + ell
}

// itoa2 is a tiny int->string for indicator rows.
func itoa2(n int) string {
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

// RankMatch scores a token match of q inside s: every space-separated token
// must appear as a contiguous case-insensitive substring. 0 is no match;
// higher wins: label prefix > word start > substring elsewhere. Words split
// on space, '-', '_', '/' and '.'.
func RankMatch(q, s string) int {
	q = strings.ToLower(strings.TrimSpace(q))
	s = strings.ToLower(s)
	if q == "" {
		return 1
	}
	worst := 3
	for _, tok := range strings.Fields(q) {
		r := tokenRank(tok, s)
		if r == 0 {
			return 0
		}
		worst = min(worst, r)
	}
	return worst
}

// tokenRank scores one token: 3 label prefix, 2 word start, 1 substring.
func tokenRank(tok, s string) int {
	if strings.HasPrefix(s, tok) {
		return 3
	}
	for i := 1; i+len(tok) <= len(s); i++ {
		if s[i-1] == ' ' || s[i-1] == '-' || s[i-1] == '_' || s[i-1] == '/' || s[i-1] == '.' {
			if s[i:i+len(tok)] == tok {
				return 2
			}
		}
	}
	if strings.Contains(s, tok) {
		return 1
	}
	return 0
}
