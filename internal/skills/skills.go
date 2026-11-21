// Package skills installs tsunagi's agent skills into the shared skill
// directories other coding agents read, and tracks what it wrote in a
// manifest so updates and removal never touch a user's own files.
package skills

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/silent9669/tsunagi/internal/config"
)

//go:embed content/*/SKILL.md
var content embed.FS

// Target is one install destination: a directory every listed agent reads.
type Target struct {
	ID   string // "claude" or "agents"
	Name string
	Dir  string // resolved at call time, ~/.claude/skills or ~/.agents/skills
}

// Targets returns the two install targets with home resolved now.
func Targets() []Target {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []Target{
		{ID: "claude", Name: "Claude Code", Dir: filepath.Join(home, ".claude", "skills")},
		{ID: "agents", Name: "Codex, Cursor, opencode", Dir: filepath.Join(home, ".agents", "skills")},
	}
}

// Names lists the embedded skill names, sorted.
func Names() []string {
	var out []string
	entries, err := fs.ReadDir(content, "content")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Files returns the embedded skill bodies by name, for tests and tooling.
func Files() map[string]string {
	out := map[string]string{}
	for _, n := range Names() {
		if b, err := skillBytes(n); err == nil {
			out[n] = string(b)
		}
	}
	return out
}

// skillBytes returns the embedded SKILL.md for a skill name.
func skillBytes(name string) ([]byte, error) {
	return content.ReadFile("content/" + name + "/SKILL.md")
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fileEntry is one installed file in the manifest.
type fileEntry struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
}

// manifest records tsunagi's version at install time, which target skills
// dirs tsunagi created itself, and every file it wrote.
type manifest struct {
	Version string               `json:"version"`
	Created map[string]bool      `json:"created,omitempty"`
	Files   map[string]fileEntry `json:"files"`
}

func manifestPath() string {
	p := config.Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "skills.json")
}

func loadManifest() manifest {
	m := manifest{Files: map[string]fileEntry{}}
	b, err := os.ReadFile(manifestPath())
	if err != nil {
		return m
	}
	if json.Unmarshal(b, &m) != nil {
		return manifest{Files: map[string]fileEntry{}}
	}
	if m.Files == nil {
		m.Files = map[string]fileEntry{}
	}
	return m
}

// saveManifest writes the manifest atomically with 0600.
func saveManifest(m manifest) error {
	path := manifestPath()
	if path == "" {
		return errors.New("no config dir")
	}
	if len(m.Files) == 0 {
		return os.Remove(path) // nothing left to track
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Result is the outcome for one target x skill pair.
type Result struct {
	Target string `json:"target"`
	Skill  string `json:"skill"`
	State  string `json:"state"`            // installed, current, removed, missing, skipped
	Reason string `json:"reason,omitempty"` // modified, foreign, symlink, or an error
}

// StatusEntry is the on-disk state of one target x skill pair.
type StatusEntry struct {
	Target string `json:"target"`
	Skill  string `json:"skill"`
	State  string `json:"state"` // missing, installed, outdated, modified, foreign
}

func key(target, skill string) string { return target + "/" + skill }

func isLink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// diskHash returns the sha256 of <dir>/SKILL.md, or "" when absent.
func diskHash(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return ""
	}
	return sha(b)
}

// stateOf computes the Status state for one pair.
func stateOf(t Target, skill string, m manifest, embHash string) StatusEntry {
	dir := filepath.Join(t.Dir, skill)
	h := diskHash(dir)
	if h == "" {
		return StatusEntry{Target: t.ID, Skill: skill, State: "missing"}
	}
	e, tracked := m.Files[key(t.ID, skill)]
	if !tracked {
		return StatusEntry{Target: t.ID, Skill: skill, State: "foreign"}
	}
	switch {
	case h != e.SHA:
		return StatusEntry{Target: t.ID, Skill: skill, State: "modified"}
	case e.SHA == embHash:
		return StatusEntry{Target: t.ID, Skill: skill, State: "installed"}
	default:
		return StatusEntry{Target: t.ID, Skill: skill, State: "outdated"}
	}
}

// Status reports every target x skill pair's state.
func Status() []StatusEntry {
	m := loadManifest()
	var out []StatusEntry
	for _, t := range Targets() {
		for _, s := range Names() {
			b, err := skillBytes(s)
			if err != nil {
				continue
			}
			out = append(out, stateOf(t, s, m, sha(b)))
		}
	}
	return out
}

// resolveTargets maps ids ("claude", "agents", "all") to Target values.
func resolveTargets(ids []string) ([]Target, error) {
	all := Targets()
	if len(ids) == 0 {
		return all, nil
	}
	var out []Target
	for _, id := range ids {
		if id == "all" {
			out = append(out, all...)
			continue
		}
		found := false
		for _, t := range all {
			if t.ID == id {
				out = append(out, t)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown target %q (want claude, agents or all)", id)
		}
	}
	return out, nil
}

// Install writes missing or outdated skills into the given targets. It never
// overwrites a modified or foreign file, and refuses symlinked dirs.
func Install(ids []string, version string) []Result {
	targets, err := resolveTargets(ids)
	if err != nil {
		return []Result{{State: "error", Reason: err.Error()}}
	}
	m := loadManifest()
	if m.Created == nil {
		m.Created = map[string]bool{}
	}
	var out []Result
	changed := false
	for _, t := range targets {
		// refuse to write through a symlinked skills dir
		if isLink(t.Dir) {
			for _, s := range Names() {
				out = append(out, Result{Target: t.ID, Skill: s, State: "skipped", Reason: "symlink"})
			}
			continue
		}
		for _, s := range Names() {
			res := installOne(t, s, version, &m, &changed)
			out = append(out, res)
		}
	}
	if changed || m.Version != version {
		m.Version = version
		if err := saveManifest(m); err != nil {
			for i := range out {
				if out[i].State == "installed" {
					out[i].State = "skipped"
					out[i].Reason = "manifest: " + err.Error()
				}
			}
		}
	}
	return out
}

// installOne handles one target x skill pair and may update m.
func installOne(t Target, skill, version string, m *manifest, changed *bool) Result {
	b, err := skillBytes(skill)
	if err != nil {
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: err.Error()}
	}
	embHash := sha(b)
	dir := filepath.Join(t.Dir, skill)
	k := key(t.ID, skill)

	if isLink(dir) {
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "symlink"}
	}
	h := diskHash(dir)
	e, tracked := m.Files[k]

	// nothing on disk: write fresh. A dir that exists without SKILL.md is
	// foreign unless the manifest says we put it there.
	if h == "" {
		if _, err := os.Stat(dir); err == nil && !tracked {
			return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "foreign"}
		}
		_, targetErr := os.Stat(t.Dir)
		hadTarget := targetErr == nil
		if err := writeSkill(t.Dir, dir, b); err != nil {
			return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: err.Error()}
		}
		m.Files[k] = fileEntry{Path: filepath.Join(dir, "SKILL.md"), SHA: embHash}
		if !hadTarget {
			m.Created[t.ID] = true
		}
		*changed = true
		return Result{Target: t.ID, Skill: skill, State: "installed"}
	}

	// file on disk
	if tracked && h != e.SHA {
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "modified"}
	}
	if !tracked {
		if h == embHash {
			// identical content we never recorded: adopt it
			m.Files[k] = fileEntry{Path: filepath.Join(dir, "SKILL.md"), SHA: embHash}
			*changed = true
			return Result{Target: t.ID, Skill: skill, State: "current"}
		}
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "foreign"}
	}
	if h == embHash {
		return Result{Target: t.ID, Skill: skill, State: "current"}
	}
	// tracked, unmodified on disk, embedded changed: update
	if err := writeSkill(t.Dir, dir, b); err != nil {
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: err.Error()}
	}
	m.Files[k] = fileEntry{Path: filepath.Join(dir, "SKILL.md"), SHA: embHash}
	*changed = true
	return Result{Target: t.ID, Skill: skill, State: "installed"}
}

// writeSkill creates the skill dir and writes SKILL.md 0644.
func writeSkill(targetDir, skillDir string, b []byte) error {
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(skillDir, "SKILL.md")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Remove deletes tracked, unmodified skill files and cleans up empty dirs.
func Remove(ids []string) []Result {
	targets, err := resolveTargets(ids)
	if err != nil {
		return []Result{{State: "error", Reason: err.Error()}}
	}
	m := loadManifest()
	var out []Result
	changed := false
	for _, t := range targets {
		for _, s := range Names() {
			out = append(out, removeOne(t, s, &m, &changed))
		}
		// drop the target skills dir only if we created it and it is empty
		if m.Created[t.ID] {
			if entries, err := os.ReadDir(t.Dir); err == nil && len(entries) == 0 {
				if err := os.Remove(t.Dir); err == nil {
					delete(m.Created, t.ID)
					changed = true
				}
			}
		}
	}
	if changed {
		if err := saveManifest(m); err != nil {
			for i := range out {
				if out[i].State == "removed" {
					out[i].State = "skipped"
					out[i].Reason = "manifest: " + err.Error()
				}
			}
		}
	}
	return out
}

// removeOne handles one target x skill pair.
func removeOne(t Target, skill string, m *manifest, changed *bool) Result {
	k := key(t.ID, skill)
	e, tracked := m.Files[k]
	dir := filepath.Join(t.Dir, skill)
	p := filepath.Join(dir, "SKILL.md")

	if isLink(dir) || isLink(t.Dir) {
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "symlink"}
	}
	if !tracked {
		if _, err := os.Stat(p); err == nil {
			return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "foreign"}
		}
		return Result{Target: t.ID, Skill: skill, State: "missing"}
	}
	b, err := os.ReadFile(p)
	if err != nil || sha(b) != e.SHA {
		// missing or modified: leave it and its manifest entry alone
		if _, serr := os.Stat(p); serr != nil {
			delete(m.Files, k)
			*changed = true
			return Result{Target: t.ID, Skill: skill, State: "missing"}
		}
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: "modified"}
	}
	if err := os.Remove(p); err != nil {
		return Result{Target: t.ID, Skill: skill, State: "skipped", Reason: err.Error()}
	}
	// remove the skill dir when empty
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		os.Remove(dir)
	}
	delete(m.Files, k)
	*changed = true
	return Result{Target: t.ID, Skill: skill, State: "removed"}
}
