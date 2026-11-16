package ui

import "strings"

// Each glyph is six rows; letters sit on row 4 and descend to row 5.
var glyphs = map[byte][6]string{
	't': {" _   ", "| |_ ", "| __|", "| |_ ", " \\__|", "     "},
	's': {"     ", " ___ ", "/ __|", "\\__ \\", "|___/", "     "},
	'u': {"       ", " _   _ ", "| | | |", "| |_| |", " \\__,_|", "       "},
	'n': {"       ", " _ __  ", "| '_ \\ ", "| | | |", "|_| |_|", "       "},
	'a': {"       ", "  __ _ ", " / _` |", "| (_| |", " \\__,_|", "       "},
	'g': {"       ", "  __ _ ", " / _` |", "| (_| |", " \\__, |", " |___/ "},
	'i': {" _ ", "(_)", "| |", "| |", "|_|", "   "},
}

// LogoLines returns the word "tsunagi" as six equal-width ASCII rows.
func LogoLines() []string {
	rows := make([]string, 6)
	for i, ch := range []byte("tsunagi") {
		g := glyphs[ch]
		for r := range rows {
			if i > 0 {
				rows[r] += " "
			}
			rows[r] += g[r]
		}
	}
	w := 0
	for _, r := range rows {
		if len(r) > w {
			w = len(r)
		}
	}
	for r := range rows {
		rows[r] += strings.Repeat(" ", w-len(rows[r]))
	}
	return rows
}
