package main

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/opencode"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/ui"
)

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
			case !provider.ValidName(oc.ID):
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
	_, _, rows, result, err := runImport(cfg)
	if err != nil {
		return fail(t, "%v", err)
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

// cmdModels lists the names to type after /model: from the catalogue, or live
// from the provider (always for gateways, or with --live).
func cmdModels(t *ui.Term, args []string) int {
	args, asJSON := popFlag(args, "--json")
	if len(args) == 1 && args[0] == "--summary" {
		return modelsSummary(t, asJSON)
	}
	live := false
	var rest []string
	for _, a := range args {
		if a == "--live" {
			live = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return fail(t, "usage: tsunagi models [--live] [--json] <provider>")
	}
	name := rest[0]
	emit := func(list []string) int {
		if asJSON {
			if list == nil {
				list = []string{}
			}
			return printJSON(t, list)
		}
		for _, m := range list {
			t.Printf("%s\n", m)
		}
		return 0
	}
	if name == "chatgpt" { // the list depends on the account, so ask live
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		models, err := chatgptFor("").ListModels(ctx)
		if err != nil {
			return fail(t, "%v", err)
		}
		var names []string
		for _, m := range models {
			names = append(names, "chatgpt/"+m.Slug)
		}
		return emit(names)
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	if !live {
		for _, e := range catalog.Load() {
			if e.ID == name && len(e.Models) > 0 {
				var names []string
				for _, m := range e.Models {
					names = append(names, e.ID+"/"+m)
				}
				return emit(names)
			}
		}
	}
	for _, p := range cfg.All() {
		if p.Name != name {
			continue
		}
		key := ""
		if !p.Local {
			k, err := keyFor(cfg)(p, router.Account(p.Name, ""))
			if err != nil {
				return fail(t, "no key for %s to ask it. Run: tsunagi login %s", name, name)
			}
			key = k
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		models, err := liveModels(ctx, p, key)
		if err != nil {
			return fail(t, "%v", err)
		}
		var names []string
		for _, m := range models {
			names = append(names, name+"/"+m)
		}
		return emit(names)
	}
	return fail(t, "unknown provider %q. Run: tsunagi providers --all", name)
}

func pinHostByName(cfg config.Config, name string) string {
	for _, p := range cfg.Catalog {
		if p.Name == baseOf(name) {
			return pinHost(cfg, p)
		}
	}
	return ""
}

// modelsSummary shows a few catalogue model names for each ready provider, so
// a slash command can list them without taking an argument.
func modelsSummary(t *ui.Term, asJSON bool) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	byID := map[string][]string{}
	for _, e := range catalog.Load() {
		byID[e.ID] = e.Models
	}
	data := map[string][]string{}
	var names []string
	for _, p := range cfg.All() {
		if p.Local || ready(p) == 0 {
			continue
		}
		models := byID[p.Name]
		if len(models) > 6 {
			models = models[:6]
		}
		data[p.Name] = models
		names = append(names, p.Name)
	}
	if asJSON {
		return printJSON(t, data)
	}
	if len(names) == 0 {
		t.Printf("  %s\n", t.Dim("no ready providers. Run: tsunagi login <provider>"))
		return 0
	}
	for _, n := range names {
		list := "(run: tsunagi models " + n + ")"
		if len(data[n]) > 0 {
			list = n + "/" + strings.Join(data[n], ", "+n+"/")
		}
		t.Printf("  %-14s %s\n", n, list)
	}
	t.Printf("  %s\n", t.Dim("all names: tsunagi models <provider>. Switch with /model <provider>/<model>"))
	return 0
}
