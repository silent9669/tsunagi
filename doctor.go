package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/silent9669/tsunagi/internal/catalog"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/slash"
	"github.com/silent9669/tsunagi/internal/ui"
)

// cmdDoctor checks the things a launch depends on and says what to fix.
func cmdDoctor(t *ui.Term) int {
	bad := 0
	check := func(ok bool, name, good, fix string) {
		if ok {
			t.Printf("  %s %-10s %s\n", t.Paint(ui.Mint, "[ok]"), name, t.Dim(good))
			return
		}
		bad++
		t.Printf("  %s %-10s %s\n", t.Paint(ui.Rose, "[!!]"), name, fix)
	}

	path, err := exec.LookPath("claude")
	ver := ""
	if err == nil {
		if out, e := exec.Command(path, "--version").Output(); e == nil {
			ver = strings.TrimSpace(string(out))
		}
	}
	check(err == nil, "claude", path+" "+ver, "not found in PATH. Install Claude Code first")

	_, err = exec.LookPath("security")
	check(err == nil, "keychain", "macOS security tool found", "the macOS security tool is missing; keys cannot be stored")

	cfg, err := config.Load()
	if err != nil {
		check(false, "config", "", err.Error())
	} else {
		check(true, "config", config.Path(), "")
	}

	if len(catalog.Load()) > 0 {
		st, _ := os.Stat(catalog.Path())
		age := "unknown age"
		if st != nil {
			age = ago(st.ModTime())
		}
		check(true, "catalogue", "synced "+age, "")
	} else {
		check(false, "catalogue", "", "not synced: run tsunagi sync for the provider catalogue (built-in providers work without it)")
	}

	n := 0
	for _, p := range cfg.All() {
		if !p.Local && ready(p) > 0 {
			n++
		}
	}
	check(n > 0, "accounts", strconv.Itoa(n)+" provider(s) ready", "no keys yet: run tsunagi login <provider>")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		ln.Close()
	}
	check(err == nil, "router", "can listen on 127.0.0.1", "cannot open a loopback port: "+errText(err))

	dir, err := events.Dir()
	if err == nil {
		f := filepath.Join(dir, ".doctor")
		err = os.WriteFile(f, []byte("x"), 0o600)
		os.Remove(f)
	}
	check(err == nil, "state", "cache directory writable", "cannot write the cache directory: "+errText(err))

	exe, err := os.Executable()
	if err == nil {
		err = slash.Write(filepath.Join(os.TempDir(), "tsunagi-doctor-plugin"), exe)
		os.RemoveAll(filepath.Join(os.TempDir(), "tsunagi-doctor-plugin"))
	}
	check(err == nil, "slash", strings.Join(slash.Names()[:3], " ")+" ...", errText(err)+" (set TSUNAGI_NO_PLUGIN=1 to ignore)")

	if bad > 0 {
		return 1
	}
	return 0
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
