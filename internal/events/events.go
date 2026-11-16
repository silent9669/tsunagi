// Package events records one line per routed request so `tsunagi watch` can
// show traffic from another terminal. Events never hold headers or bodies.
package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Event struct {
	Time     time.Time `json:"t"`
	Provider string    `json:"provider"`
	Model    string    `json:"model"`
	Path     string    `json:"path"`
	Status   int       `json:"status"`
	Millis   int64     `json:"ms"`
	// Account is the key or sign-in used, such as "openai" or "openai.work".
	Account string `json:"account,omitempty"`
	// In and Out are token counts the reply reported, when it reported them.
	In  int `json:"in,omitempty"`
	Out int `json:"out,omitempty"`
	// Skipped counts accounts passed over (rate limited, out of quota) first.
	Skipped int `json:"skipped,omitempty"`
}

// Dir is the private state directory, created on demand.
func Dir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "tsunagi")
	return dir, os.MkdirAll(dir, 0o700)
}

type Writer struct {
	mu sync.Mutex
	f  *os.File
}

// NewWriter opens events-<pid>.jsonl in dir and removes files older than a week.
func NewWriter(dir string) (*Writer, error) {
	old, _ := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	for _, p := range old {
		if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) > 7*24*time.Hour {
			os.Remove(p)
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("events-%d.jsonl", os.Getpid())),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f}, nil
}

func (w *Writer) Write(e Event) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.f.Write(append(b, '\n'))
}

func (w *Writer) Close() { w.f.Close() }

// Latest returns the newest events file in dir, or "" when there is none.
func Latest(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	var best string
	var bestT time.Time
	for _, p := range files {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(bestT) {
			best, bestT = p, st.ModTime()
		}
	}
	return best
}

// Tail returns events appended to a file since the last call.
type Tail struct {
	Path  string
	off   int64
	carry []byte
}

func (t *Tail) Poll() []Event {
	f, err := os.Open(t.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	t.off += int64(len(data))
	data = append(t.carry, data...)
	t.carry = nil
	var out []Event
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			t.carry = append([]byte(nil), data...)
			break
		}
		var e Event
		if json.Unmarshal(data[:i], &e) == nil {
			out = append(out, e)
		}
		data = data[i+1:]
	}
	return out
}

// ReadAll returns every event in dir since the given time, oldest first.
func ReadAll(dir string, since time.Time) []Event {
	files, _ := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	var out []Event
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range bytes.Split(b, []byte("\n")) {
			var e Event
			if len(line) > 0 && json.Unmarshal(line, &e) == nil && !e.Time.Before(since) {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}
