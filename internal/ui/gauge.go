package ui

import (
	"fmt"
	"strings"
)

// Gauge draws [#####-----] for a share between 0 and 1, colored by how much is
// left: mint above half, amber above a fifth, rose below.
func (t *Term) Gauge(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac*float64(width) + 0.5)
	c := Mint
	switch {
	case frac < 0.2:
		c = Rose
	case frac < 0.5:
		c = Amber
	}
	return t.Dim("[") + t.Paint(c, strings.Repeat("#", n)) + t.Dim(strings.Repeat("-", width-n)+"]") + fmt.Sprintf(" %3d%%", int(frac*100+0.5))
}

// Count formats a token count compactly: 950, 12.3k, 4.6M.
func Count(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}
