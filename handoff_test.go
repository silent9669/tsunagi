package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/handoff"
)

func TestStopHookArgs(t *testing.T) {
	t.Setenv("TSUNAGI_NO_HOOK", "")
	got := stopHookArgs([]string{"-p", "hi"}, "/opt/my bin/tsunagi")
	if len(got) != 4 || got[0] != "--settings" || got[2] != "-p" || got[3] != "hi" {
		t.Fatalf("args = %v", got)
	}
	if !strings.Contains(got[1], `'/opt/my bin/tsunagi' done --hook`) || !json.Valid([]byte(got[1])) {
		t.Fatalf("settings = %s", got[1])
	}
	t.Setenv("TSUNAGI_STATUSLINE", "1")
	if g := stopHookArgs([]string{"a"}, "/t"); !strings.Contains(g[1], `"statusLine"`) {
		t.Fatalf("TSUNAGI_STATUSLINE should add the status line: %s", g[1])
	}
	t.Setenv("TSUNAGI_STATUSLINE", "")
	own := []string{"--settings", "x.json"}
	if g := stopHookArgs(own, "/t"); len(g) != 2 {
		t.Fatalf("the user's own --settings must be left alone: %v", g)
	}
	if g := stopHookArgs([]string{"--settings=x.json"}, "/t"); len(g) != 1 {
		t.Fatalf("--settings=…: %v", g)
	}
	t.Setenv("TSUNAGI_NO_HOOK", "1")
	if g := stopHookArgs([]string{"a"}, "/t"); len(g) != 1 {
		t.Fatalf("TSUNAGI_NO_HOOK ignored: %v", g)
	}
}

func TestPickSession(t *testing.T) {
	a, b := handoff.Session{PID: 10}, handoff.Session{PID: 20}
	if s, err := pickSession([]handoff.Session{a, b}, 0, 10); err != nil || s.PID != 20 {
		t.Fatalf("the only other session: %+v %v", s, err)
	}
	if _, err := pickSession([]handoff.Session{a, b}, 0, 99); err == nil {
		t.Fatal("two candidates need --session")
	}
	if s, err := pickSession([]handoff.Session{a, b}, 20, 99); err != nil || s.PID != 20 {
		t.Fatalf("named: %+v %v", s, err)
	}
	if _, err := pickSession([]handoff.Session{a}, 0, 10); err == nil {
		t.Fatal("a session must not hand off to itself")
	}
	if _, err := pickSession([]handoff.Session{a}, 77, 99); err == nil {
		t.Fatal("unknown session")
	}
}

func TestSessionFlag(t *testing.T) {
	rest, id, to, err := sessionFlag([]string{"--session", "42", "--timeout", "90s"})
	if err != nil || len(rest) != 0 || id != 42 || to.Seconds() != 90 {
		t.Fatalf("rest=%v id=%d to=%v err=%v", rest, id, to, err)
	}
	for _, bad := range [][]string{{"--session"}, {"--session", "x"}, {"--timeout", "soon"}} {
		if _, _, _, err := sessionFlag(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestPinArgs(t *testing.T) {
	pins := map[string]string{"ANTHROPIC_BASE_URL": "http://127.0.0.1:5/tok"}
	if got := pinArgs([]string{"-p", "hi"}, nil); len(got) != 2 {
		t.Fatalf("no pins must change nothing: %v", got)
	}
	got := pinArgs([]string{"-p", "hi"}, pins)
	if len(got) != 4 || got[0] != "--settings" || !strings.Contains(got[1], `"ANTHROPIC_BASE_URL":"http://127.0.0.1:5/tok"`) {
		t.Fatalf("pin not added as inline settings: %v", got)
	}
	// merged into the Stop hook settings tsunagi already adds
	base := stopHookArgs([]string{"a"}, "/t")
	merged := pinArgs(base, pins)
	if len(merged) != len(base) || !strings.Contains(merged[1], `"hooks"`) || !strings.Contains(merged[1], `"env"`) {
		t.Fatalf("hook settings and pin must share one --settings: %v", merged)
	}
	if base[1] == merged[1] {
		t.Fatal("the original args slice must not be modified")
	}
	// a settings file from the user cannot be merged
	if g := pinArgs([]string{"--settings", "my.json", "x"}, pins); g[1] != "my.json" || len(g) != 3 {
		t.Fatalf("user settings file must be left alone: %v", g)
	}
	if g := pinArgs([]string{"--settings=my.json"}, pins); len(g) != 1 {
		t.Fatalf("user settings file must be left alone: %v", g)
	}
	// a user JSON document gets the pin added and keeps its own keys
	g := pinArgs([]string{"--settings", `{"model":"haiku"}`}, pins)
	if !strings.Contains(g[1], `"model":"haiku"`) || !strings.Contains(g[1], "ANTHROPIC_BASE_URL") {
		t.Fatalf("merge lost a key: %v", g)
	}
}
