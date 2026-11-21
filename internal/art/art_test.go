package art

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// goldenRenders the eel at cell size for every look and phase and compares
// against testdata/<look>-<phase>.txt.
func TestGoldenLooks(t *testing.T) {
	const cw, ch = 60, 14
	for _, look := range Looks {
		for _, phase := range []float64{0, 0.25} {
			dx, dy := density(look)
			bm := Eel(cw*dx, ch*dy, phase)
			cells := Render(bm, look, true)
			var sb strings.Builder
			for _, row := range cells {
				for _, c := range row {
					sb.WriteRune(c.R)
				}
				sb.WriteByte('\n')
			}
			name := string(look) + "-" + map[float64]string{0: "0", 0.25: "025"}[phase] + ".txt"
			path := filepath.Join("testdata", name)
			if *update {
				os.MkdirAll("testdata", 0o755)
				os.WriteFile(path, []byte(sb.String()), 0o644)
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s: %v (run with -update)", name, err)
			}
			if sb.String() != string(want) {
				t.Fatalf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, sb.String(), want)
			}
		}
	}
}

// TestDitherASCIIFallback checks the utf8=false path emits ASCII only.
func TestDitherASCIIFallback(t *testing.T) {
	dx, dy := density(Braille)
	bm := Eel(60*dx, 14*dy, 0.3)
	cells := Render(bm, Braille, false)
	for _, row := range cells {
		for _, c := range row {
			if c.R > 127 {
				t.Fatalf("non-ASCII rune %q in fallback", c.R)
			}
		}
	}
	// bricks also falls back
	cells = Render(bm, Bricks, false)
	for _, row := range cells {
		for _, c := range row {
			if c.R > 127 {
				t.Fatalf("non-ASCII rune %q in bricks fallback", c.R)
			}
		}
	}
}

func TestEelDeterministic(t *testing.T) {
	a := Eel(120, 56, 0.7)
	b := Eel(120, 56, 0.7)
	for i := range a.Lum {
		if a.Lum[i] != b.Lum[i] || a.Mask[i] != b.Mask[i] {
			t.Fatal("Eel not deterministic")
		}
	}
	ca, cb := Render(a, Braille, true), Render(b, Braille, true)
	if len(ca) != len(cb) {
		t.Fatal("render dims differ")
	}
	for y := range ca {
		for x := range ca[y] {
			if ca[y][x] != cb[y][x] {
				t.Fatal("Render not deterministic")
			}
		}
	}
}

func TestRenderDimensions(t *testing.T) {
	bm := Eel(120, 56, 0)
	for _, look := range Looks {
		dx, dy := density(look)
		cells := Render(bm, look, true)
		if len(cells) != 56/dy || len(cells[0]) != 120/dx {
			t.Fatalf("%s: got %dx%d cells", look, len(cells[0]), len(cells))
		}
	}
}

func TestBrailleBits(t *testing.T) {
	// hand-made 2x4 bitmap, one pixel on at a time
	for _, tc := range []struct {
		x, y int
		want rune
	}{
		{0, 0, 0x01}, {0, 1, 0x02}, {0, 2, 0x04}, {1, 0, 0x08},
		{1, 1, 0x10}, {1, 2, 0x20}, {0, 3, 0x40}, {1, 3, 0x80},
	} {
		bm := NewBitmap(2, 4)
		bm.Lum[tc.y*2+tc.x] = 1
		cells := Render(*bm, Braille, true)
		if got := cells[0][0].R; got != rune(0x2800)+tc.want {
			t.Fatalf("(%d,%d): got U+%04X want U+%04X", tc.x, tc.y, got, 0x2800+tc.want)
		}
	}
	// all dots on -> U+28FF
	bm := NewBitmap(2, 4)
	for i := range bm.Lum {
		bm.Lum[i] = 1
	}
	if got := Render(*bm, Braille, true)[0][0].R; got != 0x28FF {
		t.Fatalf("all dots: got U+%04X", got)
	}
}

func TestBricksMapping(t *testing.T) {
	bm := NewBitmap(2, 4)
	// cell (0,0) covers pixels (0,0) upper and (0,1) lower
	bm.Lum[0] = 0.9 // upper on
	bm.Lum[2] = 0.9 // lower on -> full block
	cells := Render(*bm, Bricks, true)
	if cells[0][0].R != '█' {
		t.Fatalf("both on: got %q", cells[0][0].R)
	}
	bm = NewBitmap(2, 4)
	bm.Lum[0] = 0.9 // upper only
	cells = Render(*bm, Bricks, true)
	if cells[0][0].R != '▀' {
		t.Fatalf("upper only: got %q", cells[0][0].R)
	}
	bm = NewBitmap(2, 4)
	bm.Lum[2] = 0.9 // lower only
	cells = Render(*bm, Bricks, true)
	if cells[0][0].R != '▄' {
		t.Fatalf("lower only: got %q", cells[0][0].R)
	}
	bm = NewBitmap(2, 4)
	cells = Render(*bm, Bricks, true)
	if cells[0][0].R != ' ' {
		t.Fatalf("none: got %q", cells[0][0].R)
	}
}

func TestDissolve(t *testing.T) {
	a := [][]Cell{{{R: 'a'}, {R: 'b'}}, {{R: 'c'}, {R: 'd'}}}
	b := [][]Cell{{{R: 'w'}, {R: 'x'}}, {{R: 'y'}, {R: 'z'}}}
	if got := Dissolve(a, b, 0); got[0][0].R != 'a' || got[1][1].R != 'd' {
		t.Fatal("t=0 must equal a")
	}
	if got := Dissolve(a, b, 1); got[0][0].R != 'w' || got[1][1].R != 'z' {
		t.Fatal("t=1 must equal b")
	}
	// mid t mixes deterministically
	mid := Dissolve(a, b, 0.5)
	seen := map[rune]bool{}
	for _, row := range mid {
		for _, c := range row {
			seen[c.R] = true
		}
	}
	if len(seen) < 2 {
		t.Fatal("t=0.5 should mix a and b cells")
	}
	// inputs unchanged
	if a[0][0].R != 'a' || b[0][0].R != 'w' {
		t.Fatal("Dissolve mutated inputs")
	}
}

func TestCellTAndOn(t *testing.T) {
	bm := Eel(120, 56, 0)
	cells := Render(bm, Braille, true)
	var ons, withT int
	for _, row := range cells {
		for _, c := range row {
			if c.On {
				ons++
			}
			if c.T > 0 && c.T <= 1 {
				withT++
			}
		}
	}
	if ons == 0 || withT == 0 {
		t.Fatalf("eel produced no subject cells: on=%d t=%d", ons, withT)
	}
}
