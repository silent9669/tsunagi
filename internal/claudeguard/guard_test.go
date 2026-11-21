package claudeguard

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ext(m string) bool { return strings.Contains(m, "/") }

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func modelOf(t *testing.T, p string) (string, bool) {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal([]byte(read(t, p)), &d); err != nil {
		t.Fatalf("settings is not valid JSON: %v\n%s", err, read(t, p))
	}
	v, ok := d["model"].(string)
	return v, ok
}

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for i := 0; i < 60; i++ {
		if f() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", what)
}

func TestEditsKeepTheRestOfTheFile(t *testing.T) {
	docs := map[string]string{
		"middle": "{\n  \"a\": 1,\n  \"model\": \"openrouter/x\",\n  \"b\": {\"model\": \"keep\"}\n}\n",
		"first":  "{\"model\": \"openrouter/x\", \"a\": [1, 2]}",
		"last":   "{\n  \"a\": 1,\n  \"model\": \"openrouter/x\"\n}\n",
		"only":   "{\n  \"model\": \"openrouter/x\"\n}\n",
	}
	for name, doc := range docs {
		m, ok, err := findModel([]byte(doc))
		if err != nil || !ok || !m.isStr || m.str != "openrouter/x" {
			t.Fatalf("%s: findModel = %+v %v %v", name, m, ok, err)
		}
		got := string(replace([]byte(doc), m, "sonnet"))
		want := strings.Replace(doc, "openrouter/x", "sonnet", 1)
		if got != want {
			t.Fatalf("%s: replace\n got %q\nwant %q", name, got, want)
		}
		rm := remove([]byte(doc), m)
		var d map[string]any
		if err := json.Unmarshal(rm, &d); err != nil {
			t.Fatalf("%s: remove left invalid JSON: %v\n%s", name, err, rm)
		}
		if _, has := d["model"]; has {
			t.Fatalf("%s: model still present after remove: %s", name, rm)
		}
		if name == "middle" && !strings.Contains(string(rm), `"b": {"model": "keep"}`) {
			t.Fatalf("nested model key lost: %s", rm)
		}
	}
}

func TestFindModelIgnoresNestedAndAbsent(t *testing.T) {
	if _, ok, err := findModel([]byte(`{"hooks": {"model": "x"}, "a": 1}`)); err != nil || ok {
		t.Fatalf("nested model must not count: ok=%v err=%v", ok, err)
	}
	if _, _, err := findModel([]byte(`[1]`)); err == nil {
		t.Fatal("non-object should error")
	}
	if m, ok, err := findModel([]byte(`{"model": 3}`)); err != nil || !ok || m.isStr {
		t.Fatalf("non-string model: %+v %v %v", m, ok, err)
	}
}

func TestGuardRestoresRouteAndKeepsOtherChanges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, "{\n  \"model\": \"sonnet\",\n  \"theme\": \"dark\"\n}\n")
	g := Start(p, "", ext)
	defer g.Stop()
	// Claude Code writes the route, and another session changes the theme in
	// the same write
	write(t, p, "{\n  \"model\": \"openrouter/free\",\n  \"theme\": \"light\",\n  \"x\": 1\n}\n")
	eventually(t, "model restored", func() bool { m, _ := modelOf(t, p); return m == "sonnet" })
	if got := read(t, p); !strings.Contains(got, `"theme": "light"`) || !strings.Contains(got, `"x": 1`) {
		t.Fatalf("other keys were not preserved:\n%s", got)
	}
}

func TestGuardAdoptsClaudeModelChoice(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, `{"model": "sonnet"}`)
	g := Start(p, "", ext)
	defer g.Stop()
	write(t, p, `{"model": "opus[1m]"}`)
	time.Sleep(400 * time.Millisecond)
	if m, _ := modelOf(t, p); m != "opus[1m]" {
		t.Fatalf("a Claude model choice must be left alone, got %q", m)
	}
	write(t, p, `{"model": "groq/llama-3"}`)
	eventually(t, "back to the last Claude model", func() bool { m, _ := modelOf(t, p); return m == "opus[1m]" })
}

func TestGuardRemovesMemberWhenNoneWasSaved(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, "{\n  \"theme\": \"dark\"\n}\n")
	g := Start(p, "", ext)
	defer g.Stop()
	write(t, p, "{\n  \"theme\": \"dark\",\n  \"model\": \"google/gemini\"\n}\n")
	eventually(t, "member removed", func() bool { _, ok := modelOf(t, p); return !ok })
	if th := read(t, p); !strings.Contains(th, `"theme": "dark"`) {
		t.Fatalf("theme lost: %s", th)
	}
}

func TestStopDoesAFinalRestore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, `{"model": "haiku"}`)
	g := Start(p, "", ext)
	write(t, p, `{"model": "ollama/foo"}`)
	g.Stop() // before any poll could run
	if m, _ := modelOf(t, p); m != "haiku" {
		t.Fatalf("model = %q after Stop, want haiku", m)
	}
}

func TestGuardWritesThroughSymlinkAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles-settings.json")
	link := filepath.Join(dir, "settings.json")
	write(t, real, `{"model": "sonnet"}`)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	g := Start(link, "", ext)
	defer g.Stop()
	write(t, real, `{"model": "x/y"}`)
	eventually(t, "restored via symlink", func() bool { m, _ := modelOf(t, real); return m == "sonnet" })
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
}

func TestMissingOrBrokenSettingsAreHarmless(t *testing.T) {
	dir := t.TempDir()
	g := Start(filepath.Join(dir, "nope.json"), "", ext)
	g.Stop()
	p := filepath.Join(dir, "settings.json")
	write(t, p, "{not json")
	g = Start(p, "", ext)
	time.Sleep(250 * time.Millisecond)
	g.Stop()
	if read(t, p) != "{not json" {
		t.Fatal("a broken file must be left exactly as it was")
	}
	Start("", "", ext).Stop()
}

func TestRecoverRepairsAfterACrash(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	p := filepath.Join(dir, "settings.json")
	write(t, p, `{"model": "mistral/large", "a": 1}`)
	// a state file left by a tsunagi whose process has exited
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skip("no true binary")
	}
	dead := cmd.Process.Pid
	os.MkdirAll(state, 0o700)
	rec, _ := json.Marshal(map[string]any{"path": p, "present": true, "value": "sonnet"})
	write(t, filepath.Join(state, "guard-"+itoa(dead)+".json"), string(rec))
	// and one that belongs to a live process (our parent) which must be skipped
	live := filepath.Join(state, "guard-"+itoa(os.Getppid())+".json")
	write(t, live, string(rec))

	if n := Recover(state, ext); n != 1 {
		t.Fatalf("Recover = %d, want 1", n)
	}
	if m, _ := modelOf(t, p); m != "sonnet" {
		t.Fatalf("model = %q, want sonnet", m)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("the live process's state file must stay")
	}
	if _, err := os.Stat(filepath.Join(state, "guard-"+itoa(dead)+".json")); err == nil {
		t.Fatal("the stale state file should be removed")
	}
}

func TestRecoverLeavesClaudeModelsAlone(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	write(t, p, `{"model": "opus"}`)
	cmd := exec.Command("true")
	if cmd.Run() != nil {
		t.Skip("no true binary")
	}
	rec, _ := json.Marshal(map[string]any{"path": p, "present": true, "value": "sonnet"})
	write(t, filepath.Join(dir, "guard-"+itoa(cmd.Process.Pid)+".json"), string(rec))
	if n := Recover(dir, ext); n != 0 {
		t.Fatalf("Recover = %d, want 0", n)
	}
	if m, _ := modelOf(t, p); m != "opus" {
		t.Fatalf("model = %q, a Claude model must not be rewritten", m)
	}
}

func TestStateFileLifecycle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	write(t, p, `{"model": "sonnet"}`)
	g := Start(p, filepath.Join(dir, "state"), ext)
	files, _ := filepath.Glob(filepath.Join(dir, "state", "guard-*.json"))
	if len(files) != 1 {
		t.Fatalf("state files while running = %v", files)
	}
	g.Stop()
	files, _ = filepath.Glob(filepath.Join(dir, "state", "guard-*.json"))
	if len(files) != 0 {
		t.Fatalf("state file left after Stop: %v", files)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
