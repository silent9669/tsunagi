package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/signin"
	"github.com/silent9669/tsunagi/internal/ui"
)

func fail(t *ui.Term, format string, a ...any) int {
	t.Printf("  %s %s\n", t.Paint(ui.Rose, "[!!]"), fmt.Sprintf(format, a...))
	return 1
}

func findProvider(t *ui.Term, args []string) (provider.Provider, config.Config, int) {
	cfg, err := config.Load()
	if err != nil {
		return provider.Provider{}, cfg, fail(t, "%v", err)
	}
	if len(args) != 1 {
		return provider.Provider{}, cfg, fail(t, "name one provider: %s", providerNames(cfg))
	}
	for _, p := range cfg.All() {
		if p.Name == args[0] {
			return p, cfg, 0
		}
	}
	return provider.Provider{}, cfg, fail(t, "unknown provider %q. Known: %s", args[0], providerNames(cfg))
}

// pinHost is the endpoint host a key is pinned to. Only catalogue providers
// are pinned: their endpoint comes from a downloaded list, the others from the
// user.
func pinHost(cfg config.Config, p provider.Provider) string {
	for _, c := range cfg.Catalog {
		if c.Name == p.Name {
			return config.Host(c.BaseURL)
		}
	}
	return ""
}

// keyFor returns the key lookup the router uses. It refuses to hand a
// catalogue provider's key to an endpoint other than the one it was saved for.
func keyFor(cfg config.Config, gpt *signin.ChatGPT) func(string) (string, error) {
	catalog := map[string]provider.Provider{}
	for _, p := range cfg.Catalog {
		catalog[p.Name] = p
	}
	return func(name string) (string, error) {
		if name == "chatgpt" && gpt != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			tok, err := gpt.AccessToken(ctx)
			if err != nil {
				return "", router.KeyError{Msg: "tsunagi: " + err.Error()}
			}
			return tok, nil
		}
		if p, ok := catalog[name]; ok {
			host := config.Host(p.BaseURL)
			// Deny by default: a missing or unreadable pin is not trusted.
			switch pin := config.Pin(name); {
			case pin == "":
				return "", router.KeyError{Msg: fmt.Sprintf(
					"tsunagi: no endpoint is recorded for the %s key. Run: tsunagi login %s", name, name)}
			case pin != host:
				return "", router.KeyError{Msg: fmt.Sprintf(
					"tsunagi: the catalogue now sends %s to %s, but its key was saved for %s. Run: tsunagi sync, then tsunagi login %s if that is expected",
					name, host, pin, name)}
			}
		}
		return keychain.Get(name)
	}
}

func providerNames(cfg config.Config) string {
	var n []string
	for _, p := range cfg.Providers {
		n = append(n, p.Name)
	}
	return strings.Join(n, ", ") + " (see also: tsunagi providers --all)"
}

func newChatGPT() *signin.ChatGPT {
	g := signin.New(filepath.Dir(config.Path()))
	g.Open = func(u string) error { return exec.Command("open", u).Start() }
	return g
}

// hasLogin reports whether a provider has a key or sign-in stored.
func hasLogin(gpt *signin.ChatGPT, p provider.Provider) bool {
	if p.Auth == "chatgpt" {
		return gpt.Signed()
	}
	return keychain.Has(p.Name)
}

func cmdLogin(t *ui.Term, args []string) int {
	fresh := len(args) == 2 && args[1] == "--new"
	if fresh {
		args = args[:1]
	}
	p, cfg, code := findProvider(t, args)
	if code != 0 {
		return code
	}
	if p.Auth == "chatgpt" {
		return loginChatGPT(t, fresh)
	}
	// The key is read from the terminal, never from a flag, so it stays out of
	// shell history and `ps`.
	key, err := t.ReadSecret(fmt.Sprintf("  %s key: ", p.Name))
	if err != nil || key == "" {
		return fail(t, "no key entered")
	}
	sp := t.Spin("keychain")
	if err := keychain.Set(p.Name, key); err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	config.SetKeyed(p.Name, true, pinHost(cfg, p))
	pause(t)
	sp.Stop(true, p.Name+" saved")
	return 0
}

func loginChatGPT(t *ui.Term, fresh bool) int {
	g := newChatGPT()
	if fresh {
		g.NewRegistration()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var sp *ui.Spinner
	email, err := g.SignIn(ctx, func(u string) {
		t.Printf("  %s\n  %s\n", t.Dim("opening your browser. If it does not open, visit:"), u)
		sp = t.Spin("chatgpt") // after the URL, so the spinner owns its line alone
	})
	if sp == nil {
		return fail(t, "%v", err)
	}
	if err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	config.SetKeyed("chatgpt", true, "")
	sp.Stop(true, "signed in as "+email)
	return 0
}

func cmdLogout(t *ui.Term, args []string) int {
	p, _, code := findProvider(t, args)
	if code != 0 {
		return code
	}
	if p.Auth == "chatgpt" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		revoked, err := newChatGPT().SignOut(ctx)
		if err != nil {
			return fail(t, "%v", err)
		}
		config.SetKeyed("chatgpt", false, "")
		note := "signed out"
		if !revoked {
			note += " locally; OpenAI did not confirm revocation, disconnect tsunagi in ChatGPT settings"
		}
		t.Printf("  %s %s\n", t.Paint(ui.Mint, "[ok]"), note)
		return 0
	}
	if err := keychain.Delete(p.Name); err != nil {
		return fail(t, "no key stored for %s", p.Name)
	}
	config.SetKeyed(p.Name, false, "")
	t.Printf("  %s %s\n", t.Paint(ui.Mint, "[ok]"), p.Name+" key removed")
	return 0
}

// cmdProviders lists the built-in and configured providers plus any catalogue
// provider that has a key. --all adds the rest of the catalogue.
func cmdProviders(t *ui.Term, args []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	list := cfg.Providers
	if len(args) == 1 && args[0] == "--all" {
		list = cfg.All()
	} else {
		keyed := map[string]bool{}
		for _, n := range config.Keyed() {
			keyed[n] = true
		}
		for _, p := range cfg.Catalog {
			if keyed[p.Name] {
				list = append(list, p)
			}
		}
	}
	rows := make([][]string, 0, len(list))
	for _, p := range list {
		name := p.Name
		if name == cfg.Default {
			name += " *"
		}
		rows = append(rows, []string{name, string(p.Dialect), status(t, p), p.BaseURL})
	}
	t.Table([]string{"provider", "dialect", "status", "endpoint"}, rows)
	if len(args) == 0 && len(cfg.Catalog) > 0 {
		t.Printf("  %s\n", t.Dim(strconv.Itoa(len(cfg.Catalog))+" more from the catalogue: tsunagi providers --all"))
	}
	return 0
}

func status(t *ui.Term, p provider.Provider) string {
	switch {
	case !p.Ready():
		return t.Dim("soon")
	case p.Local:
		return t.Paint(ui.Mint, "local")
	case p.Untrusted && config.Pin(p.Name) == "" && keychain.Has(p.Name):
		return t.Paint(ui.Amber, "re-login") // key saved before endpoints were pinned
	case hasLogin(newChatGPT(), p):
		return t.Paint(ui.Mint, "ready")
	case p.Auth != "":
		return t.Paint(ui.Amber, "no login")
	}
	return t.Paint(ui.Amber, "no key")
}
