package handoff

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegisterListAndPruneDeadSessions(t *testing.T) {
	base := t.TempDir()
	cleanup, err := Register(base, Session{PID: os.Getpid(), CWD: "/work", Started: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	// a session whose process is gone
	dead, _ := json.Marshal(Session{PID: 99999999, CWD: "/gone", Started: time.Now().Add(-time.Hour)})
	os.WriteFile(filepath.Join(base, "sessions", "99999999.json"), dead, 0o600)

	got := List(base)
	if len(got) != 1 || got[0].PID != os.Getpid() || got[0].Done != DonePath(base, os.Getpid()) {
		t.Fatalf("list = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(base, "sessions", "99999999.json")); !os.IsNotExist(err) {
		t.Fatal("dead session entry was not pruned")
	}
	WriteDone(DonePath(base, os.Getpid()), Done{Summary: "x"})
	cleanup()
	if len(List(base)) != 0 {
		t.Fatal("cleanup must remove the entry")
	}
	if _, ok := ReadDone(DonePath(base, os.Getpid())); !ok {
		t.Fatal("the done file must outlive the session so a waiter can still read it")
	}
}

func TestWaitSeesOnlyCompletionsAfterTheStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	WriteDone(path, Done{Summary: "old", Time: time.Now().Add(-time.Minute)})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		time.Sleep(150 * time.Millisecond)
		WriteDone(path, Done{Summary: "fresh", Source: "hook"})
	}()
	d, err := Wait(ctx, path, time.Now(), 20*time.Millisecond)
	if err != nil || d.Summary != "fresh" {
		t.Fatalf("done=%+v err=%v (the old completion must not count)", d, err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if _, err := Wait(ctx2, filepath.Join(t.TempDir(), "none"), time.Now(), 20*time.Millisecond); err == nil {
		t.Fatal("wait must time out when nothing arrives")
	}
}

func TestLastAssistantTextSkipsToolOnlyTurns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"do it"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Working on it."},{"type":"tool_use","id":"t","name":"Bash","input":{}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"ok"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All done: 3 files changed."}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"u","name":"Read","input":{}}]}}`,
		`not json`,
	}
	os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600)
	if got := LastAssistantText(p); got != "All done: 3 files changed." {
		t.Fatalf("got %q", got)
	}
	if LastAssistantText(filepath.Join(t.TempDir(), "missing")) != "" {
		t.Fatal("missing file")
	}
}

func TestHookSettingsAndQuote(t *testing.T) {
	var s struct {
		Hooks struct {
			Stop []struct {
				Hooks []struct{ Type, Command string }
			}
		}
	}
	if err := json.Unmarshal([]byte(HookSettings("'/a b/tsunagi' done --hook")), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Hooks.Stop) != 1 || s.Hooks.Stop[0].Hooks[0].Type != "command" || s.Hooks.Stop[0].Hooks[0].Command != "'/a b/tsunagi' done --hook" {
		t.Fatalf("settings = %+v", s)
	}
	if ShellQuote("it's") != `'it'\''s'` {
		t.Fatalf("quote = %s", ShellQuote("it's"))
	}
}

func TestSettledAssistantTextWaitsForTheFinalMessage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	early := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Running it."},{"type":"tool_use","id":"t","name":"Bash","input":{}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"ok"}]}}` + "\n"
	os.WriteFile(p, []byte(early), 0o600)
	go func() {
		time.Sleep(300 * time.Millisecond)
		f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
		f.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Final answer."}]}}` + "\n")
		f.Close()
	}()
	if got := SettledAssistantText(p, 3*time.Second); got != "Final answer." {
		t.Fatalf("got %q", got)
	}
	// nothing more ever arrives: give up after the wait and use what there is
	os.WriteFile(p, []byte(early), 0o600)
	start := time.Now()
	if got := SettledAssistantText(p, 400*time.Millisecond); got != "Running it." || time.Since(start) < 300*time.Millisecond {
		t.Fatalf("got %q after %v", got, time.Since(start))
	}
}
