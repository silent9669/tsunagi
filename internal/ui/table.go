package ui

import "strings"

// Table prints an ASCII table with +---+ borders. Cells may contain ANSI color.
func (t *Term) Table(headers []string, rows [][]string) {
	w := make([]int, len(headers))
	for i, h := range headers {
		w[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if n := VisibleLen(c); n > w[i] {
				w[i] = n
			}
		}
	}
	rule := "+"
	for _, n := range w {
		rule += strings.Repeat("-", n+2) + "+"
	}
	line := func(cells []string, style func(string) string) {
		var b strings.Builder
		b.WriteString(t.Dim("|"))
		for i, c := range cells {
			b.WriteString(" " + pad(style(c), w[i]) + " " + t.Dim("|"))
		}
		t.Printf("  %s\n", b.String())
	}
	t.Printf("  %s\n", t.Dim(rule))
	line(headers, t.Bold)
	t.Printf("  %s\n", t.Dim(rule))
	for _, r := range rows {
		line(r, func(s string) string { return s })
	}
	t.Printf("  %s\n", t.Dim(rule))
}
