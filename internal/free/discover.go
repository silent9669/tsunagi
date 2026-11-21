package free

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/catalog"
)

// Freshness is how long a cached discovery snapshot stays good.
const Freshness = 6 * time.Hour

// The public model lists discovery reads. They are variables so tests can
// point them at fake servers; production code never changes them.
var (
	OpenRouterURL = "https://openrouter.ai/api/v1/models"
	ZenURL        = "https://opencode.ai/zen/v1/models"
)

// FreeModel is one model a provider offers at no cost.
type FreeModel struct {
	Provider  string `json:"provider"` // tsunagi provider name
	ID        string `json:"id"`       // model id as the provider names it
	Context   int    `json:"context"`
	Tools     *bool  `json:"tools,omitempty"` // nil = unknown
	Reasoning bool   `json:"reasoning"`
}

// Snapshot is the cached result of the last successful discovery.
type Snapshot struct {
	FetchedAt time.Time   `json:"fetched_at"`
	Models    []FreeModel `json:"models"`
}

// Path is the discovery cache file; "" when there is no cache directory.
func Path() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tsunagi", "free.json")
}

// Load returns the cached snapshot; ok is false when there is none or it does
// not parse.
func Load() (Snapshot, bool) { return LoadFrom(Path()) }

// LoadFrom reads a snapshot file.
func LoadFrom(path string) (Snapshot, bool) {
	var s Snapshot
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &s) != nil {
		return Snapshot{}, false
	}
	return s, true
}

// store writes the snapshot atomically with 0600, like catalog.Sync.
func store(path string, s Snapshot) error {
	if path == "" {
		return fmt.Errorf("no cache directory")
	}
	b, err := json.Marshal(s)
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

// Refresh fetches both public lists and stores them, ignoring any cache.
func Refresh(ctx context.Context, c *http.Client) error {
	if c == nil {
		c = http.DefaultClient
	}
	var models []FreeModel
	for _, src := range []struct {
		url   string
		parse func([]byte) ([]FreeModel, error)
	}{
		{OpenRouterURL, parseOpenRouter},
		{ZenURL, parseZen},
	} {
		ms, err := fetch(ctx, c, src.url)
		if err != nil {
			return err
		}
		parsed, err := src.parse(ms)
		if err != nil {
			return fmt.Errorf("%s: %v", src.url, err)
		}
		models = append(models, parsed...)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})
	return store(Path(), Snapshot{FetchedAt: time.Now().UTC(), Models: models})
}

// Ensure returns the snapshot to display: the cache while it is fresh, a
// refresh when it is stale or missing. When the refresh fails it returns
// whatever the cache holds with offline=true; with no cache at all the
// snapshot is empty and offline is still true.
func Ensure(ctx context.Context, c *http.Client) (Snapshot, bool) {
	if s, ok := Load(); ok && time.Since(s.FetchedAt) < Freshness {
		return s, false
	}
	if err := Refresh(ctx, c); err == nil {
		if s, ok := Load(); ok {
			return s, false
		}
	}
	s, _ := Load()
	return s, true
}

func fetch(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// parseOpenRouter reads GET /api/v1/models: data[] with id, context_length,
// pricing.prompt/completion (strings) and supported_parameters. A model is
// free when both prices are zero.
func parseOpenRouter(body []byte) ([]FreeModel, error) {
	var resp struct {
		Data []struct {
			ID                  string                              `json:"id"`
			ContextLength       int                                 `json:"context_length"`
			Pricing             struct{ Prompt, Completion string } `json:"pricing"`
			SupportedParameters []string                            `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	var out []FreeModel
	for _, m := range resp.Data {
		free := zeroPrice(m.Pricing.Prompt) && zeroPrice(m.Pricing.Completion)
		if m.ID == "openrouter/free" {
			free = true
		}
		if !free {
			continue
		}
		fm := FreeModel{Provider: "openrouter", ID: m.ID, Context: m.ContextLength}
		if m.SupportedParameters != nil {
			tools := false
			for _, p := range m.SupportedParameters {
				if p == "tools" {
					tools = true
				}
			}
			fm.Tools = &tools
		}
		for _, p := range m.SupportedParameters {
			if p == "reasoning" || p == "include_reasoning" {
				fm.Reasoning = true
			}
		}
		out = append(out, fm)
	}
	return out, nil
}

// zeroPrice reports whether a models.dev/OpenRouter price string is zero.
func zeroPrice(s string) bool {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil && f == 0
}

// parseZen reads GET /zen/v1/models: data[] with id, limit.context,
// capabilities.{tools,reasoning}.supported and cost.text prices (numbers). A
// model is free when both prices are zero.
func parseZen(body []byte) ([]FreeModel, error) {
	var resp struct {
		Data []struct {
			ID    string `json:"id"`
			Limit struct {
				Context int `json:"context"`
			} `json:"limit"`
			Capabilities struct {
				Tools struct {
					Supported bool `json:"supported"`
				} `json:"tools"`
				Reasoning struct {
					Supported bool `json:"supported"`
				} `json:"reasoning"`
			} `json:"capabilities"`
			Cost struct {
				Text struct {
					Input struct {
						Price *float64 `json:"price"`
					} `json:"input"`
					Output struct {
						Price *float64 `json:"price"`
					} `json:"output"`
				} `json:"text"`
			} `json:"cost"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	var out []FreeModel
	for _, m := range resp.Data {
		// both prices must be present and zero; a missing cost is not "free"
		if m.Cost.Text.Input.Price == nil || m.Cost.Text.Output.Price == nil ||
			*m.Cost.Text.Input.Price != 0 || *m.Cost.Text.Output.Price != 0 {
			continue
		}
		tools := m.Capabilities.Tools.Supported
		out = append(out, FreeModel{
			Provider:  "opencode",
			ID:        m.ID,
			Context:   m.Limit.Context,
			Tools:     &tools,
			Reasoning: m.Capabilities.Reasoning.Supported,
		})
	}
	return out, nil
}

// UnverifiedProvider is a catalogue provider that is not in the registry but
// has models priced at zero on models.dev. Shown by `free --all` as
// unverified; it never enters a combo.
type UnverifiedProvider struct {
	Provider string
	Models   int // models priced at zero on models.dev
}

// Unverified lists catalogue providers with zero-cost models that are not
// registry providers. Ids containing "token-plan" or "coding-plan" are
// subscription plans, never free. Providers tsunagi cannot route (empty
// Dialect) are skipped: their cost data cannot be acted on.
func Unverified() []UnverifiedProvider {
	reg := map[string]bool{}
	for _, e := range Registry() {
		reg[e.ID] = true
	}
	var out []UnverifiedProvider
	for _, e := range catalog.Load() {
		if reg[e.ID] || e.Dialect == "" || isPlan(e.ID) {
			continue
		}
		n := 0
		for _, mi := range e.Caps {
			if mi.In != nil && mi.Out != nil && *mi.In == 0 && *mi.Out == 0 {
				n++
			}
		}
		if n > 0 {
			out = append(out, UnverifiedProvider{Provider: e.ID, Models: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

func isPlan(id string) bool {
	return strings.Contains(id, "token-plan") || strings.Contains(id, "coding-plan")
}
