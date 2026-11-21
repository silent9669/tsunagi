package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// CardsOK reports whether a bordered card fits: below these bounds callers
// fall back to plain indented lines.
func CardsOK(w, h int) bool { return w >= 50 && h >= 14 }

// BrandBar renders the one-row top bar: the gem, name and version on the
// left, chips or a breadcrumb dim on the right.
func (t Theme) BrandBar(version, right string, w int, utf8 bool) string {
	gem := "◆"
	if !utf8 {
		gem = "*"
	}
	left := t.Accent.Render(gem + " tsunagi")
	if version != "" {
		left += " " + t.Dim.Render(version)
	}
	r := t.Dim.Render(truncateCells(right, max(w-lipgloss.Width(left)-2, 0)))
	gap := w - lipgloss.Width(left) - lipgloss.Width(r)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + r
}

// Card draws a titled rounded box with optional right-aligned badges in the
// top border and one column of padding. Callers check CardsOK first.
func (t Theme) Card(title string, badges []string, body []string, w int, utf8 bool) []string {
	if w < 10 {
		var out []string
		for _, l := range body {
			out = append(out, truncateCells(l, w))
		}
		return out
	}
	h, v := "─", "│"
	tl, tr, bl, br := "╭", "╮", "╰", "╯"
	if !utf8 {
		h, v, tl, tr, bl, br = "-", "|", "+", "+", "+", "+"
	}
	var bs string
	if len(badges) > 0 {
		parts := make([]string, len(badges))
		for i, b := range badges {
			parts[i] = t.Badge(b)
		}
		bs = " " + strings.Join(parts, " ") + " "
	}
	head := t.Border.Render(tl+h) + " " + t.Title.Render(truncateCells(title, max(w-8, 4))) + " "
	tail := bs + t.Border.Render(tr)
	fill := w - lipgloss.Width(head) - lipgloss.Width(tail)
	if fill < 0 {
		fill = 0
	}
	top := head + t.Border.Render(strings.Repeat(h, fill)) + tail
	inner := max(w-4, 1)
	out := []string{top}
	for _, l := range body {
		l = truncateCells(l, inner)
		pad := inner - lipgloss.Width(l)
		if pad < 0 {
			pad = 0
		}
		out = append(out, t.Border.Render(v)+" "+l+strings.Repeat(" ", pad)+" "+t.Border.Render(v))
	}
	out = append(out, t.Border.Render(bl+strings.Repeat(h, w-2)+br))
	return out
}

// FieldBox wraps one content row in the rounded field border used for
// inputs. Below a usable width it returns the line itself.
func (t Theme) FieldBox(line string, w int, utf8 bool) []string {
	if w < 8 {
		return []string{line}
	}
	h, v := "─", "│"
	tl, tr, bl, br := "╭", "╮", "╰", "╯"
	if !utf8 {
		h, v, tl, tr, bl, br = "-", "|", "+", "+", "+", "+"
	}
	line = truncateCells(line, max(w-4, 1))
	pad := max(w-4-lipgloss.Width(line), 0)
	b := t.Border
	return []string{
		b.Render(tl + strings.Repeat(h, w-2) + tr),
		b.Render(v) + " " + line + strings.Repeat(" ", pad) + " " + b.Render(v),
		b.Render(bl + strings.Repeat(h, w-2) + br),
	}
}

// RailStep is one stage of a vertical flow rail; Note is the dim result the
// step leaves behind once it is done.
type RailStep struct {
	Label string
	Note  string
}

// Rail renders a vertical step rail: a start corner with the flow title,
// done steps with their result, the current step with its body embedded
// below it, pending steps dim, and the end corner.
//
//	┌ Set up Groq          ┌
//	◇ Account  ok          +
//	◆ Key                  *
//	│   <body>             |   <body>
//	○ Verify               .
//	└                      +
func (t Theme) Rail(title string, steps []RailStep, cur int, body []string, w int, utf8 bool) []string {
	start, link, done, now, next, end := "┌", "│", "◇", "◆", "○", "└"
	if !utf8 {
		start, link, done, now, next, end = "+", "|", "o", "*", ".", "+"
	}
	var out []string
	out = append(out, t.Accent.Render(start)+" "+t.Title.Render(truncateCells(title, max(w-2, 1))))
	out = append(out, t.Dim.Render(link))
	for i, st := range steps {
		glyph, style := next, t.Dim
		switch {
		case i < cur:
			glyph, style = done, t.Text
		case i == cur:
			glyph, style = now, t.Accent
		}
		line := style.Render(glyph) + " " + truncateCells(st.Label, max(w-6, 4))
		if st.Note != "" {
			line += "  " + t.Dim.Render(truncateCells(st.Note, max(w-lipgloss.Width(line)-2, 4)))
		}
		out = append(out, line)
		if i == cur {
			for _, l := range body {
				out = append(out, t.Dim.Render(link)+"   "+truncateCells(l, max(w-4, 1)))
			}
		}
	}
	out = append(out, t.Dim.Render(end))
	return out
}
