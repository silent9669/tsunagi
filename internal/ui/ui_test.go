package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func testTerm(color bool) *Term {
	return &Term{Out: &bytes.Buffer{}, TTY: true, Color: color, True: color, Anim: color, W: 80, H: 24}
}

func TestLogoRowsAreEqualWidthAndASCII(t *testing.T) {
	rows := LogoLines()
	if len(rows) != 6 {
		t.Fatalf("rows = %d", len(rows))
	}
	for _, r := range rows {
		if len(r) != len(rows[0]) {
			t.Fatalf("ragged logo: %q", r)
		}
	}
}

func TestSplashFramesAreASCIIAndFixedSize(t *testing.T) {
	for _, color := range []bool{true, false} {
		tm := testTerm(color)
		for _, ts := range []float64{0, 0.1, 0.4, 0.7, 1.0, 1.2, 2, 5} {
			for _, final := range []bool{false, true} {
				lines := tm.SplashFrame(ts, final)
				if len(lines) != splashH {
					t.Fatalf("t=%v lines = %d", ts, len(lines))
				}
				for _, l := range lines {
					if VisibleLen(l) != splashWidth(LogoLines()) {
						t.Fatalf("t=%v width %d: %q", ts, VisibleLen(l), l)
					}
					for i := 0; i < len(l); i++ {
						if l[i] >= 0x80 {
							t.Fatalf("non-ASCII byte in %q", l)
						}
					}
				}
			}
		}
	}
}

func TestNoColorFramesHaveNoEscapes(t *testing.T) {
	for _, l := range testTerm(false).SplashFrame(0.5, false) {
		if strings.Contains(l, "\x1b") {
			t.Fatalf("escape in no-color output: %q", l)
		}
	}
}

func TestEelStaysOnCanvasForAnyHeadPosition(t *testing.T) {
	c := NewCanvas(40, 9)
	for hx := -80; hx < 160; hx++ {
		DrawEel(c, hx, 4, 2, float64(hx)/3, hx%2 == 0)
	}
}

func TestSpinFrameHasFixedWidth(t *testing.T) {
	for i := 0; i < 80; i++ {
		if f := spinFrame(i); len(f) != trackW+2 {
			t.Fatalf("frame %d = %q", i, f)
		}
	}
}

func TestTableAlignsAroundColorCodes(t *testing.T) {
	var buf bytes.Buffer
	tm := &Term{Out: &buf, Color: true, True: true}
	tm.Table([]string{"name", "status"}, [][]string{{"a", tm.Paint(Mint, "ready")}, {"bbbb", "no key"}})
	var widths []int
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		widths = append(widths, VisibleLen(l))
	}
	for _, w := range widths {
		if w != widths[0] {
			t.Fatalf("ragged table: %v\n%s", widths, buf.String())
		}
	}
}

func TestDumpFrames(t *testing.T) {
	if os.Getenv("TSUNAGI_DUMP") == "" {
		t.Skip("set TSUNAGI_DUMP=1 to print frames")
	}
	tm := testTerm(false)
	for _, ts := range []float64{0.2, 0.45, 0.7, 1.0, 1.1} {
		t.Logf("t=%.2f\n%s", ts, strings.Join(tm.SplashFrame(ts, false), "\n"))
	}
	t.Logf("final\n%s", strings.Join(tm.SplashFrame(0, true), "\n"))
}

func TestRoutesFramesAreASCIIAndStableHeight(t *testing.T) {
	for _, color := range []bool{true, false} {
		tm := testTerm(color)
		for _, targets := range [][]string{nil, {"anthropic"}, {"anthropic", "chatgpt x2", "openrouter"}} {
			want := len(targets)
			if want == 0 {
				want = 1
			}
			var width int
			for _, ts := range []float64{0, 0.2, 0.5, 0.79} {
				for _, final := range []bool{false, true} {
					lines := tm.RoutesFrame(ts, targets, final)
					if len(lines) != want {
						t.Fatalf("lines = %d, want %d", len(lines), want)
					}
					for _, l := range lines {
						for i := 0; i < len(l); i++ {
							if l[i] >= 0x80 {
								t.Fatalf("non-ASCII in %q", l)
							}
						}
						if !color && strings.Contains(l, "\x1b") {
							t.Fatalf("escape in plain output: %q", l)
						}
					}
					if width == 0 {
						width = VisibleLen(lines[0])
					}
				}
			}
			if len(targets) > 0 && !strings.Contains(tm.RoutesFrame(0, targets, true)[len(targets)/2], "claude") {
				t.Fatal("the claude node is missing from the middle row")
			}
		}
	}
}

func TestGaugeAndCount(t *testing.T) {
	tm := testTerm(false)
	if g := tm.Gauge(0.5, 10); g != "[#####-----]  50%" {
		t.Fatalf("gauge = %q", g)
	}
	if tm.Gauge(-1, 4) != "[----]   0%" || tm.Gauge(9, 4) != "[####] 100%" {
		t.Fatal("gauge must clamp")
	}
	for n, want := range map[int]string{0: "0", 950: "950", 12345: "12.3k", 4_600_000: "4.6M"} {
		if got := Count(n); got != want {
			t.Errorf("Count(%d) = %s, want %s", n, got, want)
		}
	}
}
