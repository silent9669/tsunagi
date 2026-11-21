package claudecfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, files map[string]string) (cwd string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cwd = filepath.Join(root, "proj")
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(cwd, 0o755)
	return cwd
}

func TestReadsModelAndPrecedence(t *testing.T) {
	cwd := setup(t, map[string]string{
		"home/.claude/settings.json":       `{"model":"sonnet","statusLine":{"type":"command"}}`,
		"proj/.claude/settings.local.json": `{"model":"haiku"}`,
	})
	in := Read(cwd)
	if m, sc := in.Model(); m != "haiku" || sc != Local {
		t.Fatalf("Model = %q from %s, want haiku from local", m, sc)
	}
	if !in.StatusLine() {
		t.Fatal("status line not seen")
	}
}

func TestMissingAndBrokenFilesAreSkipped(t *testing.T) {
	cwd := setup(t, map[string]string{"home/.claude/settings.json": `{broken`})
	in := Read(cwd)
	if len(in.Files) != 0 {
		t.Fatalf("files = %v", in.Files)
	}
	if m, _ := in.Model(); m != "" || len(in.Conflicts("http://127.0.0.1:1/t")) != 0 {
		t.Fatal("nothing should be reported")
	}
}

func TestBaseURLConflictIsFixableUnlessManaged(t *testing.T) {
	cwd := setup(t, map[string]string{
		"proj/.claude/settings.json": `{"env":{"ANTHROPIC_BASE_URL":"https://gw.example.com/secret-path?key=1"}}`,
	})
	in := Read(cwd)
	cs := in.Conflicts("http://127.0.0.1:5/tok")
	if len(cs) != 1 || cs[0].Key != "ANTHROPIC_BASE_URL" || !cs[0].Fixable || cs[0].Scope != Project {
		t.Fatalf("conflicts = %+v", cs)
	}
	if strings.Contains(cs[0].Detail, "secret-path") || strings.Contains(cs[0].Detail, "key=1") {
		t.Fatalf("detail leaks the path: %q", cs[0].Detail)
	}
	if p := in.Pins("http://127.0.0.1:5/tok"); p["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:5/tok" {
		t.Fatalf("pins = %v", p)
	}
	if len(in.Pins("https://gw.example.com/secret-path?key=1")) != 0 {
		t.Fatal("no pin needed when the value already matches")
	}
	if (Conflict{Scope: Managed}).Fixable || Managed.Pinnable() {
		t.Fatal("managed settings cannot be overridden")
	}
}

func TestOtherConflicts(t *testing.T) {
	cwd := setup(t, map[string]string{
		"home/.claude/settings.json": `{"env":{"CLAUDE_CODE_USE_BEDROCK":"1","ANTHROPIC_MODEL":"x"},"availableModels":["sonnet"],"hooks":{"PreModelSwitch":[]}}`,
	})
	got := map[string]bool{}
	for _, c := range Read(cwd).Conflicts("http://127.0.0.1:5/tok") {
		got[c.Key] = c.Fixable
	}
	for _, k := range []string{"CLAUDE_CODE_USE_BEDROCK", "availableModels", "PreModelSwitch"} {
		if fixable, ok := got[k]; !ok || fixable {
			t.Fatalf("%s: present=%v fixable=%v in %v", k, ok, fixable, got)
		}
	}
	if len(Read(cwd).Pins("http://127.0.0.1:5/tok")) != 0 {
		t.Fatal("only a base url conflict is pinned")
	}
}

func TestClaudeConfigDirIsHonoured(t *testing.T) {
	cwd := setup(t, map[string]string{"alt/settings.json": `{"model":"opus"}`})
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(filepath.Dir(cwd), "alt"))
	if m, _ := Read(cwd).Model(); m != "opus" {
		t.Fatalf("model = %q", m)
	}
}

func TestReadIsReadOnly(t *testing.T) {
	cwd := setup(t, map[string]string{"home/.claude/settings.json": `{"model":"sonnet"}`})
	p := filepath.Join(filepath.Dir(cwd), "home", ".claude", "settings.json")
	before, _ := os.Stat(p)
	Read(cwd).Conflicts("x")
	after, _ := os.Stat(p)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("file touched")
	}
}
