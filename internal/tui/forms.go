package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Chosen carries the Value of the menu item the user picked.
type Chosen struct{ Value string }

// Confirmed carries the Values of every checked checklist item.
type Confirmed struct{ Values []string }

// Submitted carries the text the user entered.
type Submitted struct{ Value string }

// Cancelled is emitted on escape or ctrl+c from a form.
type Cancelled struct{}

// Quit asks the program to exit without a result.
type Quit struct{}

func keyMsg(m tea.Msg) (tea.KeyPressMsg, bool) {
	k, ok := m.(tea.KeyPressMsg)
	return k, ok
}

// digitIndex maps the rune '1'..'9' to a zero-based index, or -1.
func digitIndex(r rune) int {
	if r >= '1' && r <= '9' {
		return int(r - '1')
	}
	return -1
}

// Menu is a vertical list of items with a cursor.
type Item struct {
	Label    string
	Value    string
	Disabled bool
}

type Menu struct {
	Items []Item
	sel   int
	top   int
	theme Theme
	utf8  bool
	w, h  int
}

// NewMenu builds a menu; initial selects the starting cursor index.
func NewMenu(items []Item, initial int, th Theme, utf8 bool) Menu {
	if initial < 0 || initial >= len(items) {
		initial = 0
	}
	return Menu{Items: items, sel: initial, theme: th, utf8: utf8, w: 80, h: 24}
}

// Cursor reports the selected index (for tests and callers).
func (m Menu) Cursor() int { return m.sel }

func (m Menu) enabled(i int) bool {
	return i >= 0 && i < len(m.Items) && !m.Items[i].Disabled
}

// move steps the cursor by delta, skipping disabled items.
func (m *Menu) move(delta int) {
	n := len(m.Items)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		m.sel = (m.sel + delta + n) % n
		if m.enabled(m.sel) {
			return
		}
	}
}

func (m Menu) emit(v string) tea.Cmd {
	return func() tea.Msg { return Chosen{Value: v} }
}

func (m Menu) quit() tea.Cmd {
	return func() tea.Msg { return Quit{} }
}

func (m Menu) Init() tea.Cmd { return nil }

func (m Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = v.Width, v.Height
		return m, nil
	case tea.KeyPressMsg:
		code, text := v.Code, v.Text
		switch {
		case code == tea.KeyUp || text == "k":
			m.move(-1)
		case code == tea.KeyDown || text == "j":
			m.move(1)
		case code == tea.KeyHome:
			for i := range m.Items {
				if m.enabled(i) {
					m.sel = i
					break
				}
			}
		case code == tea.KeyEnd:
			for i := len(m.Items) - 1; i >= 0; i-- {
				if m.enabled(i) {
					m.sel = i
					break
				}
			}
		case code == tea.KeyEnter || code == '\r':
			if m.enabled(m.sel) {
				return m, m.emit(m.Items[m.sel].Value)
			}
		case text == "q" || code == tea.KeyEscape || (code == 'c' && v.Mod&tea.ModCtrl != 0):
			return m, m.quit()
		default:
			if di := digitIndex(code); di >= 0 && di < len(m.Items) {
				m.sel = di
				if m.enabled(di) {
					return m, m.emit(m.Items[di].Value)
				}
			}
		}
	}
	return m, nil
}

// View renders the items; the cursor row is marked and styled Selected.
func (m Menu) View() tea.View {
	marker := ">"
	if m.utf8 {
		marker = "▏"
	}
	// keep the cursor inside the visible window
	if m.sel < m.top {
		m.top = m.sel
	}
	if m.sel >= m.top+m.h {
		m.top = m.sel - m.h + 1
	}
	var sb strings.Builder
	for i := m.top; i < len(m.Items) && i < m.top+m.h; i++ {
		it := m.Items[i]
		prefix := "  "
		if i == m.sel {
			prefix = marker + " "
		}
		line := prefix + it.Label
		if lipgloss.Width(line) > m.w {
			line = truncateCells(line, m.w)
		}
		if it.Disabled {
			sb.WriteString(m.theme.Dim.Render(line))
		} else if i == m.sel {
			sb.WriteString(m.theme.Selected.Render(line))
		} else {
			sb.WriteString(m.theme.Text.Render(line))
		}
		sb.WriteByte('\n')
	}
	return tea.NewView(strings.TrimRight(sb.String(), "\n"))
}

// truncateCells cuts s to at most w terminal cells.
func truncateCells(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > w {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String()
}

// State is a checklist item's tri-state.
type State int

const (
	None    State = iota // empty box
	Pending              // half-done marker
	Done                 // checked
)

// CItem is one checklist row; Tag is a short right-aligned annotation.
type CItem struct {
	Label string
	Value string
	Tag   string
	State State
}

// Section groups checklist items under a heading.
type Section struct {
	Title string
	Items []CItem
}

// Checklist is a grouped multi-select.
type Checklist struct {
	Sections []Section
	sel      int // flat item index
	top      int // first visible row
	theme    Theme
	utf8     bool
	w, h     int
}

// NewChecklist builds a checklist; sections may be empty-titled.
func NewChecklist(sections []Section, th Theme, utf8 bool) Checklist {
	return Checklist{Sections: sections, theme: th, utf8: utf8, w: 80, h: 24}
}

// itemCount totals selectable rows.
func (c Checklist) itemCount() int {
	n := 0
	for _, s := range c.Sections {
		n += len(s.Items)
	}
	return n
}

// rowCount totals heading + item rows for scrolling.
func (c Checklist) rowCount() int {
	n := 0
	for _, s := range c.Sections {
		if s.Title != "" {
			n++
		}
		n += len(s.Items)
	}
	return n
}

// itemAt maps a flat item index to its section/item position and the row
// index the item occupies in the flattened view.
func (c Checklist) locate(itemIdx int) (sec, it, row int) {
	row = 0
	for si, s := range c.Sections {
		if s.Title != "" {
			row++
		}
		for ii := range s.Items {
			if itemIdx == 0 {
				return si, ii, row
			}
			itemIdx--
			row++
		}
	}
	return -1, -1, -1
}

// rowOfItem returns the flattened row index of flat item index i.
func (c Checklist) rowOfItem(i int) int {
	_, _, r := c.locate(i)
	return r
}

func (c *Checklist) move(delta int) {
	n := c.itemCount()
	if n == 0 {
		return
	}
	c.sel = (c.sel + delta + n) % n
}

func (c Checklist) values() []string {
	var out []string
	for _, s := range c.Sections {
		for _, it := range s.Items {
			if it.State != None {
				out = append(out, it.Value)
			}
		}
	}
	return out
}

func (c Checklist) Init() tea.Cmd { return nil }

func (c Checklist) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		c.w, c.h = v.Width, v.Height
		return c, nil
	case tea.KeyPressMsg:
		code, text := v.Code, v.Text
		switch {
		case code == tea.KeyUp || text == "k":
			c.move(-1)
		case code == tea.KeyDown || text == "j":
			c.move(1)
		case code == tea.KeyHome:
			c.sel = 0
		case code == tea.KeyEnd:
			c.sel = c.itemCount() - 1
		case text == " " || code == ' ':
			c.toggle()
		case code == tea.KeyEnter || code == '\r':
			return c, func() tea.Msg { return Confirmed{Values: c.values()} }
		case text == "q" || code == tea.KeyEscape || (code == 'c' && v.Mod&tea.ModCtrl != 0):
			return c, func() tea.Msg { return Quit{} }
		}
	}
	return c, nil
}

// toggle advances the focused item's state None -> Pending -> Done -> None.
func (c *Checklist) toggle() {
	si, ii, _ := c.locate(c.sel)
	if si < 0 {
		return
	}
	it := &c.Sections[si].Items[ii]
	it.State = (it.State + 1) % 3
}

func (c Checklist) glyph(s State) string {
	switch s {
	case Done:
		if c.utf8 {
			return "[✓]"
		}
		return "[x]"
	case Pending:
		return "[~]"
	}
	return "[ ]"
}

// View renders sections with heading rows and checkable items; the focused
// item row is styled Selected. It scrolls when taller than the area.
func (c Checklist) View() tea.View {
	// keep the focused item visible
	if r := c.rowOfItem(c.sel); r >= 0 {
		if r < c.top {
			c.top = r
		}
		if r >= c.top+c.h {
			c.top = r - c.h + 1
		}
	}
	maxRow := c.top + c.h
	var sb strings.Builder
	itemIdx := 0 // flat item index of the row being rendered
	row := 0
	for _, s := range c.Sections {
		if s.Title != "" {
			if row >= c.top && row < maxRow {
				sb.WriteString(c.theme.Title.Render(s.Title))
				sb.WriteByte('\n')
			}
			row++
		}
		for _, it := range s.Items {
			if row >= c.top && row < maxRow {
				line := c.itemLine(it, itemIdx == c.sel)
				if lipgloss.Width(line) > c.w {
					line = truncateCells(line, c.w)
				}
				if itemIdx == c.sel {
					sb.WriteString(c.theme.Selected.Render(line))
				} else if it.State == Done {
					sb.WriteString(c.theme.Dim.Render(line))
				} else {
					sb.WriteString(c.theme.Text.Render(line))
				}
				sb.WriteByte('\n')
			}
			itemIdx++
			row++
		}
	}
	return tea.NewView(strings.TrimRight(sb.String(), "\n"))
}

// itemLine renders one row: glyph, label, right-aligned tag.
func (c Checklist) itemLine(it CItem, focused bool) string {
	glyph := c.glyph(it.State)
	label := glyph + " " + it.Label
	if it.Tag == "" {
		return label
	}
	// right-align the tag inside the available width
	pad := c.w - lipgloss.Width(label) - lipgloss.Width(it.Tag) - 1
	if pad < 1 {
		pad = 1
	}
	return label + strings.Repeat(" ", pad) + it.Tag
}

// Input is a single-line text field, optionally masked for secrets.
type Input struct {
	ti     textinput.Model
	theme  Theme
	masked bool
	utf8   bool
	w      int
}

// NewInput builds a text field. masked hides the value behind a fixed run of
// bullets so its length never leaks.
func NewInput(placeholder string, masked bool, th Theme, utf8 bool) Input {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 4096
	ti.SetWidth(80)
	ti.Focus() // value semantics: focus must be set before the model is stored
	return Input{ti: ti, theme: th, masked: masked, utf8: utf8, w: 80}
}

// Value returns the entered text (always the real value, even when masked).
func (m Input) Value() string { return m.ti.Value() }

func (m Input) Init() tea.Cmd { return m.ti.Focus() }

func (m Input) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.w = v.Width
		m.ti.SetWidth(v.Width)
		return m, nil
	case tea.KeyPressMsg:
		code := v.Code
		switch {
		case code == tea.KeyEnter || code == '\r':
			return m, func() tea.Msg { return Submitted{Value: m.ti.Value()} }
		case code == tea.KeyEscape:
			return m, func() tea.Msg { return Cancelled{} }
		case code == 'c' && v.Mod&tea.ModCtrl != 0:
			return m, func() tea.Msg { return Cancelled{} }
		}
	}
	// delegate everything else (typing, paste, cursor keys) to textinput
	ti, cmd := m.ti.Update(msg)
	m.ti = ti
	return m, cmd
}

// View renders the field. Masked input shows a fixed run of bullets capped
// at 24 characters regardless of the real length.
func (m Input) View() tea.View {
	if m.masked {
		n := len([]rune(m.ti.Value()))
		if n > 24 {
			n = 24
		}
		bullet := "*"
		if m.utf8 {
			bullet = "•"
		}
		return tea.NewView(m.theme.Accent.Render(strings.Repeat(bullet, n)))
	}
	return tea.NewView(m.theme.Text.Render(m.ti.View()))
}
