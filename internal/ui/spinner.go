package ui

import (
	"fmt"
	"strings"
	"time"
)

const (
	trackW   = 14
	miniEel  = "~=-(o>"
	spinStep = 70 * time.Millisecond
)

// spinFrame draws a tiny eel crossing a track.
func spinFrame(i int) string {
	span := trackW + len(miniEel)
	p := i%span - len(miniEel)
	b := []byte(strings.Repeat(" ", trackW))
	for k := 0; k < len(miniEel); k++ {
		if x := p + k; x >= 0 && x < trackW {
			b[x] = miniEel[k]
		}
	}
	return "[" + string(b) + "]"
}

type Spinner struct {
	t     *Term
	label string
	stop  chan struct{}
	done  chan struct{}
}

// Spin shows an animated line until Stop. Without animation it stays quiet
// and Stop prints the result line.
func (t *Term) Spin(label string) *Spinner {
	s := &Spinner{t: t, label: label, stop: make(chan struct{}), done: make(chan struct{})}
	if !t.Anim {
		close(s.done)
		return s
	}
	restore := t.guardCursor()
	go func() {
		defer close(s.done)
		defer restore()
		tick := time.NewTicker(spinStep)
		defer tick.Stop()
		for i := 0; ; i++ {
			fmt.Fprintf(t.Out, "\r  %s %s\x1b[K", t.Paint(Teal, spinFrame(i)), label)
			select {
			case <-s.stop:
				return
			case <-tick.C:
			}
		}
	}()
	return s
}

// Stop ends the spinner and prints "[ok]" or "[!!]" with a detail column.
func (s *Spinner) Stop(ok bool, detail string) {
	if s.t.Anim {
		close(s.stop)
	}
	<-s.done
	mark := s.t.Paint(Mint, "[ok]")
	if !ok {
		mark = s.t.Paint(Rose, "[!!]")
	}
	lead, tail := "", ""
	if s.t.Anim {
		lead, tail = "\r", "\x1b[K" // overwrite the spinner line
	}
	s.t.Printf("%s  %s %s %s%s\n", lead, mark, pad(s.label, 10), s.t.Dim(detail), tail)
}
