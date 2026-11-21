package art

// Look selects how a Bitmap becomes terminal cells.
type Look string

const (
	Braille  Look = "braille"  // 2x4 dots per cell, U+2800 range
	Dither   Look = "dither"   // ASCII ramp with 4x4 Bayer threshold
	Halftone Look = "halftone" // ASCII ramp on a staggered grid
	Bricks   Look = "bricks"   // half blocks, two vertical pixels per cell
	Gradient Look = "gradient" // classic 70-char ASCII luminance ramp
)

// Looks lists every look in display order.
var Looks = []Look{Braille, Dither, Halftone, Bricks, Gradient}

// ParseLook validates a look name.
func ParseLook(s string) (Look, bool) {
	for _, l := range Looks {
		if string(l) == s {
			return l, true
		}
	}
	return "", false
}

// Cell is one rendered terminal cell. T is the subject parameter (0 tail,
// 1 head) for color gradients; On marks cells containing subject pixels.
type Cell struct {
	R  rune
	T  float64
	On bool
}

var (
	bayer4 = [4][4]float64{
		{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5},
	}
	// bayer8 is the classic 8x8 ordered matrix, values 0..63.
	bayer8 = [8][8]int{
		{0, 32, 8, 40, 2, 34, 10, 42},
		{48, 16, 56, 24, 50, 18, 58, 26},
		{12, 44, 4, 36, 14, 46, 6, 38},
		{60, 28, 52, 20, 62, 30, 54, 22},
		{3, 35, 11, 43, 1, 33, 9, 41},
		{51, 19, 59, 27, 49, 17, 57, 25},
		{15, 47, 7, 39, 13, 45, 5, 37},
		{63, 31, 55, 23, 61, 29, 53, 21},
	}
	ditherRamp   = " .:-=+*#%@"
	halftoneRamp = " .:oO0@"
	// gradientRamp is the classic 70-level ASCII luminance ramp.
	gradientRamp = " .'`^\",:;Il!i~+_-?][}{1)(|\\/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$"
)

// density is the subpixel block each look consumes per cell: braille, dither
// and halftone use 2x4, bricks and gradient 1x2.
func density(l Look) (dx, dy int) {
	switch l {
	case Bricks, Gradient:
		return 1, 2
	}
	return 2, 4
}

// Render turns a bitmap into cells for the given look. When utf8 is false,
// braille and bricks fall back to the dither ramp.
func Render(b Bitmap, look Look, utf8 bool) [][]Cell {
	if !utf8 && (look == Braille || look == Bricks) {
		look = Dither
	}
	dx, dy := density(look)
	cw, ch := b.W/dx, b.H/dy
	out := make([][]Cell, ch)
	for cy := 0; cy < ch; cy++ {
		row := make([]Cell, cw)
		for cx := 0; cx < cw; cx++ {
			row[cx] = renderCell(b, look, cx*dx, cy*dy, dx, dy)
		}
		out[cy] = row
	}
	return out
}

// renderCell computes one cell from its dx by dy subpixel block starting at
// (x0,y0). It also derives T (lum-weighted subject parameter) and On.
func renderCell(b Bitmap, look Look, x0, y0, dx, dy int) Cell {
	c := Cell{R: ' ', T: -1, On: false}
	var tw, tl float64
	switch look {
	case Braille:
		var bits rune
		for sy := 0; sy < 4; sy++ {
			for sx := 0; sx < 2; sx++ {
				x, y := x0+sx, y0+sy
				i := b.At(x, y)
				if i < 0 {
					continue
				}
				lum := b.Lum[i]
				thr := (bayer4[(y)%4][(x)%4] + 0.5) / 16
				if lum > thr {
					bits |= brailleBit(sx, sy)
				}
				c.On = c.On || b.Mask[i]
				tw, tl = accumulate(tw, tl, b, i, lum)
			}
		}
		c.R = rune(0x2800) + bits
	case Dither:
		on := 0
		for sy := 0; sy < dy; sy++ {
			for sx := 0; sx < dx; sx++ {
				x, y := x0+sx, y0+sy
				i := b.At(x, y)
				if i < 0 {
					continue
				}
				thr := (bayer4[(y)%4][(x)%4] + 0.5) / 16
				if b.Lum[i] > thr {
					on++
				}
				c.On = c.On || b.Mask[i]
				tw, tl = accumulate(tw, tl, b, i, b.Lum[i])
			}
		}
		idx := int(float64(on) / float64(dx*dy) * float64(len(ditherRamp)))
		if idx >= len(ditherRamp) {
			idx = len(ditherRamp) - 1
		}
		c.R = rune(ditherRamp[idx])
	case Halftone:
		// staggered sampling: odd rows shift the sample window one pixel
		var sum float64
		n := 0
		sx0 := x0
		if (y0/dy)%2 == 1 {
			sx0--
		}
		for sy := 0; sy < dy; sy++ {
			for sx := 0; sx < dx; sx++ {
				x, y := sx0+sx, y0+sy
				i := b.At(x, y)
				if i < 0 {
					continue
				}
				sum += b.Lum[i]
				n++
				c.On = c.On || b.Mask[i]
				tw, tl = accumulate(tw, tl, b, i, b.Lum[i])
			}
		}
		lum := 0.0
		if n > 0 {
			lum = sum / float64(n)
		}
		idx := int(lum * float64(len(halftoneRamp)))
		if idx >= len(halftoneRamp) {
			idx = len(halftoneRamp) - 1
		}
		c.R = rune(halftoneRamp[idx])
	case Bricks:
		up, lo := lumAt(b, x0, y0), lumAt(b, x0, y0+1)
		ou, ol := maskAt(b, x0, y0), maskAt(b, x0, y0+1)
		c.On = ou || ol
		if iu, il := b.At(x0, y0), b.At(x0, y0+1); iu >= 0 && il >= 0 {
			tw, tl = accumulate(tw, tl, b, iu, up)
			tw, tl = accumulate(tw, tl, b, il, lo)
		}
		switch {
		case up > 0.42 && lo > 0.42:
			c.R = '█'
		case up > 0.42:
			c.R = '▀'
		case lo > 0.42:
			c.R = '▄'
		}
	case Gradient:
		var sum float64
		n := 0
		for sy := 0; sy < dy; sy++ {
			for sx := 0; sx < dx; sx++ {
				i := b.At(x0+sx, y0+sy)
				if i < 0 {
					continue
				}
				sum += b.Lum[i]
				n++
				c.On = c.On || b.Mask[i]
				tw, tl = accumulate(tw, tl, b, i, b.Lum[i])
			}
		}
		lum := 0.0
		if n > 0 {
			lum = sum / float64(n)
		}
		idx := int(lum * float64(len(gradientRamp)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(gradientRamp) {
			idx = len(gradientRamp) - 1
		}
		c.R = rune(gradientRamp[idx])
	}
	if tl > 0 {
		c.T = tw / tl
	}
	return c
}

// brailleBit maps a 2x4 dot position to its U+2800 bit. Dots run
// 1,2,3,7 down the left and 4,5,6,8 down the right.
func brailleBit(sx, sy int) rune {
	bits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	return bits[sy][sx]
}

func lumAt(b Bitmap, x, y int) float64 {
	if i := b.At(x, y); i >= 0 {
		return b.Lum[i]
	}
	return 0
}

func maskAt(b Bitmap, x, y int) bool {
	if i := b.At(x, y); i >= 0 {
		return b.Mask[i]
	}
	return false
}

// accumulate adds a masked pixel's parameter weighted by luminance.
func accumulate(tw, tl float64, b Bitmap, i int, lum float64) (float64, float64) {
	if i >= 0 && i < len(b.param) && b.Mask[i] && b.param[i] >= 0 {
		w := lum + 0.05
		return tw + b.param[i]*w, tl + w
	}
	return tw, tl
}

// Dissolve mixes two same-size cell grids with an 8x8 ordered-dither
// threshold: cells flip from a to b as t goes 0 to 1. The result cell is
// taken wholesale from the shown source.
func Dissolve(a, b [][]Cell, t float64) [][]Cell {
	if len(a) != len(b) {
		return cloneCells(a)
	}
	if t <= 0 {
		return cloneCells(a)
	}
	if t >= 1 {
		return cloneCells(b)
	}
	out := make([][]Cell, len(a))
	for y, row := range a {
		if len(row) != len(b[y]) {
			return cloneCells(a)
		}
		out[y] = make([]Cell, len(row))
		for x, ca := range row {
			if float64(bayer8[y%8][x%8]) < t*64 {
				out[y][x] = b[y][x]
			} else {
				out[y][x] = ca
			}
		}
	}
	return out
}

func cloneCells(g [][]Cell) [][]Cell {
	out := make([][]Cell, len(g))
	for y, row := range g {
		out[y] = make([]Cell, len(row))
		copy(out[y], row)
	}
	return out
}
