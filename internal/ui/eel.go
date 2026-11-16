package ui

import "math"

// EelLen is the body length in columns, not counting the head.
const EelLen = 30

func eelAmp(k int) float64 { return 0.55 + 0.45*math.Sin(math.Pi*float64(k)/float64(EelLen)) }

func eelRow(my, amp, phase float64, k int) int {
	return int(math.Round(my + amp*eelAmp(k)*math.Sin(phase-float64(k)*0.30)))
}

// DrawEel draws an eel swimming right with its head at column hx.
// The wave travels from head to tail as phase grows.
func DrawEel(c *Canvas, hx int, my, amp, phase float64, blink bool) {
	for k := 0; k < EelLen; k++ {
		y := eelRow(my, amp, phase, k)
		behind := eelRow(my, amp, phase, k+1)
		ch := byte('=')
		switch {
		case y > behind:
			ch = '\\'
		case y < behind:
			ch = '/'
		}
		if ch == '=' {
			switch {
			case k >= EelLen-3:
				ch = '.'
			case k >= EelLen-7:
				ch = '-'
			}
		}
		t := 1 - float64(k)/EelLen + 0.12*math.Sin(phase*2-float64(k)*0.5)
		c.Set(hx-2-k, y, ch, EelGrad(t))
	}
	hy := eelRow(my, amp, phase, 0)
	eye := byte('o')
	if blink {
		eye = '-'
	}
	c.Set(hx-1, hy, '(', Mint)
	c.Set(hx, hy, eye, White)
	c.Set(hx+1, hy, '>', Mint)
}

// DrawWater scatters slow ripples. It is a pure function of tsec.
func DrawWater(c *Canvas, tsec float64) {
	s := int(tsec * 8)
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			switch {
			case (x*7+y*13+s)%29 == 0:
				c.Set(x, y, '.', Lerp(Water, Deep, 0.4))
			case (x*5+y*3+4000-s/2)%53 == 0:
				c.Set(x, y, '~', Water)
			}
		}
	}
}

// Blinks reports whether the eye is shut at time tsec.
func Blinks(tsec float64) bool { return math.Mod(tsec, 2.6) > 2.45 }
