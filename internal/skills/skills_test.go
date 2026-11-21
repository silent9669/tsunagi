package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestEmbeddedSkills(t *testing.T) {
	want := []string{"tsunagi-free", "tsunagi-handoff", "tsunagi-models", "tsunagi-setup"}
	got := Names()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Names()=%v want %v", got, want)
	}
	for _, n := range want {
		b, err := skillBytes(n)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		body := string(b)
		if !strings.HasPrefix(body, "---\n") {
			t.Fatalf("%s: missing frontmatter", n)
		}
		fm := strings.SplitN(body[4:], "\n---", 2)
		if len(fm) != 2 {
			t.Fatalf("%s: frontmatter not closed", n)
		}
		var name, desc string
		for _, line := range strings.Split(fm[0], "\n") {
			if strings.HasPrefix(line, "name: ") {
				name = strings.TrimSpace(strings.TrimPrefix(line, "name: "))
			}
			if strings.HasPrefix(line, "description: ") {
				desc = strings.TrimSpace(strings.TrimPrefix(line, "description: "))
			}
		}
		if name != n {
			t.Fatalf("%s: frontmatter name %q", n, name)
		}
		if desc == "" {
			t.Fatalf("%s: empty description", n)
		}
	}
}

func fileState(t *testing.T, target, skill string) string {
	t.Helper()
	for _, s := range Status() {
		if s.Target == target && s.Skill == skill {
			return s.State
		}
	}
	t.Fatalf("no status for %s/%s", target, skill)
	return ""
}

func manifestEntry(t *testing.T, target, skill string) (fileEntry, bool) {
	t.Helper()
	m := loadManifest()
	e, ok := m.Files[key(target, skill)]
	return e, ok
}

func TestInstallStatusRemove(t *testing.T) {
	home := setHome(t)

	// fresh install into both targets
	res := Install([]string{"claude", "agents"}, "test")
	for _, r := range res {
		if r.State != "installed" {
			t.Fatalf("install %s/%s: %s %s", r.Target, r.Skill, r.State, r.Reason)
		}
	}
	for _, tg := range Targets() {
		for _, s := range Names() {
			p := filepath.Join(tg.Dir, s, "SKILL.md")
			if _, err := os.Stat(p); err != nil {
				t.Fatalf("missing %s", p)
			}
			if st := fileState(t, tg.ID, s); st != "installed" {
				t.Fatalf("%s/%s state=%s", tg.ID, s, st)
			}
		}
	}
	// manifest written 0600
	mp := manifestPath()
	if fi, err := os.Stat(mp); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode: %v %v", fi, err)
	}
	// second install is a no-op
	res = Install([]string{"claude", "agents"}, "test")
	for _, r := range res {
		if r.State != "current" {
			t.Fatalf("reinstall %s/%s: %s", r.Target, r.Skill, r.State)
		}
	}

	// modified file is skipped and survives remove
	modPath := filepath.Join(home, ".claude", "skills", "tsunagi-free", "SKILL.md")
	if err := os.WriteFile(modPath, []byte("user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := fileState(t, "claude", "tsunagi-free"); st != "modified" {
		t.Fatalf("state=%s", st)
	}
	res = Install([]string{"claude"}, "test")
	for _, r := range res {
		if r.Skill == "tsunagi-free" && (r.State != "skipped" || r.Reason != "modified") {
			t.Fatalf("modified install: %+v", r)
		}
	}
	b, _ := os.ReadFile(modPath)
	if string(b) != "user edit\n" {
		t.Fatal("modified file was overwritten")
	}
	res = Remove([]string{"claude"})
	for _, r := range res {
		if r.Skill == "tsunagi-free" && (r.State != "skipped" || r.Reason != "modified") {
			t.Fatalf("modified remove: %+v", r)
		}
	}
	if _, err := os.Stat(modPath); err != nil {
		t.Fatal("modified file was removed")
	}

	// remove the rest; modified file and its dir stay
	Remove([]string{"agents"})
	res = Remove([]string{"claude"})
	for _, r := range res {
		if r.Skill == "tsunagi-free" {
			continue
		}
		if r.State != "removed" && r.State != "missing" {
			t.Fatalf("remove %s: %+v", r.Skill, r)
		}
	}
	// agents target dir was created by us and is now empty -> gone
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatal("created agents skills dir should be removed")
	}
	// claude skills dir pre-existed? it was created by install too -> but the
	// modified skill dir keeps it non-empty, so it stays
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "tsunagi-free")); err != nil {
		t.Fatal("modified skill dir should remain")
	}
}

func TestOutdatedGetsUpdated(t *testing.T) {
	setHome(t)
	Install([]string{"claude"}, "test")
	// fake an older embedded version: rewrite file + manifest consistently
	dir := filepath.Join(os.Getenv("HOME"), ".claude", "skills", "tsunagi-models")
	old := []byte("old content\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), old, 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadManifest()
	k := key("claude", "tsunagi-models")
	m.Files[k] = fileEntry{Path: m.Files[k].Path, SHA: sha(old)}
	if err := saveManifest(m); err != nil {
		t.Fatal(err)
	}
	if st := fileState(t, "claude", "tsunagi-models"); st != "outdated" {
		t.Fatalf("state=%s", st)
	}
	res := Install([]string{"claude"}, "test")
	for _, r := range res {
		if r.Skill == "tsunagi-models" && r.State != "installed" {
			t.Fatalf("outdated install: %+v", r)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	emb, _ := skillBytes("tsunagi-models")
	if string(b) != string(emb) {
		t.Fatal("outdated file not updated to embedded")
	}
}

func TestForeignAndSymlink(t *testing.T) {
	home := setHome(t)
	// foreign skill dir with the same name, never written by us
	fdir := filepath.Join(home, ".claude", "skills", "tsunagi-handoff")
	if err := os.MkdirAll(fdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fdir, "SKILL.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := fileState(t, "claude", "tsunagi-handoff"); st != "foreign" {
		t.Fatalf("state=%s", st)
	}
	res := Install([]string{"claude"}, "test")
	for _, r := range res {
		if r.Skill == "tsunagi-handoff" && (r.State != "skipped" || r.Reason != "foreign") {
			t.Fatalf("foreign install: %+v", r)
		}
	}
	b, _ := os.ReadFile(filepath.Join(fdir, "SKILL.md"))
	if string(b) != "mine\n" {
		t.Fatal("foreign file overwritten")
	}
	res = Remove([]string{"claude"})
	for _, r := range res {
		if r.Skill == "tsunagi-handoff" && (r.State != "skipped" || r.Reason != "foreign") {
			t.Fatalf("foreign remove: %+v", r)
		}
	}

	// symlinked skill dir is refused
	setHome(t)
	Install([]string{"agents"}, "test")
	target := filepath.Join(os.Getenv("HOME"), ".agents", "skills", "tsunagi-free")
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(os.Getenv("HOME"), "elsewhere")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, target); err != nil {
		t.Fatal(err)
	}
	res = Install([]string{"agents"}, "test")
	for _, r := range res {
		if r.Skill == "tsunagi-free" && (r.State != "skipped" || r.Reason != "symlink") {
			t.Fatalf("symlink install: %+v", r)
		}
	}
	res = Remove([]string{"agents"})
	for _, r := range res {
		if r.Skill == "tsunagi-free" && (r.State != "skipped" || r.Reason != "symlink") {
			t.Fatalf("symlink remove: %+v", r)
		}
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatal("symlink was removed")
	}
}

func TestPreexistingTargetDirKept(t *testing.T) {
	home := setHome(t)
	// ~/.claude/skills exists before tsunagi runs, with a canary file
	sd := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(sd, "my-own-skill")
	if err := os.MkdirAll(canary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canary, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a canary outside the target dirs too
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	Install([]string{"claude"}, "test")
	Remove([]string{"claude"})
	if _, err := os.Stat(filepath.Join(canary, "SKILL.md")); err != nil {
		t.Fatal("pre-existing skill dir contents removed")
	}
	if _, err := os.Stat(sd); err != nil {
		t.Fatal("pre-existing target dir removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatal("settings.json touched")
	}
	// manifest should be gone (all entries removed)
	if _, err := os.Stat(manifestPath()); !os.IsNotExist(err) {
		t.Fatal("manifest should be deleted when empty")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	setHome(t)
	Install([]string{"claude"}, "v1")
	m := loadManifest()
	if m.Version != "v1" {
		t.Fatalf("version=%q", m.Version)
	}
	if !m.Created["claude"] {
		t.Fatal("created flag missing")
	}
	// json is valid and has the required fields
	b, err := os.ReadFile(manifestPath())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["version"] != "v1" {
		t.Fatalf("manifest version %v", raw["version"])
	}
	files, _ := raw["files"].(map[string]any)
	if len(files) != len(Names()) {
		t.Fatalf("manifest files=%d", len(files))
	}
}
