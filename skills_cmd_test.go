package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/skills"
)

func setSkillHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))
	return home
}

func TestSkillsJSON(t *testing.T) {
	setSkillHome(t)
	code, out := captureRun(t, "skills", "--json")
	if code != 0 {
		t.Fatalf("code=%d out=%s", code, out)
	}
	var st []struct {
		Target string `json:"target"`
		Skill  string `json:"skill"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(st) != 8 {
		t.Fatalf("entries=%d", len(st))
	}
	for _, s := range st {
		if s.State != "missing" {
			t.Fatalf("fresh state=%s", s.State)
		}
	}
}

func TestSkillsInstallAndRemove(t *testing.T) {
	home := setSkillHome(t)
	code, out := captureRun(t, "skills", "install", "--target", "claude")
	if code != 0 {
		t.Fatalf("install code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "claude/tsunagi-free") {
		t.Fatalf("missing rows:\n%s", out)
	}
	p := filepath.Join(home, ".claude", "skills", "tsunagi-free", "SKILL.md")
	if _, err := os.Stat(p); err != nil {
		t.Fatal("skill file not installed")
	}
	// agents target untouched
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatal("agents dir created unexpectedly")
	}
	code, _ = captureRun(t, "skills", "remove", "--target", "claude")
	if code != 0 {
		t.Fatalf("remove code=%d", code)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("skill file not removed")
	}
}

func TestSkillsNoTargetNonTTY(t *testing.T) {
	setSkillHome(t)
	for _, verb := range []string{"install", "remove"} {
		code, out := captureRun(t, "skills", verb)
		if code == 0 {
			t.Fatalf("%s without --target succeeded on non-TTY", verb)
		}
		if !strings.Contains(out, "--target") {
			t.Fatalf("%s: no usage error:\n%s", verb, out)
		}
	}
}

// TestSkillDocsUseRealCommands extracts every `tsunagi <word>` mention in the
// embedded skill docs and checks the command exists and any subcommand is one
// the command actually handles.
func TestSkillDocsUseRealCommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range commandList() {
		names[c.name] = true
	}
	// subcommands each command accepts (kept small on purpose)
	sub := map[string][]string{
		"free":      {"probe", "combo", "allow", "deny", "refresh", "opencode", "openrouter"},
		"models":    {},
		"sessions":  {},
		"handoff":   {},
		"wait":      {},
		"done":      {},
		"login":     {},
		"add":       {},
		"route":     {},
		"ping":      {},
		"usage":     {},
		"doctor":    {},
		"providers": {},
		"accounts":  {},
	}
	re := regexp.MustCompile(`tsunagi[ \t]+([a-z][a-z0-9-]*)`)
	subRe := map[string]*regexp.Regexp{}
	for cmd := range sub {
		subRe[cmd] = regexp.MustCompile(`tsunagi[ \t]+` + cmd + `[ \t]+([a-z][a-z0-9-]*)`)
	}
	for name, body := range skills.Files() {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			if !names[m[1]] {
				t.Fatalf("%s: unknown command %q", name, m[1])
			}
		}
		for cmd, sre := range subRe {
			for _, m := range sre.FindAllStringSubmatch(body, -1) {
				ok := false
				for _, s := range sub[cmd] {
					if m[1] == s {
						ok = true
					}
				}
				if !ok {
					t.Fatalf("%s: %s %s not a real subcommand", name, cmd, m[1])
				}
			}
		}
	}
}

// --- uninstall ---

func setupUninstallHome(t *testing.T) string {
	t.Helper()
	home := setSkillHome(t)
	// skills installed, cache populated, config + keys present
	code, _ := captureRun(t, "skills", "install", "--target", "all")
	if code != 0 {
		t.Fatal("setup install failed")
	}
	cacheDir := filepath.Join(home, "Library", "Caches", "tsunagi")
	// macOS UserCacheDir is ~/Library/Caches; XDG fallback if elsewhere
	if base, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(base, "tsunagi")
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, "plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"models.json", "free.json", "probe.json", "events-1.jsonl"} {
		if err := os.WriteFile(filepath.Join(cacheDir, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfgDir := filepath.Join(home, ".config", "tsunagi")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.SetKeyed("work/main", true, "api.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "chatgpt-auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// canaries the uninstall must never touch
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func cachePath(t *testing.T) string {
	t.Helper()
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(base, "tsunagi")
}

func TestUninstallDryNonTTY(t *testing.T) {
	home := setupUninstallHome(t)
	code, out := captureRun(t, "uninstall")
	if code != 2 {
		t.Fatalf("code=%d want 2\nout=%s", code, out)
	}
	if !strings.Contains(out, ".claude/skills") {
		t.Fatalf("plan missing skills:\n%s", out)
	}
	// nothing removed
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "tsunagi-free", "SKILL.md")); err != nil {
		t.Fatal("dry run removed a skill")
	}
	if _, err := os.Stat(cachePath(t)); err != nil {
		t.Fatal("dry run removed cache")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "tsunagi")); err != nil {
		t.Fatal("dry run removed config")
	}
}

func TestUninstallYesKeepsKeysAndConfig(t *testing.T) {
	home := setupUninstallHome(t)
	code, out := captureRun(t, "uninstall", "--yes")
	if code != 0 {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "binary stays") {
		t.Fatalf("missing binary hint:\n%s", out)
	}
	// skills + cache gone
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "tsunagi-free")); !os.IsNotExist(err) {
		t.Fatal("skill dir remains")
	}
	if _, err := os.Stat(cachePath(t)); !os.IsNotExist(err) {
		t.Fatal("cache dir remains")
	}
	// keys/config/canary remain
	for _, p := range []string{
		filepath.Join(home, ".config", "tsunagi", "config.json"),
		filepath.Join(home, ".config", "tsunagi", "keys.json"),
		filepath.Join(home, ".config", "tsunagi", "chatgpt-auth.json"),
		filepath.Join(home, ".claude", "settings.json"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s removed: %v", p, err)
		}
	}
}

func TestUninstallKeys(t *testing.T) {
	setupUninstallHome(t)
	var deleted []string
	old := keychainDelete
	keychainDelete = func(name string) error {
		deleted = append(deleted, name)
		return nil
	}
	defer func() { keychainDelete = old }()
	code, _ := captureRun(t, "uninstall", "--keys", "--yes")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(deleted) != 1 || deleted[0] != "work/main" {
		t.Fatalf("keychain deletes=%v", deleted)
	}
	// chatgpt file gone, keys.json and config.json stay
	home := os.Getenv("HOME")
	if _, err := os.Stat(filepath.Join(home, ".config", "tsunagi", "chatgpt-auth.json")); !os.IsNotExist(err) {
		t.Fatal("chatgpt-auth.json remains")
	}
	for _, f := range []string{"keys.json", "config.json"} {
		if _, err := os.Stat(filepath.Join(home, ".config", "tsunagi", f)); err != nil {
			t.Fatalf("%s removed", f)
		}
	}
}

func TestUninstallConfig(t *testing.T) {
	home := setupUninstallHome(t)
	code, _ := captureRun(t, "uninstall", "--config", "--yes")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "tsunagi")); !os.IsNotExist(err) {
		t.Fatal("config dir remains")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatal("settings.json touched")
	}
}
