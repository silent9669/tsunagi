package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/free"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/keyprefix"
	"github.com/silent9669/tsunagi/internal/opencode"
	"github.com/silent9669/tsunagi/internal/plans"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/tui/app"
)

// backend implements app.Backend on top of the existing commands' logic.
type backend struct{}

var appVersion = version

func (backend) Snapshot() app.Snapshot {
	var snap app.Snapshot
	snap.ConfigPath = config.Path()
	if p, err := claudeBin(); err == nil {
		snap.ClaudePath = p
	}
	cfg, err := config.Load()
	if err != nil {
		return snap
	}
	snap.Onboarded = cfg.UI.Onboarded != nil && *cfg.UI.Onboarded
	regIDs := map[string]free.Entry{}
	for _, e := range free.Registry() {
		regIDs[e.ID] = e
	}
	seen := map[string]bool{}
	for _, p := range cfg.All() {
		row := app.ProviderRow{Name: p.Name, Accounts: accountsOf(p.Name)}
		row.Ready = p.Local || ready(p) > 0
		for _, a := range row.Accounts {
			if hasAccount(p, a) {
				row.Ready = true
			}
		}
		if e, ok := regIDs[p.Name]; ok {
			row.Group = "free"
			row.Tag = e.Data
			row.Setup = e.Setup
			row.FreeEntry = true
			row.Limits = e.Limits
			// a keyless provider is usable but not "set up": Ready stays
			// false so chips and counts do not treat it as configured
			if e.Access == free.Keyless {
				row.Keyless = true
			}
		} else if pe, ok := plans.Find(p.Name); ok {
			row.Group = "plan"
			row.Tag = "plan"
			row.Plan = &app.PlanInfo{
				Price: pe.Price, Quota: pe.Quota, Models: pe.Models, Needs: pe.Needs,
				KeyHint: pe.KeyHint, Terms: pe.Terms, Source: pe.Source, Checked: pe.Checked,
				Signup: pe.Signup, KeyURL: pe.KeyURL, Verified: true,
			}
		} else if p.Auth == "chatgpt" {
			row.Group = "plan"
			row.Tag = "plan"
		} else if p.Local {
			row.Group = "local"
			row.Ready = true
		} else if p.Untrusted && plans.LooksLikePlan(p.Name) {
			row.Group = "plan"
			row.Tag = "plan"
			row.Plan = &app.PlanInfo{Terms: "flat-fee plan, details not checked"}
		} else if p.Untrusted && len(row.Accounts) == 0 && !row.Ready {
			// unkeyed catalogue providers stay browseable behind search
		} else {
			row.Group = "paid"
		}
		seen[p.Name] = true
		snap.Providers = append(snap.Providers, row)
	}
	// registry providers not configured at all (e.g. cloudflare)
	for _, e := range free.Registry() {
		if !seen[e.ID] {
			snap.Providers = append(snap.Providers, app.ProviderRow{
				Name: e.ID, Group: "free", Tag: e.Data, Setup: e.Setup, FreeEntry: true,
				Keyless: e.Access == free.Keyless, Limits: e.Limits,
			})
		}
	}
	if c, ok := cfg.Combos["free/auto"]; ok && c.Auto {
		snap.FreeTargets = c.Targets
	}
	snap.AllowTrains = map[string]bool{}
	for _, n := range cfg.Free.AllowTrains {
		snap.AllowTrains[n] = true
	}
	snap.Skills = skills.Status()
	return snap
}

// SaveKey stores a provider key: keychain first, then the key index.
func (backend) SaveKey(providerName, account, key string) error {
	if account == "" {
		account = providerName
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, p := range cfg.All() {
		if p.Name == providerName {
			if err := keychain.Set(account, key); err != nil {
				return err
			}
			config.SetKeyed(account, true, pinHost(cfg, p))
			return nil
		}
	}
	return fmt.Errorf("unknown provider %q", providerName)
}

// RemoveKey deletes a key or ChatGPT sign-in.
func (backend) RemoveKey(providerName, account string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, p := range cfg.All() {
		if p.Name != providerName {
			continue
		}
		if p.Auth == "chatgpt" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := chatgptFor(labelOf(account)).SignOut(ctx); err != nil {
				return err
			}
			config.SetKeyed(account, false, "")
			return nil
		}
		if err := keychain.Delete(account); err != nil {
			return err
		}
		config.SetKeyed(account, false, "")
		return nil
	}
	return fmt.Errorf("unknown provider %q", providerName)
}

// SignInChatGPT runs the browser sign-in; onURL gets the auth URL.
func (backend) SignInChatGPT(ctx context.Context, account string, onURL func(url string)) (string, error) {
	g := chatgptFor(labelOf(account))
	g.Open = func(u string) error {
		onURL(u)
		return openBrowser(u)
	}
	email, err := g.SignIn(ctx, onURL)
	if err != nil {
		return "", err
	}
	config.SetKeyed(account, true, "")
	return email, nil
}

// AddGateway saves a user provider like `tsunagi add` does.
func (backend) AddGateway(name, url, dialect string, local bool) error {
	args := []string{name, url}
	if dialect != "" {
		args = append(args, "--dialect", dialect)
	}
	if local {
		args = append(args, "--local")
	}
	p, err := parseAdd(args)
	if err != nil {
		return err
	}
	def, user, err := config.User(config.Path())
	if err != nil {
		return err
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
	return config.Save(config.Path(), def, user)
}

// ImportOpencode takes providers and keys from opencode, like `tsunagi import`.
func (backend) ImportOpencode() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	adds, _, _, _, err := runImport(cfg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("added %d providers", len(adds)), nil
}

// Verify probes one candidate model of the provider through the router.
func (backend) Verify(ctx context.Context, providerName string) app.VerifyResult {
	cfg, err := config.Load()
	if err != nil {
		return app.VerifyResult{Err: err.Error()}
	}
	var p provider.Provider
	found := false
	for _, cand := range cfg.All() {
		if cand.Name == providerName {
			p, found = cand, true
			break
		}
	}
	if !found {
		return app.VerifyResult{Err: "unknown provider " + providerName}
	}
	target, err := verifyCandidate(ctx, cfg, p)
	if err != nil {
		return app.VerifyResult{Err: shortErr(err.Error())}
	}
	host, token, stop, err := probeRouter(cfg)
	if err != nil {
		return app.VerifyResult{Err: err.Error()}
	}
	defer stop()
	client := &http.Client{Timeout: 2 * time.Minute}
	status, res, ms := probeSend(client, host, token, target, true)
	out := app.VerifyResult{Model: target, Millis: ms}
	switch {
	case status == 200 && res.err == "":
		out.OK = true
	case res.err != "":
		out.Err = shortErr(res.err)
	case status == 0:
		out.Err = shortErr(res.err)
	default:
		out.Err = fmt.Sprintf("HTTP %d", status)
	}
	return out
}

// verifyCandidate picks the model id to probe for a provider: a discovered
// or catalogue free model first, then the live /models list.
func verifyCandidate(ctx context.Context, cfg config.Config, p provider.Provider) (string, error) {
	// chatgpt model ids come from the account's own list
	if p.Auth == "chatgpt" {
		models, err := chatgptFor("").ListModels(ctx)
		if err != nil || len(models) == 0 {
			return "", errors.New("no models listed for chatgpt")
		}
		return "chatgpt/" + models[0].Slug, nil
	}
	// free providers: first discovered free model
	if snap, ok := free.Load(); ok {
		for _, m := range snap.Models {
			if m.Provider == p.Name {
				return p.Name + "/" + m.ID, nil
			}
		}
	}
	// catalogue entry
	for _, e := range catalog.Load() {
		if e.ID == p.Name && len(e.Models) > 0 {
			return p.Name + "/" + e.Models[0], nil
		}
	}
	// live list
	key := ""
	if !p.Local {
		k, err := fetchKey(cfg, p)
		if err != nil {
			return "", err
		}
		key = k
	}
	models, err := liveModels(ctx, p, key)
	if err != nil || len(models) == 0 {
		return "", errors.New("provider lists no models")
	}
	return p.Name + "/" + models[0], nil
}

// BuildFreeCombo writes the free/auto combo from currently keyed providers.
func (backend) BuildFreeCombo(allowTrains map[string]bool) ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if c, ok := cfg.Combos["free/auto"]; ok && !c.Auto {
		return nil, errors.New("combos.free/auto exists without auto:true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snap, _ := free.Ensure(ctx, &http.Client{Timeout: 20 * time.Second})
	models := freeModels(ctx, cfg, snap)
	in := autoInput(cfg, models, false)
	for n := range allowTrains {
		in.AllowTrains[n] = true
	}
	spec := free.BuildAuto(in)
	if len(spec.Targets) == 0 {
		return nil, errors.New("no free candidates passed the gates")
	}
	if err := writeAutoCombo(spec); err != nil {
		return nil, err
	}
	return spec.Targets, nil
}

// SetAllowTrains opts a trains provider into or out of free/auto.
func (backend) SetAllowTrains(providerName string, on bool) error {
	e, ok := registryEntry(providerName)
	if !ok || e.Data != free.Trains {
		return fmt.Errorf("%s is not a trains provider", providerName)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	list := append([]string(nil), cfg.Free.AllowTrains...)
	found := false
	var kept []string
	for _, n := range list {
		if n == providerName {
			found = true
			continue
		}
		kept = append(kept, n)
	}
	if on && !found {
		kept = append(kept, providerName)
		sort.Strings(kept)
	}
	return config.SetKey(config.Path(), "free", config.FreeConfig{AllowTrains: kept})
}

func (backend) OpenURL(url string) error { return openBrowser(url) }

func (backend) InstallSkills(targets []string) []skills.Result {
	return skills.Install(targets, appVersion)
}

func (backend) RemoveSkills(targets []string) []skills.Result {
	return skills.Remove(targets)
}

func (backend) MarkOnboarded() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	on := true
	ui := cfg.UI
	ui.Onboarded = &on
	return config.SetKey(config.Path(), "ui", ui)
}

// ProbeLocals checks the builtin local providers' default ports in the
// background and reports which answer, once.
func (backend) ProbeLocals(ctx context.Context) <-chan map[string]bool {
	ch := make(chan map[string]bool, 1)
	go func() {
		defer close(ch)
		out := map[string]bool{}
		for _, p := range provider.Builtin() {
			if !p.Local {
				continue
			}
			out[p.Name] = probeLocal(ctx, p)
		}
		select {
		case ch <- out:
		case <-ctx.Done():
		}
	}()
	return ch
}

// probeLocal reports whether the provider's endpoint answers within 300 ms.
func probeLocal(ctx context.Context, p provider.Provider) bool {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	u := strings.TrimRight(p.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		// a TCP-level answer still counts as "something is there"
		var ne net.Error
		if errors.As(err, &ne) {
			return false
		}
		if _, derr := net.DialTimeout("tcp", req.URL.Host, 200*time.Millisecond); derr == nil {
			return true
		}
		return false
	}
	resp.Body.Close()
	return true
}

// runImport is the shared body of `tsunagi import opencode` and the app's
// ImportOpencode: read opencode config+auth, plan, save providers and keys.
// It returns the added providers, the planned keys, the display rows and the
// per-provider key outcome ("imported", "kept (already stored)", "failed: ..").
func runImport(cfg config.Config) ([]provider.Provider, map[string]string, []importRow, map[string]string, error) {
	ocs, found, err := opencode.ReadConfig(opencode.ConfigPaths())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	auth, err := opencode.ReadAuth(opencode.AuthPath())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if !found && len(auth) == 0 {
		return nil, nil, nil, nil, errors.New("no opencode config or logins found")
	}
	known := map[string]bool{}
	for _, p := range cfg.All() {
		known[p.Name] = true
	}
	adds, keys, rows := planImport(known, ocs, auth)
	if len(adds) > 0 {
		def, user, err := config.User(config.Path())
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := config.Save(config.Path(), def, append(user, adds...)); err != nil {
			return nil, nil, nil, nil, err
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
	return adds, keys, rows, result, nil
}

// Status runs the doctor checks.
func (backend) Status() []app.StatusRow {
	var out []app.StatusRow
	for _, r := range doctorChecks() {
		out = append(out, app.StatusRow{Name: r.name, OK: r.ok, Detail: r.detail})
	}
	return out
}

// Settings returns the routing and appearance values the Settings screen edits.
func (backend) Settings() app.SettingsView {
	v := app.SettingsView{Tiers: map[string]string{}, Scenarios: map[string]string{}, Look: "braille", Theme: "mono", Anim: true}
	cfg, err := config.Load()
	if err != nil {
		return v
	}
	for k, t := range cfg.Tiers {
		v.Tiers[k] = t
	}
	for k, t := range cfg.Scenarios {
		v.Scenarios[k] = t
	}
	for name := range cfg.Combos {
		v.Combos = append(v.Combos, name)
	}
	sort.Strings(v.Combos)
	if cfg.UI.Look != "" {
		v.Look = cfg.UI.Look
	}
	if cfg.UI.Theme != "" {
		v.Theme = cfg.UI.Theme
	}
	if cfg.UI.Anim != nil {
		v.Anim = *cfg.UI.Anim
	}
	return v
}

// SetTier points a Claude tier at a model or combo; an empty value clears it.
func (backend) SetTier(tier, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	tiers := map[string]string{}
	for k, v := range cfg.Tiers {
		tiers[k] = v
	}
	if value == "" {
		delete(tiers, tier)
	} else {
		tiers[tier] = value
	}
	if len(tiers) == 0 {
		return config.SetKey(config.Path(), "tiers", nil)
	}
	return config.SetKey(config.Path(), "tiers", tiers)
}

// SetScenario points a request scenario at a model or combo; an empty value
// clears it.
func (backend) SetScenario(name, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sc := map[string]string{}
	for k, v := range cfg.Scenarios {
		sc[k] = v
	}
	if value == "" {
		delete(sc, name)
	} else {
		sc[name] = value
	}
	if len(sc) == 0 {
		return config.SetKey(config.Path(), "scenarios", nil)
	}
	return config.SetKey(config.Path(), "scenarios", sc)
}

// SetUI saves the look, theme and animation switch.
func (backend) SetUI(look, theme string, anim bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ui := cfg.UI
	ui.Look, ui.Theme, ui.Anim = look, theme, &anim
	return config.SetKey(config.Path(), "ui", ui)
}

// ResetUI clears the saved appearance and keeps the onboarding flag.
func (backend) ResetUI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return config.SetKey(config.Path(), "ui", config.UIConfig{Onboarded: cfg.UI.Onboarded})
}

// Plan returns what the router would do with a tool-using request for model.
func (backend) Plan(model string) []string {
	cfg, err := config.Load()
	if err != nil {
		return []string{err.Error()}
	}
	plan := newRouter(cfg, "", "").Plan(model, router.Need{Tools: true})
	lines := []string{model}
	if plan.Resolved != model {
		lines = append(lines, "as "+plan.Resolved)
	}
	if plan.Why != "" {
		lines = append(lines, plan.Why)
	}
	for _, s := range plan.Steps {
		line := s.Target + "  " + s.Status
		if s.Reason != "" {
			line += "  " + s.Reason
		}
		lines = append(lines, line)
	}
	return lines
}

// ProbeFree probes the keyed free providers' models.
func (backend) ProbeFree(ctx context.Context) ([]app.ProbeRow, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	snap, _ := free.Ensure(ctx, &http.Client{Timeout: 20 * time.Second})
	rs, err := probeKeyed(cfg, freeModels(ctx, cfg, snap))
	var out []app.ProbeRow
	for _, r := range rs {
		out = append(out, app.ProbeRow{Target: r.Target, OK: r.OK, Tools: r.Tools, Millis: r.Millis, Err: r.Err})
	}
	return out, err
}

// storeKey saves a key for an account; a var so tests never touch the
// keychain.
var storeKey = keychain.Set

// envKeys lists unkeyed routable providers an environment variable can key,
// provider and variable names only.
func envKeys(cfg config.Config) []app.EnvKey {
	vars := map[string][]string{}
	for _, e := range catalog.Load() {
		vars[e.ID] = e.Env
	}
	var out []app.EnvKey
	for _, p := range cfg.All() {
		if p.Name == "anthropic" || p.Name == "claude" || ready(p) > 0 {
			continue
		}
		for _, v := range vars[p.Name] {
			if v != "ANTHROPIC_API_KEY" && os.Getenv(v) != "" {
				out = append(out, app.EnvKey{Provider: p.Name, Var: v})
				break
			}
		}
	}
	return out
}

// EnvKeys reports the providers ImportEnvKey will accept.
func (backend) EnvKeys() []app.EnvKey {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	return envKeys(cfg)
}

// ImportEnvKey saves the environment variable's value for the provider,
// exactly like SaveKey. The value never leaves this function.
func (backend) ImportEnvKey(providerName string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	want := ""
	for _, ek := range envKeys(cfg) {
		if ek.Provider == providerName {
			want = ek.Var
			break
		}
	}
	v := ""
	if want != "" {
		v = os.Getenv(want)
	}
	if v == "" {
		return fmt.Errorf("no environment key for %s", providerName)
	}
	for _, p := range cfg.All() {
		if p.Name == providerName {
			if err := storeKey(providerName, v); err != nil {
				return err
			}
			config.SetKeyed(providerName, true, pinHost(cfg, p))
			return nil
		}
	}
	return fmt.Errorf("unknown provider %q", providerName)
}

// DetectKey guesses providers from a pasted key's prefix, keeping only
// names that show up in the snapshot.
func (backend) DetectKey(key string) ([]string, error) {
	names, err := keyprefix.Check(key)
	if err != nil || len(names) == 0 {
		return names, err
	}
	known := map[string]bool{}
	for _, r := range (backend{}).Snapshot().Providers {
		known[r.Name] = true
	}
	var out []string
	for _, n := range names {
		if known[n] {
			out = append(out, n)
		}
	}
	return out, nil
}
