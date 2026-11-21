package art

import "math"

// Eel draws a lit eel on a w by h pixel grid: one continuous body that
// undulates about one and a half times, a pointed head on the right (param 1)
// and a thin tail on the left (param 0). The wave travels from head to tail
// as phase grows. Bubbles and a faint water line are low-luminance
// decorations with Mask false.
func Eel(w, h int, phase float64) Bitmap {
	b := NewBitmap(w, h)
	if w < 8 || h < 8 {
		return *b
	}
	fw, fh := float64(w), float64(h)
	n := w*2 + 1
	x0, x1 := fw*0.06, fw*0.94
	cy := fh * 0.54
	amp := fh * 0.16
	R := math.Min(fh*0.13, 10)

	pts := make([][2]float64, n)
	rad := make([]float64, n)
	for i := range pts {
		s := float64(i) / float64(n-1)
		env := 0.3 + 0.7*(1-s)
		pts[i] = [2]float64{x0 + (x1-x0)*s, cy + amp*env*math.Sin(2*math.Pi*1.5*s+phase)}
		rad[i] = R * eelProfile(s)
	}
	// up returns the unit normal at point i that points up the screen.
	up := func(i int) (float64, float64) {
		j, k := max(i-1, 0), min(i+1, n-1)
		dx, dy := pts[k][0]-pts[j][0], pts[k][1]-pts[j][1]
		l := math.Hypot(dx, dy)
		nx, ny := dy/l, -dx/l
		if ny > 0 {
			nx, ny = -nx, -ny
		}
		return nx, ny
	}

	// body: a dim tube, then a narrower highlight riding the back
	b.Ribbon(pts, rad, true, func(d, s float64) float64 { return 0.5 + 0.3*math.Sqrt(math.Max(0, 1-d*d)) })
	hp := make([][2]float64, n)
	hr := make([]float64, n)
	for i, p := range pts {
		nx, ny := up(i)
		hp[i] = [2]float64{p[0] + nx*rad[i]*0.35, p[1] + ny*rad[i]*0.35}
		hr[i] = rad[i] * 0.5
	}
	b.Ribbon(hp, hr, true, func(d, s float64) float64 { return math.Sqrt(math.Max(0, 1-d*d)) })

	// dorsal fin: a thin ridge on the back from behind the head into the tail
	fp := make([][2]float64, 0, n)
	fr := make([]float64, 0, n)
	for i, p := range pts {
		s := float64(i) / float64(n-1)
		if s > 0.78 {
			break
		}
		nx, ny := up(i)
		fp = append(fp, [2]float64{p[0] + nx*rad[i]*1.02, p[1] + ny*rad[i]*1.02})
		fr = append(fr, math.Max(0.5, math.Min(rad[i]*0.14, (0.78-s)*18)))
	}
	b.Ribbon(fp, fr, true, func(d, s float64) float64 { return 0.7 })

	// head details: eye, mouth and gill slit cut out of the body
	at := func(s float64) int { return int(s * float64(n-1)) }
	ie := at(0.90)
	nx, ny := up(ie)
	ex, ey := pts[ie][0]+nx*rad[ie]*0.3, pts[ie][1]+ny*rad[ie]*0.3
	er := math.Max(1.2, R*0.2)
	b.Ring(ex, ey, er+1.3, 1.4, 1)
	b.Cut(ex, ey, er)
	im := at(0.985)
	b.CutLine(pts[im][0]-fw*0.07, pts[im][1]+rad[ie]*0.28, pts[im][0], pts[im][1]+rad[ie]*0.18, 0.55)
	ig := at(0.8)
	gx, gy := pts[ig][0], pts[ig][1]
	b.CutLine(gx+rad[ig]*0.1, gy-rad[ig]*0.55, gx-rad[ig]*0.2, gy+rad[ig]*0.55, 0.6)

	// three small bubbles rising from the snout
	hx, hy := pts[n-1][0], pts[n-1][1]
	for i := 0; i < 3; i++ {
		t := frac(phase/(2*math.Pi)*0.6 + float64(i)/3)
		bx := hx + fw*0.015*float64(i+1) + math.Sin(t*6+float64(i))*1.5
		by := hy - rad[ie]*0.8 - t*fh*0.4
		b.Ring(bx, by, 1.2+0.4*float64(i), 0.9, 0.45)
	}

	// a faint water line along the top
	for x := 1; x < w-1; x++ {
		y0 := 1.8 + 1.2*math.Sin(float64(x)*0.12+phase)
		y1 := 1.8 + 1.2*math.Sin(float64(x+1)*0.12+phase)
		b.Line(float64(x), y0, float64(x+1), y1, 0.6, 0.18)
	}
	return *b
}

// eelProfile is the body radius at arclength s (0 tail, 1 snout) relative
// to the full radius: a thin tail, an even body, a slightly fuller head and
// a pointed snout.
func eelProfile(s float64) float64 {
	switch {
	case s < 0.35:
		return 0.14 + 0.86*math.Pow(s/0.35, 0.8)
	case s < 0.86:
		return 1 + 0.12*math.Sin((s-0.35)/0.51*math.Pi/2)
	}
	t := (s - 0.86) / 0.14
	return math.Max(0.2, 1.12*math.Pow(1-t*t, 0.55))
}

func frac(f float64) float64 { return f - math.Floor(f) }
