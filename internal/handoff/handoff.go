// Package handoff lets one Claude Code session pass a plan to a tsunagi session
// and learn when it is done. tsunagi gives each session a done file and a
// registry entry; a Stop hook fills the file when the session ends its turn, so
// completion does not depend on the model remembering to say so.
package handoff

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Session is one running tsunagi launch.
type Session struct {
	PID     int       `json:"pid"`
	CWD     string    `json:"cwd"`
	TTY     string    `json:"tty,omitempty"`
	Started time.Time `json:"started"`
	Done    string    `json:"done"`
}

// Done is what a finished turn leaves behind.
type Done struct {
	Time    time.Time `json:"time"`
	Session int       `json:"session"`
	Source  string    `json:"source"` // "hook" or "manual"
	Summary string    `json:"summary"`
}

func sessionsDir(base string) string { return filepath.Join(base, "sessions") }

// DonePath is where session pid's completion lands.
func DonePath(base string, pid int) string {
	return filepath.Join(sessionsDir(base), fmt.Sprintf("%d.done.json", pid))
}

// Register records a live session and returns a function that removes it.
func Register(base string, s Session) (func(), error) {
	dir := sessionsDir(base)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s.Done = DonePath(base, s.PID)
	b, _ := json.Marshal(s)
	path := filepath.Join(dir, fmt.Sprintf("%d.json", s.PID))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	// The done file outlives the session on purpose: a waiter that started
	// first must still be able to read the final completion.
	return func() { os.Remove(path) }, nil
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// List returns live sessions, newest first, and removes entries of sessions
// that died without cleaning up.
func List(base string) []Session {
	files, _ := filepath.Glob(filepath.Join(sessionsDir(base), "*.json"))
	var out []Session
	for _, f := range files {
		if strings.HasSuffix(f, ".done.json") {
			continue
		}
		b, err := os.ReadFile(f)
		var s Session
		if err != nil || json.Unmarshal(b, &s) != nil || s.PID == 0 {
			continue
		}
		if !alive(s.PID) {
			os.Remove(f)
			os.Remove(DonePath(base, s.PID))
			continue
		}
		out = append(out, s)
	}
	// done files of sessions that are gone
	dones, _ := filepath.Glob(filepath.Join(sessionsDir(base), "*.done.json"))
	for _, f := range dones {
		var pid int
		if _, err := fmt.Sscanf(filepath.Base(f), "%d.done.json", &pid); err == nil && !alive(pid) {
			if st, err := os.Stat(f); err == nil && time.Since(st.ModTime()) > time.Minute {
				os.Remove(f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out
}

// WriteDone stores a completion, atomically.
func WriteDone(path string, d Done) error {
	if d.Time.IsZero() {
		d.Time = time.Now()
	}
	if len(d.Summary) > 8000 {
		d.Summary = d.Summary[:8000]
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadDone returns the stored completion, if any.
func ReadDone(path string) (Done, bool) {
	var d Done
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &d) != nil {
		return d, false
	}
	return d, true
}

// Wait blocks until a completion newer than since appears.
func Wait(ctx context.Context, path string, since time.Time, poll time.Duration) (Done, error) {
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		if d, ok := ReadDone(path); ok && d.Time.After(since) {
			return d, nil
		}
		select {
		case <-ctx.Done():
			return Done{}, ctx.Err()
		case <-t.C:
		}
	}
}

// LastAssistantText returns the text of the last assistant message in a Claude
// Code transcript (a JSONL file), which is the session's closing summary.
func LastAssistantText(path string) string {
	text, _ := lastAssistant(path)
	return text
}

// lastAssistant also reports whether the transcript ends with that assistant
// message. A Stop hook can run before the final message is written, in which
// case the transcript still ends with a tool result.
func lastAssistant(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	var last string
	endsWithText := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.Type != "assistant" {
			if line.Type == "user" || line.Type == "assistant" {
				endsWithText = false
			}
			continue
		}
		endsWithText = false
		var text []string
		var blocks []struct{ Type, Text string }
		if json.Unmarshal(line.Message.Content, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					text = append(text, b.Text)
				}
			}
		}
		if len(text) > 0 { // a turn that only called tools keeps the earlier text
			last = strings.Join(text, "\n\n")
			endsWithText = true
		}
	}
	return strings.TrimSpace(last), endsWithText
}

// SettledAssistantText is LastAssistantText for a Stop hook: it waits up to
// the given time for the final assistant message to reach the transcript.
func SettledAssistantText(path string, wait time.Duration) string {
	deadline := time.Now().Add(wait)
	for {
		text, final := lastAssistant(path)
		if final || time.Now().After(deadline) {
			return strings.TrimSpace(text)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// HookSettings is the --settings JSON that adds a Stop hook running command.
func HookSettings(command string) string {
	b, _ := json.Marshal(map[string]any{"hooks": map[string]any{"Stop": []any{
		map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}},
	}}})
	return string(b)
}

// ShellQuote wraps s in single quotes for a shell command line.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
