// Package claudecfg reads Claude Code's settings files, read-only, to find
// what would get in the way of a tsunagi session: an env block that points
// Claude somewhere else, a model allow-list that blocks `/model provider/x`.
// It never writes. Writing the default model back is claudeguard's job.
package claudecfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scope orders the settings files from weakest to strongest, as Claude Code
// applies them. A launch-time `--settings` flag sits between local and managed.
type Scope string

const (
	User    Scope = "user"
	Project Scope = "project"
	Local   Scope = "local"
	Managed Scope = "managed"
)

var order = []Scope{User, Project, Local, Managed}

// Pinnable reports whether a session's `--settings` flag can still override a
// value set in scope s: everything but managed settings.
func (s Scope) Pinnable() bool { return s != Managed }

// File is one settings file that exists and parsed.
type File struct {
	Scope Scope
	Path  string
	doc   map[string]json.RawMessage
}

// Info is what the settings files say about the things tsunagi cares about.
type Info struct {
	Files []File
}

// Paths returns where each scope's file lives for a launch in cwd.
func Paths(cwd string) map[Scope]string {
	m := map[Scope]string{}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		m[User] = filepath.Join(d, "settings.json")
	} else if home, err := os.UserHomeDir(); err == nil {
		m[User] = filepath.Join(home, ".claude", "settings.json")
	}
	if cwd != "" {
		m[Project] = filepath.Join(cwd, ".claude", "settings.json")
		m[Local] = filepath.Join(cwd, ".claude", "settings.local.json")
	}
	m[Managed] = "/Library/Application Support/ClaudeCode/managed-settings.json"
	return m
}

// Read loads every settings file that exists. Unreadable or invalid files
// are skipped: Claude Code itself ignores them.
func Read(cwd string) Info {
	var in Info
	paths := Paths(cwd)
	for _, sc := range order {
		p := paths[sc]
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var doc map[string]json.RawMessage
		if json.Unmarshal(b, &doc) != nil {
			continue
		}
		in.Files = append(in.Files, File{Scope: sc, Path: p, doc: doc})
	}
	return in
}

// Model is the saved default model and the scope that set it, "" if none.
func (in Info) Model() (string, Scope) {
	var m string
	var at Scope
	for _, f := range in.Files {
		var v string
		if raw, ok := f.doc["model"]; ok && json.Unmarshal(raw, &v) == nil && v != "" {
			m, at = v, f.Scope
		}
	}
	return m, at
}

// EnvValue is an env-block value and where it was set.
type EnvValue struct {
	Value string
	Scope Scope
}

// Env merges the env blocks; a stronger scope wins.
func (in Info) Env() map[string]EnvValue {
	out := map[string]EnvValue{}
	for _, f := range in.Files {
		var env map[string]any
		if raw, ok := f.doc["env"]; ok && json.Unmarshal(raw, &env) == nil {
			for k, v := range env {
				switch x := v.(type) {
				case string:
					out[k] = EnvValue{x, f.Scope}
				case bool, float64:
					b, _ := json.Marshal(x)
					out[k] = EnvValue{string(b), f.Scope}
				}
			}
		}
	}
	return out
}

// AvailableModels is the `availableModels` allow-list, nil when unset.
func (in Info) AvailableModels() []string {
	var out []string
	for _, f := range in.Files {
		var v []string
		if raw, ok := f.doc["availableModels"]; ok && json.Unmarshal(raw, &v) == nil {
			out = v
		}
	}
	return out
}

// HasHook reports whether any file defines a hook for the event.
func (in Info) HasHook(event string) bool {
	for _, f := range in.Files {
		var h map[string]json.RawMessage
		if raw, ok := f.doc["hooks"]; ok && json.Unmarshal(raw, &h) == nil {
			if _, ok := h[event]; ok {
				return true
			}
		}
	}
	return false
}

// StatusLine reports whether a status line is configured.
func (in Info) StatusLine() bool {
	for _, f := range in.Files {
		if _, ok := f.doc["statusLine"]; ok {
			return true
		}
	}
	return false
}

// Conflict is something in the settings that changes how a tsunagi session
// behaves. Fixable means the session's own `--settings` can win over it.
type Conflict struct {
	Key     string
	Scope   Scope
	Detail  string
	Fixable bool
}

// thirdParty are the env switches that make Claude Code skip ANTHROPIC_BASE_URL.
var thirdParty = []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CODE_USE_ANTHROPIC_AWS"}

// Conflicts lists what would stop routerURL from being used or `/model
// provider/model` from being accepted. The router address is never part of
// the detail: it carries the session token.
func (in Info) Conflicts(routerURL string) []Conflict {
	var out []Conflict
	env := in.Env()
	if v, ok := env["ANTHROPIC_BASE_URL"]; ok && v.Value != routerURL {
		out = append(out, Conflict{"ANTHROPIC_BASE_URL", v.Scope,
			"the " + string(v.Scope) + " settings send Claude to " + redactURL(v.Value), v.Scope.Pinnable()})
	}
	for _, k := range thirdParty {
		if v, ok := env[k]; ok && truthy(v.Value) {
			out = append(out, Conflict{k, v.Scope, "Claude Code is set to use another cloud, so it skips the router", false})
		}
	}
	if len(in.AvailableModels()) > 0 {
		out = append(out, Conflict{"availableModels", User, "the model allow-list rejects names it does not list, so /model provider/model is refused unless listed", false})
	}
	if in.HasHook("PreModelSwitch") {
		out = append(out, Conflict{"PreModelSwitch", User, "a PreModelSwitch hook can block a switch to another provider", false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func truthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true"
}

// redactURL keeps scheme and host so the detail is useful without showing a
// path or token.
func redactURL(u string) string {
	i := strings.Index(u, "://")
	if i < 0 {
		return "another address"
	}
	rest := u[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	return u[:i+3] + rest
}

// Pins is the env a session's `--settings` should carry so tsunagi's router
// address wins over every pinnable conflict.
func (in Info) Pins(routerURL string) map[string]string {
	for _, c := range in.Conflicts(routerURL) {
		if c.Key == "ANTHROPIC_BASE_URL" && c.Fixable {
			return map[string]string{"ANTHROPIC_BASE_URL": routerURL}
		}
	}
	return nil
}
