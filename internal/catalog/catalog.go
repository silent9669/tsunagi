// Package catalog keeps a local copy of the models.dev provider list, the
// catalogue opencode uses. It adds hundreds of OpenAI-compatible providers
// without shipping them in the binary.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/silent9669/tsunagi/internal/provider"
)

const URL = "https://models.dev/api.json"

// ModelInfo is what the catalogue knows about one model's capabilities.
// In and Out are the cost per million tokens, when models.dev lists them.
type ModelInfo struct {
	Tools   bool     `json:"tools,omitempty"`
	Context int      `json:"context,omitempty"`
	Output  int      `json:"output,omitempty"`
	In      *float64 `json:"in,omitempty"`
	Out     *float64 `json:"out,omitempty"`
}

// Entry is one provider, reduced to what the router needs.
type Entry struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	API     string               `json:"api"`
	Dialect string               `json:"dialect"`
	Env     []string             `json:"env,omitempty"`
	Models  []string             `json:"models,omitempty"`
	Caps    map[string]ModelInfo `json:"caps,omitempty"`
}

// defaultAPI covers providers that models.dev lists without an endpoint.
var defaultAPI = map[string]string{
	"cerebras":  "https://api.cerebras.ai/v1",
	"deepinfra": "https://api.deepinfra.com/v1/openai",
}

func Path() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tsunagi", "models.json")
}

// modelRaw is the part of a models.dev model object tsunagi keeps.
type modelRaw struct {
	ToolCall bool `json:"tool_call"`
	Limit    struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input  *float64 `json:"input"`
		Output *float64 `json:"output"`
	} `json:"cost"`
}

// Parse reduces a models.dev api.json body to the providers tsunagi can use.
func Parse(body []byte) ([]Entry, error) {
	var raw map[string]struct {
		Name   string              `json:"name"`
		API    string              `json:"api"`
		NPM    string              `json:"npm"`
		Env    []string            `json:"env"`
		Models map[string]modelRaw `json:"models"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	var out []Entry
	for id, p := range raw {
		// Claude has its own route. Ids with dots or other odd characters could
		// imitate "<provider>.<label>" account names, so they are not accepted.
		if id == "anthropic" || !provider.ValidName(id) {
			continue
		}
		api := p.API
		if api == "" {
			api = defaultAPI[id]
		}
		d, ok := provider.DialectForPackage(p.NPM)
		e := Entry{ID: id, Name: p.Name, API: api, Env: p.Env}
		if ok {
			// Providers whose package has no known Dialect stay in the cache
			// with an empty one: they can never become a routable Provider, but
			// their model capabilities still answer catalog.Caps.
			if !safeEndpoint(api) {
				continue
			}
			e.Dialect = string(d)
		}
		for m, mm := range p.Models {
			e.Models = append(e.Models, m)
			mi := ModelInfo{Tools: mm.ToolCall, Context: mm.Limit.Context, Output: mm.Limit.Output, In: mm.Cost.Input, Out: mm.Cost.Output}
			if mi.Tools || mi.Context > 0 || mi.Output > 0 || mi.In != nil || mi.Out != nil {
				if e.Caps == nil {
					e.Caps = map[string]ModelInfo{}
				}
				e.Caps[m] = mi
			}
		}
		sort.Strings(e.Models)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Sync downloads the catalogue and stores the reduced copy.
func Sync(c *http.Client, url string) (int, error) {
	path := Path()
	if path == "" {
		return 0, fmt.Errorf("no cache directory")
	}
	resp, err := c.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return 0, err
	}
	entries, err := Parse(body)
	if err != nil {
		return 0, err
	}
	b, _ := json.Marshal(map[string]any{"fetched": time.Now().UTC(), "providers": entries})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return 0, err
	}
	return len(entries), os.Rename(tmp, path)
}

// Load returns the cached entries, or nil when there is no cache.
func Load() []Entry { return LoadFrom(Path()) }

func LoadFrom(path string) []Entry {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f struct {
		Providers []Entry `json:"providers"`
	}
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	return f.Providers
}

// capsIndex is the cached catalogue as a provider -> model lookup, loaded
// once per process.
var capsIndex = sync.OnceValue(func() map[string]map[string]ModelInfo {
	m := map[string]map[string]ModelInfo{}
	for _, e := range Load() {
		if len(e.Caps) == 0 {
			continue
		}
		models := map[string]ModelInfo{}
		for k, v := range e.Caps {
			models[k] = v
		}
		m[e.ID] = models
	}
	return m
})

// Caps returns what the catalogue knows about a provider's model. ok is false
// for providers or models it does not know.
func Caps(provider, model string) (ModelInfo, bool) {
	mi, ok := capsIndex()[provider][model]
	return mi, ok
}

func (e Entry) Provider() provider.Provider {
	return provider.Provider{Name: e.ID, BaseURL: e.API, Dialect: provider.Dialect(e.Dialect), Untrusted: true}
}

// safeEndpoint accepts only https URLs with a real host. The catalogue is
// downloaded, so it must not be able to point a key at a plain-text, loopback
// or private address, or hide a different host behind user info.
func safeEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return false
	}
	if ip := net.ParseIP(h); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return false
	}
	return true
}
