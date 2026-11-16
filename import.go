package main

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/opencode"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/ui"
)

var validName = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

type importRow struct{ ID, Config, Key string }

// planImport decides what to take from opencode. It touches nothing: known
// names are providers tsunagi already has (built in, configured or from the
// catalogue), and are not overridden.
func planImport(known map[string]bool, ocs []opencode.Provider, auth map[string]opencode.Auth) (adds []provider.Provider, keys map[string]string, rows []importRow) {
	state := map[string]*importRow{}
	row := func(id string) *importRow {
		if state[id] == nil {
			state[id] = &importRow{ID: id, Config: "-", Key: "-"}
		}
		return state[id]
	}
	keys = map[string]string{}
	for _, oc := range ocs {
		r := row(oc.ID)
		switch {
		case known[oc.ID]:
			r.Config = "known"
		default:
			d, ok := provider.DialectForPackage(oc.Package)
			switch {
			case oc.Package == "":
				r.Config = "skip: no package"
			case !ok:
				r.Config = "skip: package " + oc.Package
			case oc.BaseURL == "":
				r.Config = "skip: no baseURL"
			case !validName.MatchString(oc.ID):
				r.Config = "skip: bad name"
			default:
				adds = append(adds, provider.Provider{Name: oc.ID, BaseURL: oc.BaseURL, Dialect: d})
				known[oc.ID] = true
				r.Config = "added"
			}
		}
		if oc.Key != "" {
			keys[oc.ID] = oc.Key
		}
	}
	for id, a := range auth {
		r := row(id)
		switch {
		case a.Type != "api" || a.Key == "":
			r.Key = "skip: " + a.Type + " login"
		case keys[id] == "":
			keys[id] = a.Key
		}
	}
	for id := range keys {
		if !known[id] {
			row(id).Key = "skip: unknown provider"
			delete(keys, id)
		}
	}
	for _, r := range state {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return adds, keys, rows
}

func cmdImport(t *ui.Term, args []string) int {
	if len(args) != 1 || args[0] != "opencode" {
		return fail(t, "usage: tsunagi import opencode")
	}
	if len(catalog.Load()) == 0 {
		if code := cmdSync(t); code != 0 {
			return code
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	ocs, found, err := opencode.ReadConfig(opencode.ConfigPaths())
	if err != nil {
		return fail(t, "%v", err)
	}
	auth, err := opencode.ReadAuth(opencode.AuthPath())
	if err != nil {
		return fail(t, "%v", err)
	}
	if !found && len(auth) == 0 {
		return fail(t, "no opencode config or logins found")
	}
	known := map[string]bool{}
	for _, p := range cfg.All() {
		known[p.Name] = true
	}
	adds, keys, rows := planImport(known, ocs, auth)

	if len(adds) > 0 {
		def, user, err := config.User(config.Path())
		if err != nil {
			return fail(t, "%v", err)
		}
		if err := config.Save(config.Path(), def, append(user, adds...)); err != nil {
			return fail(t, "%v", err)
		}
	}
	result := map[string]string{}
	for name, key := range keys {
		switch {
		case keychain.Has(name):
			result[name] = "kept (already stored)"
		default:
			if err := keychain.Set(name, key); err != nil {
				result[name] = "failed: " + err.Error()
				continue
			}
			config.SetKeyed(name, true, pinHostByName(cfg, name))
			result[name] = "imported"
		}
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		if s, ok := result[r.ID]; ok {
			r.Key = s
		}
		table = append(table, []string{r.ID, r.Config, r.Key})
	}
	t.Table([]string{"provider", "config", "key"}, table)
	return 0
}

func cmdSync(t *ui.Term) int {
	sp := t.Spin("catalogue")
	n, err := catalog.Sync(&http.Client{Timeout: 60 * time.Second}, catalog.URL)
	if err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	sp.Stop(true, strconv.Itoa(n)+" providers from models.dev")
	return 0
}

func cmdModels(t *ui.Term, args []string) int {
	if len(args) != 1 {
		return fail(t, "name one provider: tsunagi models <provider>")
	}
	if args[0] == "chatgpt" { // the list depends on the account, so ask live
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		models, err := newChatGPT().ListModels(ctx)
		if err != nil {
			return fail(t, "%v", err)
		}
		for _, m := range models {
			t.Printf("chatgpt/%s\n", m.Slug)
		}
		return 0
	}
	for _, e := range catalog.Load() {
		if e.ID == args[0] {
			for _, m := range e.Models {
				t.Printf("%s/%s\n", e.ID, m)
			}
			return 0
		}
	}
	return fail(t, "no model list for %q. Run: tsunagi sync", args[0])
}

func pinHostByName(cfg config.Config, name string) string {
	for _, p := range cfg.Catalog {
		if p.Name == name {
			return pinHost(cfg, p)
		}
	}
	return ""
}
