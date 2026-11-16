package main

import (
	"strings"
	"testing"
)

func TestStatusLineNamesTheModelAndProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	got := statusLine("opencode/glm-5.3", "x")
	if !strings.HasPrefix(got, "tsunagi | opencode/glm-5.3") || !strings.HasSuffix(got, "opencode") {
		t.Fatalf("got %q", got)
	}
	if got := statusLine("claude-sonnet-5-5", ""); !strings.HasSuffix(got, "claude") {
		t.Fatalf("got %q", got)
	}
	if got := statusLine("", "Sonnet"); !strings.Contains(got, "Sonnet") {
		t.Fatalf("got %q", got)
	}
	if got := statusLine("m\x1b[2J", ""); strings.Contains(got, "\x1b") {
		t.Fatalf("escape in status line: %q", got)
	}
}
