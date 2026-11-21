package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/claudeguard"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/ui"
)

// pause lets a spinner show for a beat; the work behind it is instant.
func pause(t *ui.Term) {
	if t.Anim {
		time.Sleep(260 * time.Millisecond)
	}
}

// launch starts the router, then runs claude as a child that alone sees the
// router's address. While claude runs, this process writes nothing to the
// terminal. splash=false skips the intro when the app already showed it.
func launch(t *ui.Term, args []string, splash bool) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	if splash {
		t.Splash()
	}

	sp := t.Spin("router")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	host := ln.Addr().String()
	token, err := randomToken()
	if err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	rt := newRouter(cfg, host, token)
	if dir, err := events.Dir(); err == nil {
		if w, err := events.NewWriter(dir); err == nil {
			defer w.Close()
			rt.OnEvent = w.Write
		}
	}
	srv := &http.Server{Handler: rt, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()
	pause(t)
	sp.Stop(true, host)

	sp = t.Spin("claude")
	claude, err := claudeBin()
	pause(t)
	if err != nil {
		sp.Stop(false, "not found in PATH")
		return 127
	}
	sp.Stop(true, claude)

	sp = t.Spin("providers")
	var names []string
	for _, p := range cfg.All() {
		if !p.Ready() || p.Local {
			continue
		}
		switch n := ready(p); {
		case n == 1:
			names = append(names, p.Name)
		case n > 1:
			names = append(names, fmt.Sprintf("%s x%d", p.Name, n))
		}
	}
	pause(t)
	if len(names) == 0 {
		sp.Stop(true, "no keys yet. Run: tsunagi login "+cfg.Default)
	} else {
		sp.Stop(true, strings.Join(names, ", "))
	}
	targets := []string{"anthropic"}
	for _, n := range names {
		if len(targets) == 6 {
			targets = append(targets, fmt.Sprintf("+%d more", len(names)-5))
			break
		}
		targets = append(targets, n)
	}
	t.Printf("\n")
	t.Routes(targets)
	t.Printf("\n")

	sessionEnv, unregister := registerSession()
	defer unregister()
	if exe, err := os.Executable(); err == nil {
		args = stopHookArgs(args, exe)
		args = pluginArgs(args, exe)
	}
	cmd := exec.Command(claude, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(append(withoutEnv(os.Environ(), "ANTHROPIC_BASE_URL"),
		"ANTHROPIC_BASE_URL=http://"+host+"/"+token), sessionEnv...)

	// Ctrl+C reaches claude directly through the terminal; the router must
	// outlive it, so only forward termination signals.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		return fail(t, "%v", err)
	}
	guard := startModelGuard(cfg)
	go func() {
		for s := range sigs {
			if s != syscall.SIGINT {
				cmd.Process.Signal(s)
			}
		}
	}()
	err = cmd.Wait()
	guard.Stop()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 1
}

// newRouter builds the request router the same way for launch, ping and route.
func newRouter(cfg config.Config, host, token string) *router.Router {
	rt := router.New(host, token, cfg.All(), cfg.Default, keyFor(cfg))
	rt.Accounts = accountsOf
	rt.Headroom = newHeadroom(cfg).get
	rt.Aliases = cfg.Aliases
	rt.Tiers = cfg.Tiers
	rt.Combos = routerCombos(cfg.Combos)
	rt.Caps = catalogCaps
	return rt
}

func routerCombos(in map[string]config.Combo) map[string]router.Combo {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]router.Combo, len(in))
	for name, c := range in {
		out[name] = router.Combo{Targets: c.Targets, Strategy: c.Strategy}
	}
	return out
}

// catalogCaps adapts the catalogue's per-model info to the router's Caps.
func catalogCaps(p, m string) (router.Caps, bool) {
	mi, ok := catalog.Caps(p, m)
	return router.Caps{Tools: mi.Tools, Context: mi.Context}, ok
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func withoutEnv(env []string, name string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// startModelGuard keeps `/model` choices made in this session out of the
// user's Claude Code settings, which Claude Desktop and other sessions share
// (see internal/claudeguard). It first repairs what a killed tsunagi left.
func startModelGuard(cfg config.Config) *claudeguard.Guard {
	if os.Getenv("TSUNAGI_NO_MODEL_GUARD") != "" {
		return claudeguard.Start("", "", nil)
	}
	state, err := events.Dir()
	if err != nil {
		state = ""
	}
	external := routedAway(cfg)
	if state != "" {
		claudeguard.Recover(state, external)
	}
	return claudeguard.Start(claudeguard.SettingsPath(), state, external)
}

// routedAway reports whether a model name saved in Claude's settings is one
// tsunagi sends to another provider: a provider/model pair, or one of the
// user's own combo or alias names. Claude's own names never are.
func routedAway(cfg config.Config) func(string) bool {
	return func(m string) bool {
		base := strings.TrimSuffix(m, "[1m]")
		switch {
		case strings.HasPrefix(m, "arn:"), strings.HasPrefix(m, "claude-"):
			return false
		case strings.Contains(m, "/"):
			return true
		}
		switch base {
		case "default", "best", "fable", "opus", "sonnet", "haiku", "opusplan":
			return false
		}
		_, alias := cfg.Aliases[m]
		_, combo := cfg.Combos[m]
		return alias || combo
	}
}
