package ui

import "strings"

type cell struct {
	ch byte
	c  RGB
}

// Canvas is a fixed grid of ASCII cells with a color each.
type Canvas struct {
	W, H  int
	cells []cell
}

func NewCanvas(w, h int) *Canvas {
	c := &Canvas{W: w, H: h, cells: make([]cell, w*h)}
	for i := range c.cells {
		c.cells[i].ch = ' '
	}
	return c
}

// Set ignores coordinates outside the grid.
func (c *Canvas) Set(x, y int, ch byte, col RGB) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.cells[y*c.W+x] = cell{ch, col}
}

func (c *Canvas) At(x, y int) byte {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return ' '
	}
	return c.cells[y*c.W+x].ch
}

// Lines renders every row at full width, switching color only when it changes.
func (c *Canvas) Lines(t *Term) []string {
	out := make([]string, c.H)
	for y := 0; y < c.H; y++ {
		var b strings.Builder
		var cur RGB
		have := false
		for x := 0; x < c.W; x++ {
			ce := c.cells[y*c.W+x]
			if ce.ch != ' ' && t.Color && (!have || ce.c != cur) {
				b.WriteString(t.fg(ce.c))
				cur, have = ce.c, true
			}
			b.WriteByte(ce.ch)
		}
		if have {
			b.WriteString("\x1b[0m")
		}
		out[y] = b.String()
	}
	return out
}
