package art

import "math"

// Eel draws a lit, tapered eel on a w by h pixel grid. The spine is a
// sinusoid whose wave travels head to tail as phase grows; the head is at
// the right (param 1) and the tail at the left (param 0). Bubbles and a
// faint wave line are low-luminance decorations with Mask false.
func Eel(w, h int, phase float64) Bitmap {
	b := NewBitmap(w, h)
	if w < 8 || h < 8 {
		return *b
	}

	// Spine: s runs 0 (tail, left) to 1 (head, right).
	n := w*2 + 1
	pts := make([][2]float64, n)
	rad := make([]float64, n)
	fw, fh := float64(w), float64(h)
	x0, x1 := fw*0.07, fw*0.93
	cy := fh * 0.56
	maxR := math.Min(fh*0.10, 8)
	amp := fh * 0.18
	for i := 0; i < n; i++ {
		s := float64(i) / float64(n-1)
		x := x0 + (x1-x0)*s
		// amplitude envelope: steady head, full body wave, thin tail
		env := 0.3 + 0.7*math.Pow(math.Sin(math.Pi*(1-s)), 0.8)
		// the wave travels toward the tail (s=0) as phase grows
		y := cy + amp*env*math.Sin(phase-2*math.Pi*1.7*(1-s))
		pts[i] = [2]float64{x, y}
		// radius: rounded head, tapering to a thin tail
		rad[i] = maxR * (0.12 + 0.88*math.Pow(s, 0.8))
	}

	// tube shading: sqrt(1-d^2), brightest on the spine
	body := func(d, s float64) float64 { return math.Sqrt(math.Max(0, 1-d*d)) }
	b.Ribbon(pts, rad, true, body)

	// dorsal fin: a faint ribbon riding the top of the body
	fpts := make([][2]float64, n)
	frad := make([]float64, n)
	for i, p := range pts {
		// upward normal approx: previous point's direction
		var nx, ny float64 = 0, -1
		if i+1 < n {
			dx, dy := pts[i+1][0]-p[0], pts[i+1][1]-p[1]
			l := math.Hypot(dx, dy)
			if l > 0 {
				nx, ny = -dy/l, dx/l
			}
		}
		// the fin sits on the upper side of the spine
		if ny > 0 {
			nx, ny = -nx, -ny
		}
		fpts[i] = [2]float64{p[0] + nx*rad[i]*0.72, p[1] + ny*rad[i]*0.72}
		frad[i] = math.Max(1.2, rad[i]*0.16)
	}
	fin := func(d, s float64) float64 { return 0.38 * math.Sqrt(math.Max(0, 1-d*d)) }
	b.Ribbon(fpts, frad, true, fin)

	// eye: a dark hole near the head, slightly above the spine
	if n > 4 {
		i := int(float64(n-1) * 0.93)
		ex, ey := pts[i][0], pts[i][1]-rad[i]*0.4
		b.Disc(ex, ey, math.Max(1.6, rad[i]*0.17), func(float64) float64 { return 0 })
		// gill slit: a short dark arc behind the eye
		gx, gy := pts[i-6][0], pts[i-6][1]
		for k := 0; k <= 8; k++ {
			t := float64(k) / 8
			b.Line(gx+6*t, gy-3*t, gx+6*t+1, gy+3*t-1, 0.9, 0.10)
		}
	}

	// a few rising bubbles: rings at fixed columns, height from phase
	for i := 0; i < 6; i++ {
		bx := fw * (0.12 + 0.76*frac(float64(i)*0.618+0.13))
		rise := (phase*22 + float64(i)*31) - math.Floor((phase*22+float64(i)*31)/(fh*0.5))*(fh*0.5)
		by := fh*0.18 + fh*0.5 - rise
		if by < 2 || by > fh-2 || bx < 2 || bx > fw-2 {
			continue
		}
		b.Ring(bx, by, 1.4+float64(i%3)*0.5, 0.9, 0.30)
	}

	// a faint wave line along the top
	for x := 1; x < w-1; x++ {
		y := 2.5 + 1.6*math.Sin(float64(x)*0.11+phase*0.8)
		b.Line(float64(x), y, float64(x+1), 2.5+1.6*math.Sin(float64(x+1)*0.11+phase*0.8), 0.7, 0.10)
	}
	return *b
}

func frac(f float64) float64 { return f - math.Floor(f) }
