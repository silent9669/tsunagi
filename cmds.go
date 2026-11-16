package main

import (
	"fmt"
	"strings"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/ui"
)

func fail(t *ui.Term, format string, a ...any) int {
	t.Printf("  %s %s\n", t.Paint(ui.Rose, "[!!]"), fmt.Sprintf(format, a...))
	return 1
}

func findProvider(t *ui.Term, args []string) (provider.Provider, int) {
	cfg, err := config.Load()
	if err != nil {
		return provider.Provider{}, fail(t, "%v", err)
	}
	if len(args) != 1 {
		return provider.Provider{}, fail(t, "name one provider: %s", providerNames(cfg))
	}
	for _, p := range cfg.Providers {
		if p.Name == args[0] {
			return p, 0
		}
	}
	return provider.Provider{}, fail(t, "unknown provider %q. Known: %s", args[0], providerNames(cfg))
}

func providerNames(cfg config.Config) string {
	var n []string
	for _, p := range cfg.Providers {
		n = append(n, p.Name)
	}
	return strings.Join(n, ", ")
}

func cmdLogin(t *ui.Term, args []string) int {
	p, code := findProvider(t, args)
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
	pause(t)
	sp.Stop(true, p.Name+" saved")
	return 0
}

func cmdLogout(t *ui.Term, args []string) int {
	p, code := findProvider(t, args)
	if code != 0 {
		return code
	}
	if err := keychain.Delete(p.Name); err != nil {
		return fail(t, "no key stored for %s", p.Name)
	}
	t.Printf("  %s %s\n", t.Paint(ui.Mint, "[ok]"), p.Name+" key removed")
	return 0
}

func cmdProviders(t *ui.Term) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	rows := make([][]string, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		name := p.Name
		if name == cfg.Default {
			name += " *"
		}
		rows = append(rows, []string{name, string(p.Dialect), status(t, p), p.BaseURL})
	}
	t.Table([]string{"provider", "dialect", "status", "endpoint"}, rows)
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
