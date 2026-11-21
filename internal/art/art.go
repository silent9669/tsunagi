// Package art draws procedural pictures into a luminance bitmap and renders
// them as terminal cells in several looks. Everything is deterministic: the
// same inputs always give the same cells.
package art

import "math"

// Bitmap is a grid of luminance values in [0,1]. Mask marks pixels that
// belong to the subject (the eel's body); decorations leave it false. param
// optionally carries a per-pixel subject coordinate used for Cell.T.
type Bitmap struct {
	W, H  int
	Lum   []float64
	Mask  []bool
	param []float64
}

// NewBitmap returns a w by h bitmap with every pixel off.
func NewBitmap(w, h int) *Bitmap {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Bitmap{W: w, H: h, Lum: make([]float64, w*h), Mask: make([]bool, w*h), param: make([]float64, w*h)}
}

// At returns the pixel index, or -1 outside the grid.
func (b *Bitmap) At(x, y int) int {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return -1
	}
	return y*b.W + x
}

// add composites lum into pixel i keeping the maximum, and records param for
// masked pixels.
func (b *Bitmap) add(i int, lum, param float64, mask bool) {
	if i < 0 || i >= len(b.Lum) {
		return
	}
	if lum > b.Lum[i] {
		b.Lum[i] = lum
	}
	if mask {
		b.Mask[i] = true
		if param > b.param[i] {
			b.param[i] = param
		}
	}
}

// Disc stamps a filled circle of radius r centered at (cx,cy). shade, when
// non-nil, maps the normalized distance d in [0,1] to luminance.
func (b *Bitmap) Disc(cx, cy, r float64, shade func(d float64) float64) {
	if r <= 0 {
		return
	}
	x0 := int(math.Floor(cx - r))
	x1 := int(math.Ceil(cx + r))
	y0 := int(math.Floor(cy - r))
	y1 := int(math.Ceil(cy + r))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(dx, dy) / r
			if d > 1 {
				continue
			}
			lum := 1.0
			if shade != nil {
				lum = shade(d)
			}
			if i := b.At(x, y); i >= 0 {
				b.Lum[i] = math.Max(b.Lum[i], lum)
			}
		}
	}
}

// Line stamps a thick segment from (x0,y0) to (x1,y1) with the given radius.
func (b *Bitmap) Line(x0, y0, x1, y1, r float64, lum float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l == 0 {
		b.Disc(x0, y0, r, func(float64) float64 { return lum })
		return
	}
	steps := int(math.Ceil(l)) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		b.Disc(x0+dx*t, y0+dy*t, r, func(float64) float64 { return lum })
	}
}

// Ring stamps a circle outline: pixels whose distance to the center is
// within half the stroke width of r.
func (b *Bitmap) Ring(cx, cy, r, stroke, lum float64) {
	if r <= 0 {
		return
	}
	x0 := int(math.Floor(cx - r - stroke))
	x1 := int(math.Ceil(cx + r + stroke))
	y0 := int(math.Floor(cy - r - stroke))
	y1 := int(math.Ceil(cy + r + stroke))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			d := math.Abs(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - r)
			if d <= stroke/2 {
				if i := b.At(x, y); i >= 0 {
					b.Lum[i] = math.Max(b.Lum[i], lum)
				}
			}
		}
	}
}

// capsule stamps the area within radius r of segment a->b. For each covered
// pixel it computes the normalized distance d = dist/r, calls shade(d, s)
// with s the arclength parameter along the whole polyline, and keeps the
// brightest result. mask marks the pixels as subject; param (the s of the
// brightest contributor) is stored per pixel.
func (b *Bitmap) capsule(ax, ay, bx, by, ra, rb, s0, s1 float64, shade func(d, s float64) float64, mask bool) {
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return
	}
	rmax := math.Max(ra, rb)
	x0 := int(math.Floor(math.Min(ax, bx) - rmax))
	x1 := int(math.Ceil(math.Max(ax, bx) + rmax))
	y0 := int(math.Floor(math.Min(ay, by) - rmax))
	y1 := int(math.Ceil(math.Max(ay, by) + rmax))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := ((px-ax)*dx + (py-ay)*dy) / l2
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			qx, qy := ax+dx*t, ay+dy*t
			dist := math.Hypot(px-qx, py-qy)
			r := ra + (rb-ra)*t
			if r <= 0 || dist > r {
				continue
			}
			d := dist / r
			s := s0 + (s1-s0)*t
			lum := shade(d, s)
			i := b.At(x, y)
			if i < 0 {
				continue
			}
			if lum > b.Lum[i] {
				b.Lum[i] = lum
				if mask {
					b.param[i] = s
				}
			}
			if mask {
				b.Mask[i] = true
			}
		}
	}
}

// Ribbon stamps a filled shape along the polyline pts with per-point radii.
// shade maps (normalized distance, arclength parameter in [0,1]) to
// luminance; mask marks covered pixels as subject and stores the parameter.
func (b *Bitmap) Ribbon(pts [][2]float64, radii []float64, mask bool, shade func(d, s float64) float64) {
	if len(pts) < 2 || len(radii) != len(pts) {
		return
	}
	// arclength for the parameter
	cum := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		cum[i] = cum[i-1] + math.Hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1])
	}
	total := cum[len(cum)-1]
	if total == 0 {
		return
	}
	for i := 0; i+1 < len(pts); i++ {
		s0, s1 := cum[i]/total, cum[i+1]/total
		b.capsule(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], radii[i], radii[i+1], s0, s1, shade, mask)
	}
}

// Param returns the stored subject parameter of pixel (x,y), or -1.
func (b *Bitmap) Param(x, y int) float64 {
	i := b.At(x, y)
	if i < 0 || b.param == nil {
		return -1
	}
	return b.param[i]
}

// Cut clears a disc of radius r at (cx,cy) to luminance 0, leaving the mask
// as it is: used for the eye, gill and mouth.
func (b *Bitmap) Cut(cx, cy, r float64) {
	for y := int(math.Floor(cy - r)); y <= int(math.Ceil(cy+r)); y++ {
		for x := int(math.Floor(cx - r)); x <= int(math.Ceil(cx+r)); x++ {
			if math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) <= r {
				if i := b.At(x, y); i >= 0 {
					b.Lum[i] = 0
				}
			}
		}
	}
}

// CutLine clears a thick segment from (x0,y0) to (x1,y1).
func (b *Bitmap) CutLine(x0, y0, x1, y1, r float64) {
	l := math.Hypot(x1-x0, y1-y0)
	steps := int(math.Ceil(l)) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		b.Cut(x0+(x1-x0)*t, y0+(y1-y0)*t, r)
	}
}
