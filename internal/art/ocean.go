package art

import "math"

// OceanStaticPhase is the phase of a still frame: the waves are mid-swell and
// the eel is about the middle of the picture.
const OceanStaticPhase = 36.0

// Ocean draws a sea scene on a w by h pixel grid: a moving surface with foam,
// a dim water body that fades with depth, two sub-surface swell lines,
// swaying seaweed on the bottom, and an eel that swims across from left to
// right as phase grows, with a trail of bubbles. Every element is part of the
// subject (Mask true); Param colors them: 0..0.4 for the water by depth, 0.5
// to 1 along the eel's body.
func Ocean(w, h int, phase float64) Bitmap {
	b := NewBitmap(w, h)
	if w < 16 || h < 10 {
		return *b
	}
	fw, fh := float64(w), float64(h)
	surface := func(x float64) float64 {
		return fh*0.17 + fh*0.04*math.Sin(x*0.085+phase) + fh*0.022*math.Sin(x*0.19-phase*1.4)
	}
	put := func(x, y int, lum, param float64) {
		if i := b.At(x, y); i >= 0 && lum > 0 {
			b.add(i, lum, param, true)
		}
	}

	// water body: faint, thinning with depth, with a foam crest at the top
	for x := 0; x < w; x++ {
		sy := surface(float64(x))
		for y := int(sy); y < h; y++ {
			depth := (float64(y) - sy) / math.Max(fh-sy, 1)
			d := float64(y) - sy
			lum := 0.2 - 0.12*depth
			switch {
			case d < 1.4:
				lum = 0.95
			case d < 3.2:
				lum = 0.5
			}
			put(x, y, lum, 0.04+0.32*depth)
		}
	}

	// two swell lines under the surface
	for k, off := range []float64{0.33, 0.55} {
		for x := 0; x < w; x++ {
			y := fh*off + fh*0.03*math.Sin(float64(x)*0.11+phase*(0.8+0.3*float64(k))+float64(k)*2)
			put(x, int(y), 0.42, 0.14+0.1*float64(k))
		}
	}

	// seaweed: stalks rooted on the bottom, swaying with the swell
	for i := 0; i < 5; i++ {
		x0 := fw * (0.08 + 0.2*float64(i) + 0.03*math.Sin(float64(i)*7))
		height := fh * (0.2 + 0.07*math.Sin(float64(i)*3+1))
		for t := 0.0; t < 1; t += 0.5 / math.Max(height, 1) {
			x := x0 + 2.2*t*height*0.12*math.Sin(phase*0.9+float64(i)*1.7+t*4)
			y := fh - 1 - t*height
			put(int(x), int(y), 0.62, 0.26)
			put(int(x)+1, int(y), 0.5, 0.26)
		}
	}

	// the eel crosses the picture, bobbing a little
	eelLen := fw * 0.46
	span := fw + eelLen
	head := math.Mod(math.Max(phase, 0)*0.018*span, span)
	cy := fh*0.56 + fh*0.05*math.Sin(phase*0.3)
	R := math.Min(math.Max(fh*0.1, 2.2), 9)
	e := NewBitmap(w, h)
	tip := drawEel(e, head-eelLen, head, cy, fh*0.11, R, phase*1.6)
	for i := range e.Lum {
		if !e.Mask[i] {
			continue
		}
		b.Lum[i] = e.Lum[i]
		b.Mask[i] = true
		b.param[i] = 0.5 + 0.5*e.param[i]
	}

	// bubbles rising from the snout
	for i := 0; i < 4; i++ {
		t := frac(phase*0.07 + float64(i)/4)
		bx := tip[0] + 2 + float64(i)*1.5 + math.Sin(t*7+float64(i))*1.6
		by := tip[1] - R*0.7 - t*(cy-surface(bx))*0.95
		if by > surface(bx)+1 {
			r := 1 + 0.35*float64(i%3)
			for a := 0.0; a < 6.28; a += 0.6 {
				put(int(bx+r*math.Cos(a)), int(by+r*math.Sin(a)), 0.8, 0.45)
			}
		}
	}
	return *b
}
