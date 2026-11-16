package main

import (
	"fmt"
	"strconv"
	"strings"

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

// keyFor returns the key lookup the router uses. It refuses to hand a
// catalogue provider's key to an endpoint other than the one it was saved for.
func keyFor(cfg config.Config) func(string) (string, error) {
	catalog := map[string]provider.Provider{}
	for _, p := range cfg.Catalog {
		catalog[p.Name] = p
	}
	return func(name string) (string, error) {
		if p, ok := catalog[name]; ok {
			host := config.Host(p.BaseURL)
			switch pin := config.Pin(name); {
			case pin == "":
				config.SetKeyed(name, true, host) // key saved before pinning existed
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

func cmdLogin(t *ui.Term, args []string) int {
	p, cfg, code := findProvider(t, args)
	if code != 0 {
		return code
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

func cmdLogout(t *ui.Term, args []string) int {
	p, _, code := findProvider(t, args)
	if code != 0 {
		return code
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
	case keychain.Has(p.Name):
		return t.Paint(ui.Mint, "ready")
	}
	return t.Paint(ui.Amber, "no key")
}
