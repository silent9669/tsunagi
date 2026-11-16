package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/ui"
)

// parseAdd reads: add <name> <url> [--dialect d] [--local] [--header "K: V"]...
// [--auth-header name] [--strategy s].
func parseAdd(args []string) (provider.Provider, error) {
	var p provider.Provider
	var pos []string
	local := false
	localSet := false
	for i := 0; i < len(args); i++ {
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", args[i])
			}
			i++
			return args[i], nil
		}
		switch args[i] {
		case "--local", "--no-key":
			local, localSet = true, true
		case "--key":
			local, localSet = false, true
		case "--dialect":
			v, err := next()
			if err != nil {
				return p, err
			}
			p.Dialect = provider.Dialect(v)
		case "--auth-header":
			v, err := next()
			if err != nil {
				return p, err
			}
			p.AuthHeader = v
		case "--strategy":
			v, err := next()
			if err != nil {
				return p, err
			}
			p.Strategy = v
		case "--header":
			v, err := next()
			if err != nil {
				return p, err
			}
			k, val, ok := strings.Cut(v, ":")
			if !ok || strings.TrimSpace(k) == "" {
				return p, fmt.Errorf(`--header wants "Name: value"`)
			}
			if p.Headers == nil {
				p.Headers = map[string]string{}
			}
			p.Headers[strings.TrimSpace(k)] = strings.TrimSpace(val)
		default:
			if strings.HasPrefix(args[i], "--") {
				return p, fmt.Errorf("unknown option %s", args[i])
			}
			pos = append(pos, args[i])
		}
	}
	if len(pos) != 2 {
		return p, fmt.Errorf("usage: tsunagi add <name> <base-url> [--dialect anthropic|openai-chat|openai-responses] [--local] [--header \"K: V\"]")
	}
	p.Name, p.BaseURL = pos[0], strings.TrimRight(pos[1], "/")
	if !provider.ValidName(p.Name) {
		return p, fmt.Errorf("name must be lowercase letters, digits and dashes")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return p, fmt.Errorf("base URL must be http(s)://host[:port]/path, without user info")
	}
	if p.Dialect == "" {
		p.Dialect = provider.OpenAIChat // what LiteLLM, vLLM, LM Studio, Ollama and most gateways speak
		if strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/anthropic") {
			p.Dialect = provider.Anthropic
		}
	}
	if !p.Ready() {
		return p, fmt.Errorf("dialect must be anthropic, openai-chat or openai-responses")
	}
	// A gateway on this machine usually needs no key.
	if !localSet {
		local = isLoopback(u.Hostname())
	}
	p.Local = local
	if p.Strategy != "" && p.Strategy != "sequential" && p.Strategy != "round_robin" && p.Strategy != "usage" {
		return p, fmt.Errorf("strategy must be sequential, round_robin or usage")
	}
	return p, nil
}

func isLoopback(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// cmdAdd saves a personal gateway, or any provider tsunagi does not list, in
// the config file, then checks it answers.
func cmdAdd(t *ui.Term, args []string) int {
	p, err := parseAdd(args)
	if err != nil {
		return fail(t, "%v", err)
	}
	def, user, err := config.User(config.Path())
	if err != nil {
		return fail(t, "%v", err)
	}
	replaced := false
	for i := range user {
		if user[i].Name == p.Name {
			user[i], replaced = p, true
		}
	}
	if !replaced {
		user = append(user, p)
	}
	if err := config.Save(config.Path(), def, user); err != nil {
		return fail(t, "%v", err)
	}
	t.Printf("  %s %s %s\n", t.Paint(ui.Mint, "[ok]"), p.Name, t.Dim(string(p.Dialect)+" "+p.BaseURL))

	if !p.Local {
		t.Printf("  %s\n", t.Dim("needs a key: tsunagi login "+p.Name))
		return 0
	}
	sp := t.Spin("probe")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	models, err := liveModels(ctx, p, "")
	if err != nil {
		sp.Stop(false, err.Error()+" (saved anyway)")
		return 0
	}
	sp.Stop(true, fmt.Sprintf("%d models. Try: /model %s/%s", len(models), p.Name, firstOr(models, "<model>")))
	return 0
}

func firstOr(s []string, def string) string {
	if len(s) > 0 {
		return s[0]
	}
	return def
}

func cmdRemove(t *ui.Term, args []string) int {
	if len(args) != 1 {
		return fail(t, "usage: tsunagi remove <name>")
	}
	def, user, err := config.User(config.Path())
	if err != nil {
		return fail(t, "%v", err)
	}
	var keep []provider.Provider
	for _, p := range user {
		if p.Name != args[0] {
			keep = append(keep, p)
		}
	}
	if len(keep) == len(user) {
		return fail(t, "%q is not in your config (built-in and catalogue providers cannot be removed)", args[0])
	}
	if err := config.Save(config.Path(), def, keep); err != nil {
		return fail(t, "%v", err)
	}
	t.Printf("  %s %s removed. Keys stay in the keychain: tsunagi logout %s\n", t.Paint(ui.Mint, "[ok]"), args[0], args[0])
	return 0
}

// liveModels asks a provider which models it serves: GET <base>/models for the
// OpenAI dialects, <base>/v1/models for Anthropic. Local gateways answer with
// whatever they have loaded.
func liveModels(ctx context.Context, p provider.Provider, key string) ([]string, error) {
	u := strings.TrimRight(p.BaseURL, "/") + "/models"
	if p.Dialect == provider.Anthropic {
		u = strings.TrimRight(p.BaseURL, "/") + "/v1/models"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	if key != "" {
		if p.AuthHeader != "" {
			req.Header.Set(p.AuthHeader, key)
		} else {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	// A key goes with this request, so never follow a redirect with it, and keep
	// catalogue providers away from internal addresses, as the router does.
	client := &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if p.Untrusted {
		client.Transport = router.GuardedTransport()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s", req.URL.Host)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s answered HTTP %d", req.URL.Host, resp.StatusCode)
	}
	var d struct {
		Data   []struct{ ID string } `json:"data"`
		Models []struct{ ID, Name string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&d); err != nil {
		return nil, fmt.Errorf("unexpected model list")
	}
	var out []string
	for _, m := range d.Data {
		out = append(out, m.ID)
	}
	for _, m := range d.Models { // Ollama's native shape also fits
		if m.ID != "" {
			out = append(out, m.ID)
		} else if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out, nil
}
