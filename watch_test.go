package main

import (
	"testing"
	"time"

	"github.com/silent9669/tsunagi/internal/events"
)

func TestCleanDropsEscapeSequences(t *testing.T) {
	if got := clean("gpt\x1b[2J\x07-5\u009b"); got != "gpt[2J-5" {
		t.Fatalf("got %q", got)
	}
}

func TestSparklineBucketsAndRamp(t *testing.T) {
	now := time.Now()
	evs := []events.Event{
		{Time: now.Add(-1 * time.Second)},
		{Time: now.Add(-2 * time.Second)},
		{Time: now.Add(-2 * time.Second)},
		{Time: now.Add(-7 * time.Second)},
		{Time: now.Add(-10 * time.Minute)}, // outside the window
	}
	got := sparkline(evs, now, 4, 5*time.Second)
	if len(got) != 4 || got[0] != '_' || got[1] != '_' || got[2] != ':' || got[3] != '#' {
		t.Fatalf("sparkline = %q", got)
	}
	if sparkline(nil, now, 0, time.Second) != "" || sparkline(nil, now, 3, time.Second) != "___" {
		t.Fatal("empty cases")
	}
}
