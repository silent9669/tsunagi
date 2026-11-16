package ui

import (
	"fmt"
	"time"
)

const (
	splashH     = 9
	splashX0    = 7
	splashLogoY = 1
	splashSpeed = 120.0 // columns per second
	frameDelay  = 25 * time.Millisecond
)

func splashWidth(logo []string) int { return len(logo[0]) + 2*splashX0 }

// SplashFrame is a pure function of time: the eel swims across, painting the
// logo behind it, then a glint sweeps the finished word. final drops the
// water and the eel for the frame left in the scrollback.
func (t *Term) SplashFrame(tsec float64, final bool) []string {
	logo := LogoLines()
	lw := len(logo[0])
	c := NewCanvas(splashWidth(logo), splashH)

	hx := int(tsec*splashSpeed) - 4
	swimEnd := float64(c.W+EelLen+8) / splashSpeed
	glint := -100
	if tsec > swimEnd {
		glint = int((tsec - swimEnd) * 200)
	}
	if !final {
		DrawWater(c, tsec)
	}
	for r, row := range logo {
		for i := 0; i < lw; i++ {
			if row[i] == ' ' {
				continue
			}
			x := splashX0 + i
			col := Lerp(Teal, Mint, float64(i)/float64(lw))
			if !final {
				if x > hx-6 {
					continue
				}
				col = Lerp(Deep, col, float64(hx-6-x)/14)
				if d := abs(x - glint); d < 4 {
					col = Lerp(col, White, 1-float64(d)/4)
				}
			}
			c.Set(x, splashLogoY+r, row[i], col)
		}
	}
	if !final && hx-EelLen-2 < c.W {
		DrawEel(c, hx, 4, 2.0, tsec*9, Blinks(tsec))
	}
	return c.Lines(t)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Splash plays the intro on a color terminal and prints the still logo
// everywhere else. It takes about a second.
func (t *Term) Splash() {
	logo := LogoLines()
	if t.W < splashWidth(logo) {
		return // too narrow to draw without wrapping
	}
	if !t.Anim {
		if t.TTY {
			t.draw(t.SplashFrame(0, true))
		}
		return
	}
	restore := t.guardCursor()
	defer restore()
	total := float64(splashWidth(logo)+EelLen+8)/splashSpeed + float64(splashWidth(logo)+8)/200
	start := time.Now()
	first := true
	for {
		el := time.Since(start).Seconds()
		if el >= total {
			break
		}
		if !first {
			fmt.Fprintf(t.Out, "\x1b[%dA", splashH)
		}
		t.draw(t.SplashFrame(el, false))
		first = false
		time.Sleep(frameDelay)
	}
	if !first {
		fmt.Fprintf(t.Out, "\x1b[%dA", splashH)
	}
	t.draw(t.SplashFrame(0, true))
}

func (t *Term) draw(lines []string) {
	for _, l := range lines {
		fmt.Fprint(t.Out, "\r"+l+"\x1b[K\n")
	}
}
