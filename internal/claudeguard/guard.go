// Package claudeguard keeps a tsunagi session from changing the user's
// Claude Code default model.
//
// Claude Code saves every `/model` choice as `model` in the user settings
// file, so switching to another provider's model inside a tsunagi session
// would become the default of every other Claude Code session, Claude Desktop
// included. The running session keeps its model in memory, so the guard puts
// the saved value back as soon as it sees a tsunagi route written there. It
// edits that one member in place and never touches anything else in the file.
package claudeguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Interval is how often the settings file is checked while a session runs.
const Interval = 100 * time.Millisecond

// SettingsPath is the user settings file of the Claude Code that tsunagi
// launches: $CLAUDE_CONFIG_DIR/settings.json, else ~/.claude/settings.json.
func SettingsPath() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "settings.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// model is the saved default: absent, or a raw string value.
type model struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

// member locates the top-level "model" member of a JSON object: the byte
// range of the key and of the value, and the value if it is a string.
type member struct {
	keyStart, valStart, valEnd int
	str                        string
	isStr                      bool
}

func findModel(doc []byte) (member, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return member{}, false, errors.New("settings file is not a JSON object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return member{}, false, err
		}
		key, _ := kt.(string)
		keyEnd := int(dec.InputOffset())
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return member{}, false, err
		}
		if key != "model" {
			continue
		}
		keyStart := keyEnd - len(`"model"`)
		if keyStart < 0 || string(doc[keyStart:keyEnd]) != `"model"` {
			return member{}, false, errors.New("unusual spelling of the model key")
		}
		m := member{keyStart: keyStart, valEnd: int(dec.InputOffset())}
		m.valStart = m.valEnd - len(raw)
		m.isStr = json.Unmarshal(raw, &m.str) == nil
		return m, true, nil
	}
	return member{}, false, nil
}

// replace sets the value of the member; remove deletes the member with its
// separating comma.
func replace(doc []byte, m member, v string) []byte {
	nv, _ := json.Marshal(v)
	out := append([]byte(nil), doc[:m.valStart]...)
	out = append(out, nv...)
	return append(out, doc[m.valEnd:]...)
}

func remove(doc []byte, m member) []byte {
	start, end := m.keyStart, m.valEnd
	// pull in the indentation before the key
	for start > 0 && (doc[start-1] == ' ' || doc[start-1] == '\t') {
		start--
	}
	i := end
	for i < len(doc) && (doc[i] == ' ' || doc[i] == '\t' || doc[i] == '\r' || doc[i] == '\n') {
		i++
	}
	if i < len(doc) && doc[i] == ',' {
		// a member follows: drop the comma and the line break after it
		end = i + 1
		for end < len(doc) && (doc[end] == ' ' || doc[end] == '\t') {
			end++
		}
		if end < len(doc) && doc[end] == '\r' {
			end++
		}
		if end < len(doc) && doc[end] == '\n' {
			end++
		}
	} else {
		// last member: drop the comma before it instead
		j := start
		for j > 0 && (doc[j-1] == ' ' || doc[j-1] == '\t' || doc[j-1] == '\r' || doc[j-1] == '\n') {
			j--
		}
		if j > 0 && doc[j-1] == ',' {
			start = j - 1
		}
	}
	return append(append([]byte(nil), doc[:start]...), doc[end:]...)
}

// writeFile replaces the settings file keeping its mode, writing through a
// symlink if the file is one.
func writeFile(path string, data []byte) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(real)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(real), ".settings-guard-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(st.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), real)
}

// Guard watches one settings file for the length of a session.
type Guard struct {
	path     string
	external func(string) bool
	stateDir string

	mu    sync.Mutex
	saved model // the default to keep
	last  os.FileInfo
	stop  chan struct{}
	done  chan struct{}
}

// Start records the current default and begins watching path. external says
// whether a model name is one tsunagi routes to another provider. stateDir
// receives a small crash-recovery file and may be empty to skip it. A missing
// or unreadable file is not an error: there is nothing to guard.
func Start(path, stateDir string, external func(string) bool) *Guard {
	g := &Guard{path: path, external: external, stateDir: stateDir, stop: make(chan struct{}), done: make(chan struct{})}
	if path == "" {
		close(g.done)
		return g
	}
	if doc, err := os.ReadFile(path); err == nil {
		if m, ok, err := findModel(doc); err == nil {
			g.saved = model{Present: ok}
			if ok && m.isStr {
				g.saved.Value = m.str
			}
			if ok && !m.isStr {
				g.saved = model{Present: true}
			}
		}
	}
	g.writeState()
	go g.run()
	return g
}

func (g *Guard) run() {
	defer close(g.done)
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		select {
		case <-g.stop:
			return
		case <-t.C:
			g.check(false)
		}
	}
}

// Stop does a last check, restores the default if needed and stops the watch.
func (g *Guard) Stop() {
	select {
	case <-g.stop:
	default:
		close(g.stop)
	}
	<-g.done
	if g.path != "" {
		g.check(true)
		g.clearState()
	}
}

// check looks at the settings file, and puts the saved default back when a
// tsunagi route was written over it. force skips the unchanged-file shortcut.
func (g *Guard) check(force bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, err := os.Stat(g.path)
	if err != nil {
		return
	}
	if !force && g.last != nil && st.ModTime().Equal(g.last.ModTime()) && st.Size() == g.last.Size() {
		return
	}
	doc, err := os.ReadFile(g.path)
	if err != nil {
		return
	}
	m, ok, err := findModel(doc)
	if err != nil {
		g.last = st
		return
	}
	switch {
	case ok && m.isStr && g.external(m.str):
		if g.saved.Present && g.saved.Value == m.str {
			break
		}
		var out []byte
		if g.saved.Present {
			out = replace(doc, m, g.saved.Value)
		} else {
			out = remove(doc, m)
		}
		if writeFile(g.path, out) == nil {
			if st2, err := os.Stat(g.path); err == nil {
				st = st2
			}
		}
	case ok && m.isStr:
		// the user picked a Claude model in the session: that is the new default
		g.saved = model{Present: true, Value: m.str}
		g.writeState()
	case !ok && g.saved.Present:
		g.saved = model{}
		g.writeState()
	}
	g.last = st
}

func (g *Guard) statePath() string {
	if g.stateDir == "" {
		return ""
	}
	return filepath.Join(g.stateDir, fmt.Sprintf("guard-%d.json", os.Getpid()))
}

func (g *Guard) writeState() {
	p := g.statePath()
	if p == "" {
		return
	}
	b, _ := json.Marshal(struct {
		Path string `json:"path"`
		model
	}{g.path, g.saved})
	if os.MkdirAll(g.stateDir, 0o700) == nil {
		_ = os.WriteFile(p, b, 0o600)
	}
}

func (g *Guard) clearState() {
	if p := g.statePath(); p != "" {
		os.Remove(p)
	}
}

// Recover repairs the settings file after a tsunagi that was killed before it
// could restore the default: for every state file whose process is gone, a
// tsunagi route saved as the default is replaced by the recorded value. It
// returns the number of files repaired.
func Recover(stateDir string, external func(string) bool) int {
	files, _ := filepath.Glob(filepath.Join(stateDir, "guard-*.json"))
	fixed := 0
	for _, f := range files {
		var pid int
		if _, err := fmt.Sscanf(filepath.Base(f), "guard-%d.json", &pid); err != nil || pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, 0); err == nil || errors.Is(err, syscall.EPERM) {
			continue // still running
		}
		var st struct {
			Path string `json:"path"`
			model
		}
		b, err := os.ReadFile(f)
		os.Remove(f)
		if err != nil || json.Unmarshal(b, &st) != nil || st.Path == "" || !strings.HasSuffix(st.Path, "settings.json") {
			continue
		}
		doc, err := os.ReadFile(st.Path)
		if err != nil {
			continue
		}
		m, ok, err := findModel(doc)
		if err != nil || !ok || !m.isStr || !external(m.str) {
			continue
		}
		var out []byte
		if st.Present {
			out = replace(doc, m, st.Value)
		} else {
			out = remove(doc, m)
		}
		if writeFile(st.Path, out) == nil {
			fixed++
		}
	}
	return fixed
}
