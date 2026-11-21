package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/free"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/ui"
)

// hasFreeKey reports whether a registry provider has a usable key or is
// local. It is a var so tests can stand in for the keychain.
var hasFreeKey = func(cfg config.Config, name string) bool {
	for _, p := range cfg.All() {
		if p.Name == name {
			return p.Local || ready(p) > 0
		}
	}
	return false
}

// liveFreeModels asks a tier provider which models it serves. It is a var so
// tests can fake the network and the keychain.
var liveFreeModels = func(ctx context.Context, cfg config.Config, p provider.Provider) ([]string, error) {
	key := ""
	if !p.Local {
		k, err := keyFor(cfg)(p, router.Account(p.Name, ""))
		if err != nil {
			return nil, err
		}
		key = k
	}
	return liveModels(ctx, p, key)
}

func registryEntry(id string) (free.Entry, bool) {
	for _, e := range free.Registry() {
		if e.ID == id {
			return e, true
		}
	}
	return free.Entry{}, false
}

func findFreeProvider(cfg config.Config, name string) (provider.Provider, bool) {
	for _, p := range cfg.All() {
		if p.Name == name {
			return p, true
		}
	}
	return provider.Provider{}, false
}

// keyStatus is the key column of the free table.
func keyStatus(cfg config.Config, e free.Entry) string {
	if e.Setup != "" {
		return "setup"
	}
	return keyStatusName(cfg, e.ID)
}

// dataColor paints the data-policy cell.
func dataColor(t *ui.Term, data string) string {
	switch data {
	case free.Private:
		return t.Paint(ui.Mint, data)
	case free.Trains:
		return t.Paint(ui.Amber, data)
	}
	return t.Dim(data)
}

func ageStr(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes())+1)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

// freeRow is one line of the `tsunagi free` table, and its JSON form.
type freeRow struct {
	Provider string `json:"provider"`
	Name     string `json:"name,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Free     any    `json:"free"` // model count, "all models", or null
	Limits   string `json:"limits,omitempty"`
	Data     string `json:"data"`
	Key      string `json:"key"`
	Note     string `json:"note,omitempty"`
}

// freeRows assembles the table rows: registry providers in order, then
// catalogue providers with unverified zero-cost models when showAll.
func freeRows(cfg config.Config, snap free.Snapshot, haveSnap bool, showAll bool) []freeRow {
	counts := map[string]int{}
	for _, m := range snap.Models {
		counts[m.Provider]++
	}
	var rows []freeRow
	for _, e := range free.Registry() {
		r := freeRow{Provider: e.ID, Name: e.Name, Mode: e.Mode, Limits: e.Limits, Data: e.Data, Key: keyStatus(cfg, e)}
		switch {
		case e.Mode == free.TierMode:
			r.Free = "all models"
		case e.FromCatalog:
			r.Free = len(catalogFreeCands(catalog.Load(), e.ID))
		case haveSnap:
			r.Free = counts[e.ID]
		}
		rows = append(rows, r)
	}
	if showAll {
		for _, u := range free.Unverified() {
			rows = append(rows, freeRow{
				Provider: u.Provider, Free: u.Models, Limits: "unverified",
				Data: free.Unknown, Key: keyStatusName(cfg, u.Provider), Note: "unverified",
			})
		}
	}
	return rows
}

// keyStatusName is keyStatus for a provider that is not in the registry.
func keyStatusName(cfg config.Config, name string) string {
	p, ok := findFreeProvider(cfg, name)
	if !ok {
		return "no"
	}
	if p.Local {
		return "local"
	}
	if ready(p) > 0 {
		return "yes"
	}
	return "no"
}

func cmdFree(t *ui.Term, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "refresh":
			return cmdFreeRefresh(t, args[1:])
		case "probe":
			return cmdFreeProbe(t, args[1:])
		case "combo":
			return cmdFreeCombo(t, args[1:])
		case "allow", "deny":
			return cmdFreeAllow(t, args[1:], args[0] == "allow")
		}
	}
	args, asJSON := popFlag(args, "--json")
	showAll := false
	var rest []string
	for _, a := range args {
		if a == "--all" {
			showAll = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) > 1 {
		return fail(t, "usage: tsunagi free [--all] [--json] [provider]")
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snap, offline := free.Ensure(ctx, &http.Client{Timeout: 20 * time.Second})
	haveSnap := len(snap.Models) > 0
	if len(rest) == 1 {
		return freeDetail(t, cfg, snap, rest[0], asJSON)
	}
	rows := freeRows(cfg, snap, haveSnap, showAll)
	if asJSON {
		out := map[string]any{"providers": rows}
		if offline && snap.FetchedAt.After(time.Time{}) {
			out["offline"] = true
			out["cached"] = ageStr(time.Since(snap.FetchedAt))
		}
		return printJSON(t, out)
	}
	var table [][]string
	for _, r := range rows {
		freeCell := "-"
		if r.Free != nil {
			freeCell = fmt.Sprint(r.Free)
		}
		table = append(table, []string{r.Provider, freeCell, r.Limits, dataColor(t, r.Data), r.Key})
	}
	t.Table([]string{"provider", "free", "limits", "data", "key"}, table)
	if offline && snap.FetchedAt.After(time.Time{}) {
		t.Printf("  %s offline, cached %s\n", t.Dim("[--]"), ageStr(time.Since(snap.FetchedAt)))
	}
	return 0
}

// toolsMark renders a *bool tools flag as yes/no/? for display.
func toolsMark(b *bool) string {
	if b == nil {
		return "?"
	}
	if *b {
		return "yes"
	}
	return "no"
}

// freeDetail shows one registry provider: its free models, then the registry
// facts (the only place the URLs are printed).
func freeDetail(t *ui.Term, cfg config.Config, snap free.Snapshot, name string, asJSON bool) int {
	e, ok := registryEntry(name)
	if !ok {
		return fail(t, "%q is not a free-offer provider. Run: tsunagi free", name)
	}
	type modelRow struct {
		ID      string `json:"id"`
		Context int    `json:"context"`
		Tools   string `json:"tools"`
		Note    string `json:"note,omitempty"`
	}
	var models []modelRow
	var listErr string
	switch e.Mode {
	case free.ModelsMode:
		if e.FromCatalog {
			for _, c := range catalogFreeCands(catalog.Load(), e.ID) {
				models = append(models, modelRow{ID: c.ID, Context: c.Context, Tools: toolsMark(c.Tools)})
			}
		} else {
			for _, m := range snap.Models {
				if m.Provider == e.ID {
					models = append(models, modelRow{ID: m.ID, Context: m.Context, Tools: toolsMark(m.Tools)})
				}
			}
		}
	case free.TierMode:
		p, ok := findFreeProvider(cfg, e.ID)
		switch {
		case !ok:
			listErr = "no provider named " + e.ID + " is configured"
		case !hasFreeKey(cfg, e.ID):
			listErr = "needs a key: tsunagi login " + e.ID
		default:
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			ids, err := liveFreeModels(ctx, cfg, p)
			cancel()
			if err != nil {
				listErr = err.Error()
			} else {
				sort.Strings(ids)
				for _, id := range ids {
					mr := modelRow{ID: id}
					if mi, ok := catalog.Caps(e.ID, id); ok {
						mr.Context = mi.Context
						if mi.Tools {
							mr.Tools = "yes"
						} else {
							mr.Tools = "no"
						}
					} else {
						mr.Tools = "?"
						mr.Note = "untested"
					}
					models = append(models, mr)
				}
			}
		}
	}
	if asJSON {
		out := map[string]any{
			"provider": e.ID, "name": e.Name, "mode": e.Mode,
			"signup": e.Signup, "key_url": e.KeyURL, "needs": e.Needs,
			"limits": e.Limits, "data": e.Data, "data_note": e.DataNote,
			"source": e.Source, "checked": e.Checked, "setup": e.Setup,
			"models": models, "model_error": listErr,
		}
		return printJSON(t, out)
	}
	var rows [][]string
	for _, m := range models {
		ctx := "-"
		if m.Context > 0 {
			ctx = fmt.Sprint(m.Context)
		}
		rows = append(rows, []string{m.ID, ctx, m.Tools, m.Note})
	}
	if len(rows) > 0 {
		t.Table([]string{"model", "context", "tools", "note"}, rows)
	}
	if listErr != "" {
		t.Printf("  %s %s\n", t.Dim("[--]"), listErr)
	}
	t.Printf("  %s %s\n", t.Dim("signup "), e.Signup)
	t.Printf("  %s %s\n", t.Dim("key    "), e.KeyURL)
	if e.Needs != "" {
		t.Printf("  %s %s\n", t.Dim("needs  "), e.Needs)
	}
	if e.Setup != "" {
		t.Printf("  %s %s\n", t.Dim("setup  "), e.Setup)
	}
	t.Printf("  %s %s\n", t.Dim("limits "), e.Limits)
	t.Printf("  %s %s — %s\n", t.Dim("data   "), dataColor(t, e.Data), e.DataNote)
	t.Printf("  %s %s\n", t.Dim("source "), e.Source)
	t.Printf("  %s %s\n", t.Dim("checked"), e.Checked)
	return 0
}

func cmdFreeRefresh(t *ui.Term, args []string) int {
	if len(args) != 0 {
		return fail(t, "usage: tsunagi free refresh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := free.Refresh(ctx, &http.Client{Timeout: 30 * time.Second}); err != nil {
		return fail(t, "%v", err)
	}
	snap, _ := free.Load()
	counts := map[string]int{}
	for _, m := range snap.Models {
		counts[m.Provider]++
	}
	for _, e := range free.Registry() {
		if e.Mode != free.ModelsMode {
			continue
		}
		n := counts[e.ID]
		if e.FromCatalog {
			n = len(catalogFreeCands(catalog.Load(), e.ID))
		}
		t.Printf("  %s %d free models\n", t.Dim(e.ID), n)
	}
	t.Printf("  %s cached %s\n", t.Paint(ui.Mint, "[ok]"), free.Path())
	return 0
}

// freeModels gathers the candidate model lists BuildAuto and probing share:
// discovered free models for ModelsMode providers, and for TierMode
// providers the live /models list intersected with catalog caps (tools
// required, plan ids and catalogue-unknown models dropped).
func freeModels(ctx context.Context, cfg config.Config, snap free.Snapshot) map[string][]free.ModelCand {
	out := map[string][]free.ModelCand{}
	for _, e := range free.Registry() {
		switch e.Mode {
		case free.ModelsMode:
			if e.FromCatalog {
				out[e.ID] = catalogFreeCands(catalog.Load(), e.ID)
				break
			}
			for _, m := range snap.Models {
				if m.Provider == e.ID {
					out[e.ID] = append(out[e.ID], free.ModelCand{
						ID: m.ID, Context: m.Context, Tools: m.Tools, Reasoning: m.Reasoning,
					})
				}
			}
		case free.TierMode:
			p, ok := findFreeProvider(cfg, e.ID)
			if !ok || !hasFreeKey(cfg, e.ID) {
				continue
			}
			lctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			ids, err := liveFreeModels(lctx, cfg, p)
			cancel()
			if err != nil {
				continue
			}
			for _, id := range ids {
				if strings.Contains(id, "token-plan") || strings.Contains(id, "coding-plan") {
					continue
				}
				mi, ok := catalog.Caps(e.ID, id)
				if !ok || !mi.Tools {
					continue
				}
				out[e.ID] = append(out[e.ID], free.ModelCand{ID: id, Context: mi.Context})
			}
		}
	}
	return out
}

// catalogFreeCands lists a provider's catalogue models whose input and
// output prices are both zero, for FromCatalog registry entries.
func catalogFreeCands(entries []catalog.Entry, id string) []free.ModelCand {
	var out []free.ModelCand
	for _, e := range entries {
		if e.ID != id {
			continue
		}
		ids := make([]string, 0, len(e.Caps))
		for m := range e.Caps {
			ids = append(ids, m)
		}
		sort.Strings(ids)
		for _, m := range ids {
			mi := e.Caps[m]
			if mi.In == nil || mi.Out == nil || *mi.In != 0 || *mi.Out != 0 {
				continue
			}
			tools := mi.Tools
			out = append(out, free.ModelCand{ID: m, Context: mi.Context, Tools: &tools})
		}
	}
	return out
}

// autoInput builds the BuildAuto input shared by probe and combo.
func autoInput(cfg config.Config, models map[string][]free.ModelCand, probeOnly bool) free.AutoInput {
	allow := map[string]bool{}
	for _, n := range cfg.Free.AllowTrains {
		allow[n] = true
	}
	return free.AutoInput{
		Models:      models,
		Probes:      free.LoadProbes(),
		AllowTrains: allow,
		HasKey:      func(name string) bool { return hasFreeKey(cfg, name) },
		ProbeOnly:   probeOnly,
	}
}

// probeCandidates returns the gated providers and the top models to probe:
// the same candidate rules as combos, minus probe results.
func probeCandidates(cfg config.Config, models map[string][]free.ModelCand, only string) []free.ProviderCands {
	in := autoInput(cfg, models, false)
	var out []free.ProviderCands
	for _, pc := range in.Candidates() {
		if only != "" && pc.Entry.ID != only {
			continue
		}
		var top []free.ModelCand
		for _, m := range pc.Models {
			if m.Tools != nil && !*m.Tools {
				continue
			}
			top = append(top, m)
			if len(top) == 3 {
				break
			}
		}
		if len(top) > 0 {
			out = append(out, free.ProviderCands{Entry: pc.Entry, Models: top})
		}
	}
	return out
}

// shortErr trims a probe failure reason for storage and display.
func shortErr(s string) string {
	s = strings.Join(strings.Fields(clean(s)), " ")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func cmdFreeProbe(t *ui.Term, args []string) int {
	args, asJSON := popFlag(args, "--json")
	var only string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return fail(t, "usage: tsunagi free probe [--json] [provider]")
		}
		if only != "" {
			return fail(t, "usage: tsunagi free probe [--json] [provider]")
		}
		only = a
	}
	if only != "" {
		if _, ok := registryEntry(only); !ok {
			return fail(t, "%q is not a free-offer provider. Run: tsunagi free", only)
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snap, _ := free.Ensure(ctx, &http.Client{Timeout: 20 * time.Second})
	models := freeModels(ctx, cfg, snap)
	cands := probeCandidates(cfg, models, only)
	if len(cands) == 0 {
		if only != "" {
			return fail(t, "no free candidates for %s", only)
		}
		return fail(t, "no free candidates")
	}
	host, token, stop, err := probeRouter(cfg)
	if err != nil {
		return fail(t, "%v", err)
	}
	defer stop()
	client := &http.Client{Timeout: 2 * time.Minute}
	probes := free.LoadProbes()
	var results []free.Result
	for _, pc := range cands {
		for _, m := range pc.Models {
			target := pc.Entry.ID + "/" + m.ID
			status, res, ms := probeSend(client, host, token, target, true)
			r := free.Result{Target: target, Millis: ms, At: time.Now()}
			if status == 200 && res.err == "" {
				r.OK = true
				r.Tools = len(res.tools) > 0
			} else if res.err != "" {
				r.Err = shortErr(res.err)
			} else {
				r.Err = fmt.Sprintf("HTTP %d", status)
			}
			probes[target] = r
			results = append(results, r)
		}
	}
	if err := free.SaveProbes(probes); err != nil {
		return fail(t, "%v", err)
	}
	if asJSON {
		return printJSON(t, results)
	}
	var rows [][]string
	for _, r := range results {
		ok := "fail"
		if r.OK {
			ok = t.Paint(ui.Mint, "ok")
		} else {
			ok = t.Paint(ui.Rose, "fail")
		}
		tools := "no"
		if r.Tools {
			tools = "yes"
		}
		rows = append(rows, []string{r.Target, ok, tools, fmt.Sprint(r.Millis), r.Err})
	}
	t.Table([]string{"target", "ok", "tools", "ms", "error"}, rows)
	return 0
}

func cmdFreeCombo(t *ui.Term, args []string) int {
	probe := true
	dryRun := false
	for _, a := range args {
		switch a {
		case "--no-probe":
			probe = false
		case "--dry-run":
			dryRun = true
		default:
			return fail(t, "usage: tsunagi free combo [--no-probe] [--dry-run]")
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	if c, ok := cfg.Combos["free/auto"]; ok && !c.Auto {
		return fail(t, "combos.free/auto exists without auto:true; refusing to overwrite a user combo. Rename or remove it first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snap, _ := free.Ensure(ctx, &http.Client{Timeout: 20 * time.Second})
	models := freeModels(ctx, cfg, snap)
	if probe {
		// Probe first so BuildAuto sees fresh results.
		if code := probeAll(t, cfg, models); code != 0 {
			return code
		}
	}
	spec := free.BuildAuto(autoInput(cfg, models, probe))
	if len(spec.Targets) == 0 {
		if probe {
			return fail(t, "no probed free models passed the gates; run: tsunagi free probe")
		}
		return fail(t, "no free candidates passed the gates")
	}
	if dryRun {
		for _, target := range spec.Targets {
			t.Printf("%s\n", target)
		}
		return 0
	}
	if err := writeAutoCombo(spec); err != nil {
		return fail(t, "%v", err)
	}
	t.Printf("  %s %s with %d targets\n", t.Paint(ui.Mint, "[ok]"), "free/auto", len(spec.Targets))
	for _, target := range spec.Targets {
		t.Printf("    %s\n", target)
	}
	return 0
}

// probeAll runs the same probing `free probe` does, without printing a table.
func probeAll(t *ui.Term, cfg config.Config, models map[string][]free.ModelCand) int {
	if _, err := probeKeyed(cfg, models); err != nil {
		return fail(t, "%v", err)
	}
	return 0
}

// probeKeyed probes every candidate model of the keyed free providers, saves
// the results and returns them in probing order.
func probeKeyed(cfg config.Config, models map[string][]free.ModelCand) ([]free.Result, error) {
	cands := probeCandidates(cfg, models, "")
	if len(cands) == 0 {
		return nil, nil
	}
	host, token, stop, err := probeRouter(cfg)
	if err != nil {
		return nil, err
	}
	defer stop()
	client := &http.Client{Timeout: 2 * time.Minute}
	probes := free.LoadProbes()
	var out []free.Result
	for _, pc := range cands {
		for _, m := range pc.Models {
			target := pc.Entry.ID + "/" + m.ID
			status, res, ms := probeSend(client, host, token, target, true)
			r := free.Result{Target: target, Millis: ms, At: time.Now()}
			if status == 200 && res.err == "" {
				r.OK = true
				r.Tools = len(res.tools) > 0
			} else if res.err != "" {
				r.Err = shortErr(res.err)
			} else {
				r.Err = fmt.Sprintf("HTTP %d", status)
			}
			probes[target] = r
			out = append(out, r)
		}
	}
	return out, free.SaveProbes(probes)
}

// writeAutoCombo merges free/auto into the combos map and writes it through
// SetKey, preserving every other key and combo.
func writeAutoCombo(spec free.ComboSpec) error {
	path := config.Path()
	combos := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil {
		var doc map[string]json.RawMessage
		if json.Unmarshal(b, &doc) == nil {
			if raw, ok := doc["combos"]; ok {
				json.Unmarshal(raw, &combos)
			}
		}
	}
	entry, err := json.Marshal(config.Combo{Targets: spec.Targets, Strategy: spec.Strategy, Auto: true})
	if err != nil {
		return err
	}
	combos["free/auto"] = entry
	return config.SetKey(path, "combos", combos)
}

func cmdFreeAllow(t *ui.Term, args []string, allow bool) int {
	verb := "deny"
	if allow {
		verb = "allow"
	}
	if len(args) != 1 {
		return fail(t, "usage: tsunagi free %s <provider>", verb)
	}
	e, ok := registryEntry(args[0])
	if !ok {
		return fail(t, "%q is not a free-offer provider. Run: tsunagi free", args[0])
	}
	if e.Data != free.Trains {
		return fail(t, "%s data policy is %s; only trains providers need %s", e.ID, e.Data, verb)
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	list := append([]string(nil), cfg.Free.AllowTrains...)
	found := false
	var kept []string
	for _, n := range list {
		if n == e.ID {
			found = true
			continue
		}
		kept = append(kept, n)
	}
	if allow && !found {
		kept = append(kept, e.ID)
		sort.Strings(kept)
	}
	if !allow && !found {
		t.Printf("  %s %s is not in free.allow_trains\n", t.Dim("[--]"), e.ID)
		return 0
	}
	if err := config.SetKey(config.Path(), "free", config.FreeConfig{AllowTrains: kept}); err != nil {
		return fail(t, "%v", err)
	}
	if allow {
		t.Printf("  %s %s added to free.allow_trains\n", t.Paint(ui.Mint, "[ok]"), e.ID)
	} else {
		t.Printf("  %s %s removed from free.allow_trains\n", t.Paint(ui.Mint, "[ok]"), e.ID)
	}
	return 0
}
