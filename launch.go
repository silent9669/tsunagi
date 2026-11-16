package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/keychain"
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
// terminal.
func launch(t *ui.Term, args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	t.Splash()

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
	rt := router.New(host, token, cfg.All(), cfg.Default, keyFor(cfg))
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
	claude, err := exec.LookPath("claude")
	pause(t)
	if err != nil {
		sp.Stop(false, "not found in PATH")
		return 127
	}
	sp.Stop(true, claude)

	sp = t.Spin("providers")
	var ready []string
	byName := map[string]bool{}
	for _, p := range cfg.All() {
		byName[p.Name] = p.Ready() && !p.Local
	}
	for _, n := range config.Keyed() {
		if byName[n] && keychain.Has(n) {
			ready = append(ready, n)
		}
	}
	pause(t)
	if len(ready) == 0 {
		sp.Stop(true, "no keys yet. Run: tsunagi login "+cfg.Default)
	} else {
		sp.Stop(true, strings.Join(ready, ", "))
	}
	t.Printf("\n")

	cmd := exec.Command(claude, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(withoutEnv(os.Environ(), "ANTHROPIC_BASE_URL"),
		"ANTHROPIC_BASE_URL=http://"+host+"/"+token)

	// Ctrl+C reaches claude directly through the terminal; the router must
	// outlive it, so only forward termination signals.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		return fail(t, "%v", err)
	}
	go func() {
		for s := range sigs {
			if s != syscall.SIGINT {
				cmd.Process.Signal(s)
			}
		}
	}()
	err = cmd.Wait()
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
