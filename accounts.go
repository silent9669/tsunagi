package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/signin"
)

// An account is a key or sign-in for a provider. The first one has the
// provider's name ("openai"); more carry a label ("openai.work").
var labelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,19}$`)

// parseAs removes "--as <label>" from args.
func parseAs(args []string) ([]string, string, error) {
	var rest []string
	label := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "--as" {
			rest = append(rest, args[i])
			continue
		}
		if i+1 >= len(args) || !labelRe.MatchString(args[i+1]) {
			return nil, "", fmt.Errorf("--as needs a label of lowercase letters, digits and dashes (up to 20)")
		}
		label = args[i+1]
		i++
	}
	if label == "default" {
		label = ""
	}
	return rest, label, nil
}

func baseOf(account string) string {
	b, _, _ := strings.Cut(account, ".")
	return b
}

func labelOf(account string) string {
	_, l, _ := strings.Cut(account, ".")
	return l
}

// accountsOf lists a provider's accounts from the key index, the default one
// first. The router calls it per request, so a login in another terminal is
// picked up without a restart.
func accountsOf(providerName string) []string {
	var out []string
	for _, a := range config.Keyed() {
		if baseOf(a) == providerName {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] == providerName && out[j] != providerName })
	return out
}

var (
	gptMu sync.Mutex
	gpts  = map[string]*signin.ChatGPT{}
)

// chatgptFor returns the sign-in for an account label. One instance per label
// keeps its access token in memory across requests.
func chatgptFor(label string) *signin.ChatGPT {
	gptMu.Lock()
	defer gptMu.Unlock()
	if g, ok := gpts[label]; ok {
		return g
	}
	g := signin.New(filepath.Dir(config.Path()))
	g.Label = label
	g.Open = func(u string) error { return exec.Command("open", u).Start() }
	gpts[label] = g
	return g
}

// hasAccount reports whether the account's key or sign-in is stored.
func hasAccount(p provider.Provider, account string) bool {
	if p.Auth == "chatgpt" {
		return chatgptFor(labelOf(account)).Signed()
	}
	return keychain.Has(account)
}

// ready counts the accounts of p that are stored and usable.
func ready(p provider.Provider) int {
	n := 0
	for _, a := range accountsOf(p.Name) {
		if hasAccount(p, a) && !(p.Untrusted && config.Pin(a) == "") {
			n++
		}
	}
	return n
}
