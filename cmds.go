package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
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

// keyFor returns the key lookup the router uses. It is given the provider the
// request is really going to and the account chosen for it, and it checks that
// they belong together before handing anything out: an account name must be the
// provider's own name or "<provider>.<label>", sign-in tokens only go to
// providers that are configured for sign-in, and a catalogue provider's key only
// goes to the endpoint host it was saved for.
func keyFor(cfg config.Config) func(provider.Provider, string) (string, error) {
	return func(p provider.Provider, account string) (string, error) {
		if account != p.Name && !strings.HasPrefix(account, p.Name+".") {
			return "", router.KeyError{Msg: "tsunagi: account " + account + " does not belong to " + p.Name}
		}
		if p.Auth == "chatgpt" && !p.Untrusted {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			tok, err := chatgptFor(strings.TrimPrefix(strings.TrimPrefix(account, p.Name), ".")).AccessToken(ctx)
			if err != nil {
				return "", router.KeyError{Msg: "tsunagi: " + err.Error()}
			}
			return tok, nil
		}
		if p.Untrusted {
			host := config.Host(p.BaseURL)
			// Deny by default: a missing or unreadable pin is not trusted.
			switch pin := config.Pin(account); {
			case pin == "":
				return "", router.KeyError{Msg: fmt.Sprintf(
					"tsunagi: no endpoint is recorded for the %s key. Run: tsunagi login %s", account, p.Name)}
			case pin != host:
				return "", router.KeyError{Msg: fmt.Sprintf(
					"tsunagi: the catalogue now sends %s to %s, but its key was saved for %s. Run: tsunagi sync, then tsunagi login %s if that is expected",
					p.Name, host, pin, p.Name)}
			}
		}
		return keychain.Get(account)
	}
}

func providerNames(cfg config.Config) string {
	var n []string
	for _, p := range cfg.Providers {
		n = append(n, p.Name)
	}
	return strings.Join(n, ", ") + " (see also: tsunagi providers --all)"
}

func cmdLogin(t *ui.Term, args []string) int {
	fresh := false
	var rest []string
	for _, a := range args {
		if a == "--new" {
			fresh = true
		} else {
			rest = append(rest, a)
		}
	}
	rest, label, err := parseAs(rest)
	if err != nil {
		return fail(t, "%v", err)
	}
	p, cfg, code := findProvider(t, rest)
	if code != 0 {
		return code
	}
	account := router.Account(p.Name, label)
	if p.Auth == "chatgpt" {
		return loginChatGPT(t, account, fresh)
	}
	// The key is read from the terminal, never from a flag, so it stays out of
	// shell history and `ps`.
	key, err := t.ReadSecret(fmt.Sprintf("  %s key: ", account))
	if err != nil || key == "" {
		return fail(t, "no key entered")
	}
	sp := t.Spin("keychain")
	if err := keychain.Set(account, key); err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	config.SetKeyed(account, true, pinHost(cfg, p))
	pause(t)
	sp.Stop(true, account+" saved")
	return 0
}

func loginChatGPT(t *ui.Term, account string, fresh bool) int {
	g := chatgptFor(labelOf(account))
	if fresh {
		g.NewRegistration()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var sp *ui.Spinner
	email, err := g.SignIn(ctx, func(u string) {
		t.Printf("  %s\n  %s\n", t.Dim("opening your browser. If it does not open, visit:"), u)
		sp = t.Spin(account) // after the URL, so the spinner owns its line alone
	})
	if sp == nil {
		return fail(t, "%v", err)
	}
	if err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	config.SetKeyed(account, true, "")
	sp.Stop(true, "signed in as "+email)
	return 0
}

func cmdLogout(t *ui.Term, args []string) int {
	rest, label, err := parseAs(args)
	if err != nil {
		return fail(t, "%v", err)
	}
	p, _, code := findProvider(t, rest)
	if code != 0 {
		return code
	}
	account := router.Account(p.Name, label)
	if p.Auth == "chatgpt" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		revoked, err := chatgptFor(label).SignOut(ctx)
		if err != nil {
			return fail(t, "%v", err)
		}
		config.SetKeyed(account, false, "")
		note := account + " signed out"
		if !revoked {
			note += " locally; OpenAI did not confirm revocation, disconnect tsunagi in ChatGPT settings"
		}
		t.Printf("  %s %s\n", t.Paint(ui.Mint, "[ok]"), note)
		return 0
	}
	if err := keychain.Delete(account); err != nil {
		return fail(t, "no key stored for %s", account)
	}
	config.SetKeyed(account, false, "")
	t.Printf("  %s %s\n", t.Paint(ui.Mint, "[ok]"), account+" key removed")
	return 0
}

// cmdAccounts lists every stored key and sign-in.
func cmdAccounts(t *ui.Term, args []string) int {
	args, asJSON := popFlag(args, "--json")
	if len(args) > 0 {
		return fail(t, "usage: tsunagi accounts [--json]")
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	byName := map[string]provider.Provider{}
	for _, p := range cfg.All() {
		byName[p.Name] = p
	}
	var rows [][]string
	var data []map[string]any
	for _, acct := range config.Keyed() {
		p, ok := byName[baseOf(acct)]
		if !ok {
			continue
		}
		label := labelOf(acct)
		if label == "" {
			label = "default"
		}
		kind, detail := "key", config.Pin(acct)
		if p.Auth == "chatgpt" {
			kind, detail = "sign-in", chatgptFor(labelOf(acct)).Email()
		}
		rows = append(rows, []string{p.Name, label, kind, accountState(t, p, acct), detail})
		data = append(data, map[string]any{"provider": p.Name, "account": acct, "label": label, "kind": kind,
			"state": accountStateText(p, acct), "detail": detail})
	}
	if asJSON {
		if data == nil {
			data = []map[string]any{}
		}
		return printJSON(t, data)
	}
	if len(rows) == 0 {
		t.Printf("  %s\n", t.Dim("no accounts yet. Run: tsunagi login <provider> [--as label]"))
		return 0
	}
	t.Table([]string{"provider", "account", "kind", "state", "detail"}, rows)
	return 0
}

func accountStateText(p provider.Provider, account string) string {
	switch {
	case p.Untrusted && config.Pin(account) == "" && keychain.Has(account):
		return "re-login" // key saved before endpoints were pinned
	case hasAccount(p, account):
		return "ready"
	}
	return "missing"
}

func accountState(t *ui.Term, p provider.Provider, account string) string {
	st := accountStateText(p, account)
	switch st {
	case "ready":
		return t.Paint(ui.Mint, st)
	case "re-login":
		return t.Paint(ui.Amber, st)
	}
	return t.Paint(ui.Rose, st)
}

// cmdProviders lists the built-in and configured providers plus any catalogue
// provider that has a key. --all adds the rest of the catalogue.
func cmdProviders(t *ui.Term, args []string) int {
	args, asJSON := popFlag(args, "--json")
	if len(args) == 1 && args[0] == "--names" { // for shell completion
		return printProviderNames(t)
	}
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
	if asJSON {
		data := make([]map[string]any, 0, len(list))
		for _, p := range list {
			data = append(data, map[string]any{"name": p.Name, "dialect": string(p.Dialect), "endpoint": p.BaseURL,
				"status": statusText(p), "default": p.Name == cfg.Default, "accounts": ready(p), "local": p.Local})
		}
		return printJSON(t, data)
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

// statusText is the provider's state without color.
func statusText(p provider.Provider) string {
	switch {
	case !p.Ready():
		return "soon"
	case p.Local:
		return "local"
	case ready(p) > 1:
		return fmt.Sprintf("ready x%d", ready(p))
	case ready(p) == 1:
		return "ready"
	case p.Untrusted && config.Pin(p.Name) == "" && keychain.Has(p.Name):
		return "re-login"
	case p.Auth != "":
		return "no login"
	}
	return "no key"
}

func status(t *ui.Term, p provider.Provider) string {
	st := statusText(p)
	switch {
	case st == "soon":
		return t.Dim(st)
	case strings.HasPrefix(st, "ready"), st == "local":
		return t.Paint(ui.Mint, st)
	}
	return t.Paint(ui.Amber, st)
}
