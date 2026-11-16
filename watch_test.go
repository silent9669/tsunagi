package main

import "testing"

func TestCleanDropsEscapeSequences(t *testing.T) {
	if got := clean("gpt\x1b[2J\x07-5\u009b"); got != "gpt[2J-5" {
		t.Fatalf("got %q", got)
	}
}
