package slash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMakesAPluginWithSafeCommands(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "/opt/bin/tsunagi"); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	b, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if json.Unmarshal(b, &m) != nil || m["name"] != "tsunagi" {
		t.Fatalf("manifest = %s", b)
	}
	b, err := os.ReadFile(filepath.Join(dir, "commands", "models.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"allowed-tools: Bash(/opt/bin/tsunagi models --summary)", "disable-model-invocation: true", "!`/opt/bin/tsunagi models --summary`",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("models.md lacks %q:\n%s", want, s)
		}
	}
	for _, c := range commands {
		body, _ := os.ReadFile(filepath.Join(dir, "commands", c.name+".md"))
		if strings.Contains(string(body), "$ARGUMENTS") || strings.Contains(string(body), ":*)") {
			t.Errorf("%s: user text or a wildcard grant in a shell line:\n%s", c.name, body)
		}
	}
	if len(Names()) != len(commands) || Names()[0] != "/tsunagi:usage" {
		t.Fatalf("names = %v", Names())
	}
}

func TestWriteRefusesPathsThatNeedQuoting(t *testing.T) {
	for _, bad := range []string{"/with space/tsunagi", "/x/$(evil)", "relative/tsunagi", "/a;b"} {
		if err := Write(t.TempDir(), bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
